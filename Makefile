APP=momobase
GOFILES=$$(find . -name '*.go' -not -path './vendor/*')

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

quality: fmt-check vet test

seed-admin:
	go run ./cmd/$(APP) seed-admin --email admin@example.com --password password123 --name "Super Admin"

seed-demo:
	go run ./cmd/$(APP) seed-demo

smoke:
	scripts/smoke_backend.sh

smoke-api:
	scripts/smoke_api.sh

sdk-build:
	cd packages/sdk && npm install && npm run build
