#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$HOME/.cargo/bin:$PATH"
target="${SHIELD3_RUST_TARGET:-}"
if [[ -z "$target" && "${GOOS:-}" == windows ]]; then target=x86_64-pc-windows-gnu; fi
case "${OSTYPE:-}" in msys*|cygwin*) target="${target:-x86_64-pc-windows-gnu}";; esac
args=(build --release --locked --manifest-path "$root/zk/shielded3/stark/Cargo.toml" -j "${CARGO_BUILD_JOBS:-2}")
if [[ -n "$target" ]]; then
  args+=(--target "$target" --lib)
  case "$target" in
    x86_64-pc-windows-gnu) export CARGO_TARGET_X86_64_PC_WINDOWS_GNU_LINKER="${CC:-x86_64-w64-mingw32-gcc}";;
    aarch64-linux-android) export CARGO_TARGET_AARCH64_LINUX_ANDROID_LINKER="${CC:?Android NDK compiler required}";;
  esac
fi
"${CARGO:-cargo}" "${args[@]}"
