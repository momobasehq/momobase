# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make run                 # go run ./cmd/momobase serve
make build               # binary in bin/
make quality             # fmt-check + vet + test + lint (run before pushing)
make docs                # regenerate docs/swagger.json from swag annotations
make sdk-build           # pnpm -C web build of @momobase/sdk (pnpm, not npm)
make seed-admin          # requires ADMIN_PASSWORD in the environment
make smoke-api           # scripts/smoke_api.sh against a running server
make snapshot            # local GoReleaser build (needs gcc-aarch64-linux-gnu)
```

Tests:

```sh
go test ./internal/service/identity -run TestCoreFlows          # single test / package
go test -race ./...                                            # CI runs this
go test -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...
```

CI gates (`.github/workflows/tests.yml`): `gofmt -l`, `go mod tidy -diff`, `go mod verify`, tests with **≥55% total coverage** (measured with `-coverpkg=./...`, so a test credits every package it exercises, not just its own), the race detector, and golangci-lint (including vet and Staticcheck, version pinned in `.golangci-lint-version`). Govulncheck runs in `.github/workflows/security.yml`.

**The toolchain is pinned.** `go.mod` carries `toolchain go1.26.6` alongside `go 1.25.0`. The `go` line stays at the compatibility floor for applications embedding momobase as a library; the `toolchain` line is what this repository builds and ships with, and it is there because govulncheck's findings are almost all standard-library ones that only a patch release closes. Every workflow resolves its Go through `go-version-file: go.mod`, so raising the pin is the one edit that moves CI, the release build, and the vulnerability scan together — with the Docker base image (`golang:1.26-bookworm`) kept in step.

**cgo is required.** SQLite is linked through cgo; a `CGO_ENABLED=0` binary compiles and even prints `version`, then fails on its first query. Release and Docker builds use `CGO_ENABLED=1` with a static link.

`docs/README.md` is the long-form operational reference (workflows, env vars, curl examples, deployment notes) — read it rather than re-deriving that material.

## Architecture

Momobase is both a Go **library** and a **service**. `cmd/momobase` is a thin standard-library CLI that calls `momobase.New(...)`; anything an embedding application can do, the binary does the same way.

The root package (`momobase.go`, `doc.go`) is the embedding facade: it re-exports `internal/bootstrap` config types and owns instance construction and options. The provider contract and adapter helpers live only in the public `providers` package.

**No providers are registered by default.** `momobase.New` rejects a build with an empty registry rather than starting a server that cannot execute a payment. `cmd/momobase/main.go` registers `dummy` for the shipped binary. Real adapters such as `providers/mtn` remain opt-in packages that embedding applications register explicitly.

Layers, outermost first:

- `internal/http` — `NewRouter` builds a `*fiber.App`. A global `Use` stack (request context, request ID, structured logging, recovery, helmet, CORS, compression) sits in front of route groups: `/api/v1` (app-token payments), `/api/admin` (JWT + one permission per route), `/webhooks` (body-capped), plus the optional embedded dashboard from `web/dashboard`, served at `/dashboard/` when `DASHBOARD_ENABLED` is set **and** the binary was built with the `dashboard` tag. Fiber's own middleware is used rather than reimplemented; `internal/http/middleware` holds only what Fiber has no equivalent for — authentication, `RequirePermission`/`RequireAppScope`, `JSONOnly`, `NoCache`, the slog request logger, and the request-ID length bound.
- `internal/dto` — every request body the API accepts, with its rules as `validate:` tags and its `Normalize`. A payload validates itself; nothing below the HTTP layer re-checks a field's shape.
- `internal/repository` — the **only** package that reaches the database. One repository per persisted entity, a `Set` holding all fourteen, and `UnitOfWork.Within` as the single transaction boundary.
- `internal/service/identity` — identity and tenancy only: admin auth/users, app auth, apps and credentials, authorization, analytics. It imports none of the payment packages.
- `internal/service/webhook` — authenticates, dedupes, and applies provider callbacks to their transactions.
- `internal/service/reconciliation` — the worker-driven settle path for transactions the request path left unresolved.
- `internal/service/payment` — `payment.Orchestrator` runs the create path end to end, plus the idempotency hash.
- `internal/service/routing` — `routing.Engine` selects a provider account for a request; `routing.AdminService` maintains the routes.
- `internal/service/provider` — the adapter lifecycle: `RuntimeManager` (loaded adapters), `Executor` (timeout + circuit breaker), `AdminService` (accounts and encrypted config), `HealthService`. Distinct from the top-level `providers` package, which is the contract an adapter implements.
- `internal/service/audit` — `audit.Service`, the best-effort audit-log recorder. A leaf, so any service package may take one.
- `providers` — the public adapter contract plus adapter helpers (`DoJSON`, `Redact`, configuration accessors, amount/status normalization, references). `providers/dummy` is the in-tree simulator and `providers/mtn` is the optional MTN Mobile Money adapter; applications register either package explicitly.
- `internal/domain` — GORM models, the shared service/status/circuit constants, and behaviour belonging to a model (`Transaction.Transition`, `AdminUser.ActorID`).
- `internal/utils` — dependency-free helpers shared across the module: identifier/account shape checks, ISO-3166 country normalization, raw-payload redaction, and the map, string, and error helpers an adapter reads its decrypted config with. Nothing here touches the database.
- `internal/platform` — AES-256-GCM encryptor, HS256 JWT token manager, bcrypt, IDs, strict JSON request decoding, the `{success,data,error}` response envelope, pagination.
- `internal/workers` — one `Manager` owning all background goroutines (`health`, `reconciliation`, `cleanup`), stopped by context cancellation.
- `internal/bootstrap` — env config + validation, database open/migrate, dependency wiring in `NewApp`, and the serve/close lifecycle.

### Payment path

`POST /api/v1/collections` → app-bearer + scope middleware → `payment.Orchestrator.Create` → `dto.Check` (`Normalize`, then the `validate:` rules) → `paymentRequestHash` → idempotency lookup by `(app_id, idempotency_key)` → `routing.Engine.SelectProvider` → `provider.Executor.ValidateRequest` (the provider's optional `providers.RequestValidator`) → persist `Transaction` + `TransactionAttempt` → `provider.Executor.Collect` (timeout + circuit breaker + structured logging) → `persist` applies the state machine.

**That order is load-bearing.** The hash is taken over the *normalized* request, so two spellings of one payment are one request; and it is taken *before* the provider's `RequestValidator` may rewrite `Account`/`Scheme`, so a provider's rewrite cannot change the identity of a request already made. Getting it wrong changes what counts as a replay without failing anything, which is why `TestIdempotencyIsDecidedAfterNormalizationAndBeforeTheProvider` pins both halves.

**Accounts are opaque, and the payload is flat.** A payment carries a top-level `account` (mobile number, bank account, card token, wallet address) plus optional `scheme` and `metadata`; the engine only checks the account's shape. `GET /api/v1/payment-methods` lists what a client may pay with, reusing `routing.Engine`'s own candidate check so the listing cannot drift from routing. A provider that needs a particular kind of identifier implements `providers.RequestValidator`, which runs after routing and before any row is written, may rewrite `Account`/`Scheme` only, and whose rejection is a client error that never trips the circuit breaker. The engine carries no account-format logic of its own — no phone, IBAN, or card validation — so an adapter that needs a format brings its own. `payment_method` is free-form and only ever compared against a route — there are no payment-method constants.

`routing.Engine` picks the lowest-`priority` active route whose provider account is active, which declares a capability for the service, which is eligible for the request country, and whose circuit and health snapshot are not open/down. Country eligibility (`countryEligible`): an account that declares `countries` requires a request country it lists; an account that declares none is unrestricted. There is no fallback among country-scoped accounts.

**Capabilities name the service only** (`Capability{ServiceType}`). Which rails reach an account is decided by its routes, so `providers.Supports(caps, service)` is the only capability question.

### Provider runtime

`provider.RuntimeManager` holds initialized adapters in a mutex-guarded map keyed by provider account ID. `Reload` decrypts the stored config, constructs the adapter, runs `Init` + `HealthCheck`, and only then swaps it in — a failed reload leaves the previous working runtime untouched. `provider.AdminService` commits the DB change first, then reloads synchronously and **rolls the row back** if the reload fails (see `UpdateConfig`, `UpdateCountries`, `Activate`).

Each runtime carries its own circuit breaker: 3 consecutive failures open it, it half-opens after 30s and allows one probe. Caller cancellation is deliberately not counted as a provider failure.

## Invariants

These are load-bearing; breaking one is a silent correctness bug.

- **The database is reachable only through `internal/repository`.** The
  `depguard` rule denies `gorm.io/gorm` to `internal/service`, `internal/http`,
  `internal/platform`, `internal/utils`, `internal/domain` and `internal/workers`;
  `bootstrap` opens the handle and `migrations` owns the schema, so those keep it. A
  service takes repositories, never a `*gorm.DB`, and cannot widen a `WHERE` clause
  or start a transaction without the linter refusing it.

- **Every** transaction status change goes through `(*domain.Transaction).Transition`, which enforces the legal state graph. Never assign `Status` directly. Use `domain.Terminal()` rather than comparing status strings.
- `repository.UnitOfWork.Within` is the only place a DB transaction starts, and the
  `*repository.Set` it hands the closure is transaction-bound by construction.
  **Never** wrap a provider network call in one.
- Zero-row writes are errors: the repository base turns them into
  `repository.ErrNotFound`, so a write that matched nothing never reads as success.
  Use `repository.IsNotFound` rather than reaching for the driver's own error.
- Anything derived from a provider — errors, raw payloads — passes through `providers.Redact` / `utils.RedactRawMap` before it is logged or persisted.
- Request handlers launch no detached goroutines. Background work belongs to `workers.Manager` and stops on context cancellation.
- Idempotency is `(app_id, idempotency_key)` unique index **plus** `RequestHash` comparison; a reused key with a different body is an error, not a replay. The hash is taken over the DTO *after* `Normalize` and *before* the provider's `RequestValidator`, so two spellings of one request are one request and a provider's rewrite cannot change an identity already fixed.

- **A request body validates itself.** Rules live on the `internal/dto` type as
  `validate:` tags, and `dto.Check` normalizes before it validates. A service never
  re-checks a field's shape; it decides only what needs the database or the caller —
  whether the row exists, whether the role is real, whether the caller may act.
- Webhooks authenticate with a constant-time compare of `X-Webhook-Secret` against the account's decrypted `webhook_secret`, dedupe on `(provider_account_id, payload_hash)` via `ON CONFLICT DO NOTHING`, and are validated field-by-field against the target transaction before being applied. `ProviderWebhookEvent.Account` is compared **exactly** against `Transaction.CustomerAccount`, so an adapter that normalizes in `ValidateRequest` must report the same form here.
- Secrets are never stored in plaintext: provider configs are AES-GCM encrypted, passwords and client secrets are bcrypt/SHA-256 hashed, and models mark them `json:"-"`.
- **Authorization is data, not literals.** `domain.Permissions` is the only place a permission is defined; `AuthzService.Seed` upserts it on every boot. Every admin route names exactly one permission via `middlewarex.RequirePermission`, and there is no `RequireRole`. Effective permissions are resolved in `activeUser` per request — never from token claims, or revocation would wait for a refresh. `super_admin` holds `*`. System roles are read-only so re-seeding them is safe.

## Making changes

**Adding a provider.** Implement the eight-method `providers.PaymentProvider` interface and a `func(*slog.Logger) providers.PaymentProvider` factory; register with `WithProvider(code, factory)`. Implement `providers.RequestValidator` too when the rail constrains what an account may be. Use the constants and helpers in `providers` (`providers.ServiceCollection`, `providers.PaymentStatus`, …). `examples/customprovider/main.go` is a complete reference implementation. Config always arrives as `providers.ProviderConfig`, includes the provider account's authoritative `environment` value, and must include `webhook_secret`.

**Exposing a new helper to third-party providers.** Put it in `providers/`; it may delegate pure value handling to `internal/utils`. `momobase_test.go` compiles a stub provider from the public contract so regressions fail at compile time.

**Adding an HTTP endpoint.** Request payload in `internal/dto` with its `validate:` tags and `Normalize` → handler in the matching `internal/http/*` package with swag annotations, decoding through `bind[dto.X]` so the payload validates itself → register in `internal/http/router.go` with its guards (`RequirePermission`/`RequireAppScope`, `JSONOnly`, `NoCache`) → any new query goes on a repository, never in the handler → `make docs` → mirror in `web/sdk/src/client.ts` (+`types.ts`), which the dashboard consumes through the pnpm workspace. There is no second client to keep in step.

An admin handler dependency is added to `adminh.Deps`, never as another positional argument: its fields come from five packages and several share a shape, so a swapped pair would still compile.

**Adding a persisted entity.** Model in `internal/domain` → a repository in `internal/repository` with methods named for the question they answer, not for CRUD → a field on `repository.Set` → a migration in `internal/migrations`. Nothing outside the repository may name its table.

**Adding a config value.** `internal/bootstrap/config.go` (`env`/`boolean`/`duration`/`list`), a rule in `Config.Validate` if it is unsafe by default in staging/production, then `.env.example`, `.env.docker.example`, and `docker-compose.yml`.

Note that `.env` autoload comes from a `godotenv/autoload` import in `cmd/momobase/main.go` only — embedding applications supply their own configuration.

## Conventions

- Every exported symbol has a doc comment, including struct fields on API payload types. Match that density.
- Line limit is 160 (`golines` via golangci `formatters`); long call signatures are broken one argument per line.
- Small constructors and predicates are written tightly, often without a blank line between them. Follow the surrounding file.
- Tests use in-memory or temp-dir SQLite (`internal/testsupport` `New(t)` is the shared fixture), no mocking framework, and `t.Fatalf("Method() error = %v", err)`-style messages.
- Tests that use `testsupport` **must** be external (`package foo_test`): `testsupport` imports the packages under test, so an in-package test importing it is a cycle. A test covering an unexported helper stays in-package and builds its own fixtures — see `internal/service/identity/internal_test.go`, where all three cases are pure functions needing no database.
- `momobase_test.go` is deliberately in `momobase_test` to prove the public API works from outside the module.
- Commit messages are short and concise: a conventional-commit subject line, and a body only when it says something the diff cannot.
