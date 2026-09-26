#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
overlay_source="${repo_root}/zk/zkevm/go-overlay/kernel_version_linux.go"
hostname_source="${repo_root}/zk/zkevm/go-overlay/os_sys_linux.go"
netpoll_source="${repo_root}/zk/zkevm/go-overlay/netpoll_epoll_linux.go"
getrandom_source="${repo_root}/zk/zkevm/go-overlay/getrandom_linux.go"
output="${1:-${repo_root}/build/keeper-mipsle}"
build_parallelism="${TKM_KEEPER_BUILD_PARALLELISM:-2}"
# The keeper is part of the repository's root Go module.  Keep all toolchain
# and module lookups rooted there so a checkout no longer needs a second,
# stale cmd/keeper/go.mod file.
goroot="$(cd "${repo_root}" && go env GOROOT)"
gomodcache="$(cd "${repo_root}" && go env GOMODCACHE)"
# Go refuses overlays that replace files below GOMODCACHE. This happens when
# automatic toolchain selection downloads the active toolchain there. Stage a
# private copy only in that case; normal runner/toolcache installations use the
# original GOROOT directly.
staged_goroot=""
if [[ "${goroot}" == "${gomodcache}"/* ]]; then
  staged_goroot="$(mktemp -d "${TMPDIR:-/tmp}/tkm-goroot.XXXXXX")"
  cp -R "${goroot}/." "${staged_goroot}/"
  chmod -R u+rwX "${staged_goroot}"
  goroot="${staged_goroot}"
  export GOROOT="${goroot}"
fi
printf 'guest toolchain: %s\n' "${goroot}" >&2
xsys_module="$(cd "${repo_root}" && GOOS=linux GOARCH=mipsle go list -mod=readonly -m -f '{{.Dir}}' golang.org/x/sys)"
staged_xsys="$(mktemp -d "${TMPDIR:-/tmp}/tkm-xsys.XXXXXX")"
cp -R "${xsys_module}/." "${staged_xsys}/"
chmod -R u+rwX "${staged_xsys}"
modfile="$(mktemp "${TMPDIR:-/tmp}/tkm-keeper.XXXXXX.mod")"
sumfile="${modfile%.mod}.sum"
cp "${repo_root}/go.mod" "${modfile}"
cp "${repo_root}/go.sum" "${sumfile}"
printf '\nreplace golang.org/x/sys => %s\n' "${staged_xsys}" >> "${modfile}"
overlay_json="$(mktemp "${TMPDIR:-/tmp}/tkm-keeper-overlay.XXXXXX.json")"
runtime_patch="$(mktemp "${TMPDIR:-/tmp}/tkm-runtime-os-linux.XXXXXX.go")"
runtime_vma_patch="$(mktemp "${TMPDIR:-/tmp}/tkm-runtime-vma-linux.XXXXXX.go")"
runtime_syscall_patch="$(mktemp "${TMPDIR:-/tmp}/tkm-runtime-syscall-linux.XXXXXX.go")"
syscall_patch="$(mktemp "${TMPDIR:-/tmp}/tkm-syscall-mipsle.XXXXXX.go")"
xsys_dir="${staged_xsys}/unix"
xsys_patch="$(mktemp "${TMPDIR:-/tmp}/tkm-xsys-linux.XXXXXX.go")"
getrandom_patch="$(mktemp "${TMPDIR:-/tmp}/tkm-getrandom-linux.XXXXXX.go")"
trap 'rm -f "${modfile}" "${sumfile}" "${overlay_json}" "${runtime_patch}" "${runtime_vma_patch}" "${runtime_syscall_patch}" "${syscall_patch}" "${xsys_patch}" "${getrandom_patch}"; rm -rf "${staged_xsys}"; [[ -z "${staged_goroot}" ]] || rm -rf "${staged_goroot}"' EXIT

# The overlay generator writes to the requested output path. Create its parent
# before generating the overlay so a fresh checkout without build/ succeeds.
mkdir -p "$(dirname "${output}")"

python3 - "${goroot}" "${overlay_source}" "${hostname_source}" "${netpoll_source}" "${getrandom_source}" "${runtime_patch}" "${runtime_vma_patch}" "${runtime_syscall_patch}" "${syscall_patch}" "${xsys_dir}/zsyscall_linux.go" "${xsys_patch}" "${getrandom_patch}" "${overlay_json}" <<'PY'
import json
import pathlib
import re
import sys

goroot, kernel_replacement, hostname_replacement, netpoll_replacement, getrandom_replacement, runtime_replacement, runtime_vma_replacement, runtime_syscall_replacement, syscall_replacement, xsys_original_path, xsys_replacement, getrandom_patch, output = sys.argv[1:]

def patch_function(source, signature, replacement):
    pattern = re.escape(signature) + r"\s*\{.*?\n\}"
    patched, count = re.subn(pattern, replacement, source, count=1, flags=re.S)
    if count != 1:
        raise SystemExit(f"unable to patch {signature}")
    return patched

runtime_original = pathlib.Path(goroot) / "src/runtime/os_linux.go"
runtime_text = runtime_original.read_text()
pattern = r"func getKernelVersion\(\) \(kv kernelVersion, ok bool\) \{.*?\n\}\n\n// parseRelease"
replacement = (
    "func getKernelVersion() (kv kernelVersion, ok bool) {\n"
    "\treturn kernelVersion{major: 5, minor: 3}, true\n"
    "}\n\n// parseRelease"
)
patched, count = re.subn(pattern, replacement, runtime_text, count=1, flags=re.S)
if count != 1:
    raise SystemExit("unable to patch Go runtime kernel-version probe")
pathlib.Path(runtime_replacement).write_text(patched)
runtime_vma_original = pathlib.Path(goroot) / "src/runtime/set_vma_name_linux.go"
runtime_vma_text = patch_function(
    runtime_vma_original.read_text(),
    "func setVMANameSupported() bool",
    "func setVMANameSupported() bool {\n\treturn false\n}",
)
pathlib.Path(runtime_vma_replacement).write_text(runtime_vma_text)
runtime_syscall_original = pathlib.Path(goroot) / "src/internal/runtime/syscall/linux/syscall_linux.go"
runtime_syscall_text = patch_function(
    runtime_syscall_original.read_text(),
    "func Uname(buf *Utsname) (errno uintptr)",
    "func Uname(buf *Utsname) (errno uintptr) {\n\treturn 0\n}",
)
pathlib.Path(runtime_syscall_replacement).write_text(runtime_syscall_text)
syscall_original = pathlib.Path(goroot) / "src/syscall/zsyscall_linux_mipsle.go"
syscall_text = patch_function(
    syscall_original.read_text(),
    "func Uname(buf *Utsname) (err error)",
    "func Uname(buf *Utsname) (err error) {\n\treturn nil\n}",
)
pathlib.Path(syscall_replacement).write_text(syscall_text)
xsys_text = patch_function(
    pathlib.Path(xsys_original_path).read_text(),
    "func Uname(buf *Utsname) (err error)",
    "func Uname(buf *Utsname) (err error) {\n\treturn nil\n}",
)
pathlib.Path(xsys_replacement).write_text(xsys_text)
getrandom_original = pathlib.Path(goroot) / "src/internal/syscall/unix/getrandom.go"
pathlib.Path(getrandom_patch).write_text(pathlib.Path(getrandom_replacement).read_text())
replacements = {
    str(pathlib.Path(goroot) / "src/internal/syscall/unix/kernel_version_linux.go"): kernel_replacement,
    str(pathlib.Path(goroot) / "src/os/sys_linux.go"): hostname_replacement,
    str(pathlib.Path(goroot) / "src/runtime/netpoll_epoll.go"): netpoll_replacement,
    str(runtime_original): runtime_replacement,
    str(runtime_vma_original): runtime_vma_replacement,
    str(runtime_syscall_original): runtime_syscall_replacement,
    str(syscall_original): syscall_replacement,
    xsys_original_path: xsys_replacement,
    str(getrandom_original): getrandom_patch,
}
pathlib.Path(output).write_text(json.dumps({"Replace": replacements}) + "\n")
PY

(cd "${repo_root}" && \
  GOTOOLCHAIN=local GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
    "${goroot}/bin/go" build -p "${build_parallelism}" -mod=mod -modfile "${modfile}" -tags ziren -trimpath -overlay "${overlay_json}" \
    -o "${output}" ./cmd/keeper)
printf 'keeper guest: %s\n' "${output}"
