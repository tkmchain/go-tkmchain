# tkmnet

`tkmnet` is TKMChain's encrypted service transport. It is deliberately kept
outside consensus: Shield3 and Shield4 transaction encoding, proof checking,
nullifier handling, and stamp rules are unchanged.

## Protocol properties

- Fixed-size v1 packets (`PacketSize`) and extended v2 packets
  (`ExtendedPacketSize`) for payloads up to 65,535 bytes. The version is
  visible in the fixed header; payload length and content remain encrypted.
- Three relay layers using FIPS 203 ML-KEM-1024 and XChaCha20-Poly1305.
- A relay can decrypt only its own route layer and learns only the next relay
  identifier. The final relay receives the service payload.
- Payloads are padded and authenticated with a service-specific context.
- Circuit/sequence replay protection is bounded by `ReplayCache`.
- `SOCKS5Dialer` accepts only `.onion` destinations and never falls back to a
  direct connection.

`tkmnet` does not make public blockchain data private. Shield3/Shield4 provide
ledger privacy; tkmnet protects transport origin and service payloads.

## Shield3 username lookup

`ServiceUsername`, the fixed-width username query encoding, and
`PlanPrivateUsernameLookup` define the transport-level lookup plan. Lookup
fanout adapts to the configured directory set, up to 16 distinct, locally
pinned onion operators. The network requires at least two directory operators;
two can split the target and one cover alias, while larger sets provide more
query separation. Each alias is assigned to a different
operator and sent over its own TKMNet circuit. The plan rejects missing,
expired, duplicate, and unpinned peers instead of falling back to local-only
lookup.

This is a non-collusion privacy design, not cryptographic PIR: operators that
collude can compare their queries, and a malicious client can still query a
chosen name. Signed bindings let the wallet authenticate returned mappings,
but they do not prove that separate keys belong to separate operators.

The query planner, fixed-width request, 3-hop request builder, authenticated
encrypted response, cover selection, directory handler, and wallet RPC route
are implemented. Activated lookups use one cover source and query as many
distinct pinned directory operators as are configured, up to 16. With two
operators, the target and one cover go to separate operators. This depends on non-colluding directory
operators and is not PIR.

Configure `Tkmnet.SOCKS5Proxy`, `Tkmnet.RelayPort`, `Tkmnet.TransitPeers`, and
`Tkmnet.DirectoryPeers`; transit nodes also need corresponding signed onion
descriptors in `Tkmnet.RelayPeers`. TKMNet rejects unpinned or duplicate
directory keys and uses no direct-network fallback.

The username DB is persisted locally and reconciles each binding against the
canonical block hash/height to prune reorged records. After activation,
registration publishes its signed binding and full Shield3 payment code to all
configured directory operators over Tor. Exact retries are idempotent, and
the RPC reports success only after every operator acknowledges. At first
network use, a node republishes its saved live records and probes every
configured operator; `networkReady` is true only after the pass succeeds.
Records are still off-consensus metadata. Replication is not a consensus
quorum, and concurrent conflicting claims can be observed in different
arrival orders. A username must not be treated as a consensus identity.

## Integration boundary

The package implements `node.Lifecycle` and is registered by `gtkm` when
`--tkmnet.enable` is set. It listens only on the local side of a Tor onion
service, uses a separate `~/.tkmchain/tkmnet` state directory, and exposes no
clearnet listener. The node owns startup and shutdown ordering: stopping the
node cancels the relay context, closes active connections, and waits for all
relay goroutines to exit.

The package is published at
[`github.com/tkmchain/tkmnet`](https://github.com/tkmchain/tkmnet). TKMChain
currently compiles the matching copy in its main module so the transport and
consensus code are released together.

TKMNet becomes a required node service at the Antartical hardfork. Before the
fork, operators may leave it disabled or enable it explicitly. Once the
canonical head reaches Antartical, `gtkm` enables the relay automatically and
fails startup if the loopback relay cannot be constructed.

## Production rehearsal

Run [`scripts/tor-tkmnet-rehearsal.sh`](../scripts/tor-tkmnet-rehearsal.sh)
on a node with Tor and the onion-published pool to verify the complete path:
onion enode advertisement, peer connectivity, EVM and RandomX RPCs, pool HTTP,
Stratum subscribe/authorize, TKMNet lifecycle, and the Antartical activation
gate. The command is read-only and refuses clearnet RPC or pool endpoints. See
[`docs/TOR_TKMNET_REHEARSAL.md`](../docs/TOR_TKMNET_REHEARSAL.md) for setup and
environment overrides.
