#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

target=aarch64-apple-darwin
randomx_build="build/_workspace/randomx/build-darwin-arm64"
randomx_src="build/_workspace/randomx/src"
shield6_archive="zk/shielded3/stark/target/$target/release/libtkm_shield3_stark.a"

export PATH="$HOME/.cargo/bin:$PATH"
if ! rustup target list --installed | grep -Fxq "$target"; then
	rustup target add "$target"
fi

# Build both native archives for the same Apple Silicon target. The Go cgo
# bridge uses the stable target/release path for Darwin, so stage the verified
# cross-target Rust archive there before linking any Go binaries.
make randomx-darwin-arm64
SHIELD4_RUST_TARGET="$target" ./scripts/shield4-build.sh
test -s "$shield6_archive"
mkdir -p zk/shielded3/stark/target/release
cp "$shield6_archive" zk/shielded3/stark/target/release/libtkm_shield3_stark.a

mkdir -p build/bin
version="${VERSION:-dev}"
build_time="${BUILD_TIME:-$(git show -s --format=%cI HEAD)}"
git_commit="${GIT_COMMIT:-$(git rev-parse --short HEAD)}"
ldflags="-s -w -X main.Version=$version -X main.BuildTime=$build_time -X main.GitCommit=$git_commit"

export GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC=clang CXX=clang++
export MACOSX_DEPLOYMENT_TARGET=11.0
export CGO_CFLAGS="-I$root/$randomx_src"
export CGO_LDFLAGS="-L$root/$randomx_build -lrandomx -lc++ -lm -framework CoreFoundation -framework Security"
go build -p "${GO_BUILD_P:-2}" -trimpath -tags 'randomx,cgo,shield3' -ldflags="$ldflags" -o build/bin/gtkm ./cmd/gtkm
go build -p "${GO_BUILD_P:-2}" -trimpath -tags shield3 -ldflags="$ldflags" -o build/bin/shielded-payout-prover ./cmd/shielded-payout-prover

# The EmailVM launcher has no native dependencies; disable cgo to avoid
# accidentally linking host-architecture objects into the arm64 archive.
CGO_ENABLED=0 go build -p "${GO_BUILD_P:-2}" -trimpath -ldflags="$ldflags" -o build/bin/tkm-email ./cmd/tkm-email

for binary in build/bin/gtkm build/bin/shielded-payout-prover build/bin/tkm-email; do
	file "$binary"
	archs="$(lipo -archs "$binary")"
	if [[ "$archs" != arm64 ]]; then
		echo "unexpected macOS binary architecture for $binary: $archs" >&2
		exit 1
	fi
done

# Fail if any binary accidentally depends on a non-system dynamic library.
for binary in build/bin/gtkm build/bin/shielded-payout-prover build/bin/tkm-email; do
	if otool -L "$binary" | tail -n +2 | grep -vE '^[[:space:]]+(/usr/lib/|/System/Library/)' | grep -q .; then
		echo "unexpected non-system dynamic dependency in $binary" >&2
		otool -L "$binary" >&2
		exit 1
	fi
done
