# TKMNet transport

TKMNet is the encrypted service transport used by TKMChain. It runs inside
`gtkm` as a `node.Lifecycle` service and is independent of consensus. Shield3
and Shield4 transaction formats, proof verification, stamps, nullifiers, and
hardfork rules are unchanged.

## Run the services from one gtkm command

`--tkmprover` now starts the payout and Shield3/Shield4 builder as an HTTP
service inside the `gtkm` process. It no longer starts a second prover
executable. The listener remains loopback-only by default at
`127.0.0.1:8787`; a local pool can use that URL and the bearer token stored in
`~/.tkmchain/tkmprover/config.json`.

`--tkmname.enable` enables both the TKMNet relay lifecycle and the username
directory RPC on the same node. `--tkmnet.enable` enables the relay when a node
is serving other TKMNet services without username lookup. Directory and transit
peer descriptors, the Tor SOCKS endpoint, and relay port are read from the
node's `[Tkmnet]` TOML configuration. This flag adds no consensus rule or fork.

Example for a node already configured with valid TKMNet peers:

```bash
./build/bin/gtkm \
  --tkmprover \
  --tkmname.enable \
  --http --http.addr=127.0.0.1 --http.api=eth,net,web3,tkmname,tkmprivacy \
  --privacy.onion-only --p2p.tor-socks5=socks5://127.0.0.1:9050
```

The pool remains a separate application because it owns the miner Stratum
listener and payout accounting. It does not need a separate prover process:
it submits payout requests to the daemon's embedded prover and resolves a
miner's checksummed `@username#checksum` login through `tkmname_resolve` on the
configured node RPC endpoint. The node must have `tkmname` in its HTTP API list
if the pool uses HTTP; IPC can be used for a local pool deployment. The miner's
registered username must point to a confirmed Shield3 payment code.

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

## Host a username directory operator

For the paid four-service deployment (two pinned directories plus two
independent transit relays), provider responsibilities, node configuration,
key-pin verification, readiness checks, and the current descriptor-tooling
limitation, see [Paid third-party TKMNet operators](TKMNET_THIRD_PARTY_OPERATORS.md).

A directory operator is a `gtkm` node whose signed descriptor is included in
`Tkmnet.DirectoryPeers` on participating nodes. It stores signed name bindings
in its node database and serves requests through the username handler.
Registrations are copied to every configured directory after the
username-network activation. This is application-level replication, not
consensus state.

Use a fixed local port so Tor can publish the service. Add this mapping to the
operator's Tor configuration and restart Tor:

```text
HiddenServiceDir /var/lib/tor/gtkm-tkmnet
HiddenServiceVersion 3
HiddenServicePort 39000 127.0.0.1:39000
```

Keep the hidden-service directory readable only by Tor. Read the `.onion`
hostname from its `hostname` file. Do not open port 39000 on the public
firewall; Tor forwards to the loopback-only `gtkm` listener.

Configure `[Tkmnet]` with the local Tor SOCKS proxy, relay port `39000`,
loopback listener `127.0.0.1:39000`, and the node's peer descriptors. Keep the
relay key at its default path under the node data directory. Run `gtkm` with
`--privacy.onion-only --tkmnet.listen=127.0.0.1:39000`. Antartical nodes enable
TKMNet automatically; `--tkmnet.enable` is useful before that activation.

With `--tkmnet.enable` or `--tkmname.enable`, `gtkm` provisions its local
relay automatically: it uses `127.0.0.1:39000`, stores its ML-KEM relay key
under the node data directory, and defaults Tor egress to
`socks5://127.0.0.1:9050` (or the proxy already selected for P2P). Transit and
directory descriptors already present in `[Tkmnet]` are automatically added
to the relay forwarding set, so they do not need to be duplicated under
`RelayPeers`.

When the node has an onion hostname from `--p2p.onion-hostname`, `[Tkmnet]`,
or `tkmnet/onion.hostname` in its instance directory, startup also generates
and refreshes its own signed descriptor at `tkmnet/descriptor.json`. The
corresponding descriptor-signing seed is private (`0600`); the public
fingerprint is written to `tkmnet/signing-key.sha256`. `gtkm` also advertises
the signed descriptor over its `tkmnet/1` peer protocol. Receiving nodes check
the descriptor signature, relay-key-derived ID, onion hostname, and expiry
before caching or forwarding it.

Verified peer-gossiped descriptors are also added as candidate transit hops
automatically. They are deduplicated by relay ID/onion and rejected if a key
changes under an existing relay ID. Each node still needs two independent
transit peers to be connected and advertised before the username route can
become ready.

The daemon cannot safely manufacture remote transit or directory peers. Their
onion hostnames, signed descriptors, and directory signing-key pins must come
from real operators. A generated descriptor proves control of that node's
signing key; gossip proves the descriptor was not altered, but does not make
its owner an authorized directory operator. Directory use still requires
the operator's fingerprint to be pinned in `[Tkmnet.DirectoryPeers]`.
Without at least two transit
descriptors and two pinned directory descriptors, the local relay still
starts, but username registration and lookup report that the network is not
ready. No remote peers are guessed or accepted without signature validation.

The initial username network requires **two pinned directory operators**.
More can be added later, up to the 16-query privacy fanout. Lookups select
configured operators in randomized order; registrations replicate to every
configured directory. The three-hop route also requires two transit relay
descriptors distinct from directory descriptors. Peer gossip supplies relay
forwarding entries automatically; configured relay descriptors remain
supported for deployments that use static peers.

Descriptors contain the operator's onion host, ML-KEM relay public key,
ML-DSA signing public key, expiry, and ML-DSA signature. The `SigningKeyPin`
must be the SHA-256 fingerprint of that signing key and should be exchanged
over a trusted channel. Do not invent a descriptor or copy another operator's
pin: peers reject invalid descriptors and network readiness remains false.
There is no automatic protocol reward for directory service.

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

## Antartical requirement

TKMNet is part of the Antartical consensus schedule. Before Antartical it is
optional unless `--tkmnet.enable` is supplied. Once the canonical chain head
is at or past the Antartical timestamp, `gtkm` automatically enables and
registers the relay even when the flag is omitted. The relay is therefore a
required node service after the fork; a node that cannot construct its
loopback-only relay fails during startup instead of silently running without
the required protection.

## Lifecycle behavior

When enabled, `gtkm` constructs the relay before starting the node and
registers it with `Node.RegisterLifecycle`. The node starts the relay after
its RPC and peer-to-peer endpoints are ready. On shutdown, the lifecycle
cancels its context, closes the local listener and active connections, waits
for relay goroutines to exit, and then lets the node stop its remaining
services. The relay listener is always loopback-only; Tor must publish it as
an onion service.
