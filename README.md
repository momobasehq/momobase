<img alt="Momobase" src="web/dashboard/public/logo.svg" width="72" />

# Momobase

Momobase is a self-hosted payment orchestration service. It gives applications one
API for collections and disbursements, then routes each payment through the provider
adapters you register.

[Documentation](docs/README.md) · [OpenAPI specification](docs/swagger.json) · [Custom provider example](examples/customprovider)

## What it includes

- Collection and disbursement APIs with idempotency
- Configurable routes, provider accounts, priorities, and country eligibility
- Provider webhooks, reconciliation, health checks, and circuit breakers
- Admin API, embedded dashboard, and TypeScript SDK
- SQLite, PostgreSQL, and MySQL support

## Quick start

You need the Go toolchain declared in [`go.mod`](go.mod). SQLite is the default
database, so nothing else is required for local development.

```sh
cp .env.example .env
go run ./cmd/momobase migrate
go run ./cmd/momobase seed-admin \
  --email admin@example.com \
  --password 'replace-with-a-strong-password' \
  --name 'Super Admin'
go run ./cmd/momobase serve
```

The API listens on `http://localhost:9090`. Check it with:

```sh
curl http://localhost:9090/healthz
```

To include the administration dashboard, install Node.js 22 and pnpm, then run:

```sh
make build-dashboard
DASHBOARD_ENABLED=true ./bin/momobase serve
```

Open `http://localhost:9090/dashboard/` and sign in with the administrator created above.

## Docker Compose

The included Compose stack runs Momobase with PostgreSQL:

```sh
cp .env.docker.example .env
# Replace every placeholder password, key, secret, and public URL in .env.
docker compose up --build
```

See the [deployment guide](docs/README.md#docker-compose) for secret generation,
administrator setup, migrations, and production settings.

## Embed in Go

The root package can also be embedded in another Go application. It registers no
payment providers automatically, so the host chooses exactly which adapters are
included:

```go
package main

import (
	"log"

	"github.com/momobasehq/momobase"
	"github.com/momobasehq/momobase/providers/dummy"
)

func main() {
	instance, err := momobase.New(
		momobase.WithProvider("dummy", dummy.New),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = instance.Close() }()

	if err := instance.Run(); err != nil {
		log.Fatal(err)
	}
}
```

Use [`Serve(ctx)`](momobase.go) for controlled shutdown or [`App()`](momobase.go) to
access the underlying Fiber application.

## Extensions

Compiled Go extensions can validate new payment requests and observe committed
transaction status changes without importing Momobase's internal packages:

```go
instance, err := momobase.New(
	momobase.WithProvider("dummy", dummy.New),
)
if err != nil {
	log.Fatal(err)
}

instance.OnPaymentRequest().Bind(func(ctx context.Context, event hooks.PaymentRequestEvent) error {
	if event.AppID == shopAppID && event.Amount > 1_000_000 {
		return errors.New("payment exceeds the application limit")
	}
	return nil
})
```

Payment request hooks run only for normalized new requests, after idempotency replay
detection and before routing or persistence. A hook error rejects the payment with a
generic API response. Transaction change hooks run after persistence from the request,
provider-webhook, and reconciliation paths; their errors are logged and never alter the
committed payment result. Handlers run synchronously in registration order.

See [`examples/extension`](examples/extension) for an app-scoped example. Custom HTTP
routes can still be mounted directly on `instance.App()`.

## Providers

A provider implements the two-method [`providers.PaymentProvider`](providers/provider.go),
adds only the operation interfaces it supports, and is registered under a code with
`momobase.WithProvider`. Each provider owns one flat configuration object; Momobase
encrypts it at rest and injects the account environment during initialization.

- [`providers/dummy`](providers/dummy) simulates payments without moving money.
- [`providers/mtn`](providers/mtn) integrates MTN Mobile Money.
- [`examples/customprovider`](examples/customprovider) demonstrates a complete third-party adapter.

## Development

```sh
make quality          # format, vet, tests, and lint
make web-typecheck    # SDK and dashboard types
make build-dashboard  # web bundle and tagged Go binary
```

The long-form [documentation](docs/README.md) covers API workflows, authentication,
configuration, migrations, operations, and deployment. The TypeScript client lives in
[`web/sdk`](web/sdk).

## License

[MIT](LICENSE.txt)
