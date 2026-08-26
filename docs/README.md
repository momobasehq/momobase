# Docs

## Architecture

The backend is a modular monolith, layered so that each concern has one home:

- `internal/http`: Fiber routing, middleware, and the public, administrative, and webhook handlers.
- `internal/dto`: every request body the API accepts, carrying its own validation rules and normalization.
- `internal/service/*`: identity, payments, routing, provider runtime, health, webhooks, reconciliation, and auditing.
- `internal/repository`: the only package that reaches the database — one repository per entity, and the single transaction boundary.
- `providers`: the public provider contract and shared adapter helpers, plus optional adapters in `providers/dummy` and `providers/mtn`. Nothing here is registered automatically; a build chooses its providers through `momobase.WithProvider`.
- `internal/utils`: dependency-free helpers shared across the module — validation, country normalization, and redaction.
- `internal/workers`: bounded health, reconciliation, and session-cleanup loops.
- `internal/bootstrap`: configuration, database initialization, dependency wiring, migration, and process lifecycle.
- `internal/migrations`: ordered schema changes that `AutoMigrate` cannot express, and the ledger recording which have been applied.

## Schema migrations

Momobase settles its schema in two steps, both idempotent and both run by `momobase migrate` and by start-up when `AUTO_MIGRATE=true`:

1. **Versioned migrations** in `internal/migrations` apply ordered changes that `AutoMigrate` cannot — renames, drops, and backfills. Each is recorded in a `schema_migrations` table.
2. **`AutoMigrate`** then converges the schema with the current models, creating tables, adding columns, and widening types.

Every migration is written to be a no-op when its change is already present. That is what lets a database created by an earlier release — which has tables but no ledger — be adopted without a separate baselining step.

A migration that fails leaves its ledger row marked dirty, and the next start refuses rather than running against a half-changed schema. Recovery is deliberate and manual: verify the schema, then delete that row from `schema_migrations`.

**More than one replica:** run migrations as a pre-deploy step and disable them in the serving processes, so replicas do not race to apply the same change.

```sh
momobase migrate                 # pre-deploy, once
AUTO_MIGRATE=false momobase serve # every replica
```

With `AUTO_MIGRATE=false`, a process that finds pending migrations logs a warning naming them and still serves; refusing to start would turn a correct deployment strategy into an outage.

Important runtime guarantees:

- Provider instances are fully built before an atomic runtime-map swap.
- A failed provider reload leaves the previous working runtime untouched.
- Request paths do not launch detached goroutines.
- Worker goroutines are owned by one manager and stop through context cancellation.
- Payment and webhook status changes pass through one legal transition function.
- Webhooks update an exact provider attempt and are deduplicated by provider account and payload hash.
- Database transactions do not wrap external provider network calls.
- Provider reloads happen synchronously after committed admin changes; no internal outbox is used.

## Payment workflow

1. An application exchanges its client ID and secret for an access token.
2. The application lists the methods it can offer, then creates a collection or disbursement with an `Idempotency-Key`.
3. Momobase validates the request for shape and claims the idempotency key.
4. Routing selects the highest-priority active provider account eligible for the request country.
5. The selected provider validates and normalizes the account, if it implements `providers.RequestValidator`; a rejection ends the request before anything is persisted.
6. A transaction and provider attempt are persisted, recording the normalized account.
7. The provider executor checks runtime health and calls the selected adapter with a bounded context.
8. The normalized provider result is persisted through the transaction state machine.
9. The application reads the transaction by Momobase ID or its own reference.

Public payment routes:

```text
POST /api/v1/token
POST /api/v1/token/refresh
GET  /api/v1/payment-methods
POST /api/v1/collections
POST /api/v1/disbursements
GET  /api/v1/transactions/{id}
GET  /api/v1/transactions/by-reference/{reference}
```

## Webhook workflow

