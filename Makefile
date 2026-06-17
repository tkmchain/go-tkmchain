# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: gtkm clef devp2p abigen bootnode evm rlpdump all \
        test lint fmt clean devtools help \
        randomx randomx-clean randomx-install randomx-check \
        randomx-windows randomx-darwin randomx-linux randomx-all \
        run-solo run-pool \
        cross cross-windows cross-darwin cross-linux cross-all \
        cross-windows-all cross-darwin-all cross-linux-all cross-all-all \
        clean-cross dist

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

# RandomX build directories per platform
RANDOMX_BUILD_DIR_HOST ?= $(RANDOMX_DIR)/build-host
RANDOMX_BUILD_DIR_WINDOWS ?= $(RANDOMX_DIR)/build-windows
RANDOMX_BUILD_DIR_DARWIN ?= $(RANDOMX_DIR)/build-darwin
RANDOMX_BUILD_DIR_LINUX ?= $(RANDOMX_DIR)/build-linux

# Use the posix versions of the compilers
MINGW64_CC = x86_64-w64-mingw32-gcc-posix
MINGW64_CXX = x86_64-w64-mingw32-g++-posix
MINGW32_CC = i686-w64-mingw32-gcc-posix
MINGW32_CXX = i686-w64-mingw32-g++-posix
AARCH64_CC = aarch64-linux-gnu-gcc
AARCH64_CXX = aarch64-linux-gnu-g++

# Library paths per platform
RANDOMX_LIB_STATIC = librandomx.a
RANDOMX_LIB_HOST = $(RANDOMX_BUILD_DIR_HOST)/$(RANDOMX_LIB_STATIC)
RANDOMX_LIB_WINDOWS = $(RANDOMX_BUILD_DIR_WINDOWS)/$(RANDOMX_LIB_STATIC)
RANDOMX_LIB_DARWIN = $(RANDOMX_BUILD_DIR_DARWIN)/$(RANDOMX_LIB_STATIC)
RANDOMX_LIB_LINUX = $(RANDOMX_BUILD_DIR_LINUX)/$(RANDOMX_LIB_STATIC)

# Cross-compilation targets
CROSS_OUTPUT_DIR = ./build/dist
CROSS_WINDOWS_EXT = .exe

# Static linking flags for Windows - simplified
WIN_STATIC_LDFLAGS = -static-libgcc -static-libstdc++ -Wl,-Bstatic -lstdc++ -lpthread -Wl,-Bdynamic

# List of all commands to build
CMDS = gtkm clef devp2p abigen bootnode evm rlpdump

# Check if 32-bit mingw is available
HAS_MINGW32 := $(shell command -v i686-w64-mingw32-g++-posix 2>/dev/null || echo "")

