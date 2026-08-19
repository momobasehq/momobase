# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make run                 # go run ./cmd/momobase serve
make build               # binary in bin/
make quality             # fmt-check + vet + test + lint (run before pushing)
make docs                # regenerate docs/swagger.{json,yaml} from swag annotations
make sdk-build           # pnpm -C web build of @momobase/sdk (pnpm, not npm)
make seed-admin          # requires ADMIN_PASSWORD in the environment
make smoke-api           # scripts/smoke_api.sh against a running server
make snapshot            # local GoReleaser build (needs gcc-aarch64-linux-gnu)
```

Tests:

```sh
go test ./internal/services -run TestCoreFlows                 # single test / package
go test -race ./...                                            # CI runs this
go test -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...
```

CI gates (`.github/workflows/tests.yml`): `gofmt -l`, `go mod tidy -diff`, `go mod verify`, vet, tests with **≥55% total coverage** (measured with `-coverpkg=./...`, so a test credits every package it exercises, not just its own), race detector, golangci-lint (version pinned in `.golangci-lint-version`), Staticcheck, and govulncheck.

**cgo is required.** SQLite is linked through cgo; a `CGO_ENABLED=0` binary compiles and even prints `version`, then fails on its first query. Release and Docker builds use `CGO_ENABLED=1` with a static link.

`docs/README.md` is the long-form operational reference (workflows, env vars, curl examples, deployment notes) — read it rather than re-deriving that material.

## Architecture

Momobase is both a Go **library** and a **service**. `cmd/momobase` is a thin cobra CLI that calls `momobase.New(...)`; anything an embedding application can do, the binary does the same way.

The root package (`momobase.go`, `provider.go`, `doc.go`) is a facade: it re-exports `internal/bootstrap` config types and the `providers` contract as **type aliases**, so a type implementing `momobase.PaymentProvider` satisfies the engine directly. Keep it a facade — behavior belongs in `internal/`.

**No providers are registered by default.** `bootstrap.buildRegistry` rejects a build with an empty registry rather than starting a server that cannot execute a payment. `cmd/momobase/main.go` registers `dummy` for the shipped binary — real adapters live outside this module.

Layers, outermost first:

- `internal/http` — `NewRouter` wires every route with an explicit middleware chain (`chain`/`route` helpers). Route groups: `public` (app-token payments), `admin` (bearer + role), `webhooks` (body-capped), plus the optional embedded dashboard from `web/dashboard`, served at `/dashboard/` when `DASHBOARD_ENABLED` is set **and** the binary was built with the `dashboard` tag.
- `internal/services` — identity and tenancy only: admin auth/users, app auth, apps and credentials, authorization, analytics. It imports none of the payment packages.
- `internal/webhook` — authenticates, dedupes, and applies provider callbacks to their transactions.
- `internal/reconciliation` — the worker-driven settle path for transactions the request path left unresolved.
- `internal/payment` — `payment.Orchestrator` runs the create path end to end, plus the request DTOs, payload validation, and the idempotency hash.
- `internal/routing` — `routing.Engine` selects a provider account for a request; `routing.AdminService` maintains the routes.
- `internal/provider` — the adapter lifecycle: `RuntimeManager` (loaded adapters), `Executor` (timeout + circuit breaker), `AdminService` (accounts and encrypted config), `HealthService`. Distinct from the top-level `providers` package, which is the contract an adapter implements.
- `internal/audit` — `audit.Service`, the best-effort audit-log recorder. A leaf, so any service package may take one.
- `providers` — the public adapter contract plus shared helpers (`DoJSON`, `TokenCache`, `Redact`, config accessors, amount/status normalization). `providers/dummy` is the in-tree reference adapter: it simulates payments in memory, so it is registered like any third-party one and needs no credentials.
- `internal/domain` — GORM models, the shared service/status/circuit constants, and behaviour belonging to a model (`Transaction.Transition`, `AdminUser.ActorID`).
- `internal/utils` — dependency-free helpers shared across services: identifier/account shape checks, ISO-3166 country normalization, and raw-payload redaction. Nothing here touches the database.
- `internal/store` — the only transaction boundary (`Within`) and write-result helper (`Affected`).
- `internal/platform` — AES-256-GCM encryptor, HMAC token manager, bcrypt, IDs, JSON request decoding, the `{success,data,error}` response envelope, pagination.
- `internal/workers` — one `Manager` owning all background goroutines (`health`, `reconciliation`, `cleanup`), stopped by context cancellation.
- `internal/bootstrap` — env config + validation, database open/migrate, dependency wiring in `NewApp`, and the serve/close lifecycle.

### Payment path

`POST /api/v1/collections` → app-bearer + scope middleware → `payment.Orchestrator.Create` → validate & normalize (country, currency, payment method, account shape) → idempotency lookup by `(app_id, idempotency_key)` → `routing.Engine.SelectProvider` → `provider.Executor.ValidateRequest` (the provider's optional `providers.RequestValidator`) → persist `Transaction` + `TransactionAttempt` → `provider.Executor.Collect` (timeout + circuit breaker + structured logging) → `persist` applies the state machine.

**Accounts are opaque, and the payload is flat.** A payment carries a top-level `account` (mobile number, bank account, card token, wallet address) plus optional `scheme` and `metadata`; the engine only checks the account's shape. `GET /api/v1/payment-methods` lists what a client may pay with, reusing `routing.Engine`'s own candidate check so the listing cannot drift from routing. A provider that needs a particular kind of identifier implements `providers.RequestValidator`, which runs after routing and before any row is written, may rewrite `Account`/`Scheme` only, and whose rejection is a client error that never trips the circuit breaker. The engine carries no account-format logic of its own — no phone, IBAN, or card validation — so an adapter that needs a format brings its own. `payment_method` is free-form and only ever compared against a route — there are no payment-method constants.

`routing.Engine` picks the lowest-`priority` active route whose provider account is active, which declares a capability for the service, which is eligible for the request country, and whose circuit and health snapshot are not open/down. Country eligibility (`countryEligible`): an account that declares `countries` requires a request country it lists; an account that declares none is unrestricted. There is no fallback among country-scoped accounts.

**Capabilities name the service only** (`Capability{ServiceType}`). Which rails reach an account is decided by its routes, so `providers.Supports(caps, service)` is the only capability question.

### Provider runtime

`provider.RuntimeManager` holds initialized adapters in a mutex-guarded map keyed by provider account ID. `Reload` decrypts the stored config, constructs the adapter, runs `Init` + `HealthCheck`, and only then swaps it in — a failed reload leaves the previous working runtime untouched. `provider.AdminService` commits the DB change first, then reloads synchronously and **rolls the row back** if the reload fails (see `UpdateConfig`, `UpdateCountries`, `Activate`).

Each runtime carries its own circuit breaker: 3 consecutive failures open it, it half-opens after 30s and allows one probe. Caller cancellation is deliberately not counted as a provider failure.

## Invariants

These are load-bearing; breaking one is a silent correctness bug.

- **Every** transaction status change goes through `(*domain.Transaction).Transition`, which enforces the legal state graph. Never assign `Status` directly. Use `domain.Terminal()` rather than comparing status strings.
- `store.Within` is the only place a DB transaction starts. **Never** wrap a provider network call in one.
- Zero-row writes are errors: wrap updates in `store.Affected` so they become `gorm.ErrRecordNotFound`.
- Anything derived from a provider — errors, raw payloads — passes through `providers.Redact` / `utils.RedactRawMap` before it is logged or persisted.
- Request handlers launch no detached goroutines. Background work belongs to `workers.Manager` and stops on context cancellation.
- Idempotency is `(app_id, idempotency_key)` unique index **plus** `RequestHash` comparison; a reused key with a different body is an error, not a replay. The hash is taken before provider normalization, so two spellings of one account are two different requests.
- Webhooks authenticate with a constant-time compare of `X-Webhook-Secret` against the account's decrypted `webhook_secret`, dedupe on `(provider_account_id, payload_hash)` via `ON CONFLICT DO NOTHING`, and are validated field-by-field against the target transaction before being applied. `ProviderWebhookEvent.Account` is compared **exactly** against `Transaction.CustomerAccount`, so an adapter that normalizes in `ValidateRequest` must report the same form here.
- Secrets are never stored in plaintext: provider configs are AES-GCM encrypted, passwords and client secrets are bcrypt/SHA-256 hashed, and models mark them `json:"-"`.
- **Authorization is data, not literals.** `domain.Permissions` is the only place a permission is defined; `AuthzService.Seed` upserts it on every boot. Every admin route names exactly one permission via `middlewarex.RequirePermission`, and there is no `RequireRole`. Effective permissions are resolved in `activeUser` per request — never from token claims, or revocation would wait for a refresh. `super_admin` holds `*`. System roles are read-only so re-seeding them is safe.

## Making changes

**Adding a provider.** Implement the eight-method `providers.PaymentProvider` interface and a `func(*slog.Logger) providers.PaymentProvider` factory; register with `WithProvider(code, factory)`. Implement `providers.RequestValidator` too when the rail constrains what an account may be. Bundled adapters may import `internal/domain` for constants; out-of-tree providers use the root package's re-exports (`momobase.ServiceCollection`, `momobase.PaymentStatus`, …). `examples/customprovider/main.go` is a complete reference implementation. Config always arrives as `ProviderConfig` and must include `webhook_secret`.

**Exposing a new helper to third-party providers.** Add it to `providers/`, then re-export it from the root `provider.go` — the root package is the documented surface, and `momobase_test.go` compiles a stub provider from exported types only, so it fails if that surface regresses.

**Adding an HTTP endpoint.** Handler in the matching `internal/http/*` package with swag annotations → register in `internal/http/router.go` with its middleware (role/scope, `JSONOnly`, `NoCache`) → `make docs` → mirror in `web/sdk/src/client.ts` (+`types.ts`), which the dashboard consumes through the pnpm workspace. There is no second client to keep in step.

**Adding a config value.** `internal/bootstrap/config.go` (`env`/`boolean`/`duration`/`list`), a rule in `Config.Validate` if it is unsafe by default in staging/production, then `.env.example`, `.env.docker.example`, and `docker-compose.yml`.

Note that `.env` autoload comes from a `godotenv/autoload` import in `cmd/momobase/main.go` only — embedding applications supply their own configuration.

## Conventions

- Every exported symbol has a doc comment, including struct fields on API payload types. Match that density.
- Line limit is 160 (`golines` via golangci `formatters`); long call signatures are broken one argument per line.
- Small constructors and predicates are written tightly, often without a blank line between them. Follow the surrounding file.
- Tests use in-memory or temp-dir SQLite (`internal/testsupport` `New(t)` is the shared fixture), no mocking framework, and `t.Fatalf("Method() error = %v", err)`-style messages.
- Tests that use `testsupport` **must** be external (`package foo_test`): `testsupport` imports the packages under test, so an in-package test importing it is a cycle. A test covering an unexported helper stays in-package and builds its own fixtures — see `internal/services/internal_test.go`, where all three cases are pure functions needing no database.
- `momobase_test.go` is deliberately in `momobase_test` to prove the public API works from outside the module.