1. A provider sends `POST /webhooks/{providerAccountID}`.
2. Momobase limits the body size and verifies `X-Webhook-Secret` against the encrypted provider configuration.
3. The adapter parses the provider payload into a normalized event.
4. Momobase hashes and deduplicates the payload.
5. The service finds the exact transaction attempt by provider reference.
6. The state machine applies the legal transaction and attempt transition in one database transaction.
7. Duplicate valid callbacks return success without reapplying state.

Provider configuration must contain a long random webhook secret:

```json
{
  "webhook_secret": "replace-with-long-random-secret"
}
```

## Reconciliation workflow

1. The reconciliation worker loads a bounded batch of pending, processing, or unknown transactions.
2. It queries the exact provider runtime for current status.
3. The provider response is normalized.
4. The latest attempt is updated by ID and the state machine applies any legal transaction transition.
5. Errors are logged and the next scheduled worker iteration continues.

Reconciliation is intentionally sequential by default to keep provider pressure and concurrency behavior predictable.

## Provider administration workflow

1. A super administrator creates a provider account for a registered provider code.
2. Provider credentials and settings are encrypted before persistence.
3. Testing builds a temporary provider instance and performs a health check without changing the active runtime.
4. Activation validates configuration, builds the runtime, verifies health, commits the active state, and installs it in memory.
5. Configuration changes reload the active runtime synchronously after commit.
6. Deactivation commits the inactive state and removes the runtime immediately.
7. Routes connect provider accounts to collection or disbursement traffic by payment method and priority; each provider account’s country list determines where that route is eligible.

A provider is routable only when its account and route are active, it declares a capability for the requested service, it is eligible for the transaction country, and a healthy runtime exists.

Capabilities name the service only — `{"service_type": "collection"}`. Which payment rails reach an account is decided by the routes an operator creates for it, not by the adapter.

## Authentication workflow

### Application credentials

- Admins create, rotate, and revoke application credentials.
- Client secrets are returned only when created or rotated and are stored as password hashes.
- Revocation invalidates existing sessions for that credential.
- Access and refresh tokens are signed separately from admin tokens.

### Token format

Both audiences are issued HS256 JSON Web Tokens, signed with the audience's own
secret — `APP_OAUTH_SECRET` for applications, `ADMIN_OAUTH_SECRET` for administrators.
The registered claims carry what JWT already defines a name for (`sub`, `jti`, `iat`,
`exp`) and everything else is a private claim, so a token is legible to any standard
JWT tooling.

A token proves only that it was signed by this deployment and has not expired. It
authorizes nothing on its own: the session row must still be live, an administrator's
permissions are read from their role on every request, and an application's scopes come
from a fresh read of the credential. Revoking either takes effect on the next call
rather than at the next refresh.

### Admin sessions

- Admins authenticate with password grant semantics.
- Refresh-token rotation revokes the old session transactionally.
- Disabling an admin invalidates their active sessions.
- Every administrative endpoint requires one permission, checked by middleware.

## Client addresses behind a proxy

Rate limiting and request logs both key on the same resolved client address. By default
that is the immediate peer and **no forwarded header is believed**: `X-Forwarded-For` is
one any caller can set, so honouring it unconditionally would let a client mint a fresh
bucket per request and switch the limiter off.

```env
TRUSTED_PROXY_CIDRS=10.0.0.0/8,192.0.2.1
```

With proxies named, trust becomes directional. The header is read only when the request
arrived **from** one of them, and the chain is then walked right to left to the first
address that is not a trusted proxy — the last hop this deployment did not control.
Anything further left was supplied by something upstream and is not believed. A
malformed hop ends the walk rather than being skipped.

Without this, every client behind one proxy shares a single bucket. A malformed entry
fails at start-up rather than silently disabling the feature.

Each request also carries an `X-Request-Id`, echoed on the response and included in its
log line. An inbound one is adopted when present and plausibly sized, so a trace begun by
a proxy stays one trace; it is a correlation aid and nothing authorizes on it.

## Analytics

```text
GET /api/admin/analytics/transactions?from=&to=&interval=day&app_id=&provider_account_id=
```

