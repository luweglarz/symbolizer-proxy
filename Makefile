.PHONY: build run dummy tidy fmt vet test clean fixtures up down logs


# Where test fixtures live. `testdata` is special-cased by the go tool (it is
# ignored during builds/vet), so compiled binaries can sit here safely.
FIXTURES_DIR := internal/server/testdata
# A pinned GNU build-id baked into one fixture via the linker's -B flag, so
# tests can assert an exact storage key without discovering it at runtime.
FIXED_BUILDID := 00112233445566778899aabbccddeeff00112233

build:
	go build -o bin/symbolizer ./cmd/symbolizer

dummy:
	go build -o bin/dummyapp ./examples/dummyapp

# Generate the binary fixtures the tests exercise. Reproducible from source,
# so we never commit opaque binaries (whose build-id drifts with the toolchain).
fixtures:
	mkdir -p $(FIXTURES_DIR)
	# Full binary: .symtab + .gopclntab + DWARF all present.
	go build -o $(FIXTURES_DIR)/dummyapp.normal ./examples/dummyapp
	# Stripped: .symtab and DWARF gone, only .gopclntab survives (the realistic case).
	go build -ldflags "-s -w" -o $(FIXTURES_DIR)/dummyapp.stripped ./examples/dummyapp
	# Pinned build-id: deterministic .note.gnu.build-id == $(FIXED_BUILDID).
	go build -ldflags "-B 0x$(FIXED_BUILDID)" -o $(FIXTURES_DIR)/dummyapp.fixedid ./examples/dummyapp
	# A non-ELF blob for the clean-rejection path.
	printf 'this is definitely not an ELF binary\n' > $(FIXTURES_DIR)/not-an-elf.bin

run:
	go run ./cmd/symbolizer

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

test: fixtures
	go test ./...

clean:
	rm -rf bin $(FIXTURES_DIR)
