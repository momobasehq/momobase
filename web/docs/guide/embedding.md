# Embed Momobase as a Go package

Import the root module when Momobase should run inside a Go application with compiled providers, hooks, or additional Fiber routes.

## Create an instance

From a Momobase source checkout, build the embedded dashboard before compiling your application:

```sh
make dashboard
```

Then construct and run the instance:

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
		momobase.WithAddr(":9090"),
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

`New()` reads environment configuration unless you provide `WithConfig()`. It opens the database and prepares the HTTP server, providers, and workers, so the caller must close the returned instance.

::: warning Dashboard build artifact
The Go package embeds `web/dashboard/dist`. A source build must generate that directory before Go compilation. Published source or vendored copies must include the generated dashboard files until the package distribution removes this build-time requirement.
:::

## Configure the instance

| Option | Purpose |
| --- | --- |
| `WithConfig(cfg)` | Use a complete `momobase.Config` instead of environment loading |
| `WithConfigFunc(fn)` | Modify the resolved configuration before startup |
| `WithAddr(addr)` | Override the HTTP listen address |
| `WithLogger(logger)` | Use an application-owned `slog.Logger` |
| `WithProvider(code, factory)` | Register one compiled provider |
| `WithProviders(factories)` | Register several compiled providers |

At least one provider is required. Provider codes become the values operators select when creating provider accounts.

## Control the lifecycle

- `Run()` serves until `SIGINT` or `SIGTERM` and shuts down gracefully.
- `Serve(ctx)` serves until the context is cancelled or the server stops.
- `Close()` stops the server and workers and closes the database connection pool. It is safe to call more than once.
- `Addr()` returns the configured address, or the bound address after starting with port `0`.

Use `Migrate(ctx)` and `SeedAdmin(ctx, email, password, name)` when your host application owns setup tasks.

## Extend the Fiber application

`App()` returns Momobase's `*fiber.App`. Add application-specific routes before serving it:

```go
import "github.com/gofiber/fiber/v3"

instance.App().Get("/ready", func(c fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
})
```

Momobase uses Fiber and fasthttp, so `App()` is not a standard-library `http.Handler`. Keep a separate listener or add an adapter if the rest of your application uses `net/http`.

Use `DB()` and `Logger()` only when an extension needs the instance-owned database handle or structured logger. Prefer Momobase's public APIs for payment operations.