Returns a bucketed transaction series — one point per day or hour, with quiet periods
present and zeroed so a chart shows a gap in traffic rather than joining a line across
it. Bucketing happens in SQL, so the response size depends on the range rather than on
how many transactions fall in it; a range covering more than 400 buckets is refused
rather than truncated, because a silently capped series renders as a chart that omits
part of its own range.

Volume is reported **per currency and never summed**: amounts are in each currency's
minor unit, so a single total across UGX and USD would mean nothing.

It requires `transactions:read` — the same rows the transaction list already exposes,
in aggregate — so any role that can read transactions can chart them.

The one piece of per-driver SQL in the codebase is the date-truncation expression, since
SQLite, PostgreSQL, and MySQL each spell it differently. Only SQLite is covered by the
test suite; the other two are exercised by a real deployment.

## Roles and permissions

A permission is `resource:action` — `transactions:read`, `providers:update` — and
belongs to one audience: `admin`, granted to administrators through a role, or `app`,
granted to a credential as a scope. The catalogue is defined in Go and upserted on every
start, so it always matches what the routes actually require and adding a guarded
endpoint needs no migration.

```text
GET    /api/admin/permissions?audience=admin
GET    /api/admin/roles
POST   /api/admin/roles
PATCH  /api/admin/roles/{name}
DELETE /api/admin/roles/{name}
```

`super_admin`, `operations`, and `read_only` are seeded as **system roles** and are
read-only. Their permission sets are re-synchronised on every start, which is what lets
a permission added by a new release reach `super_admin` without an operator doing
anything — and is only safe because they cannot be edited. To grant a different set,
create a role.

`super_admin` holds the wildcard `*`, so it covers permissions that do not exist yet.

An administrator's effective permissions are resolved from their role when a request
authenticates, not carried in the access token, so removing a permission takes effect on
the next call rather than the next refresh. Reassigning an administrator to another role
(`PATCH /api/admin/users/{id}/role`) needs no session revocation for the same reason.
Changing your **own** role is refused: it is both a lockout risk, since the last
`super_admin` demoting itself leaves nobody able to undo it, and a self-promotion path
that `users:update` would otherwise be enough for. `GET /api/admin/me` returns them, which is
what the dashboard gates its controls on.

An administrator whose role no longer exists resolves to no permissions and is refused
everything — the safe direction to fail. A role still assigned to an administrator
cannot be deleted.

Credential scopes are validated against the `app` catalogue when a credential is created,
so a misspelled scope fails there instead of surfacing as a 403 on the first payment.

## Worker lifecycle

The process starts only enabled workers:

- `health`: probes active provider runtimes and updates their current health state.
- `reconciliation`: converges unresolved payments with provider status APIs.
- `cleanup`: removes expired admin and application sessions.

On `SIGINT` or `SIGTERM`, the root context is cancelled, the HTTP server performs a bounded graceful shutdown, workers stop, and the process waits for all worker goroutines.

## Requirements

- Go 1.23 or later
- Node.js 22 or later and pnpm for the web workspace (`web/`)
- SQLite, PostgreSQL, or MySQL
- `curl` and `jq` for the smoke script

A `go.sum` file is intentionally not included. Generate it locally from `go.mod`:

```bash
go mod tidy
```

## Environment loading

Momobase uses `github.com/joho/godotenv/autoload` to load a local `.env` file automatically when present. Existing process environment variables keep precedence, so container and production configuration are not overwritten.

## Local development

Momobase uses one shared Redis-backed cache. Start Redis locally and configure
`REDIS_ADDR` (default `localhost:6379`); `REDIS_USERNAME`, `REDIS_PASSWORD`, and
`REDIS_DB` select authenticated or logical-database deployments. Enable
`REDIS_TLS_ENABLED` for encrypted remote connections. `CACHE_TTL_SECONDS` sets one
TTL for every cached value (default 300 seconds). Cache failures are logged by the
shared cache and fall back to the database for cached read paths.

