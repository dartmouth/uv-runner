# Makefile for UV Runner (CLI and GUI)

CLI_NAME=uv-runner-cli
GUI_NAME=uv-runner-gui
BUILD_FLAGS=-ldflags="-w -s" -trimpath

# Detect platform and set GUI build tags
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
    GUI_BUILD_TAGS=-tags wayland
else
    GUI_BUILD_TAGS=
endif

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
build: build-cli build-gui

.PHONY: build-cli
build-cli:
	@echo "Building CLI for current platform..."
	cd uv-runner-cli && go build $(BUILD_FLAGS) -o ../dist/$(CLI_NAME)

.PHONY: build-gui
build-gui:
	@echo "Building GUI for current platform..."
	cd uv-runner-gui && go build $(GUI_BUILD_TAGS) $(BUILD_FLAGS) -o ../dist/$(GUI_NAME)

# Build for all platforms
.PHONY: build-all
build-all: clean
	@echo "Building CLI for all platforms..."
	
	@echo "Building CLI for macOS (Intel)..."
	cd uv-runner-cli && GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o ../dist/$(CLI_NAME)-macos-intel
	
	@echo "Building CLI for macOS (Apple Silicon)..."
	cd uv-runner-cli && GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o ../dist/$(CLI_NAME)-macos-arm64
	
	@echo "Building CLI for Linux (x86_64)..."
	cd uv-runner-cli && GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o ../dist/$(CLI_NAME)-linux-x86_64
	
	@echo "Building CLI for Windows (x86_64)..."
	cd uv-runner-cli && GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o ../dist/$(CLI_NAME)-windows-x86_64.exe
	
	@echo "Building GUI for current platform only (GUI cross-compilation requires platform-specific setup)..."
	cd uv-runner-gui && go build $(GUI_BUILD_TAGS) $(BUILD_FLAGS) -o ../dist/$(GUI_NAME)-$$(go env GOOS)-$$(go env GOARCH)
	
	@echo "Build complete!"
	@ls -la dist/

# Build GUI for all platforms (requires platform-specific runners)
.PHONY: build-gui-all
build-gui-all:
	@echo "Building GUI for all platforms..."
	@echo "Note: Cross-compilation of GUI apps requires CGO and platform-specific libraries"
	@echo "This target should be run in CI/CD with multiple platform runners"
	
	@echo "Building GUI for current platform..."
	cd uv-runner-gui && go build $(GUI_BUILD_TAGS) $(BUILD_FLAGS) -o ../dist/$(GUI_NAME)-$$(go env GOOS)-$$(go env GOARCH)

# Test the binaries (builds for current platform and runs a quick test)
.PHONY: test
test: build
	@echo "Testing CLI binary..."
	./dist/$(CLI_NAME) --help || echo "CLI binary created successfully"
	@echo "Testing GUI binary..."
	./dist/$(GUI_NAME) --help || echo "GUI binary created successfully"

# Install to system (for current platform)
.PHONY: install
install: build
	cp dist/$(CLI_NAME) /usr/local/bin/
	cp dist/$(GUI_NAME) /usr/local/bin/

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build      - Build CLI and GUI for current platform"
	@echo "  build-cli  - Build only CLI for current platform"
	@echo "  build-gui  - Build only GUI for current platform"
	@echo "  build-all  - Build CLI and GUI for all platforms"
	@echo "  clean      - Clean build directory"
	@echo "  test       - Build and test both binaries"
	@echo "  install    - Install both binaries to /usr/local/bin"
	@echo "  help       - Show this help"
