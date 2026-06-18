.PHONY: build build-http test test-cover lint clean chroma-server

# Default: build with embedded persistent client (requires CGO)
build:
	CGO_ENABLED=1 go build -o notebrain .

# Build for HTTP-only mode (no CGO needed)
build-http:
	CGO_ENABLED=0 go build -tags http_only -o notebrain .

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
	@changed=$$(git diff --name-only --diff-filter=ACMR HEAD | grep '\.go$$' | xargs -I{} dirname {} | sort -u); \
	if [ -n "$$changed" ]; then \
		echo "Linting: $$changed"; \
		go vet $$changed; \
	else \
		echo "No Go files changed."; \
	fi

# Start ChromaDB HTTP server (alternative mode)
chroma-server:
	docker run -d --name notebrain-chroma \
		-p 8000:8000 \
		-v $(HOME)/.notebrain/chroma:/chroma/chroma \
		chromadb/chroma:1.5.9

# Clean build artifacts
clean:
	rm -f notebrain coverage.out coverage.html