```bash
cp .env.example .env
go mod tidy
go run ./cmd/momobase migrate
go run ./cmd/momobase seed-admin \
  --email admin@example.com \
  --password 'replace-with-a-strong-password' \
  --name 'Super Admin'
go run ./cmd/momobase serve
```

Enable the administration dashboard:

```env
DASHBOARD_ENABLED=true
```

The dashboard is embedded behind a build tag, because nothing under
`web/dashboard/dist` is committed. A plain `go build` carries no bundle and serves
nothing at `/dashboard/` whatever the flag says; build it with:

```sh
make build-dashboard      # pnpm build, then go build -tags dashboard
```

Then open:

```text
http://localhost:9090/dashboard/
```

The retired `/admin/` panel now redirects here.

## Docker Compose

```bash
cp .env.docker.example .env
openssl rand -base64 32
openssl rand -hex 32
openssl rand -hex 32
docker compose up -d --build
```

Compose starts both PostgreSQL and an internal Redis service. Redis is used as an
ephemeral cache and is not published on a host port.

Set the generated values as `ENCRYPTION_MASTER_KEY_BASE64`, `ADMIN_OAUTH_SECRET`, and `APP_OAUTH_SECRET`. In staging or production, also use an HTTPS `APP_PUBLIC_URL`, non-default database credentials, and an explicit CORS allowlist.

Create the initial administrator:

```bash
docker compose exec momobase momobase seed-admin \
  --email admin@example.com \
  --password 'replace-with-a-strong-password' \
  --name 'Super Admin'
```

Health endpoints:

```text
GET /ping
GET /healthz
```

## Token examples

Admin token:

```bash
curl -X POST http://localhost:9090/api/admin/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=password' \
  --data-urlencode 'username=admin@example.com' \
  --data-urlencode 'password=replace-with-a-strong-password'
```

Application token:

```bash
curl -X POST http://localhost:9090/api/v1/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode 'client_id=<client_id>' \
  --data-urlencode 'client_secret=<client_secret>'
```

Refresh token:

```bash
curl -X POST http://localhost:9090/api/v1/token/refresh \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=refresh_token' \
  --data-urlencode 'refresh_token=<refresh_token>'
```

## Payment methods and accounts

A gateway client runs the flow in that order: ask what can be paid with, let the user
choose, collect that method's details, post the payment.

```text
GET /api/v1/payment-methods?service_type=collection&country=UG
```

It answers only with methods that would route right now — the same eligibility check
routing itself applies — so a method it lists is one a payment can actually use, and a
method whose only provider is unhealthy or circuit-open is not offered. Schemes are not
listed: nothing registers them server-side, because a scheme is free-form text the
provider interprets.

The payment payload is flat, in the order a checkout fills it in:

```json
{
  "payment_method": "momo",
  "scheme": "mtn",
  "account": "256770000000",
  "amount": 50000,
  "currency": "UGX",
  "country": "UG",
  "reference": "ORDER-1",
  "customer": { "name": "Ada Lovelace", "email": "ada@example.com" }
}
```

`payment_method` and `scheme` come from the chosen method; `account` and the optional
`metadata` are what the user entered. `account` may be a mobile number, a bank account, a card token, or a wallet address. Momobase validates only its shape: at most 255 characters and no control characters. What counts as a usable account belongs to the provider, which implements the optional `providers.RequestValidator` interface:

- `ValidateRequest` runs after a route is chosen and before any row is written, so a rejection leaves no transaction behind.
- It may rewrite `Account` and `Scheme` in place. The normalized value is what the transaction records and what webhook matching later compares against, exactly.
- Rewriting anything else — amount, currency, country, reference, payment method, transaction ID — is rejected as a provider error.
- A rejection does not count against the provider's circuit breaker: malformed input is a client error, not an outage.

The engine ships no account-format logic of its own — an adapter that needs mobile numbers, IBANs, or card tokens carries its own rules, so the formats Momobase supports are never bounded by the ones it happens to know about. `examples/customprovider/main.go` shows the whole pattern for a mobile-money API.

