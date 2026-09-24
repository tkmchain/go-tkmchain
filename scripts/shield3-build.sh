#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$HOME/.cargo/bin:$PATH"
target="${SHIELD3_RUST_TARGET:-}"
if [[ -z "$target" && "${GOOS:-}" == windows ]]; then target=x86_64-pc-windows-gnu; fi
case "${OSTYPE:-}" in msys*|cygwin*) target="${target:-x86_64-pc-windows-gnu}";; esac
# Keep the artifact beside the Go package even when the runner exports a global
# CARGO_TARGET_DIR. The cgo directives use this deterministic path.
target_dir="${SHIELD3_CARGO_TARGET_DIR:-$root/zk/shielded3/stark/target}"
args=(build --release --locked --manifest-path "$root/zk/shielded3/stark/Cargo.toml" --target-dir "$target_dir" -j "${CARGO_BUILD_JOBS:-2}")
if [[ -n "$target" ]]; then
  args+=(--target "$target" --lib)
  case "$target" in
    x86_64-pc-windows-gnu) export CARGO_TARGET_X86_64_PC_WINDOWS_GNU_LINKER="${CC:-x86_64-w64-mingw32-gcc}";;
    aarch64-linux-android) export CARGO_TARGET_AARCH64_LINUX_ANDROID_LINKER="${CC:?Android NDK compiler required}";;
    aarch64-unknown-linux-gnu) export CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER="${CC:-aarch64-linux-gnu-gcc}";;
    armv7-unknown-linux-gnueabihf) export CARGO_TARGET_ARMV7_UNKNOWN_LINUX_GNUEABIHF_LINKER="${CC:-arm-linux-gnueabihf-gcc}";;
  esac
fi
"${CARGO:-cargo}" "${args[@]}"

if [[ -n "$target" ]]; then
  library="$target_dir/$target/release/libtkm_shield3_stark.a"
else
  library="$target_dir/release/libtkm_shield3_stark.a"
fi
if [[ ! -s "$library" ]]; then
  echo "Shield3 native library was not produced: $library" >&2
  exit 1
fi
printf 'Shield3 native library: %s (%s bytes)\n' "$library" "$(stat -c '%s' "$library" 2>/dev/null || wc -c < "$library")"
