# Momobase

[![Go tests](https://github.com/momobasehq/momobase/actions/workflows/tests.yml/badge.svg)](https://github.com/momobasehq/momobase/actions/workflows/tests.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/momobasehq/momobase.svg)](https://pkg.go.dev/github.com/momobasehq/momobase)
[![Release](https://img.shields.io/github/v/release/momobasehq/momobase)](https://github.com/momobasehq/momobase/releases)

Momobase is an embeddable Go package for accepting, routing, and reconciling payments through provider adapters.

## Install

```sh
go get github.com/momobasehq/momobase@latest
```

## Start an instance

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
	defer instance.Close()

	log.Fatal(instance.Run())
}
```

`New` reads environment configuration by default. Pass `WithConfig` for explicit configuration, `WithConfigFunc` for ordered overrides, `WithLogger` for a `slog.Logger`, and `WithProvider` or `WithProviders` for adapters. At least one provider is required; the included dummy provider is deterministic and moves no money.

## Public interface

- `New(...Option)` constructs the database, HTTP API, provider runtime, workers, and hooks.
- `Serve(ctx)` runs until cancellation; `Run()` adds interrupt and termination signal handling.
- `Close()` stops workers and closes owned resources. Call it for every successful `New`.
- `App()` exposes the Fiber application for tests, mounting, or extra routes.
- `DB()` and `Logger()` expose the instance-owned GORM database and structured logger.
- `Migrate(ctx)` applies schema migrations when your host controls migration timing.
- `SeedAdmin(ctx, email, password, name)` creates the first administrator.
- `OnPaymentRequest()` and `OnTransactionChanged()` expose typed lifecycle hooks.

Provider contracts and helpers live in [`providers`](https://pkg.go.dev/github.com/momobasehq/momobase/providers), the safe reference adapter lives in [`providers/dummy`](https://pkg.go.dev/github.com/momobasehq/momobase/providers/dummy), and typed lifecycle events live in [`hooks`](https://pkg.go.dev/github.com/momobasehq/momobase/hooks).

See the [documentation](https://momobasehq.github.io/) and [API reference](https://momobasehq.github.io/api-reference).

## Development

```sh
make quality
make coverage
```

Releases are ordinary semantic-version Go module tags. A release tag publishes the module, GitHub release, and release OpenAPI files.

## License

[MIT](LICENSE.txt)
