#!/usr/bin/env bash
set -euo pipefail

# Build the native Shield4 verifier/prover archive used by the Go cgo bridge.
# Shield4 intentionally lives in the same Rust crate as Shield3 so both
# protocols share the audited field and STARK implementation. The archive
# therefore keeps its historical libtkm_shield3_stark.a filename, while this
# script verifies that the exported FFI entry point (which dispatches the
# Shield4 operations 8, 9 and 10) is present in every target artifact.
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$HOME/.cargo/bin:$PATH"

target="${SHIELD4_RUST_TARGET:-${SHIELD3_RUST_TARGET:-}}"
if [[ -z "$target" && "${GOOS:-}" == windows ]]; then
  target=x86_64-pc-windows-gnu
fi
case "${OSTYPE:-}" in
  msys*|cygwin*) target="${target:-x86_64-pc-windows-gnu}" ;;
esac

# Keep the artifact beside the Go package. native_cgo.go uses this stable path
# and must not follow a runner-global CARGO_TARGET_DIR.
target_dir="${SHIELD4_CARGO_TARGET_DIR:-${SHIELD3_CARGO_TARGET_DIR:-$root/zk/shielded3/stark/target}}"
manifest="$root/zk/shielded3/stark/Cargo.toml"
args=(build --release --locked --manifest-path "$manifest" --target-dir "$target_dir" -j "${CARGO_BUILD_JOBS:-2}")
if [[ -n "$target" ]]; then
  args+=(--target "$target" --lib)
  case "$target" in
    x86_64-pc-windows-gnu)
      export CARGO_TARGET_X86_64_PC_WINDOWS_GNU_LINKER="${CC:-x86_64-w64-mingw32-gcc}" ;;
    aarch64-linux-android)
      : "${CC:?Android NDK compiler required}"
      export CARGO_TARGET_AARCH64_LINUX_ANDROID_LINKER="$CC" ;;
    aarch64-unknown-linux-gnu)
      export CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER="${CC:-aarch64-linux-gnu-gcc}" ;;
    armv7-unknown-linux-gnueabihf)
      export CARGO_TARGET_ARMV7_UNKNOWN_LINUX_GNUEABIHF_LINKER="${CC:-arm-linux-gnueabihf-gcc}" ;;
  esac
fi

"${CARGO:-cargo}" "${args[@]}"

if [[ -n "$target" ]]; then
  library="$target_dir/$target/release/libtkm_shield3_stark.a"
else
  library="$target_dir/release/libtkm_shield3_stark.a"
fi
if [[ ! -s "$library" ]]; then
  echo "Shield4 native library was not produced: $library" >&2
  exit 1
fi

# A successful Rust build alone is not enough: a release can otherwise carry
# an archive that links but has no callable native verifier. nm is available
# on all release toolchains (including MinGW); accept ELF and Mach-O spellings.
nm_tool="${NM:-nm}"
if ! command -v "$nm_tool" >/dev/null 2>&1; then
  case "$target" in
    x86_64-pc-windows-gnu) nm_tool=x86_64-w64-mingw32-nm ;;
    aarch64-unknown-linux-gnu) nm_tool=aarch64-linux-gnu-nm ;;
    armv7-unknown-linux-gnueabihf) nm_tool=arm-linux-gnueabihf-nm ;;
  esac
fi
if ! command -v "$nm_tool" >/dev/null 2>&1; then
  echo "Cannot inspect Shield4 archive: nm tool '$nm_tool' is unavailable" >&2
  exit 1
fi
symbols="$("$nm_tool" -g "$library" 2>/dev/null || true)"
if ! grep -Eq '[[:space:]]_?tkm_shield3_call$' <<<"$symbols"; then
  echo "Shield4 FFI symbol tkm_shield3_call is missing from $library" >&2
  exit 1
fi
markers="$(strings "$library" 2>/dev/null || true)"
if ! grep -q 'TKMS4STK' <<<"$markers"; then
  echo "Shield4 domain marker TKMS4STK is missing from $library" >&2
  exit 1
fi

size="$(stat -c '%s' "$library" 2>/dev/null || stat -f '%z' "$library")"
printf 'Shield4 native library: %s (%s bytes)\n' "$library" "$size"
