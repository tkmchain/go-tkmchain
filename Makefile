# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

GOBIN = ./build/bin
GO ?= latest
GORUN = go run

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# LDFLAGS for version injection
LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# RandomX configuration
RANDOMX_REPO ?= https://github.com/tevador/RandomX.git
RANDOMX_VERSION ?= v2.0.1
RANDOMX_DIR ?= build/_workspace/randomx
RANDOMX_SRC_DIR ?= $(RANDOMX_DIR)/src
RANDOMX_BUILD_DIR ?= $(RANDOMX_DIR)/build
RANDOMX_LIB = librandomx.a
RANDOMX_LIB_PATH = $(RANDOMX_BUILD_DIR)/$(RANDOMX_LIB)

# Detect OS and Architecture
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

# Get number of CPUs safely
ifeq ($(UNAME_S),Linux)
    NPROC := $(shell nproc 2>/dev/null || echo 4)
else ifeq ($(UNAME_S),Darwin)
    NPROC := $(shell sysctl -n hw.ncpu 2>/dev/null || echo 4)
else
    NPROC := 4
endif

# Set platform-specific variables
ifeq ($(UNAME_S),Linux)
    PLATFORM_OS = linux
    ifeq ($(UNAME_M),aarch64)
        PLATFORM_ARCH = arm64
        PLATFORM_TARGET = arm64
    else ifeq ($(UNAME_M),armv7l)
        PLATFORM_ARCH = arm
        PLATFORM_TARGET = arm
    else ifeq ($(UNAME_M),x86_64)
        PLATFORM_ARCH = amd64
        PLATFORM_TARGET = amd64
    else
        PLATFORM_ARCH = $(UNAME_M)
        PLATFORM_TARGET = native
    endif
endif

ifeq ($(UNAME_S),Darwin)
    PLATFORM_OS = darwin
    PLATFORM_TARGET = macos
    ifeq ($(UNAME_M),arm64)
        PLATFORM_ARCH = arm64
    else ifeq ($(UNAME_M),x86_64)
        PLATFORM_ARCH = amd64
    else
        PLATFORM_ARCH = $(UNAME_M)
    endif
endif

detect-platform:
	@echo "========================================="
	@echo "Detected Platform:"
	@echo "  OS: $(UNAME_S)"
	@echo "  Architecture: $(UNAME_M)"
	@echo "  Target: $(PLATFORM_TARGET)"
	@echo "  CPUs: $(NPROC)"
	@echo "========================================="

all: detect-platform
	@echo ""
	@echo "Building TkmChain for $(PLATFORM_OS) $(PLATFORM_ARCH)"
	@echo "========================================="
	$(MAKE) build-$(PLATFORM_TARGET)
	@echo "Build complete!"
	@echo "Binary: $(GOBIN)/gtkm"

gtkm: detect-platform
	@echo ""
	@echo "Building TkmChain for $(PLATFORM_OS) $(PLATFORM_ARCH)"
	@echo "========================================="
	$(MAKE) build-$(PLATFORM_TARGET)
	@echo "Build complete!"
	@echo "Binary: $(GOBIN)/gtkm"

# Build targets
build-amd64: randomx-amd64
	@echo "Building gtkm for Linux x86_64..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/gtkm ./cmd/gtkm
	@echo "Built: $(GOBIN)/gtkm"
	-file $(GOBIN)/gtkm 2>/dev/null || true

build-arm64: randomx-arm64
	@echo "Building gtkm for ARM64..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 \
		CGO_CFLAGS="-O2 -march=armv8-a -I$(RANDOMX_SRC_DIR)" \
		CGO_CXXFLAGS="-O2 -march=armv8-a" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/gtkm ./cmd/gtkm
	@echo "Built: $(GOBIN)/gtkm"
	-file $(GOBIN)/gtkm 2>/dev/null || true

build-arm: randomx-arm
	@echo "Building gtkm for ARM 32-bit..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 \
		CGO_CFLAGS="-O2 -march=armv7-a -I$(RANDOMX_SRC_DIR)" \
		CGO_CXXFLAGS="-O2 -march=armv7-a" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/gtkm-arm ./cmd/gtkm
	@echo "Built: $(GOBIN)/gtkm-arm"
	-file $(GOBIN)/gtkm-arm 2>/dev/null || true

build-macos: randomx-macos
	@echo "Building gtkm for macOS..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/gtkm ./cmd/gtkm
	@echo "Built: $(GOBIN)/gtkm"
	-file $(GOBIN)/gtkm 2>/dev/null || true

build-native: randomx-native
	@echo "Building gtkm for native platform..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/gtkm ./cmd/gtkm
	@echo "Built: $(GOBIN)/gtkm"

# ============================================================
# RANDOMX BUILD TARGETS
# ============================================================

randomx-amd64:
	@echo "Building RandomX for Linux x86_64..."
	@mkdir -p $(RANDOMX_DIR)
	@if [ ! -d "$(RANDOMX_DIR)/.git" ]; then \
		echo "Cloning RandomX..."; \
		rm -rf $(RANDOMX_DIR); \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) $(RANDOMX_DIR); \
	fi
	@mkdir -p $(RANDOMX_BUILD_DIR)
	@cd $(RANDOMX_BUILD_DIR) && \
		cmake .. -DARCH=native -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON && \
		make -j$(NPROC)
	@echo "RandomX built: $(RANDOMX_LIB_PATH)"

