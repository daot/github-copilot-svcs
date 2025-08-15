BINARY=github-copilot-svcs
CMD_PATH=./cmd/github-copilot-svcs
VERSION?=dev

.PHONY: build test test-unit test-integration test-e2e test-all clean run dev lint fmt vet deps update-deps security mocks docker-build docker-run help test-coverage test-short test-verbose

# Build the binary
build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) $(CMD_PATH)

# Build for specific OS/ARCH
build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-linux-amd64 $(CMD_PATH)

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-linux-arm64 $(CMD_PATH)

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-darwin-amd64 $(CMD_PATH)

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-darwin-arm64 $(CMD_PATH)

build-windows-amd64:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-windows-amd64.exe $(CMD_PATH)

build-windows-arm64:
	GOOS=windows GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY)-windows-arm64.exe $(CMD_PATH)

# Run the application
run: build
	./$(BINARY) run

# Development server with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	air -c .air.toml

# Run only unit tests
test-unit:
	go test -v -race ./internal/... ./pkg/...

# Run only integration tests
test-integration:
	go test -v -race ./test/integration/...

# Run only e2e tests
test-e2e:
	go test -v -race ./test/e2e/...

# Run all tests
test-all:
	go test -v -race ./test/...

# Default test command (unit tests)
test: test-unit

# Test with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out -coverpkg=./internal/...,./cmd/...,./pkg/... ./test/... ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out
	@echo "Coverage report generated: coverage.html"

# Test short (skip integration tests)
test-short:
	go test -short -v -race ./test/...

# Clean test artifacts and build files
clean:
	rm -f $(BINARY) coverage.out coverage.html
	go clean -testcache
	go mod tidy

# Run tests with verbose output and show which tests are running
test-verbose:
	go test -v -race ./test/... -run .

# Lint the code (requires golangci-lint)
lint:
	golangci-lint run

# Format the code
fmt:
	go fmt ./...

# Vet the code
vet:
	go vet ./...

# Install dependencies
deps:
	go mod download
	go mod verify

# Update dependencies
update-deps:
	go get -u ./...
	go mod tidy

# Security check (requires gosec: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest)
security:
	gosec ./...

# Generate mocks (requires mockery: go install github.com/vektra/mockery/v2@latest)
mocks:
	mockery --all --output=test/mocks

# Docker build
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY):$(VERSION) .

# Docker run
docker-run:
	docker run -p 8081:8081 $(BINARY):$(VERSION)

# Help
help:
	@echo "Available targets:"
	@echo "  build                  Build the binary"
	@echo "  build-linux-amd64      Build for Linux amd64"
	@echo "  build-linux-arm64      Build for Linux arm64"
	@echo "  build-darwin-amd64     Build for macOS amd64"
	@echo "  build-darwin-arm64     Build for macOS arm64"
	@echo "  build-windows-amd64    Build for Windows amd64"
	@echo "  build-windows-arm64    Build for Windows arm64"
	@echo "  run                    Build and run the application"
	@echo "  dev                    Run development server with hot reload"
	@echo "  test                   Run unit tests (default)"
	@echo "  test-unit              Run only unit tests"
	@echo "  test-integration       Run only integration tests"
	@echo "  test-e2e               Run only e2e tests"
	@echo "  test-all               Run all tests"
	@echo "  test-coverage          Run tests with coverage report"
	@echo "  test-short             Run tests (skip integration tests)"
	@echo "  test-verbose           Run tests with verbose output"
	@echo "  lint                   Lint the code"
	@echo "  fmt                    Format the code"
	@echo "  vet                    Vet the code"
	@echo "  clean                  Clean build artifacts and test cache"
	@echo "  deps                   Install dependencies"
	@echo "  update-deps            Update dependencies"
	@echo "  security               Run security checks"
	@echo "  mocks                  Generate mocks"
	@echo "  docker-build           Build Docker image"
	@echo "  docker-run             Run Docker container"
	@echo "  help                   Show this help message"
