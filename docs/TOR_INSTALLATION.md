# Tor installation for TKMChain

TKMChain's onion-only profile routes peer traffic through a local Tor SOCKS5
proxy. Install and start Tor before launching `gtkm`. The node normally expects
SOCKS5 at `127.0.0.1:9050`.

## Linux

### Ubuntu or Debian

```sh
sudo apt update
sudo apt install tor
sudo systemctl enable --now tor
systemctl is-active tor
```

### Fedora, RHEL, or CentOS Stream

```sh
sudo dnf install tor
sudo systemctl enable --now tor
systemctl is-active tor
```

### Arch Linux

```sh
sudo pacman -S tor
sudo systemctl enable --now tor
systemctl is-active tor
```

Check that the SOCKS listener is available:

```sh
ss -ltn | grep ':9050'
```

Some distributions use the `tor@default` unit instead of `tor`:

```sh
sudo systemctl enable --now tor@default
```

## macOS

Install the Tor daemon with Homebrew:

```sh
brew install tor
brew services start tor
```

The default SOCKS listener is `127.0.0.1:9050`. Stop it with:

```sh
brew services stop tor
```

## Windows

Install the **Tor Expert Bundle** from the [official Tor Project download
page](https://www.torproject.org/download/tor/). The Expert Bundle provides the
Tor daemon and service tools; Tor Browser alone is intended for browsing and is
not the node service.

After installation, start the Tor service from an elevated PowerShell prompt
(the service name may be `tor` or `Tor` depending on the bundle version):

```powershell
Get-Service *tor*
Start-Service tor
```

If the service is not installed, run `tor.exe` with a configuration file that
contains:

```text
SocksPort 127.0.0.1:9050
```

Verify the listener:

```powershell
Test-NetConnection 127.0.0.1 -Port 9050
```

## Android

For the Android wallet, install **Orbot** from [The Guardian Project](https://orbot.app/)
or F-Droid and start the VPN or SOCKS mode. Configure the application to expose
its local SOCKS5 proxy, normally `127.0.0.1:9050` (some Orbot profiles use
`127.0.0.1:9150`). Use the port shown by Orbot in the wallet's Tor settings.

On rooted or Termux systems, the daemon can also be installed with:

```sh
pkg update
pkg install tor
tor --SocksPort 127.0.0.1:9050
```

Do not run both Orbot and a Termux Tor daemon on the same port.

## Optional onion service for a node

To accept incoming onion connections, add a hidden service to Tor's
`torrc` (usually `/etc/tor/torrc` on Linux):

```text
HiddenServiceDir /var/lib/tor/tkmchain/
HiddenServicePort 3000 127.0.0.1:3000
```

Restart Tor and read the generated hostname:

```sh
sudo systemctl restart tor
sudo cat /var/lib/tor/tkmchain/hostname
```

Keep the `private_key` in that directory secret. Use the generated `.onion`
hostname with `--p2p.onion-hostname`; never substitute a public IP address.

## Verify Tor before starting `gtkm`

This command should return a Tor exit-node address through the local proxy:

```sh
curl --socks5-hostname 127.0.0.1:9050 https://check.torproject.org/api/ip
```

Start a node with local RPC bindings and onion-only P2P:

```sh
./build/bin/gtkm \
  --privacy.onion-only \
  --p2p.tor-socks5=socks5://127.0.0.1:9050 \
  --p2p.onion-hostname=<this-node>.onion \
  --bootnodes='enode://<peer-key>@<peer>.onion:3000?discport=0' \
  --http --http.addr=127.0.0.1 --http.port=8545 \
  --ws --ws.addr=127.0.0.1 --ws.port=8546
```

Keep HTTP and WebSocket RPC on loopback unless they are intentionally published
through a separately configured onion service. Do not use `--nat=extip:<ip>` in
onion-only mode.

## Automatic onion startup

Recent `gtkm` releases automatically select `socks5://127.0.0.1:9050` when an
onion bootstrap peer or local onion hostname is detected. They skip system DNS
for `.onion` peers and route those names through Tor. `gtkm` never installs Tor
or changes packages automatically.

The local onion hostname is discovered from the following sources, in order:

1. `TKM_ONION_HOSTNAME`
2. `/var/lib/tor/tkmchain/hostname`
3. `/var/lib/tor/gtkm/hostname`
4. `/var/lib/tor/hidden_service/hostname`

For example:

```sh
export TKM_ONION_HOSTNAME=eaoerarabizbzwbbawjrlcyawnrnoobj3ndy3oh627hwl5rbmedukoqd.onion
./build/bin/gtkm
```

When onion networking is detected, the node enables onion-only mode and refuses
to start without a valid local `.onion` identity. It will not advertise
`127.0.0.1`, a public IP, or a Tor SOCKS port as its peer address. If Tor is not
available at `127.0.0.1:9050`, startup stops with an installation message.

## Remove stale peer entries

A previous clearnet or proxy configuration can keep the node from syncing.
Remove old entries from the configured static and trusted peer lists, especially
entries such as `127.0.0.1:9050`, public IP addresses, or non-onion hostnames:

```sh
rm -f ~/.tkmchain/static-nodes.json ~/.tkmchain/trusted-nodes.json
```

If peers are configured in `config.toml`, remove the corresponding `P2P.StaticNodes`
or `P2P.TrustedNodes` entries there instead. Every bootstrap and static URL must
use an `.onion` hostname and the P2P port (normally `3000`), never the SOCKS
port `9050`. A stale `127.0.0.1:9050` entry can cause `block body download
canceled`, synchronization retries, and challenge timeouts.