randomx-arm64:
	@echo "Building RandomX for ARM64..."
	@mkdir -p $(RANDOMX_DIR)
	@if [ ! -d "$(RANDOMX_DIR)/.git" ]; then \
		echo "Cloning RandomX..."; \
		rm -rf $(RANDOMX_DIR); \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) $(RANDOMX_DIR); \
	fi
	@mkdir -p $(RANDOMX_BUILD_DIR)
	@cd $(RANDOMX_BUILD_DIR) && \
		cmake .. -DARCH=armv8-a -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON && \
		make -j$(NPROC)
	@echo "RandomX ARM64 built: $(RANDOMX_LIB_PATH)"

randomx-arm:
	@echo "Building RandomX for ARM 32-bit..."
	@mkdir -p $(RANDOMX_DIR)
	@if [ ! -d "$(RANDOMX_DIR)/.git" ]; then \
		echo "Cloning RandomX..."; \
		rm -rf $(RANDOMX_DIR); \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) $(RANDOMX_DIR); \
	fi
	@mkdir -p $(RANDOMX_BUILD_DIR)
	@cd $(RANDOMX_BUILD_DIR) && \
		cmake .. -DARCH=armv7-a -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON && \
		make -j$(NPROC)
	@echo "RandomX ARM built: $(RANDOMX_LIB_PATH)"

randomx-macos:
	@echo "Building RandomX for macOS..."
	@mkdir -p $(RANDOMX_DIR)
	@if [ ! -d "$(RANDOMX_DIR)/.git" ]; then \
		echo "Cloning RandomX..."; \
		rm -rf $(RANDOMX_DIR); \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) $(RANDOMX_DIR); \
	fi
	@mkdir -p $(RANDOMX_BUILD_DIR)
	@cd $(RANDOMX_BUILD_DIR) && \
		cmake .. -DARCH=native -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON && \
		make -j$(NPROC)
	@echo "RandomX macOS built: $(RANDOMX_LIB_PATH)"

randomx-native:
	@echo "Building RandomX for native platform..."
	@mkdir -p $(RANDOMX_DIR)
	@if [ ! -d "$(RANDOMX_DIR)/.git" ]; then \
		echo "Cloning RandomX..."; \
		rm -rf $(RANDOMX_DIR); \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) $(RANDOMX_DIR); \
	fi
	@mkdir -p $(RANDOMX_BUILD_DIR)
	@cd $(RANDOMX_BUILD_DIR) && \
		cmake .. -DARCH=native -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DCMAKE_POSITION_INDEPENDENT_CODE=ON && \
		make -j$(NPROC)
	@echo "RandomX native built: $(RANDOMX_LIB_PATH)"

# ============================================================
# OTHER BINARIES
# ============================================================

clef: randomx-native
	@echo "Building clef..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/clef ./cmd/clef
	@echo "Built: $(GOBIN)/clef"

devp2p: randomx-native
	@echo "Building devp2p..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/devp2p ./cmd/devp2p
	@echo "Built: $(GOBIN)/devp2p"

abigen: randomx-native
	@echo "Building abigen..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/abigen ./cmd/abigen
	@echo "Built: $(GOBIN)/abigen"

bootnode: randomx-native
	@echo "Building bootnode..."
	@mkdir -p $(GOBIN)
	@if [ -d "./cmd/bootnode" ]; then \
		CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/bootnode ./cmd/bootnode; \
		echo "Built: $(GOBIN)/bootnode"; \
	else \
		echo "bootnode directory not found, skipping..."; \
	fi

evm: randomx-native
	@echo "Building evm..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/evm ./cmd/evm
	@echo "Built: $(GOBIN)/evm"

rlpdump: randomx-native
	@echo "Building rlpdump..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/rlpdump ./cmd/rlpdump
	@echo "Built: $(GOBIN)/rlpdump"

# ============================================================
# UTILITY TARGETS
# ============================================================

test: all
	$(GORUN) build/ci.go test

lint:
	$(GORUN) build/ci.go lint

fmt:
	go fmt ./...

clean:
	@echo "Cleaning..."
	go clean -cache
	rm -rf build/_workspace/ $(GOBIN)/* build/dist/
	@echo "Clean complete"

randomx-clean:
	@echo "Cleaning RandomX..."
	rm -rf $(RANDOMX_DIR)
	@echo "RandomX clean complete"

randomx-check:
	@echo "RandomX Build Status"
	@if [ -f "$(RANDOMX_LIB_PATH)" ]; then \
		echo "RandomX library: $(RANDOMX_LIB_PATH)"; \
		file $(RANDOMX_LIB_PATH) 2>/dev/null || true; \
	else \
		echo "RandomX library not found"; \
	fi

help:
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           - Build gtkm (auto-detects platform)"
	@echo "  gtkm          - Build gtkm (auto-detects platform)"
	@echo "  arm64         - Build for ARM64"
	@echo "  arm           - Build for ARM 32-bit"
	@echo "  macos         - Build for macOS"
	@echo "  amd64         - Build for Linux x86_64"
	@echo "  clef          - Build clef"
	@echo "  devp2p        - Build devp2p"
	@echo "  abigen        - Build abigen"
	@echo "  evm           - Build evm"
	@echo "  rlpdump       - Build rlpdump"
	@echo "  clean         - Clean build artifacts"
	@echo "  randomx-clean - Clean RandomX"
	@echo "  randomx-check - Check RandomX status"
	@echo "  fmt           - Format code"
	@echo "  help          - Show this help"

devtools:
	@echo "Installing developer tools..."
	env GOBIN= go install golang.org/x/tools/cmd/stringer@latest
	env GOBIN= go install github.com/fjl/gencodec@latest
	env GOBIN= go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	env GOBIN= go install ./cmd/abigen
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

.DEFAULT_GOAL := all