#? all: Build all executables with RandomX support.
all: $(CMDS)
	@echo "✅ All executables built successfully!"
	@echo "�� Output directory: $(GOBIN)"
	@ls -la $(GOBIN)/* 2>/dev/null || echo "No binaries found."

#? gtkm: Build gtkm with RandomX support.
gtkm: randomx
	@echo "Building gtkm with RandomX..."
	@if [ ! -f "$(RANDOMX_LIB_HOST)" ]; then \
		echo "ERROR: RandomX library not found at $(RANDOMX_LIB_HOST)"; \
		echo "Please run 'make randomx' first to build the library"; \
		exit 1; \
	fi
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/gtkm ./cmd/gtkm
	@echo "✅ Built: $(GOBIN)/gtkm"

#? clef: Build clef (transaction signing tool).
clef: randomx
	@echo "Building clef..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/clef ./cmd/clef
	@echo "✅ Built: $(GOBIN)/clef"

#? devp2p: Build devp2p (networking utilities).
devp2p: randomx
	@echo "Building devp2p..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/devp2p ./cmd/devp2p
	@echo "✅ Built: $(GOBIN)/devp2p"

#? abigen: Build abigen (ABI code generator).
abigen: randomx
	@echo "Building abigen..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/abigen ./cmd/abigen
	@echo "✅ Built: $(GOBIN)/abigen"

#? bootnode: Build bootnode (bootstrap node).
bootnode: randomx
	@echo "Building bootnode..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/bootnode ./cmd/bootnode
	@echo "✅ Built: $(GOBIN)/bootnode"

#? evm: Build evm (EVM debugger).
evm: randomx
	@echo "Building evm..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/evm ./cmd/evm
	@echo "✅ Built: $(GOBIN)/evm"

#? rlpdump: Build rlpdump (RLP dumper).
rlpdump: randomx
	@echo "Building rlpdump..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/rlpdump ./cmd/rlpdump
	@echo "✅ Built: $(GOBIN)/rlpdump"

#? test: Run the tests.
test: all
	$(GORUN) build/ci.go test

#? lint: Run certain pre-selected linters.
lint:
	$(GORUN) build/ci.go lint

#? fmt: Ensure consistent code formatting.
fmt:
	gofmt -s -w $(shell find . -name "*.go")

#? clean: Clean go cache, built executables, and the auto generated folder.
clean:
	go clean -cache
	rm -fr build/_workspace/pkg/ $(GOBIN)/* $(CROSS_OUTPUT_DIR)
	@echo "✅ Clean complete"

#? devtools: Install recommended developer tools.
devtools:
	env GOBIN= go install golang.org/x/tools/cmd/stringer@latest
	env GOBIN= go install github.com/fjl/gencodec@latest
	env GOBIN= go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	env GOBIN= go install ./cmd/abigen
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

#? help: Get more info on make commands.
help: Makefile
	@echo ''
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@sed -n 's/^#?//p' $< | column -t -s ':' | sort | sed -e 's/^/ /'

# ====================================================
# RANDOMX BUILD TARGETS (per platform)
# ====================================================

#? randomx: Clone and build tevador/RandomX static library for host.
randomx: randomx-host

#? randomx-host: Build RandomX for host platform.
randomx-host:
	@set -e; \
	echo "=== Building RandomX for Host ==="; \
	SOURCE_DIR="$$(pwd)/$(RANDOMX_DIR)"; \
	if [ ! -d "$$SOURCE_DIR/.git" ]; then \
		echo "Cloning RandomX into $$SOURCE_DIR..."; \
		rm -rf "$$SOURCE_DIR"; \
		mkdir -p "$$(dirname $$SOURCE_DIR)"; \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) "$$SOURCE_DIR"; \
	else \
		echo "RandomX already cloned at $$SOURCE_DIR"; \
	fi; \
	echo "Creating build directory..."; \
	mkdir -p "$(RANDOMX_BUILD_DIR_HOST)"; \
	cd "$(RANDOMX_BUILD_DIR_HOST)"; \
	echo "Running CMake..."; \
	cmake "$$SOURCE_DIR" -DARCH=native -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF; \
	echo "Building RandomX..."; \
	make -j$$(nproc); \
	if [ -f "$(RANDOMX_LIB_STATIC)" ]; then \
		echo "✓ RandomX static library built: $(RANDOMX_BUILD_DIR_HOST)/$(RANDOMX_LIB_STATIC)"; \
	else \
		echo "ERROR: Failed to build $(RANDOMX_LIB_STATIC)"; \
		exit 1; \
	fi

#? randomx-windows: Build RandomX for Windows (cross-compile with mingw).
randomx-windows:
	@set -e; \
	echo "=== Building RandomX for Windows ==="; \
	echo "Requires: sudo apt-get install gcc-mingw-w64-x86-64 cmake"; \
	echo ""; \
	SOURCE_DIR="$$(pwd)/$(RANDOMX_DIR)"; \
	if [ ! -d "$$SOURCE_DIR/.git" ]; then \
		echo "Cloning RandomX into $$SOURCE_DIR..."; \
		rm -rf "$$SOURCE_DIR"; \
		mkdir -p "$$(dirname $$SOURCE_DIR)"; \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) "$$SOURCE_DIR"; \
	else \
		echo "RandomX already cloned at $$SOURCE_DIR"; \
	fi; \
	echo "Creating build directory..."; \
	mkdir -p "$(RANDOMX_BUILD_DIR_WINDOWS)"; \
	cd "$(RANDOMX_BUILD_DIR_WINDOWS)"; \
	echo "Using compiler: $(MINGW64_CC)"; \
	if ! command -v $(MINGW64_CC) >/dev/null 2>&1; then \
		echo "ERROR: $(MINGW64_CC) not found!"; \
		echo "Please install: sudo apt-get install gcc-mingw-w64-x86-64"; \
		exit 1; \
	fi; \
	echo "Running CMake for Windows (using mingw)..."; \
	cmake "$$SOURCE_DIR" \
		-DCMAKE_C_COMPILER=$(MINGW64_CC) \
		-DCMAKE_CXX_COMPILER=$(MINGW64_CXX) \
		-DCMAKE_SYSTEM_NAME=Windows \
		-DCMAKE_SYSTEM_PROCESSOR=x86_64 \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DCMAKE_FIND_ROOT_PATH_MODE_PROGRAM=NEVER \
		-DCMAKE_FIND_ROOT_PATH_MODE_LIBRARY=ONLY \
		-DCMAKE_FIND_ROOT_PATH_MODE_INCLUDE=ONLY; \
	echo "Building RandomX for Windows..."; \
	make -j$$(nproc); \
	if [ -f "$(RANDOMX_LIB_STATIC)" ]; then \
		echo "✓ RandomX Windows library built: $(RANDOMX_BUILD_DIR_WINDOWS)/$(RANDOMX_LIB_STATIC)"; \
		file "$(RANDOMX_LIB_STATIC)" || true; \
	else \
		echo "ERROR: Failed to build $(RANDOMX_LIB_STATIC) for Windows"; \
		echo "Build directory contents:"; \
		ls -la "$(RANDOMX_BUILD_DIR_WINDOWS)" || true; \
		exit 1; \
	fi

#? randomx-windows-386: Build RandomX for Windows 32-bit (skip if not available).
randomx-windows-386:
	@set -e; \
	if [ -z "$(HAS_MINGW32)" ]; then \
		echo "⚠️  i686-w64-mingw32-g++-posix not found. Skipping Windows 32-bit build."; \
		echo "   To install: sudo apt-get install gcc-mingw-w64-i686"; \
		exit 0; \
	fi; \
	echo "=== Building RandomX for Windows 32-bit ==="; \
	echo "Requires: sudo apt-get install gcc-mingw-w64-i686 cmake"; \
	SOURCE_DIR="$$(pwd)/$(RANDOMX_DIR)"; \
	if [ ! -d "$$SOURCE_DIR/.git" ]; then \
		echo "Cloning RandomX into $$SOURCE_DIR..."; \
		rm -rf "$$SOURCE_DIR"; \
		mkdir -p "$$(dirname $$SOURCE_DIR)"; \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) "$$SOURCE_DIR"; \
	else \
		echo "RandomX already cloned at $$SOURCE_DIR"; \
	fi; \
	echo "Creating build directory..."; \
	mkdir -p "$(RANDOMX_BUILD_DIR_WINDOWS)/386"; \
	cd "$(RANDOMX_BUILD_DIR_WINDOWS)/386"; \
	echo "Using compiler: $(MINGW32_CC)"; \
	echo "Running CMake for Windows 32-bit..."; \
	cmake "$$SOURCE_DIR" \
		-DCMAKE_C_COMPILER=$(MINGW32_CC) \
		-DCMAKE_CXX_COMPILER=$(MINGW32_CXX) \
		-DCMAKE_SYSTEM_NAME=Windows \
		-DCMAKE_SYSTEM_PROCESSOR=i686 \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DCMAKE_FIND_ROOT_PATH_MODE_PROGRAM=NEVER \
		-DCMAKE_FIND_ROOT_PATH_MODE_LIBRARY=ONLY \
		-DCMAKE_FIND_ROOT_PATH_MODE_INCLUDE=ONLY; \
	echo "Building RandomX for Windows 32-bit..."; \
	make -j$$(nproc); \
	if [ -f "$(RANDOMX_LIB_STATIC)" ]; then \
		echo "✓ RandomX Windows 32-bit library built"; \
	else \
		echo "ERROR: Failed to build $(RANDOMX_LIB_STATIC) for Windows 32-bit"; \
		exit 1; \
	fi

#? randomx-darwin: Build RandomX for macOS (requires OSXCross).
randomx-darwin:
	@set -e; \
	echo "=== Building RandomX for macOS ==="; \
	echo "Note: This requires OSXCross installed"; \
	echo "See: https://github.com/tpoechtrager/osxcross"; \
	echo ""; \
	echo "If OSXCross is not available, this will fail."; \
	echo "For macOS builds, it's recommended to build natively on macOS."; \
	SOURCE_DIR="$$(pwd)/$(RANDOMX_DIR)"; \
	if [ ! -d "$$SOURCE_DIR/.git" ]; then \
		echo "Cloning RandomX into $$SOURCE_DIR..."; \
		rm -rf "$$SOURCE_DIR"; \
		mkdir -p "$$(dirname $$SOURCE_DIR)"; \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) "$$SOURCE_DIR"; \
	else \
		echo "RandomX already cloned at $$SOURCE_DIR"; \
	fi; \
	echo "Creating build directory..."; \
	mkdir -p "$(RANDOMX_BUILD_DIR_DARWIN)"; \
	cd "$(RANDOMX_BUILD_DIR_DARWIN)"; \
	echo "Running CMake for macOS..."; \
	if command -v osxcross >/dev/null 2>&1; then \
		cmake "$$SOURCE_DIR" \
			-DCMAKE_SYSTEM_NAME=Darwin \
			-DCMAKE_OSX_DEPLOYMENT_TARGET=10.15 \
			-DCMAKE_BUILD_TYPE=Release \
			-DBUILD_SHARED_LIBS=OFF; \
		make -j$$(nproc); \
		if [ -f "$(RANDOMX_LIB_STATIC)" ]; then \
			echo "✓ RandomX macOS library built: $(RANDOMX_BUILD_DIR_DARWIN)/$(RANDOMX_LIB_STATIC)"; \
		else \
			echo "ERROR: Failed to build $(RANDOMX_LIB_STATIC) for macOS"; \
			exit 1; \
		fi; \
	else \
		echo "⚠️ OSXCross not found. Skipping macOS build."; \
		echo "To build for macOS, either:"; \
		echo "  1. Install OSXCross: https://github.com/tpoechtrager/osxcross"; \
		echo "  2. Or build natively on a Mac with: make cross-darwin"; \
	fi

#? randomx-linux: Build RandomX for Linux ARM64 (cross-compile from x86_64).
randomx-linux:
	@set -e; \
	echo "=== Building RandomX for Linux ARM64 ==="; \
	echo "Requires: sudo apt-get install gcc-aarch64-linux-gnu"; \
	SOURCE_DIR="$$(pwd)/$(RANDOMX_DIR)"; \
	if [ ! -d "$$SOURCE_DIR/.git" ]; then \
		echo "Cloning RandomX into $$SOURCE_DIR..."; \
		rm -rf "$$SOURCE_DIR"; \
		mkdir -p "$$(dirname $$SOURCE_DIR)"; \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) "$$SOURCE_DIR"; \
	else \
		echo "RandomX already cloned at $$SOURCE_DIR"; \
	fi; \
	echo "Creating build directory..."; \
	mkdir -p "$(RANDOMX_BUILD_DIR_LINUX)"; \
	cd "$(RANDOMX_BUILD_DIR_LINUX)"; \
	echo "Using compiler: $(AARCH64_CC)"; \
	if ! command -v $(AARCH64_CC) >/dev/null 2>&1; then \
		echo "ERROR: $(AARCH64_CC) not found!"; \
		echo "Please install: sudo apt-get install gcc-aarch64-linux-gnu"; \
		exit 1; \
	fi; \
	echo "Running CMake for Linux ARM64..."; \
	cmake "$$SOURCE_DIR" \
		-DCMAKE_C_COMPILER=$(AARCH64_CC) \
		-DCMAKE_CXX_COMPILER=$(AARCH64_CXX) \
		-DCMAKE_SYSTEM_NAME=Linux \
		-DCMAKE_SYSTEM_PROCESSOR=aarch64 \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF; \
	echo "Building RandomX for Linux ARM64..."; \
	make -j$$(nproc); \
	if [ -f "$(RANDOMX_LIB_STATIC)" ]; then \
		echo "✓ RandomX Linux ARM64 library built: $(RANDOMX_BUILD_DIR_LINUX)/$(RANDOMX_LIB_STATIC)"; \
	else \
		echo "ERROR: Failed to build $(RANDOMX_LIB_STATIC) for Linux ARM64"; \
		exit 1; \
	fi

#? randomx-all: Build RandomX for all platforms.
randomx-all: randomx-host randomx-windows randomx-linux
	@echo "✅ All RandomX builds complete."
	@echo "Note: macOS build requires OSXCross or native macOS."

#? randomx-clean: Remove built RandomX source and artifacts.
randomx-clean:
	@echo "Cleaning RandomX build..."
	rm -rf "$(RANDOMX_DIR)"
	@echo "RandomX clean complete."

#? randomx-install: Install RandomX library system-wide (requires sudo).
randomx-install: randomx-host
	@echo "Installing RandomX to /usr/local..."
	cd $(RANDOMX_BUILD_DIR_HOST) && sudo make install
	@echo "RandomX installed to /usr/local"

#? randomx-check: Check RandomX build status.
randomx-check:
	@echo "=== RandomX Build Status ==="
	@echo ""
	@for lib in "$(RANDOMX_LIB_HOST)" "$(RANDOMX_LIB_WINDOWS)" "$(RANDOMX_LIB_DARWIN)" "$(RANDOMX_BUILD_DIR_LINUX)/$(RANDOMX_LIB_STATIC)"; do \
		if [ -f "$$lib" ]; then \
			echo "✓ $$lib"; \
		else \
			echo "✗ $$lib (not found)"; \
		fi; \
	done

#? randomx-miner: Build standalone RandomX mining daemon
randomx-miner: randomx-host
	@echo "Building RandomX standalone mining daemon..."
	@if [ ! -f "$(RANDOMX_LIB_HOST)" ]; then \
		echo "ERROR: RandomX library not found"; \
		exit 1; \
	fi
	@mkdir -p $(GOBIN)
	CGO_ENABLED=1 CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(GOBIN)/randomx-miner ./cmd/randomx-miner
	@echo "✅ Built: $(GOBIN)/randomx-miner"

#? run-solo: Run standalone solo miner
run-solo: randomx-miner
	@echo "Starting standalone solo miner..."
	@LD_LIBRARY_PATH="$(RANDOMX_BUILD_DIR_HOST):$$LD_LIBRARY_PATH" \
	SOLO_MINE=true \
	COINBASE="$(or $(COINBASE),0x79eb43064b826570FFa9c329c5685208E5257703)" \
	THREADS="$(or $(THREADS),2)" \
	$(GOBIN)/randomx-miner

#? run-pool: Run pool mode for external miners
run-pool: randomx-miner
	@echo "Starting pool mode..."
	@LD_LIBRARY_PATH="$(RANDOMX_BUILD_DIR_HOST):$$LD_LIBRARY_PATH" \
	SOLO_MINE=false \
	RPC_PORT="$(or $(RPC_PORT),8545)" \
	$(GOBIN)/randomx-miner

# ====================================================
# CROSS-COMPILATION TARGETS (All Tools)
# ====================================================

# Define the list of tools for cross-compilation
CROSS_CMDS = gtkm clef devp2p abigen bootnode evm rlpdump

#? cross-windows: Build Windows 64-bit executable (gtkm only).
cross-windows: randomx-windows
	@echo "Building gtkm for Windows..."
	@mkdir -p $(CROSS_OUTPUT_DIR)/windows
	CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_WINDOWS) -lrandomx -Wl,-Bstatic -lstdc++ -lpthread -Wl,-Bdynamic" \
		GOOS=windows GOARCH=amd64 CC=$(MINGW64_CC) CXX=$(MINGW64_CXX) \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/windows/gtkm-windows-amd64$(CROSS_WINDOWS_EXT) ./cmd/gtkm
	@echo "✅ Windows build complete: $(CROSS_OUTPUT_DIR)/windows/"

#? cross-windows-all: Build all Windows executables (64-bit only, 32-bit optional).
cross-windows-all: randomx-windows randomx-windows-386
	@echo "Building ALL Windows executables..."
	@mkdir -p $(CROSS_OUTPUT_DIR)/windows
	@for cmd in $(CROSS_CMDS); do \
		echo "Building $$cmd for Windows 64-bit..."; \
		CGO_ENABLED=1 \
			CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
			CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_WINDOWS) -lrandomx -Wl,-Bstatic -lstdc++ -lpthread -Wl,-Bdynamic" \
			GOOS=windows GOARCH=amd64 CC=$(MINGW64_CC) CXX=$(MINGW64_CXX) \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/windows/$$cmd-windows-amd64$(CROSS_WINDOWS_EXT) ./cmd/$$cmd; \
	done
	@if [ -n "$(HAS_MINGW32)" ]; then \
		for cmd in $(CROSS_CMDS); do \
			echo "Building $$cmd for Windows 32-bit..."; \
			CGO_ENABLED=1 \
				CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
				CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_WINDOWS)/386 -lrandomx -Wl,-Bstatic -lstdc++ -lpthread -Wl,-Bdynamic" \
				GOOS=windows GOARCH=386 CC=$(MINGW32_CC) CXX=$(MINGW32_CXX) \
				go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/windows/$$cmd-windows-386$(CROSS_WINDOWS_EXT) ./cmd/$$cmd; \
		done; \
	else \
		echo "⚠️  32-bit Windows builds skipped (install gcc-mingw-w64-i686 to enable)"; \
	fi
	@echo "✅ All Windows builds complete: $(CROSS_OUTPUT_DIR)/windows/"
	@echo "�� Windows executables:"
	@ls -la $(CROSS_OUTPUT_DIR)/windows/

#? cross-windows-386: Build Windows 32-bit executable (gtkm only) - skipped if not available.
cross-windows-386: randomx-windows-386
	@echo "Building gtkm for Windows 32-bit..."
	@if [ -n "$(HAS_MINGW32)" ]; then \
		mkdir -p $(CROSS_OUTPUT_DIR)/windows; \
		CGO_ENABLED=1 \
			CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
			CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_WINDOWS)/386 -lrandomx -Wl,-Bstatic -lstdc++ -lpthread -Wl,-Bdynamic" \
			GOOS=windows GOARCH=386 CC=$(MINGW32_CC) CXX=$(MINGW32_CXX) \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/windows/gtkm-windows-386$(CROSS_WINDOWS_EXT) ./cmd/gtkm; \
		echo "✅ Windows 32-bit build complete: $(CROSS_OUTPUT_DIR)/windows/"; \
	else \
		echo "⚠️  Skipping Windows 32-bit build (install gcc-mingw-w64-i686 to enable)"; \
	fi

#? cross-darwin: Build macOS executable (gtkm only).
cross-darwin: randomx-darwin
	@echo "Building gtkm for macOS..."
	@mkdir -p $(CROSS_OUTPUT_DIR)/darwin
	@if [ -f "$(RANDOMX_LIB_DARWIN)" ]; then \
		CGO_ENABLED=1 \
			CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
			CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_DARWIN) -lrandomx -lstdc++ -lm" \
			GOOS=darwin GOARCH=amd64 \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/darwin/gtkm-darwin-amd64 ./cmd/gtkm; \
		CGO_ENABLED=1 \
			CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
			CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_DARWIN) -lrandomx -lstdc++ -lm" \
			GOOS=darwin GOARCH=arm64 \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/darwin/gtkm-darwin-arm64 ./cmd/gtkm; \
		echo "✅ macOS build complete: $(CROSS_OUTPUT_DIR)/darwin/"; \
		ls -la $(CROSS_OUTPUT_DIR)/darwin/; \
	else \
		echo "⚠️ RandomX macOS library not found. Skipping macOS build."; \
	fi

#? cross-darwin-all: Build all macOS executables.
cross-darwin-all: randomx-darwin
	@echo "Building ALL macOS executables..."
	@mkdir -p $(CROSS_OUTPUT_DIR)/darwin
	@if [ -f "$(RANDOMX_LIB_DARWIN)" ]; then \
		for cmd in $(CROSS_CMDS); do \
			echo "Building $$cmd for macOS amd64..."; \
			CGO_ENABLED=1 \
				CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
				CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_DARWIN) -lrandomx -lstdc++ -lm" \
				GOOS=darwin GOARCH=amd64 \
				go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/darwin/$$cmd-darwin-amd64 ./cmd/$$cmd; \
			echo "Building $$cmd for macOS arm64..."; \
			CGO_ENABLED=1 \
				CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
				CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_DARWIN) -lrandomx -lstdc++ -lm" \
				GOOS=darwin GOARCH=arm64 \
				go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/darwin/$$cmd-darwin-arm64 ./cmd/$$cmd; \
		done; \
		echo "✅ All macOS builds complete: $(CROSS_OUTPUT_DIR)/darwin/"; \
		ls -la $(CROSS_OUTPUT_DIR)/darwin/; \
	else \
		echo "⚠️ RandomX macOS library not found. Skipping macOS builds."; \
	fi

#? cross-linux: Build Linux executable (gtkm only).
cross-linux: randomx-linux
	@echo "Building gtkm for Linux..."
	@mkdir -p $(CROSS_OUTPUT_DIR)/linux
	CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		GOOS=linux GOARCH=amd64 \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/linux/gtkm-linux-amd64 ./cmd/gtkm
	CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
		CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
		GOOS=linux GOARCH=386 \
		go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/linux/gtkm-linux-386 ./cmd/gtkm
	@if [ -f "$(RANDOMX_BUILD_DIR_LINUX)/$(RANDOMX_LIB_STATIC)" ]; then \
		CGO_ENABLED=1 \
			CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
			CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_LINUX) -lrandomx -lstdc++ -lm" \
			GOOS=linux GOARCH=arm64 CC=$(AARCH64_CC) CXX=$(AARCH64_CXX) \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/linux/gtkm-linux-arm64 ./cmd/gtkm; \
	fi
	@echo "✅ Linux build complete: $(CROSS_OUTPUT_DIR)/linux/"

#? cross-linux-all: Build all Linux executables.
cross-linux-all: randomx-linux
	@echo "Building ALL Linux executables..."
	@mkdir -p $(CROSS_OUTPUT_DIR)/linux
	@for cmd in $(CROSS_CMDS); do \
		echo "Building $$cmd for Linux amd64..."; \
		CGO_ENABLED=1 \
			CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
			CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
			GOOS=linux GOARCH=amd64 \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/linux/$$cmd-linux-amd64 ./cmd/$$cmd; \
		echo "Building $$cmd for Linux 386..."; \
		CGO_ENABLED=1 \
			CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
			CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_HOST) -lrandomx -lstdc++ -lm" \
			GOOS=linux GOARCH=386 \
			go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/linux/$$cmd-linux-386 ./cmd/$$cmd; \
		if [ -f "$(RANDOMX_BUILD_DIR_LINUX)/$(RANDOMX_LIB_STATIC)" ]; then \
			echo "Building $$cmd for Linux arm64..."; \
			CGO_ENABLED=1 \
				CGO_CFLAGS="-I$(RANDOMX_SRC_DIR)" \
				CGO_LDFLAGS="-L$(RANDOMX_BUILD_DIR_LINUX) -lrandomx -lstdc++ -lm" \
				GOOS=linux GOARCH=arm64 CC=$(AARCH64_CC) CXX=$(AARCH64_CXX) \
				go build $(LDFLAGS) -tags "randomx,cgo" -o $(CROSS_OUTPUT_DIR)/linux/$$cmd-linux-arm64 ./cmd/$$cmd; \
		fi; \
	done
	@echo "✅ All Linux builds complete: $(CROSS_OUTPUT_DIR)/linux/"
	@ls -la $(CROSS_OUTPUT_DIR)/linux/

#? cross-all: Build gtkm only for all platforms.
cross-all: randomx-all cross-windows cross-darwin cross-linux
	@echo "=== All cross-platform gtkm builds complete ==="
	@echo "Output directory: $(CROSS_OUTPUT_DIR)"
	@ls -la $(CROSS_OUTPUT_DIR)/*/gtkm-* 2>/dev/null || echo "No builds found."

