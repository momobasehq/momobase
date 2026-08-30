# Develop and test Momobase

This guide covers the repository workflow for changing the Go service, dashboard, SDK, or documentation.

## Install the toolchain

Use the Go version and toolchain declared in `go.mod`, Node.js 22 or later, pnpm 11, Git, Make, and a C compiler for SQLite.

Install optional repository tools when you need their targets:

- `golangci-lint` for `make lint`;
- `goreleaser` for release checks and snapshots; and
- `swag` for `make docs`.

## Start the service

```sh
cp .env.example .env
make run
```

`make run` installs web dependencies, builds the embedded dashboard, and starts the Go service. Use the [local setup guide](/guide/getting-started) to migrate, seed an administrator, and configure the dummy provider.

## Work in the web workspace

Install the shared pnpm workspace once:

```sh
pnpm -C web install --frozen-lockfile
```

Run one project in development mode:

```sh
pnpm -C web --filter @momobase/dashboard dev
pnpm -C web --filter @momobase/docs dev
```

Build the SDK with `make sdk-build`. Build all browser projects with `pnpm -C web -r run build`.

## Regenerate the API specification

Swagger annotations live beside the Go handlers. After changing an endpoint or schema, run:

```sh
make docs
```

The target writes `swagger.json` and `swagger.yaml` to `web/docs/public`. The VitePress API page loads `/swagger.yaml`.

## Run checks

Use the smallest check that covers your change, then run the broader suite before submitting it:

| Change | Check |
| --- | --- |
| Go formatting | `make fmt-check` |
| Go behavior | `make test` |
| Go static analysis | `make vet` and `make lint` |
| Dashboard or SDK types | `make web-typecheck` |
| SDK package | `make sdk-build` |
| Documentation | `pnpm -C web --filter @momobase/docs build` |
| API smoke path | `make smoke-api` |
| Full backend smoke path | `make smoke` |

`make quality` runs formatting checks, vet, tests, and lint. Go targets build the dashboard first because its output is embedded in every binary.

## Build a release artifact

Run `make release-check` to validate the GoReleaser configuration. Run `make snapshot` to build local release artifacts without publishing them; arm64 cross-compilation requires the toolchain noted in the Makefile.

For container changes, build the root `Dockerfile`. Its web stage builds the dashboard before the Go stage embeds it.
