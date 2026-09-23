# Install TKMChain on Debian or Ubuntu

Release tags publish a portable `amd64` Debian package named `tkmchain`. The
package installs:

- `/usr/bin/gtkm`, the TKMChain node and wallet CLI;
- `/usr/bin/tkmchain`, a compatibility alias for `gtkm`;
- `/usr/bin/shielded-payout-prover`, when included in the release; and
- release documentation under `/usr/share/doc/tkmchain/`.

## Install a release package

Choose the release version, download its package, and install it with APT:

```bash
version=v1.21.24
curl -fL -o "/tmp/tkmchain_${version#v}_amd64.deb" \
  "https://github.com/tkmchain/go-tkmchain/releases/download/${version}/tkmchain_${version#v}_amd64.deb"
sudo apt install "/tmp/tkmchain_${version#v}_amd64.deb"
```

The `./` or absolute path is intentional: it tells APT to install the local
package file. After installation, the package is registered as `tkmchain` and
can be inspected with:

```bash
apt policy tkmchain
gtkm version
tkmchain version
```

`tkmchain` does not start a node automatically. Configure Tor and the node
data directory first, then run `gtkm` using the Tor-only instructions in the
main README. The package does not overwrite an existing node database or
wallet directory.

## Upgrade

Download the newer release package and run the same command. APT compares the
Debian package version and upgrades the existing `tkmchain` installation while
keeping `/usr/bin/gtkm` and `/usr/bin/tkmchain` in place.

Debian packages are published for `amd64`, `arm64`, and `armhf` (32-bit
ARMv7). Select the package matching the machine architecture:

- `tkmchain_VERSION_amd64.deb` for x86-64;
- `tkmchain_VERSION_arm64.deb` for AArch64; or
- `tkmchain_VERSION_armhf.deb` for ARMv7.

Portable archives are also available as `gtkm-vX-linux-arm64.tar.gz` and
`gtkm-vX-linux-armv7.tar.gz` when you prefer a package-free installation.
