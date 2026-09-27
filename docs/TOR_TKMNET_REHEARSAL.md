# Tor/TKMNet production rehearsal

`scripts/tor-tkmnet-rehearsal.sh` is the repeatable smoke test for a running
TKMChain node and its onion-published mining pool. It does not submit a
transaction, mining share, or consensus-changing RPC call.

The rehearsal checks, in order:

1. Tor's local unauthenticated SOCKS5 listener is available.
2. The node's IPC `admin.nodeInfo` enode advertises a v3 `.onion` hostname and
   does not advertise a loopback address. `net_peerCount` confirms onion peer
   connectivity.
3. EVM RPC (`eth_chainId`, `eth_blockNumber`), the RandomX work package, the
   public RPC module set, and `tkmprotocol_antarticalStatus` are available.
4. The pool status page can be read through Tor and publishes an onion Stratum
   endpoint.
5. A real Stratum `mining.subscribe` and `mining.authorize` exchange succeeds
   through the Tor SOCKS5 proxy. No share is submitted.
6. The fail-closed onion dialer, node privacy policy, TKMNet fixed-size relay,
   TKMNet lifecycle, and Antartical activation tests pass.

## Run it against production-like services

Run this on the node that has Tor, `gtkm`, and the pool's onion service:

```bash
cd ~/go-tkmchain
./scripts/tor-tkmnet-rehearsal.sh
```

The defaults are the local daemon RPC (`127.0.0.1:8545`), local IPC
(`~/.tkmchain/gtkm.ipc`), Tor SOCKS5 (`127.0.0.1:9050`), and the configured
TKM pool onion (`4aof7abdduh4vftejgdpdfqeosvxxco3xmpu4uqypnpdbi7wjuzfqhqd.onion`).
Override them without changing the script:

```bash
TKM_RPC_URL=http://127.0.0.1:9545 \
TKM_IPC_PATH=/srv/tkm/gtkm.ipc \
GTKM_BIN=/srv/tkm/gtkm \
TKM_POOL_ONION=your-56-character-v3-hostname.onion \
./scripts/tor-tkmnet-rehearsal.sh
```

The script refuses a non-loopback RPC URL and a non-`.onion` pool hostname.
Set `TKM_MIN_PEERS=0` only for an intentionally isolated single-node test;
production runs should keep the default of one connected onion peer.

## Fork behavior

The Antartical status is derived from the canonical head timestamp. Mainnet is
inactive before `1790812800` (2026-10-01 00:00:00 UTC) and active at or after
that timestamp. Egypt activates at genesis. Before activation, TKMNet remains
an optional lifecycle service; once the canonical head reaches Antartical,
`gtkm` enables the relay automatically. This transport activation does not
change Shield3/Shield4 transaction consensus before the fork.

The unit tests use a local fake SOCKS5 endpoint, so CI verifies hostname
routing and clearnet rejection without requiring a public Tor circuit. The
live script is the production check for the actual onion service, pool, and
node.