#? cross-all-all: Build ALL tools for all platforms.
cross-all-all: randomx-all cross-windows-all cross-darwin-all cross-linux-all
	@echo "=== All cross-platform ALL TOOLS builds complete ==="
	@echo "Output directory: $(CROSS_OUTPUT_DIR)"
	@for dir in windows darwin linux; do \
		if [ -d "$(CROSS_OUTPUT_DIR)/$$dir" ]; then \
			echo "--- $$dir ---"; \
			ls -la $(CROSS_OUTPUT_DIR)/$$dir/; \
		fi; \
	done

#? cross-clean: Clean cross-compilation output.
cross-clean:
	@echo "Cleaning cross-compilation artifacts..."
	rm -rf $(CROSS_OUTPUT_DIR)
	@echo "✅ Clean complete."

#? dist: Create distribution archives for all platforms.
dist: cross-all-all
	@echo "Creating distribution archives..."
	@cd $(CROSS_OUTPUT_DIR) && \
	for dir in windows darwin linux; do \
		if [ -d "$$dir" ]; then \
			cd "$$dir"; \
			tar -czf "../gtkm-$$dir-$(VERSION).tar.gz" * 2>/dev/null || true; \
			cd ..; \
		fi; \
	done
	@echo "✅ Distribution archives created in $(CROSS_OUTPUT_DIR)"
	@ls -la $(CROSS_OUTPUT_DIR)/*.tar.gz 2>/dev/null || echo "No archives created."

#? cross-build: Build all cross-platform binaries (without RandomX library).
cross-build:
	@echo "Building cross-platform binaries (RandomX disabled)..."
	@mkdir -p $(CROSS_OUTPUT_DIR)
	CGO_ENABLED=0 \
		GOOS=windows GOARCH=amd64 \
		go build $(LDFLAGS) -o $(CROSS_OUTPUT_DIR)/gtkm-windows-amd64$(CROSS_WINDOWS_EXT) ./cmd/gtkm
	CGO_ENABLED=0 \
		GOOS=windows GOARCH=386 \
		go build $(LDFLAGS) -o $(CROSS_OUTPUT_DIR)/gtkm-windows-386$(CROSS_WINDOWS_EXT) ./cmd/gtkm
	CGO_ENABLED=0 \
		GOOS=darwin GOARCH=amd64 \
		go build $(LDFLAGS) -o $(CROSS_OUTPUT_DIR)/gtkm-darwin-amd64 ./cmd/gtkm
	CGO_ENABLED=0 \
		GOOS=darwin GOARCH=arm64 \
		go build $(LDFLAGS) -o $(CROSS_OUTPUT_DIR)/gtkm-darwin-arm64 ./cmd/gtkm
	CGO_ENABLED=0 \
		GOOS=linux GOARCH=amd64 \
		go build $(LDFLAGS) -o $(CROSS_OUTPUT_DIR)/gtkm-linux-amd64 ./cmd/gtkm
	CGO_ENABLED=0 \
		GOOS=linux GOARCH=386 \
		go build $(LDFLAGS) -o $(CROSS_OUTPUT_DIR)/gtkm-linux-386 ./cmd/gtkm
	CGO_ENABLED=0 \
		GOOS=linux GOARCH=arm64 \
		go build $(LDFLAGS) -o $(CROSS_OUTPUT_DIR)/gtkm-linux-arm64 ./cmd/gtkm
	@echo "✅ Cross-platform builds complete (without RandomX)."
	@echo "Note: These builds do NOT include RandomX support."
	@echo "Output directory: $(CROSS_OUTPUT_DIR)"
