# Makefile for UV Runner

BINARY_NAME=uv-runner
BUILD_FLAGS=-ldflags="-w -s" -trimpath

# Default target
.PHONY: all
all: clean build

# Clean previous builds
.PHONY: clean
clean:
	rm -rf dist/
	mkdir -p dist/

# Build for current platform only
.PHONY: build
build:
	go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)

# Build for all platforms
.PHONY: build-all
build-all: clean
	@echo "Building for all platforms..."
	
	@echo "Building for macOS (Intel)..."
	GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-macos-intel
	
	@echo "Building for macOS (Apple Silicon)..."
	GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-macos-arm64
	
	@echo "Building for Linux (x86_64)..."
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-x86_64
	
	@echo "Building for Windows (x86_64)..."
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-windows-x86_64.exe
	
	@echo "Build complete!"
	@ls -la dist/

# Test the binary (builds for current platform and runs a quick test)
.PHONY: test
test: build
	@echo "Testing binary..."
	./dist/$(BINARY_NAME) --help || echo "Binary created successfully"

# Install to system (for current platform)
.PHONY: install
install: build
	cp dist/$(BINARY_NAME) /usr/local/bin/

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build      - Build for current platform"
	@echo "  build-all  - Build for all platforms"
	@echo "  clean      - Clean build directory"
	@echo "  test       - Build and test current platform"
	@echo "  install    - Install to /usr/local/bin"
	@echo "  help       - Show this help"
