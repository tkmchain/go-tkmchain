# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: geth evm all test lint fmt clean devtools help randomx randomx-clean randomx-install

GOBIN = ./build/bin
GO ?= latest
GORUN = go run

RANDOMX_REPO ?= https://github.com/tevador/RandomX.git
RANDOMX_COMMIT ?= 6c4340ba4561aec9a3611c1aedf9931239777fb3
RANDOMX_DIR ?= build/_workspace/randomx
RANDOMX_BUILD_DIR ?= $(RANDOMX_DIR)/build

ifeq ($(OS),Windows_NT)
RANDOMX_LIB ?= randomx.lib
RANDOMX_LIB_PATH ?= $(RANDOMX_BUILD_DIR)/Release/$(RANDOMX_LIB)
else
RANDOMX_LIB ?= librandomx.a
RANDOMX_LIB_PATH ?= $(RANDOMX_BUILD_DIR)/$(RANDOMX_LIB)
endif

# CGO flags for RandomX
CGO_CFLAGS = -I$(RANDOMX_DIR)/src
CGO_LDFLAGS = -L$(RANDOMX_BUILD_DIR) -lrandomx -lstdc++ -lm

#? geth: Build geth.
geth: randomx
	@echo "Building geth with RandomX..."
	@if [ ! -f "$(RANDOMX_LIB_PATH)" ]; then \
		echo "ERROR: RandomX library not found at $(RANDOMX_LIB_PATH)"; \
		echo "Please run 'make randomx' first"; \
		exit 1; \
	fi
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		go build -tags "randomx,cgo" -o $(GOBIN)/geth ./cmd/geth
	@echo "Done building."
	@echo "Run \"$(GOBIN)/geth\" to launch geth."

#? evm: Build evm.
evm: randomx
	@echo "Building evm with RandomX..."
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		$(GORUN) build/ci.go install ./cmd/evm
	@echo "Done building."

#? all: Build all packages and executables.
all: randomx
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		$(GORUN) build/ci.go install

#? test: Run the tests.
test: all
	$(GORUN) build/ci.go test

#? lint: Run certain pre-selected linters.
lint: ## Run linters.
	$(GORUN) build/ci.go lint

#? fmt: Ensure consistent code formatting.
fmt:
	gofmt -s -w $(shell find . -name "*.go")

#? clean: Clean go cache, built executables, and the auto generated folder.
clean:
	go clean -cache
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

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

#? randomx: Clone and build tevador/RandomX library.
randomx:
	@set -e; \
	randomx_src_dir="$$(pwd)/$(RANDOMX_DIR)"; \
	if [ ! -d "$(RANDOMX_DIR)/.git" ]; then \
		echo "Cloning RandomX into $(RANDOMX_DIR)"; \
		mkdir -p "$(dir $(RANDOMX_DIR))"; \
		git clone --depth 1 --branch v2.0.1 "$(RANDOMX_REPO)" "$(RANDOMX_DIR)"; \
	else \
		echo "RandomX already cloned, updating..."; \
		cd "$(RANDOMX_DIR)"; \
		git fetch --tags --force origin; \
	fi; \
	echo "Building RandomX..."; \
	mkdir -p "$(RANDOMX_BUILD_DIR)"; \
	cd "$(RANDOMX_BUILD_DIR)"; \
	cmake "$$randomx_src_dir" -DARCH=native -DCMAKE_BUILD_TYPE=Release; \
	cmake --build . --config Release -j$(nproc); \
	if [ ! -f "$(RANDOMX_LIB_PATH)" ]; then \
		echo "ERROR: RandomX library was not built at $(RANDOMX_LIB_PATH)"; \
		exit 1; \
	else \
		echo "✓ RandomX library built successfully at $(RANDOMX_LIB_PATH)"; \
		echo "✓ CGO_CFLAGS=$(CGO_CFLAGS)"; \
		echo "✓ CGO_LDFLAGS=$(CGO_LDFLAGS)"; \
	fi

#? randomx-clean: Remove built RandomX source and artifacts.
randomx-clean:
	@echo "Cleaning RandomX build..."
	rm -rf "$(RANDOMX_DIR)"
	@echo "RandomX clean complete."

#? randomx-install: Install RandomX library system-wide (optional)
randomx-install: randomx
	@echo "Installing RandomX to /usr/local..."
	cd $(RANDOMX_BUILD_DIR) && sudo make install
	@echo "RandomX installed to /usr/local"
	@echo "You can now use system-wide CGO flags:"
	@echo "  CGO_CFLAGS=-I/usr/local/include"
	@echo "  CGO_LDFLAGS=-L/usr/local/lib -lrandomx -lstdc++ -lm"
