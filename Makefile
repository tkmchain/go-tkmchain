# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: gtkm evm all test lint fmt clean devtools help randomx randomx-clean randomx-install randomx-check

GOBIN = ./build/bin
GO ?= latest
GORUN = go run

# RandomX configuration
RANDOMX_REPO ?= https://github.com/tevador/RandomX.git
RANDOMX_VERSION ?= v2.0.1
RANDOMX_DIR ?= build/_workspace/randomx
RANDOMX_BUILD_DIR ?= $(RANDOMX_DIR)/build
RANDOMX_SRC_DIR ?= $(RANDOMX_DIR)/src

# Library paths
RANDOMX_LIB_STATIC = librandomx.a
RANDOMX_LIB_PATH = $(RANDOMX_BUILD_DIR)/$(RANDOMX_LIB_STATIC)

# CGO flags for RandomX (local build)
CGO_CFLAGS = -I$(RANDOMX_SRC_DIR)
CGO_LDFLAGS = -L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm

#? gtkm: Build gtkm with RandomX support.
gtkm:
	@echo "Building gtkm with RandomX..."
	@if [ ! -f "$(RANDOMX_LIB_PATH)" ]; then \
		echo "ERROR: RandomX library not found at $(RANDOMX_LIB_PATH)"; \
		echo "Please run 'make randomx' first to build the library"; \
		exit 1; \
	fi
	@echo "✓ Found RandomX library at $(RANDOMX_LIB_PATH)"
	@echo "✓ Using CGO_CFLAGS=$(CGO_CFLAGS)"
	@echo "✓ Using CGO_LDFLAGS=$(CGO_LDFLAGS)"
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		go build -tags "randomx,cgo" -o $(GOBIN)/gtkm ./cmd/gtkm
	@echo "Done building."
	@echo "Run \"$(GOBIN)/gtkm\" to launch gtkm."

#? evm: Build evm.
evm: randomx
	@echo "Building evm with RandomX..."
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		$(GORUN) build/ci.go install ./cmd/evm
	@echo "Done building."

#? all: Build all packages and executables.
all: randomx
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		$(GORUN)  build/ci.go install

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
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

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
	@sed -n 's/^#?//p' $< | column -t -s ':' |  sort | sed -e 's/^/ /'

#? randomx: Clone and build tevador/RandomX static library.
randomx:
	@set -e; \
	echo "=== Building RandomX $(RANDOMX_VERSION) ==="; \
	\
	SOURCE_DIR="$$(pwd)/$(RANDOMX_DIR)"; \
	if [ ! -d "$$SOURCE_DIR/.git" ]; then \
		echo "Cloning RandomX into $$SOURCE_DIR..."; \
		rm -rf "$$SOURCE_DIR"; \
		mkdir -p "$$(dirname $$SOURCE_DIR)"; \
		git clone --depth 1 --branch $(RANDOMX_VERSION) $(RANDOMX_REPO) "$$SOURCE_DIR"; \
	else \
		echo "RandomX already cloned at $$SOURCE_DIR"; \
	fi; \
	\
	echo "Creating build directory..."; \
	mkdir -p "$(RANDOMX_BUILD_DIR)"; \
	cd "$(RANDOMX_BUILD_DIR)"; \
	\
	echo "Running CMake..."; \
	cmake "$$SOURCE_DIR" -DARCH=native -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF; \
	\
	echo "Building RandomX..."; \
	make -j$$(nproc); \
	\
	if [ -f "$(RANDOMX_LIB_STATIC)" ]; then \
		echo "✓ RandomX static library built: $(RANDOMX_BUILD_DIR)/$(RANDOMX_LIB_STATIC)"; \
		echo ""; \
		echo "✓ CGO_CFLAGS=$(CGO_CFLAGS)"; \
		echo "✓ CGO_LDFLAGS=$(CGO_LDFLAGS)"; \
		echo ""; \
		echo "Now run: make gtkm"; \
	else \
		echo "ERROR: Failed to build $(RANDOMX_LIB_STATIC)"; \
		echo "Build directory contents:"; \
		ls -la "$(RANDOMX_BUILD_DIR)"; \
		exit 1; \
	fi

#? randomx-clean: Remove built RandomX source and artifacts.
randomx-clean:
	@echo "Cleaning RandomX build..."
	rm -rf "$(RANDOMX_DIR)"
	@echo "RandomX clean complete."

#? randomx-install: Install RandomX library system-wide (requires sudo).
randomx-install: randomx
	@echo "Installing RandomX to /usr/local..."
	cd $(RANDOMX_BUILD_DIR) && sudo make install
	@echo "RandomX installed to /usr/local"
	@echo "You can now build gtkm with: CGO_ENABLED=1 go build -tags 'randomx,cgo' ./cmd/gtkm"

#? randomx-check: Check RandomX build status.
randomx-check:
	@echo "=== RandomX Build Status ==="
	@echo ""
	@if [ -f "$(RANDOMX_LIB_PATH)" ]; then \
		echo "✓ Library: $(RANDOMX_LIB_PATH)"; \
		ls -la "$(RANDOMX_LIB_PATH)"; \
	else \
		echo "✗ Library not found at $(RANDOMX_LIB_PATH)"; \
	fi
	@if [ -d "$(RANDOMX_SRC_DIR)" ]; then \
		echo "✓ Source: $(RANDOMX_SRC_DIR)"; \
		ls -la "$(RANDOMX_SRC_DIR)/randomx.h" 2>/dev/null || echo "  (header not found)"; \
	else \
		echo "✗ Source not found at $(RANDOMX_SRC_DIR)"; \
	fi
	@echo ""
	@if [ -f "$(RANDOMX_LIB_PATH)" ] && [ -f "$(RANDOMX_SRC_DIR)/randomx.h" ]; then \
		echo "✅ RandomX is ready! Run 'make gtkm' to build."; \
	else \
		echo "❌ RandomX is not ready. Run 'make randomx' to build."; \
	fi
