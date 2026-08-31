APP=momobase
GOFILES=$$(find . -name '*.go' -not -path './vendor/*')
GOLANGCI_LINT?=golangci-lint
GORELEASER?=goreleaser
PNPM?=pnpm

run: dashboard
	go run ./cmd/$(APP) serve

build: dashboard
	go build -o bin/$(APP) ./cmd/$(APP)

test: dashboard
	go test ./...

tidy:
	go mod tidy

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	@test -z "$$(gofmt -l $(GOFILES))" || (gofmt -l $(GOFILES); exit 1)

vet: dashboard
	go vet ./...

lint: dashboard
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

seed-admin: dashboard
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

# Builds the dashboard bundle embedded by every Go binary.
dashboard: web-install
	$(PNPM) -C web --filter @momobase/dashboard run build

mtn: dashboard
	go build -o ./bin/momo ./examples/mtn/main.go

# tell make that these targets are not files
.PHONY: run build test tidy fmt fmt-check vet lint lint-fix lint-format quality release-check snapshot seed-admin smoke smoke-api web-install web-typecheck dashboard mtn
