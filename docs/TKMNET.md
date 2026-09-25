# TKMNet transport

TKMNet is the encrypted service transport used by TKMChain. It runs inside
`gtkm` as a `node.Lifecycle` service and is independent of consensus. Shield3
and Shield4 transaction formats, proof verification, stamps, nullifiers, and
hardfork rules are unchanged.

## Enable the relay

Run the node with a Tor onion service forwarding to the local listener:

```bash
./build/bin/gtkm \
  --privacy.onion-only \
  --p2p.tor-socks5=socks5://127.0.0.1:9050 \
  --p2p.onion-hostname=<node>.onion \
  --tkmnet.enable \
  --tkmnet.listen=127.0.0.1:39000 \
  --tkmnet.hop=0
```

The relay key is stored at:

```text
~/.tkmchain/gtkm/tkmnet/relay-key
```

It is created with mode `0600` and must be backed up with the node's private
state. The relay's ML-KEM public key and onion hostname belong in a signed
directory descriptor; the private key is never sent over the network.

## Services

The packet format has separate authenticated service identifiers:

| ID | Service |
| --- | --- |
| 1 | Shielded transaction transport |
| 2 | Blockchain peer transport |
| 3 | EmailVM payloads |
| 4 | TKM Phone payloads |

Packets are fixed size, use three relay layers, and use ML-KEM-1024 with
XChaCha20-Poly1305. Relay sequence numbers are bounded by a replay cache.
The SOCKS5 dialer accepts only `.onion` destinations and never falls back to
direct networking. Large Shield3/Shield4 transactions are split into fixed
size chunks before transport and reassembled with a bounded transfer limit.

## Consensus boundary

TKMNet only transports opaque bytes. A node still validates every transaction
with the normal Shield3/Shield4 consensus path. TKMNet availability cannot
make a block valid or invalid, and disabling the relay does not change chain
state.

The package is published at
[`github.com/tkmchain/tkmnet`](https://github.com/tkmchain/tkmnet). The
release build currently compiles the copy in the main module, so the node and
the transport are versioned together. A submodule pointer can be introduced
after the first pinned transport release without changing the lifecycle API.

## Lifecycle behavior

When enabled, `gtkm` constructs the relay before starting the node and
registers it with `Node.RegisterLifecycle`. The node starts the relay after
its RPC and peer-to-peer endpoints are ready. On shutdown, the lifecycle
cancels its context, closes the local listener and active connections, waits
for relay goroutines to exit, and then lets the node stop its remaining
services. The relay listener is always loopback-only; Tor must publish it as
an onion service.
