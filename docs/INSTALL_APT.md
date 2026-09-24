# Install TKMChain on Debian or Ubuntu

This guide installs the release artifacts published by the TKMChain GitHub
repository. It supports Debian and Ubuntu on `amd64` (x86-64), `arm64`
(AArch64), and `armhf` (ARMv7).

The package installs:

- `/usr/bin/gtkm`, the TKMChain node and wallet CLI;
- `/usr/bin/tkmchain`, a compatibility alias for `gtkm`;
- `/usr/bin/shielded-payout-prover`, when it is included in the release; and
- release documentation under `/usr/share/doc/tkmchain/`.

The package does not start a node, create a system service, install Tor, or
modify an existing chain database or wallet.

## Install the latest release

Set `TKM_VERSION` when installing a different release. The default below is
`v1.21.29`.

```bash
set -eu

version="${TKM_VERSION:-v1.21.29}"
arch="$(dpkg --print-architecture)"

case "$arch" in
  amd64|arm64|armhf) ;;
  *)
    echo "Unsupported Debian architecture: $arch" >&2
    exit 1
    ;;
esac

sudo apt-get update
sudo apt-get install -y ca-certificates curl

package="tkmchain_${version#v}_${arch}.deb"
curl --fail --location --proto '=https' --tlsv1.2 \
  --output "/tmp/$package" \
  "https://github.com/tkmchain/go-tkmchain/releases/download/${version}/${package}"

sudo apt-get install "/tmp/$package"
```

APT uses the local package because the path starts with `/tmp/`. The package
declares its runtime dependencies, so APT installs any missing system
libraries.

## Verify the download

Every release includes `SHA256SUMS`. Verify the package before installing it
when the release is being deployed to a validator or production wallet:

```bash
version="${TKM_VERSION:-v1.21.29}"
arch="$(dpkg --print-architecture)"
package="tkmchain_${version#v}_${arch}.deb"

curl --fail --location --proto '=https' --tlsv1.2 \
  --output /tmp/SHA256SUMS \
  "https://github.com/tkmchain/go-tkmchain/releases/download/${version}/SHA256SUMS"

(cd /tmp && grep "  $package$" SHA256SUMS | sha256sum --check)
```

The command must report `OK`. Download the checksum file over HTTPS from the
same GitHub release and do not reuse a checksum from another version.

Check the installed package and binaries:

```bash
dpkg-query -W tkmchain
dpkg --print-architecture
gtkm version
tkmchain version
```

## Upgrade or remove

Stop a running node before replacing its binary, then run the install commands
with the newer `TKM_VERSION`. APT preserves the package and executable names.

To remove the binaries while keeping `~/.tkmchain` and wallet data:

```bash
sudo apt remove tkmchain
```

Review and back up the data directory before deleting it. Removing the package
does not delete chain data, keys, or configuration.

## Enable Tor before starting the node

The production network uses onion-only peer transport. Install and start Tor
before launching `gtkm`:

```bash
sudo apt-get install -y tor
sudo systemctl enable --now tor
systemctl is-active tor
ss -ltn | grep ':9050'
```

To accept inbound onion peers, add this hidden service to `/etc/tor/torrc`:

```text
HiddenServiceDir /var/lib/tor/tkmchain/
HiddenServicePort 3000 127.0.0.1:3000
```

Restart Tor and read the generated hostname:

```bash
sudo systemctl restart tor
sudo cat /var/lib/tor/tkmchain/hostname
```

Keep `/var/lib/tor/tkmchain/private_key` secret. Use the generated hostname
with `--p2p.onion-hostname`.

## Start an onion-only node

Replace the placeholders with this node's hostname and an onion bootstrap
peer. Keep RPC on loopback unless a separate Tor onion service is configured
for it:

```bash
gtkm \
  --port 3000 \
  --privacy.onion-only \
  --p2p.tor-socks5=socks5://127.0.0.1:9050 \
  --p2p.onion-hostname='<this-node>.onion' \
  --bootnodes='enode://<peer-key>@<peer>.onion:3000?discport=0' \
  --http --http.addr=127.0.0.1 --http.port=8545 \
  --http.api=eth,net,web3,tvm,tkm,tkmaccount,tkmgov,tkmprivacy,randomx \
  --http.vhosts=localhost --http.corsdomain=localhost \
  --ws --ws.addr=127.0.0.1 --ws.port=8546 \
  --ws.api=eth,net,web3,tvm,tkm --ws.origins=localhost
```

Do not add `--nat=extip:<ip>`, use an IP address in an `enode://` URL, bind
RPC to `0.0.0.0`, or use wildcard CORS/origin settings. Onion-only mode
disables clearnet discovery and refuses non-onion peers. Static and bootstrap
peers must use their `.onion` hostname and the P2P port (`3000`); `9050` is
the local Tor SOCKS proxy, not a TKMChain peer port.

Recent releases can discover `TKM_ONION_HOSTNAME` and the standard Tor
hostname files automatically. Set it explicitly when more than one hidden
service exists:

```bash
export TKM_ONION_HOSTNAME="$(sudo cat /var/lib/tor/tkmchain/hostname)"
```

Check Tor routing before starting the node:

```bash
curl --socks5-hostname 127.0.0.1:9050 \
  https://check.torproject.org/api/ip
```

For the full Tor, bootstrap, and stale-peer recovery instructions, see
[`TOR_INSTALLATION.md`](TOR_INSTALLATION.md),
[`PRIVACY_MODE.md`](PRIVACY_MODE.md), and [`BOOTSTRAP.md`](BOOTSTRAP.md).

## Fast bootstrap for a new data directory

After installing the package, a new node can import the official block archive
in batches instead of downloading every block separately. Follow the
[fast bootstrap guide](BOOTSTRAP.md), then start `gtkm` with the onion
configuration above. The default Linux data directory is `~/.tkmchain`.

## Troubleshooting

- **`Exec format error`:** check `dpkg --print-architecture` and download the
  matching `amd64`, `arm64`, or `armhf` package.
- **`.onion` DNS errors:** make sure Tor is active on `127.0.0.1:9050` and
  provide `--p2p.tor-socks5`; system DNS cannot resolve onion hostnames.
- **`datadir already used by another process`:** stop the existing `gtkm`
  process before starting another node with the same data directory.
- **No peers:** remove stale `static-nodes.json` and `trusted-nodes.json`
  entries containing public IPs or `127.0.0.1:9050`. Every peer URL must use
  an onion hostname and port `3000`.
