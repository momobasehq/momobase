# Docs

## Architecture

The backend is a modular monolith with six main areas:

- `internal/http`: standard-library routing, middleware, public/admin/webhook handlers.
- `internal/services`: authentication, payments, routing, provider runtime, health, webhooks, reconciliation, and auditing.
- `providers`: the public provider contract and shared adapter helpers, plus the in-tree reference adapter in `providers/dummy`. Nothing here is registered automatically; a build chooses its providers through `momobase.WithProvider`.
- `internal/store`: database helpers and transaction boundaries.
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
2. The application creates a collection or disbursement with an `Idempotency-Key`.
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

### Admin sessions

- Admins authenticate with password grant semantics.
- Refresh-token rotation revokes the old session transactionally.
- Disabling an admin invalidates their active sessions.
- Role middleware separates `super_admin` operations from operations/read access.

## Worker lifecycle

The process starts only enabled workers:

- `health`: probes active provider runtimes and updates their current health state.
- `reconciliation`: converges unresolved payments with provider status APIs.
- `cleanup`: removes expired admin and application sessions.

On `SIGINT` or `SIGTERM`, the root context is cancelled, the HTTP server performs a bounded graceful shutdown, workers stop, and the process waits for all worker goroutines.

## Requirements

- Go 1.23 or later
- Node.js 20 or later for the TypeScript SDK
- SQLite, PostgreSQL, or MySQL
- `curl` and `jq` for the smoke script

A `go.sum` file is intentionally not included. Generate it locally from `go.mod`:

```bash
go mod tidy
```

## Environment loading

Momobase uses `github.com/joho/godotenv/autoload` to load a local `.env` file automatically when present. Existing process environment variables keep precedence, so container and production configuration are not overwritten.

## Local development

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

Enable the browser admin panel:

```env
ADMIN_FRONTEND_ENABLED=true
```

Then open:

```text
http://localhost:9090/admin/
```

## Docker Compose

```bash
cp .env.docker.example .env
openssl rand -base64 32
openssl rand -hex 32
openssl rand -hex 32
docker compose up -d --build
```

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

## Payment accounts

Every payment carries an `account`, and the engine treats it as an opaque identifier:

```json
{
  "payment_method": "momo",
  "amount": 50000,
  "currency": "UGX",
  "country": "UG",
  "reference": "ORDER-1",
  "account": { "account": "256770000000", "scheme": "mtn" },
  "customer": { "name": "Ada Lovelace", "email": "ada@example.com" }
}
```

`account.account` may be a mobile number, a bank account, a card token, or a wallet address. Momobase validates only its shape: at most 255 characters and no control characters. What counts as a usable account belongs to the provider, which implements the optional `providers.RequestValidator` interface:

- `ValidateRequest` runs after a route is chosen and before any row is written, so a rejection leaves no transaction behind.
- It may rewrite `Account` and `Scheme` in place. The normalized value is what the transaction records and what webhook matching later compares against, exactly.
- Rewriting anything else — amount, currency, country, reference, payment method, transaction ID — is rejected as a provider error.
- A rejection does not count against the provider's circuit breaker: malformed input is a client error, not an outage.

The engine ships no account-format logic of its own — an adapter that needs mobile numbers, IBANs, or card tokens carries its own rules, so the formats Momobase supports are never bounded by the ones it happens to know about. `examples/customprovider/main.go` shows the whole pattern for a mobile-money API.

`account.scheme` optionally names a provider-specific network, bank, or card brand. `account.metadata` passes provider-specific details through to the adapter and is never persisted. `customer` and `recipient` are optional context carrying a name and email.

`payment_method` is free-form: it must match an active route, and Momobase only ever compares it. There are no built-in payment-method constants.

## Country routing

`country` is optional on a payment. A provider account that lists `countries` — ISO 3166-1 alpha-2 codes, for example `["UG", "RW"]` — serves only requests naming one of them. An account with an empty list is unrestricted and is eligible for any request, including one that carries no country, which is how a rail with no country notion is modelled. There is no global fallback among country-scoped accounts.

Create a country-scoped provider account:

```json
{
  "provider_code": "dummy",
  "name": "Sandbox provider",
  "environment": "production",
  "countries": ["UG", "RW"],
  "config": {
    "webhook_secret": "replace-with-long-random-secret"
  }
}
```

Update its supported countries independently of credentials, passing an empty array to leave the account unrestricted:

```text
PATCH /api/admin/providers/accounts/{id}/countries
{ "countries": ["UG", "RW"] }
```

A balance lookup may include `?country=UG`; it is required only when an account declares more than one country. Active-balance queries return one result per provider and supported country, or one result with an empty country for an unrestricted provider.

## TypeScript SDK and admin panel

The SDK is in `packages/sdk` and supports token management plus the public and admin API surfaces.

```bash
cd packages/sdk
npm install
npm run build
```

The minimal browser admin panel is in `web/admin` and is served by the Go binary when `ADMIN_FRONTEND_ENABLED=true`.

## Local verification

Run these checks after generating dependencies:

```bash
gofmt -w $(find . -name '*.go' -not -path './vendor/*')
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/momobase
node --check web/admin/app.js
node --check web/admin/sdk.js
bash -n scripts/smoke_api.sh
bash -n scripts/smoke_backend.sh
```

Build the SDK separately:

```bash
cd packages/sdk
npm install
npm run build
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
