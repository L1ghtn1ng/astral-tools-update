BINARY := astral-update
CMD := ./cmd/astral-update
BIN_DIR := bin
LDFLAGS := -s -w

.PHONY: all fmt fmt-check vet test build build-linux build-linux-amd64 build-linux-arm64 clean ci

all: test build

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@test -z "$(shell gofmt -l ./cmd ./internal)" || (echo "gofmt is required; run 'make fmt'" && exit 1)

vet:
	go vet ./...

test:
	go test -v ./...

build:
	@mkdir -p "$(BIN_DIR)"
	go build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(BINARY)" "$(CMD)"

build-linux: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	@mkdir -p "$(BIN_DIR)"
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(BINARY)_x86_64" "$(CMD)"

build-linux-arm64:
	@mkdir -p "$(BIN_DIR)"
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(BINARY)_arm64" "$(CMD)"

clean:
	rm -rf "$(BIN_DIR)"

ci: fmt-check vet test
