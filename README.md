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

At least one provider is required; the included dummy provider is deterministic and moves no money.

## Configure it

Momobase reads no environment variables and no configuration files. `DefaultConfig()` returns a development baseline you copy and edit, and `WithConfig` passes it back:

```go
cfg := momobase.DefaultConfig()
cfg.App.Env = "production"
cfg.App.PublicURL = "https://payments.example.com"
cfg.App.CORSAllowedOrigins = []string{"https://checkout.example.com"}
cfg.DB = momobase.DatabaseConfig{
	Type:     "postgres",
	Host:     "database.internal",
	Port:     "5432",
	User:     "momobase",
	Password: os.Getenv("DB_PASSWORD"),
	Name:     "momobase",
	SSLMode:  "require",
}
cfg.Security.EncryptionMasterKeyBase64 = os.Getenv("ENCRYPTION_MASTER_KEY_BASE64")
cfg.Security.AdminOAuthSecret = os.Getenv("ADMIN_OAUTH_SECRET")
cfg.Security.AppOAuthSecret = os.Getenv("APP_OAUTH_SECRET")

instance, err := momobase.New(
	momobase.WithConfig(cfg),
	momobase.WithProvider("dummy", dummy.New),
)
```

Where a value comes from is your application's decision: read the environment, a file, or a secret manager and assign the fields. The defaults carry placeholder credentials that startup rejects once `App.Env` is `staging` or `production`.

Mint the three real ones with `openssl rand`:

```sh
$ openssl rand -base64 32      # EncryptionMasterKeyBase64, must decode to exactly 32 bytes
hV3pR8mK2xQ7wN5tZ0cJ9bY4sL6dA1fG8eU3iO7nX2M=

$ openssl rand -hex 32         # AdminOAuthSecret
a4e91c7d5b28f036e1a7c94d0b562f83e7d15a9c3f804b6e29d7a1c58f036b4e

$ openssl rand -hex 32         # AppOAuthSecret
5d38b0e7a91c4f26d85b30a7e12f9c64b038d75a1e9f43c806b2d517a9e04f3c
```

Also available: `WithConfigFunc` for ordered overrides, `WithAddr` for the listen address alone, `WithLogger` for a `slog.Logger`, and `WithProvider` or `WithProviders` for adapters.

## Public interface

- `DefaultConfig()` returns the development configuration baseline to copy and edit.
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
