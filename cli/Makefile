VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS  = -ldflags "-X github.com/vmarinogg/leo-core/cli/internal/cmd.Version=$(VERSION) -X github.com/vmarinogg/leo-core/cli/internal/cmd.Commit=$(COMMIT)"

.PHONY: build test lint clean install

build:
	go build $(LDFLAGS) -o bin/leo ./cmd/leo

install:
	@go install $(LDFLAGS) ./cmd/leo
	@GOBIN=$$(go env GOPATH)/bin; \
	echo "leo installed to $$GOBIN/leo"; \
	if ! echo "$$PATH" | tr ':' '\n' | grep -q "$$GOBIN"; then \
		echo ""; \
		echo "  WARNING: $$GOBIN is not in your PATH."; \
		echo "  Add this to your shell profile (~/.zshrc or ~/.bashrc):"; \
		echo ""; \
		echo "    export PATH=\"$$GOBIN:\$$PATH\""; \
		echo ""; \
		echo "  Then restart your terminal or run: source ~/.zshrc"; \
	fi

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

# Cross-compilation targets
.PHONY: build-all
build-all:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/leo-darwin-arm64 ./cmd/leo
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/leo-darwin-amd64 ./cmd/leo
	GOOS=linux  GOARCH=arm64 go build $(LDFLAGS) -o bin/leo-linux-arm64  ./cmd/leo
	GOOS=linux  GOARCH=amd64 go build $(LDFLAGS) -o bin/leo-linux-amd64  ./cmd/leo
