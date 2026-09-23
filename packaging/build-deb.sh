#!/usr/bin/env bash
# Build the portable TKMChain Linux binaries into a Debian package.
set -euo pipefail

release_tag=${1:?usage: $0 <version-or-tag> [output-dir] [architecture]}
out_dir=${2:-release}
deb_arch=${3:-${DEB_ARCH:-amd64}}
version=${release_tag#v}
binary_dir=${TKMCHAIN_BINARY_DIR:-build/bin}

if [[ ! "$version" =~ ^[0-9][0-9A-Za-z.+:~-]*$ ]]; then
  echo "invalid Debian package version: $version" >&2
  exit 1
fi
case "$deb_arch" in
  amd64|arm64|armhf) ;;
  *)
    echo "unsupported Debian architecture: $deb_arch (expected amd64, arm64, or armhf)" >&2
    exit 1
    ;;
esac
if [[ ! -x "$binary_dir/gtkm" ]]; then
  echo "$binary_dir/gtkm is missing; build gtkm before packaging" >&2
  exit 1
fi

mkdir -p "$out_dir"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
root="$work_dir/tkmchain"
mkdir -p "$root/DEBIAN" "$root/usr/bin" "$root/usr/share/doc/tkmchain"

cat > "$root/DEBIAN/control" <<CONTROL
Package: tkmchain
Version: $version
Section: net
Priority: optional
Architecture: $deb_arch
Maintainer: TKMChain Developers <dev@tkmchain.site>
Depends: ca-certificates, libc6 (>= 2.31), libgcc-s1, libstdc++6
Description: TKMChain RandomX node and wallet
 TKMChain is an EVM-compatible blockchain node with RandomX proof of work,
 shielded transactions, and post-quantum account support.
CONTROL

install -Dm0755 "$binary_dir/gtkm" "$root/usr/bin/gtkm"
ln -s gtkm "$root/usr/bin/tkmchain"
if [[ -x "$binary_dir/shielded-payout-prover" ]]; then
  install -Dm0755 "$binary_dir/shielded-payout-prover" "$root/usr/bin/shielded-payout-prover"
fi
install -Dm0644 README.md "$root/usr/share/doc/tkmchain/README.md"
if [[ -f docs/TOR_INSTALLATION.md ]]; then
  install -Dm0644 docs/TOR_INSTALLATION.md "$root/usr/share/doc/tkmchain/TOR_INSTALLATION.md"
fi
if [[ -f config.example.json ]]; then
  install -Dm0644 config.example.json "$root/usr/share/doc/tkmchain/config.example.json"
fi

package="$out_dir/tkmchain_${version}_${deb_arch}.deb"
dpkg-deb --build --root-owner-group "$root" "$package" >/dev/null
echo "$package"
