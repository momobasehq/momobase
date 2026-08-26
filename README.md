<img width=75 height=75 src='./web/dashboard/public/logo.svg' />

# Momobase

Momobase is a compact, single-instance payment orchestration service.
It exposes a unified API for applications, routes payments to providers you supply, and includes an Admin API, TypeScript SDK, and an embedded administration dashboard.

It runs as a standalone service (`cmd/momobase`) and as a Go package that applications embed and extend with their own payment providers.

## Use as a package

The engine registers no providers of its own — you choose which ones the build carries, whether they ship with Momobase or you wrote them:

```go
import (
	"github.com/momobasehq/momobase"
)

instance, err := momobase.New(
	momobase.WithProvider("acme_pay", acme.New),
	momobase.WithProvider("acme_bank", acmebank.New),
)
if err != nil {
	log.Fatal(err)
}
defer instance.Close()

log.Fatal(instance.Run())
```

Registering none is rejected at startup rather than booting a server that cannot execute a payment.

`New` reads configuration from the environment unless `WithConfig` supplies it, opens the database, creates one shared Redis cache, and prepares the HTTP server, providers, and background workers. `Run` serves until the process is interrupted; use `Serve(ctx)` to control shutdown yourself, or `App()` to reach the underlying [Fiber](https://gofiber.io) application and mount Momobase inside one of your own. Redis defaults to `localhost:6379` and can be configured with `REDIS_ADDR`, `REDIS_USERNAME`, `REDIS_PASSWORD`, `REDIS_DB`, and `REDIS_TLS_ENABLED`; `CACHE_TTL_SECONDS` sets the TTL for every cached value.

Momobase serves on Fiber v3, which runs on fasthttp rather than `net/http`, so `App()` returns a `*fiber.App` and not an `http.Handler`. An application built around `net/http` adapts at its own boundary.

### Custom providers

A provider is any type implementing `momobase.PaymentProvider`:

```go
type PaymentProvider interface {
	Capabilities() []Capability
	Init(context.Context, ProviderConfig) error
	HealthCheck(context.Context) error
	Collect(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	Disburse(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	QueryTransaction(context.Context, string, string) (*ProviderTransactionStatus, error)
	QueryBalance(context.Context, string) (*ProviderBalance, error)
	VerifyWebhook(context.Context, []byte, map[string]string) (*ProviderWebhookEvent, error)
}
```

Register a factory for it under a provider code, then create, configure, and activate accounts for that code through the Admin API. The configuration stored for an account is encrypted at rest and handed to `Init` as a `ProviderConfig` whenever the account is loaded or changed. Momobase adds the account's authoritative `environment` value to that configuration before initialization.

`GET /api/admin/providers/registry` reports the codes the running build accepts, so clients discover custom providers instead of hardcoding a list:

```json
{ "success": true, "data": { "providers": ["acme_bank", "acme_pay", "dummy"] } }
```

The TypeScript SDK exposes it as `client.providers.registry()`, and the dashboard builds its provider dropdown from it, so an out-of-tree adapter appears without a client release.

The root package also exports the helpers the bundled adapters use, so a third-party provider does not have to reimplement them: `DoJSON`, `Redact`, `ConfigString`/`ConfigBool`/`ConfigInt`/`ConfigPath`, `First`, `ParseAmountToMinor`, `FormatAmountMinor`, `PaymentStatus`, and the `Service*` and `Tx*` constants. The root package is the surface an out-of-tree adapter uses: `github.com/momobasehq/momobase/providers` carries the contract and the provider-specific helpers, while the configuration accessors and `First` live in `internal/utils` and reach adapters only through the root re-exports.

See [`examples/customprovider`](examples/customprovider) for a complete custom provider, [`providers/dummy`](providers/dummy) for the in-tree simulator, and [`examples/mtn`](examples/mtn) for opting into the MTN Mobile Money adapter.

### Options

| Option | Purpose |
| --- | --- |
| `WithProvider(code, factory)` | Register a provider, replacing any registered under the same code |
| `WithProviders(map[string]ProviderFactory)` | Register several providers at once |
| `WithConfig(cfg)` | Supply configuration instead of reading the environment |
| `WithConfigFunc(fn)` | Adjust the resolved configuration before the instance is built |
| `WithAddr(addr)` | Override the HTTP listen address |
| `WithLogger(log)` | Use an existing `*slog.Logger` |

## Run as a service

```sh
make run                 # go run ./cmd/momobase serve
make build               # binary in bin/
make quality             # fmt-check, vet, test, lint
```

## Releases

Pushing a `v*.*.*` tag runs [`.github/workflows/release.yml`](.github/workflows/release.yml), which uses GoReleaser to attach `linux/amd64` and `linux/arm64` archives plus `checksums.txt` to the GitHub release and to publish a multi-platform image to `ghcr.io/momobasehq/momobase`. Image and archive names drop the tag's leading `v`, and a prerelease tag such as `v1.2.3-rc1` is published without moving `latest`:

```sh
git tag -a v1.2.3 -m "v1.2.3" && git push origin v1.2.3
docker pull ghcr.io/momobasehq/momobase:1.2.3   # also :1.2 and :latest
```

Momobase links SQLite through cgo, so released binaries are built with `CGO_ENABLED=1` and linked statically — one artifact runs on any Linux host regardless of its libc, and the `momobase version` command reports the tag, commit, and build date. Building `linux/arm64` therefore needs a cross compiler:

```sh
sudo apt-get install -y gcc-aarch64-linux-gnu
make release-check       # validate .goreleaser.yaml
make snapshot            # build the artifacts locally, publish nothing
```

Run the workflow manually from the Actions tab to produce a snapshot without tagging or publishing.