`scheme` optionally names a provider-specific network, bank, or card brand, and is matched against nothing — the adapter interprets it. `metadata` passes provider-specific details through to the adapter and is never persisted, so it cannot become a free-form store of identifiers Momobase would then have to protect; it is part of the idempotency hash. `customer` and `recipient` stay nested because they are party context rather than payment details, and they are the one part of the payload that differs by service.

`payment_method` is free-form: it must match an active route, and Momobase only ever compares it. There are no built-in payment-method constants.

## Location and transaction fees

`country` is required on a payment. Each app is pinned to one currency, and each provider account is pinned to one ISO country and currency. A route is eligible only when its service and method match and the provider account matches the payment's country and app currency.

Create a provider account with independent collection and disbursement charges:

```json
{
  "provider_code": "dummy",
  "name": "Sandbox provider",
  "environment": "production",
  "country": "UG",
  "currency": "UGX",
  "charges": {
    "collection": { "type": "percentage", "value": 1000 },
    "disbursement": { "type": "flat", "value": 500 }
  },
  "config": {
    "webhook_secret": "replace-with-long-random-secret"
  }
}
```

Update its location and charges independently of credentials:

```text
PATCH /api/admin/providers/accounts/{id}/settings
{
  "country": "UG",
  "currency": "UGX",
  "charges": {
    "collection": { "type": "percentage", "value": 1000 },
    "disbursement": { "type": "flat", "value": 500 }
  }
}
```

Flat values are currency minor units; percentage values are basis points, rounded half-up. Momobase records the calculated `provider_fee` and `platform_fee` once when it creates a transaction without changing the amount sent to the provider. App-facing APIs expose `platform_fee`; admin transaction APIs expose both values.

## TypeScript SDK and dashboard

Everything Momobase ships to a browser lives under `web/`, which is a **pnpm workspace** — `web/sdk` is the TypeScript SDK and `web/dashboard` is the administration console. pnpm is used directly; corepack is not involved.

The SDK supports token management plus the public and admin API surfaces.

```bash
make sdk-build      # or: pnpm -C web install --frozen-lockfile && pnpm -C web --filter @momobase/sdk run build
```

`--frozen-lockfile` is deliberate: it fails on a `package.json` edited without regenerating `web/pnpm-lock.yaml` rather than silently resolving something new.

The dashboard is a Vite/React application that consumes the SDK through the workspace, so there is one client for the Admin API rather than a hand-maintained twin. It routes on the URL hash, which means a deep link is only ever a request for `/dashboard/` and needs no server-side fallback.

```bash
make dashboard          # build the bundle
make build-dashboard    # bundle + binary with it embedded
```

**Deploying the dashboard separately** is supported: copy `web/dashboard/.env.example` to
`.env` and set `VITE_API_URL` to the API's origin. It defaults to the page's own origin,
which is what the embedded build wants. A cross-origin deployment also needs that origin
listed in the server's `CORS_ALLOWED_ORIGINS`, or the browser blocks every request.

## Local verification

Run these checks after generating dependencies:

```bash
gofmt -w $(find . -name '*.go' -not -path './vendor/*')
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/momobase
make web-typecheck
bash -n scripts/smoke_api.sh
bash -n scripts/smoke_backend.sh
```

Build and typecheck the web workspace separately:

```bash
make web-typecheck
make sdk-build
```

Run the API smoke workflow against a running service:

```bash
CLIENT_ID=<client_id> \
CLIENT_SECRET=<client_secret> \
ADMIN_EMAIL=admin@example.com \
ADMIN_PASSWORD='<password>' \
make smoke-api
```

The payment part of the smoke test runs only when an active MoMo route exists.

## Deployment notes

- The runtime is designed for one application instance.
- Use HTTPS at the reverse proxy or load balancer.
- Never deploy the example encryption or OAuth secrets.
- Keep provider secrets out of source control and application logs.
- `AUTO_MIGRATE=true` is suitable for a single instance; run migrations as a pre-deploy step for anything larger (see below).
- Admin UI access tokens are kept in browser memory rather than persistent browser storage.
