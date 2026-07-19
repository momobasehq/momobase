APP=momobase
GOFILES=$$(find . -name '*.go' -not -path './vendor/*')
GOLANGCI_LINT?=golangci-lint

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

seed-admin:
	@test -n "$$ADMIN_PASSWORD" || (echo "Set ADMIN_PASSWORD before running seed-admin" >&2; exit 1)
	go run ./cmd/$(APP) seed-admin --email admin@momobase.local --password "$$ADMIN_PASSWORD" --name "Super Admin"

smoke:
	scripts/smoke_backend.sh

smoke-api:
	scripts/smoke_api.sh

sdk-build:
	cd packages/sdk && npm install && npm run build

docs:
	swag init -g ./cmd/momobase/main.go --parseDependency --parseInternal --output docs --outputTypes json,yaml
	# rm -f docs/docs.go
	# install swag if not already installed
	# $ go install github.com/swaggo/swag/cmd/swag@latest

# tell make that these targets are not files
.PHONY: docs run build test tidy fmt fmt-check vet lint lint-fix lint-format quality seed-admin smoke smoke-api sdk-build docs
