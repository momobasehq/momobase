# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make run                 # go run ./cmd/momobase serve
make build               # binary in bin/
make quality             # fmt-check + vet + test + lint (run before pushing)
make docs                # regenerate docs/swagger.{json,yaml} from swag annotations
make sdk-build           # cd packages/sdk && npm install && npm run build
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

CI gates (`.github/workflows/tests.yml`): `gofmt -l`, `go mod tidy -diff`, `go mod verify`, vet, tests with **≥50% total coverage**, race detector, golangci-lint (version pinned in `.golangci-lint-version`), Staticcheck, and govulncheck.

**cgo is required.** SQLite is linked through cgo; a `CGO_ENABLED=0` binary compiles and even prints `version`, then fails on its first query. Release and Docker builds use `CGO_ENABLED=1` with a static link.

`docs/README.md` is the long-form operational reference (workflows, env vars, curl examples, deployment notes) — read it rather than re-deriving that material.

## Architecture

Momobase is both a Go **library** and a **service**. `cmd/momobase` is a thin cobra CLI that calls `momobase.New(...)`; anything an embedding application can do, the binary does the same way.

The root package (`momobase.go`, `provider.go`, `doc.go`) is a facade: it re-exports `internal/bootstrap` config types and the `providers` contract as **type aliases**, so a type implementing `momobase.PaymentProvider` satisfies the engine directly. Keep it a facade — behavior belongs in `internal/`.

**No providers are registered by default.** `bootstrap.buildRegistry` rejects a build with an empty registry rather than starting a server that cannot execute a payment. `cmd/momobase/main.go` registers `dummy` for the shipped binary — real adapters live outside this module.

Layers, outermost first:

- `internal/http` — `NewRouter` wires every route with an explicit middleware chain (`chain`/`route` helpers). Route groups: `public` (app-token payments), `admin` (bearer + role), `webhooks` (body-capped), plus the optional embedded panel from `web/admin`.
- `internal/services` — all business logic: auth, payment orchestration, routing, provider runtime/admin, webhooks, reconciliation, health, audit.
- `providers` — the public adapter contract plus shared helpers (`DoJSON`, `TokenCache`, `Redact`, config accessors, amount/status normalization). `providers/dummy` is the in-tree reference adapter: it simulates payments in memory, so it is registered like any third-party one and needs no credentials.
- `internal/domain` — GORM models and the shared status/circuit constants.
- `internal/store` — the only transaction boundary (`Within`) and write-result helper (`Affected`).
- `internal/platform` — AES-256-GCM encryptor, HMAC token manager, bcrypt, IDs, JSON request decoding, the `{success,data,error}` response envelope, pagination.
- `internal/workers` — one `Manager` owning all background goroutines (`health`, `reconciliation`, `cleanup`), stopped by context cancellation.
- `internal/bootstrap` — env config + validation, database open/migrate, dependency wiring in `NewApp`, and the serve/close lifecycle.

### Payment path

`POST /api/v1/collections` → app-bearer + scope middleware → `PaymentOrchestrator.Create` → validate & normalize (country, currency, MSISDN via libphonenumber) → idempotency lookup by `(app_id, idempotency_key)` → `RouteEngine.SelectProvider` → persist `Transaction` + `TransactionAttempt` → `RuntimeProviderExecutor.Collect` (timeout + circuit breaker + structured logging) → `persist` applies the state machine.

`RouteEngine` picks the lowest-`priority` active route whose provider account is active, whose **explicit `countries` list contains the request country**, whose runtime capabilities match, and whose circuit and health snapshot are not open/down. There is no global or fallback country.

### Provider runtime

`ProviderRuntimeManager` holds initialized adapters in a mutex-guarded map keyed by provider account ID. `Reload` decrypts the stored config, constructs the adapter, runs `Init` + `HealthCheck`, and only then swaps it in — a failed reload leaves the previous working runtime untouched. `ProviderAdminService` commits the DB change first, then reloads synchronously and **rolls the row back** if the reload fails (see `UpdateConfig`, `UpdateCountries`, `Activate`).

Each runtime carries its own circuit breaker: 3 consecutive failures open it, it half-opens after 30s and allows one probe. Caller cancellation is deliberately not counted as a provider failure.

## Invariants

These are load-bearing; breaking one is a silent correctness bug.

- **Every** transaction status change goes through `services.transition`, which enforces the legal state graph. Use `terminal()` rather than comparing status strings.
- `store.Within` is the only place a DB transaction starts. **Never** wrap a provider network call in one.
- Zero-row writes are errors: wrap updates in `store.Affected` so they become `gorm.ErrRecordNotFound`.
- Anything derived from a provider — errors, raw payloads — passes through `providers.Redact` / `redactRawMap` before it is logged or persisted.
- Request handlers launch no detached goroutines. Background work belongs to `workers.Manager` and stops on context cancellation.
- Idempotency is `(app_id, idempotency_key)` unique index **plus** `RequestHash` comparison; a reused key with a different body is an error, not a replay.
- Webhooks authenticate with a constant-time compare of `X-Webhook-Secret` against the account's decrypted `webhook_secret`, dedupe on `(provider_account_id, payload_hash)` via `ON CONFLICT DO NOTHING`, and are validated field-by-field against the target transaction before being applied.
- Secrets are never stored in plaintext: provider configs are AES-GCM encrypted, passwords and client secrets are bcrypt/SHA-256 hashed, and models mark them `json:"-"`.

## Making changes

**Adding a provider.** Implement the eight-method `providers.PaymentProvider` interface and a `func(*slog.Logger) providers.PaymentProvider` factory; register with `WithProvider(code, factory)`. Bundled adapters may import `internal/domain` for constants; out-of-tree providers use the root package's re-exports (`momobase.ServiceCollection`, `momobase.PaymentStatus`, …). `examples/customprovider/main.go` is a complete reference implementation. Config always arrives as `ProviderConfig` and must include `webhook_secret`.

**Exposing a new helper to third-party providers.** Add it to `providers/`, then re-export it from the root `provider.go` — the root package is the documented surface, and `momobase_test.go` compiles a stub provider from exported types only, so it fails if that surface regresses.

**Adding an HTTP endpoint.** Handler in the matching `internal/http/*` package with swag annotations → register in `internal/http/router.go` with its middleware (role/scope, `JSONOnly`, `NoCache`) → `make docs` → mirror in `packages/sdk/src/client.ts` (+`types.ts`) and, for admin endpoints, `web/admin/sdk.js` and `web/admin/app.js`. The browser SDK is a hand-maintained JS twin of the TypeScript one, not a build output.

**Adding a config value.** `internal/bootstrap/config.go` (`env`/`boolean`/`duration`/`list`), a rule in `Config.Validate` if it is unsafe by default in staging/production, then `.env.example`, `.env.docker.example`, and `docker-compose.yml`.

Note that `.env` autoload comes from a `godotenv/autoload` import in `cmd/momobase/main.go` only — embedding applications supply their own configuration.

## Conventions

- Every exported symbol has a doc comment, including struct fields on API payload types. Match that density.
- Line limit is 160 (`golines` via golangci `formatters`); long call signatures are broken one argument per line.
- Small constructors and predicates are written tightly, often without a blank line between them. Follow the surrounding file.
- Tests use in-memory or temp-dir SQLite (`internal/services/core_test.go` `stack()` is the shared fixture), no mocking framework, and `t.Fatalf("Method() error = %v", err)`-style messages.
- `internal/services` tests are in-package (they exercise unexported helpers); `momobase_test.go` is deliberately in `momobase_test` to prove the public API works from outside the module.
