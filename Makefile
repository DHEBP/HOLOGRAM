# HOLOGRAM Makefile
# Builds HOLOGRAM along with derod and simulator from derohe source
#
# Usage:
#   make          - Build HOLOGRAM only (uses wails build)
#   make all      - Build HOLOGRAM + derod + simulator
#   make derod    - Build derod only
#   make simulator - Build simulator only
#   make clean    - Clean build artifacts
#   make dev      - Run in development mode
#
# The derod and simulator binaries are built from the derohe dependency
# and placed alongside the HOLOGRAM executable in build/bin/

.PHONY: all hologram derod simulator clean dev help

# Detect OS and architecture
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Binary names based on platform
ifeq ($(GOOS),windows)
    HOLOGRAM_BIN = Hologram.exe
    DEROD_BIN = derod.exe
    SIMULATOR_BIN = simulator.exe
else ifeq ($(GOOS),darwin)
    HOLOGRAM_BIN = Hologram.app
    DEROD_BIN = derod-darwin
    SIMULATOR_BIN = simulator-darwin
else
    HOLOGRAM_BIN = Hologram
    DEROD_BIN = derod-linux-$(GOARCH)
    SIMULATOR_BIN = simulator-linux-$(GOARCH)
endif

# Build directories
BUILD_DIR = build/bin
DEROHE_PKG = github.com/deroproject/derohe

# Get derohe module path from go mod

# Default target - build derod/simulator FIRST, then hologram
# This order is important because wails build runs go mod tidy which
# can remove "unused" dependencies needed for derod/simulator
all: derod simulator hologram
	@echo ""
	@echo "✅ Build complete!"
	@echo "   HOLOGRAM: $(BUILD_DIR)/$(HOLOGRAM_BIN)"
	@echo "   derod:    $(BUILD_DIR)/$(DEROD_BIN)"
	@echo "   simulator: $(BUILD_DIR)/$(SIMULATOR_BIN)"
	@echo ""
ifeq ($(GOOS),darwin)
	@echo "Run with: open $(BUILD_DIR)/Hologram.app"
else
	@echo "Run with: ./$(BUILD_DIR)/$(HOLOGRAM_BIN)"
endif

# Build HOLOGRAM using wails
hologram:
	@echo "🔨 Building HOLOGRAM..."
	wails build
	@echo "✅ HOLOGRAM built"

# Build derod from derohe source
# Note: We build from HOLOGRAM's module context so dependencies resolve correctly
derod: check-derohe
	@echo "🔨 Building derod from derohe source..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(DEROD_BIN) $(DEROHE_PKG)/cmd/derod
	@chmod +x $(BUILD_DIR)/$(DEROD_BIN)
	@echo "✅ derod built: $(BUILD_DIR)/$(DEROD_BIN)"

# Build simulator from derohe source
# Note: We build from HOLOGRAM's module context so dependencies resolve correctly
simulator: check-derohe
	@echo "🔨 Building simulator from derohe source..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(SIMULATOR_BIN) $(DEROHE_PKG)/cmd/simulator
	@chmod +x $(BUILD_DIR)/$(SIMULATOR_BIN)
	@echo "✅ simulator built: $(BUILD_DIR)/$(SIMULATOR_BIN)"

# Check that derohe dependency is available and add cmd dependencies
check-derohe:
	@echo "🔍 Checking derohe dependency..."
	@go get $(DEROHE_PKG)/cmd/derod@v0.0.0-20250813215012-9b6a8b82c839 2>/dev/null || true
	@go get $(DEROHE_PKG)/cmd/simulator@v0.0.0-20250813215012-9b6a8b82c839 2>/dev/null || true

# Development mode
dev:
	wails dev

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	@echo "✅ Clean complete"

# Help
help:
	@echo "HOLOGRAM Build System"
	@echo ""
	@echo "Targets:"
	@echo "  all        - Build HOLOGRAM + derod + simulator (recommended)"
	@echo "  hologram   - Build HOLOGRAM only"
	@echo "  derod      - Build derod from derohe source"
	@echo "  simulator  - Build simulator from derohe source"
	@echo "  dev        - Run in development mode"
	@echo "  clean      - Remove build artifacts"
	@echo "  help       - Show this help"
	@echo ""
	@echo "Platform: $(GOOS)/$(GOARCH)"
	@echo ""
	@echo "The 'make all' command builds everything needed to run HOLOGRAM"
	@echo "without downloading any pre-built binaries."
