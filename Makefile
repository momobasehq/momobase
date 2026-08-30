APP=momobase
GOFILES=$$(find . -name '*.go' -not -path './vendor/*')
GOLANGCI_LINT?=golangci-lint
GORELEASER?=goreleaser
PNPM?=pnpm

run:
	go run ./cmd/$(APP) serve

build:
	go build -o bin/$(APP) ./cmd/$(APP)

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	@test -z "$$(gofmt -l $(GOFILES))" || (gofmt -l $(GOFILES); exit 1)

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run --fix ./...

lint-format:
	$(GOLANGCI_LINT) fmt ./...

quality: fmt-check vet test lint

release-check:
	$(GORELEASER) check

# Build the release artifacts locally without publishing anything. Needs the
# arm64 cross compiler: apt-get install gcc-aarch64-linux-gnu
snapshot:
	$(GORELEASER) release --clean --snapshot --skip=publish

seed-admin:
	@test -n "$$ADMIN_PASSWORD" || (echo "Set ADMIN_PASSWORD before running seed-admin" >&2; exit 1)
	go run ./cmd/$(APP) seed-admin --email admin@momobase.local --password "$$ADMIN_PASSWORD" --name "Super Admin"

smoke:
	scripts/smoke_backend.sh

smoke-api:
	scripts/smoke_api.sh

# Install the web workspace without touching the lockfile. Every front-end target
# depends on this, so a stale lockfile fails here rather than halfway through a build.
web-install:
	$(PNPM) -C web install --frozen-lockfile

web-typecheck: web-install
	$(PNPM) -C web run typecheck

sdk-build: web-install
	$(PNPM) -C web --filter @momobase/sdk run build

# Builds the dashboard bundle. Nothing under web/dashboard/dist is committed, so the
# Go embed is behind a build tag: compile with `-tags dashboard` to include it.
dashboard: web-install
	$(PNPM) -C web --filter @momobase/dashboard run build

# The binary with the dashboard embedded. Plain `make build` deliberately omits it,
# so a Go-only checkout still builds.
build-dashboard: dashboard
	go build -tags dashboard -o bin/$(APP) ./cmd/$(APP)

docs:
	swag init -g ./cmd/momobase/main.go --parseInternal --output docs --outputTypes json
	# rm -f docs/docs.go
	# install swag if not already installed
	# $ go install github.com/swaggo/swag/cmd/swag@latest

mtn:
	go build -tags dashboard -o ./bin/momo ./examples/mtn/main.go 

# tell make that these targets are not files
.PHONY: docs run build test tidy fmt fmt-check vet lint lint-fix lint-format quality release-check snapshot seed-admin smoke smoke-api web-install web-typecheck sdk-build dashboard build-dashboard docs mtn
