# Momobase

Momobase is a compact, single-instance mobile-money payment aggregator. It exposes a unified API for applications, supports MTN MoMo and Airtel Money adapters, and includes an admin API, TypeScript SDK, and minimal browser admin panel.

The refactor keeps the public payment API and database-backed payment behavior while intentionally replacing the admin provider `country` field with `countries`, and removing redundant repository interfaces, provider decorator stacks, outbox processing, legacy compatibility paths, framework-only routing, and duplicated provider/configuration machinery.

## Architecture

The backend is a modular monolith with six main areas:

- `internal/http`: standard-library routing, middleware, public/admin/webhook handlers.
- `internal/services`: authentication, payments, routing, provider runtime, health, webhooks, reconciliation, and auditing.
- `internal/providers`: normalized provider contracts plus MTN and Airtel adapters.
- `internal/store`: database helpers and transaction boundaries.
- `internal/workers`: bounded health, reconciliation, and session-cleanup loops.
- `internal/bootstrap`: configuration, database initialization, dependency wiring, migration, and process lifecycle.

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
3. Momobase validates the request and claims the idempotency key.
4. A transaction and provider attempt are persisted.
5. Routing selects the highest-priority active provider account whose explicit `countries` list contains the request country.
6. The provider executor checks runtime health and calls the selected MTN or Airtel adapter with a bounded context.
7. The normalized provider result is persisted through the transaction state machine.
8. The application reads the transaction by Momobase ID or its own reference.

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

1. A super administrator creates an MTN or Airtel provider account.
2. Provider credentials and settings are encrypted before persistence.
3. Testing builds a temporary provider instance and performs a health check without changing the active runtime.
4. Activation validates configuration, builds the runtime, verifies health, commits the active state, and installs it in memory.
5. Configuration changes reload the active runtime synchronously after commit.
6. Deactivation commits the inactive state and removes the runtime immediately.
7. Routes connect provider accounts to collection or disbursement traffic by payment method and priority; each provider account’s explicit country list determines where that route is eligible.

A provider is routable only when its account and route are active, its capabilities match the request, its `countries` list contains the transaction country, and a healthy runtime exists.

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
http://localhost:8080/admin/
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
curl -X POST http://localhost:8080/api/admin/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=password' \
  --data-urlencode 'username=admin@example.com' \
  --data-urlencode 'password=replace-with-a-strong-password'
```

Application token:

```bash
curl -X POST http://localhost:8080/api/v1/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode 'client_id=<client_id>' \
  --data-urlencode 'client_secret=<client_secret>'
```

Refresh token:

```bash
curl -X POST http://localhost:8080/api/v1/token/refresh \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=refresh_token' \
  --data-urlencode 'refresh_token=<refresh_token>'
```

## Explicit country routing and phone normalization

Each provider account has a non-empty `countries` array of ISO 3166-1 alpha-2 codes, for example `["UG", "RW"]`. Collection and disbursement requests require one country code, and Momobase considers only active routed providers whose array contains that country. There is no universal or global fallback.

Phone numbers are parsed with the Go port of Google’s libphonenumber metadata, validated against the transaction country, required to be mobile-capable, and stored as E.164 digits without the leading `+`. Local, international, and bare international-digit input are accepted when valid.

Create a provider account with explicit countries:

```json
{
  "provider_code": "airtel_money",
  "name": "Airtel East Africa",
  "environment": "production",
  "countries": ["UG", "RW"],
  "config": {
    "webhook_secret": "replace-with-long-random-secret"
  }
}
```

Update its supported countries independently of credentials:

```text
PATCH /api/admin/providers/accounts/{id}/countries
{ "countries": ["UG", "RW"] }
```

A balance lookup may include `?country=UG`; it is required when an account supports more than one country. Active-balance queries return one result per provider and supported country.

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
- `AUTO_MIGRATE=true` is suitable for initial staging; controlled migrations are safer for production.
- Admin UI access tokens are kept in browser memory rather than persistent browser storage.
