# Game Launcher MVP Makefile

# Variables
BINARY_NAME=game-launcher
GO_FILES=main.go

# Default target (builds for current platform)
all: build

# Build for current platform
build:
	@echo Building Game Launcher MVP for current platform...
	go build -o $(BINARY_NAME) $(GO_FILES)
	@echo Build successful! Created $(BINARY_NAME)

# Cross-compilation targets

# macOS Intel (x86_64)
mac-intel:
	@echo Building for macOS Intel (x86_64)...
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 $(GO_FILES)
	@echo Build successful! Created $(BINARY_NAME)-darwin-amd64

# macOS Apple Silicon (ARM64)
mac-arm:
	@echo Building for macOS Apple Silicon (ARM64)...
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 $(GO_FILES)
	@echo Build successful! Created $(BINARY_NAME)-darwin-arm64

# Windows x86_64 (no console window)
windows:
	@echo Building for Windows x86_64...
	GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o $(BINARY_NAME)-windows-amd64.exe $(GO_FILES)
	@echo Build successful! Created $(BINARY_NAME)-windows-amd64.exe

# Windows x86_64 (with console for debugging)
windows-dev:
	@echo Building Windows development version with console...
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64-dev.exe $(GO_FILES)
	@echo Development build complete! Created $(BINARY_NAME)-windows-amd64-dev.exe

# Linux x86_64 (for SteamOS/Steam Deck)
linux:
	@echo Building for Linux x86_64...
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 $(GO_FILES)
	@echo Build successful! Created $(BINARY_NAME)-linux-amd64

# SteamOS/Steam Deck specific (alias for linux)
steamos: linux
	@echo SteamOS build complete! Use $(BINARY_NAME)-linux-amd64

# Build all platforms
all-platforms: mac-intel mac-arm windows linux
	@echo All platform builds complete!
	@ls -la $(BINARY_NAME)-*

# Clean all build artifacts
clean:
	@echo Cleaning build artifacts...
	@rm -f $(BINARY_NAME) $(BINARY_NAME)-*
	@echo Clean complete

# Download dependencies
deps:
	@echo Downloading dependencies...
	go mod tidy
	@echo Dependencies updated

# Run on current platform (for development)
run:
	go run $(GO_FILES)

# Show build info
info:
	@echo Current platform: $(shell go env GOOS)/$(shell go env GOARCH)
	@echo Go version: $(shell go version)
	@echo Available targets:
	@echo "  mac-intel  - macOS Intel (x86_64)"
	@echo "  mac-arm    - macOS Apple Silicon (ARM64)"
	@echo "  windows    - Windows x86_64 (no console)"
	@echo "  windows-dev - Windows x86_64 (with console)"
	@echo "  linux      - Linux x86_64"
	@echo "  steamos    - SteamOS/Steam Deck"

# Show help
help:
	@echo Game Launcher MVP Build System
	@echo.
	@echo Platform-specific targets:
	@echo "  mac-intel    - Build for macOS Intel (x86_64)"
	@echo "  mac-arm      - Build for macOS Apple Silicon (ARM64)"
	@echo "  windows      - Build for Windows x86_64 (release)"
	@echo "  windows-dev  - Build for Windows x86_64 (debug)"
	@echo "  linux        - Build for Linux x86_64"
	@echo "  steamos      - Build for SteamOS/Steam Deck"
	@echo.
	@echo Utility targets:
	@echo "  build        - Build for current platform"
	@echo "  all-platforms - Build for all supported platforms"
	@echo "  run          - Run without building"
	@echo "  clean        - Remove all build artifacts"
	@echo "  deps         - Update dependencies"
	@echo "  info         - Show build environment info"
	@echo "  help         - Show this help"

.PHONY: all build mac-intel mac-arm windows windows-dev linux steamos all-platforms clean deps run info help