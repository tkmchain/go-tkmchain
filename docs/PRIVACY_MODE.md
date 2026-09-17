# Privacy mode deployment

The privacy controls cover the user-origin paths without changing Shield3 consensus.

## Node

Start Tor locally and run the daemon with:

```text
--p2p.tor-socks5=socks5://127.0.0.1:9050 --privacy.strict
```

Strict mode routes outbound P2P dials through SOCKS5, disables UDP discovery and NAT port mapping, rejects non-loopback HTTP/WS/auth listeners, and exposes only `eth`, `net`, `web3`, `miner`, `randomx`, and `tkmprivacy` on public RPC. Administrative, debug, tracing, and account-management APIs remain on IPC/authenticated local interfaces.

Static peers are required when discovery is disabled. The P2P listener itself is still a TCP listener; use a Tor onion service or a firewall/VPN to accept inbound private peers.

## Wallet and GUI

The GUI inherits `--p2p.tor-socks5` for relay offers and submissions. The relay client rejects `.onion` endpoints without an explicit SOCKS5 proxy, disables ambient proxy environment variables, and fails rather than redirecting to another host.

## Pool and prover

Set these fields in pool and prover configuration:

```json
{
  "torSocks5Proxy": "socks5://127.0.0.1:9050",
  "privacyStrict": true
}
```

Remote daemon/prover HTTP calls use Tor and never fall back to a direct connection. Localhost daemon/prover endpoints stay on the loopback interface because sending local traffic through an external Tor circuit is unnecessary and can fail. Strict mode requires HTTPS or `.onion` for remote endpoints.

## Propagation and public data

Shield3 transactions use the Antartical-gated Dandelion stem/fluff path and delayed batches. Public responses remove internal payout diagnostics and viewing keys. Canonical transaction hashes, commitments, nullifiers required by consensus, gas, block inclusion, and relay operators remain visible.

## Leak tests

The repository contains validation tests for SOCKS5 URL policy, strict RPC listener policy, fixed-size relay envelopes, and pool configuration. Live verification should include:

```text
curl --socks5-hostname 127.0.0.1:9050 https://check.torproject.org/api/ip
```

A privacy deployment must also firewall the public RPC ports and avoid logging request bodies, viewing keys, or bearer tokens.

## Onion-only networking

For deployments that must never connect to a clearnet peer, enable the fail-closed onion-only mode:

```bash
gtkm \
  --privacy.onion-only \
  --p2p.tor-socks5 socks5://127.0.0.1:9050 \
  --bootnodes 'enode://<peer-key>@<peer-id>.onion:3000'
```

Onion-only mode:

- routes every outbound P2P and configured remote HTTP request through the explicit Tor SOCKS5 proxy;
- rejects IP literals and non-`.onion` P2P peers, RPC endpoints, prover endpoints, and wallet relays;
- disables IPv4/IPv6 discovery, UDP discovery, NAT mapping, and clearnet fallback;
- binds the P2P listener to `127.0.0.1` so the listener is reachable only through a Tor onion service;
- keeps RPC services loopback-only and exposes the restricted public API set from strict mode.

Configure an onion service in Tor for the local P2P port, for example:

```
HiddenServiceDir /var/lib/tor/tkmchain/
HiddenServicePort 3000 127.0.0.1:3000
```

Use the generated `.onion` hostname in every static or bootstrap `enode://` URL. Discovery is intentionally disabled, so onion peers must be configured explicitly. Tor itself uses IP links between relays; onion-only mode removes your node's public IP exposure and prevents your software from dialing clearnet addresses.

The pool and payout prover expose the same `onionOnly` setting. Set `torSocks5Proxy` to `socks5://127.0.0.1:9050`, use a loopback `nodeRPC` for a local daemon or a `.onion` URL for a remote daemon, and publish a `.onion` `publicURL`/stratum address. Pool HTTP and stratum listeners are forced to `127.0.0.1`; expose them through Tor onion-service ports when miners or operators need remote access.
