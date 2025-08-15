# Frictionless Launcher Makefile

BINARY_NAME=frictionless-launcher

# Default target
all: build

# Build for current platform (development)
build:
	@echo Building for current platform...
	go build -o $(BINARY_NAME) .
	@echo Build successful! Created $(BINARY_NAME)

# Run without building (development)
run:
	go run .

# Clean build artifacts
clean:
	@echo Cleaning build artifacts...
	@rm -f $(BINARY_NAME) $(BINARY_NAME)-*
	@rm -rf dist/
	@echo Clean complete

# Download dependencies
deps:
	@echo Downloading dependencies...
	go mod tidy
	@echo Dependencies updated

# Run tests
test:
	go test ./...

# Run tests with coverage and race detection (matches CI)
test-ci:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Quick test with verbose output
test-verbose:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Check formatting
fmt-check:
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "The following files are not formatted:"; \
		gofmt -s -l .; \
		exit 1; \
	else \
		echo "All files are properly formatted"; \
	fi

# Run linter (if golangci-lint is installed)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

# Run go vet
vet:
	go vet ./...

# Run all quality checks (like CI)
check: fmt-check vet test-ci lint
	@echo "All quality checks passed!"

# Cross-compilation targets
# macOS Intel (x86_64)
mac-intel:
	@echo "Building for macOS Intel (x86_64)..."
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 .
	@echo "Build successful! Created $(BINARY_NAME)-darwin-amd64"

# macOS Apple Silicon (ARM64)
mac-arm:
	@echo "Building for macOS Apple Silicon (ARM64)..."
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 .
	@echo "Build successful! Created $(BINARY_NAME)-darwin-arm64"

# Windows x86_64 (no console window)
windows:
	@echo Building for Windows x86_64...
	GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o $(BINARY_NAME)-windows-amd64.exe .
	@echo Build successful! Created $(BINARY_NAME)-windows-amd64.exe

# Windows x86_64 (with console for debugging)
windows-dev:
	@echo Building Windows development version with console...
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64-dev.exe .
	@echo Development build complete! Created $(BINARY_NAME)-windows-amd64-dev.exe

# Linux x86_64 (for SteamOS/Steam Deck)
linux:
	@echo Building for Linux x86_64...
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 .
	@echo Build successful! Created $(BINARY_NAME)-linux-amd64

# SteamOS/Steam Deck specific (alias for linux)
steamos: linux
	@echo SteamOS build complete! Use $(BINARY_NAME)-linux-amd64

# Build all platforms
all-platforms: mac-intel mac-arm windows linux
	@echo All platform builds complete!
	@ls -la $(BINARY_NAME)-*


# Show help
help:
	@echo Frictionless Launcher Build System
	@echo
	@echo Development targets:
	@echo "  build        - Build for current platform"
	@echo "  run          - Run without building"
	@echo "  test         - Run tests"
	@echo "  test-ci      - Run tests with coverage and race detection"
	@echo "  test-verbose - Run tests with verbose output"
	@echo "  fmt          - Format code"
	@echo "  fmt-check    - Check code formatting"
	@echo "  vet          - Run go vet"
	@echo "  lint         - Run linter"
	@echo "  check        - Run all quality checks (like CI)"
	@echo
	@echo Platform-specific targets:
	@echo "  mac-intel    - Build for macOS Intel (x86_64)"
	@echo "  mac-arm      - Build for macOS Apple Silicon (ARM64)"
	@echo "  windows      - Build for Windows x86_64 (release)"
	@echo "  windows-dev  - Build for Windows x86_64 (debug)"
	@echo "  linux        - Build for Linux x86_64"
	@echo "  steamos      - Build for SteamOS/Steam Deck"
	@echo "  all-platforms - Build for all supported platforms"
	@echo
	@echo Utility targets:
	@echo "  clean     - Remove build artifacts"
	@echo "  deps      - Update dependencies"
	@echo "  help      - Show this help"

.PHONY: all build run clean deps test test-ci test-verbose fmt fmt-check vet lint check mac-intel mac-arm windows windows-dev linux steamos all-platforms help