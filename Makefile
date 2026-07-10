.PHONY: build test test-cover lint clean

# Build with embedded persistent client (requires CGO)
build:
	CGO_ENABLED=1 go build -ldflags="-s -w -X main.version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev) -X main.commit=$$(git rev-parse --short HEAD 2>/dev/null || echo none) -X main.date=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o notebrain .

# Run all tests
test:
	go test ./...

# Run tests with coverage
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run tests for a specific package (usage: make test-pkg PKG=./internal/parser)
test-pkg:
	go test -v -count=1 $(PKG)

# Lint changed packages only
lint:
	@changed=$$(git diff --name-only --diff-filter=ACMR HEAD | grep '\.go$$' | xargs -I{} dirname {} | sort -u | sed 's|^|./|'); \
	if [ -n "$$changed" ]; then \
		echo "Linting: $$changed"; \
		go vet $$changed; \
	else \
		echo "No Go files changed."; \
	fi

# Clean build artifacts
clean:
	rm -f notebrain coverage.out coverage.html
