.PHONY: build run tidy fmt vet lint ci-lint test clean fixtures

FIXTURES_DIR := internal/server/testdata
FIXED_BUILDID := 00112233445566778899aabbccddeeff00112233
GOLANGCI_LINT_VERSION := v2.12.2

build:
	go build -o bin/symbolizer ./cmd/symbolizer

fixtures:
	mkdir -p $(FIXTURES_DIR)
	go build -o $(FIXTURES_DIR)/dummyapp.normal ./examples/dummyapp
	go build -ldflags "-s -w" -o $(FIXTURES_DIR)/dummyapp.stripped ./examples/dummyapp
	go build -ldflags "-B 0x$(FIXED_BUILDID)" -o $(FIXTURES_DIR)/dummyapp.fixedid ./examples/dummyapp
	printf 'this is definitely not an ELF binary\n' > $(FIXTURES_DIR)/not-an-elf.bin

run:
	go run ./cmd/symbolizer

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --enable-only=govet,staticcheck,ineffassign,unused ./...

ci-lint: lint

test: fixtures
	go test ./...

clean:
	rm -rf bin $(FIXTURES_DIR)
