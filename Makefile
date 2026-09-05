GOLANGCI_LINT ?= golangci-lint
SWAG ?= swag

test:
	go test ./...

coverage:
	go test -shuffle=on -covermode=atomic -coverprofile=coverage.out -coverpkg=./... ./...
	go tool cover -func=coverage.out

tidy:
	go mod tidy

fmt:
	gofmt -w $$(git ls-files '*.go')

fmt-check:
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))" || (gofmt -l $$(git ls-files '*.go'); exit 1)

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run --fix ./...

lint-format:
	$(GOLANGCI_LINT) fmt ./...

docs:
	mkdir -p _public
	$(SWAG) init -g doc.go --parseInternal --output _public --outputTypes json,yaml

quality: fmt-check vet test lint

.PHONY: test coverage tidy fmt fmt-check vet lint lint-fix lint-format docs quality
