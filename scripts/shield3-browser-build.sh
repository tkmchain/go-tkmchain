#!/usr/bin/env bash
set -euo pipefail

# Build the browser-native Shield3 modules. The Rust cdylib and Go js/wasm
# worker are the same relation/builder used by gtkm; this script only packages
# their browser targets and does not introduce a second transaction format.
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_dir="${1:-$root_dir/dist/shield3-browser}"
mkdir -p "$out_dir"

rustup target add wasm32-unknown-unknown >/dev/null
RUSTFLAGS='--cfg getrandom_backend="custom"' cargo build --locked --target wasm32-unknown-unknown --release \
  --manifest-path "$root_dir/zk/shielded3/stark/Cargo.toml"

GOOS=js GOARCH=wasm go build -tags shield3 -trimpath \
  -ldflags='-s -w' -o "$out_dir/tkm-shield3-go.wasm" \
  "$root_dir/cmd/shield3-wasm"

cp "$root_dir/zk/shielded3/stark/target/wasm32-unknown-unknown/release/tkm-shield3-stark.wasm" \
  "$out_dir/tkm-shield3-stark.wasm"
go_root="$(go env GOROOT)"
cp "$go_root/lib/wasm/wasm_exec.js" "$out_dir/wasm_exec.js"

test -s "$out_dir/tkm-shield3-go.wasm"
test -s "$out_dir/tkm-shield3-stark.wasm"
test -s "$out_dir/wasm_exec.js"
printf 'Browser Shield3 artifacts written to %s\n' "$out_dir"
