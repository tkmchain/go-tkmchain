# Shield3 usernames and private lookup

## Activation schedule

The mainnet username-network service gate is scheduled for **2026-10-04
22:00:00 UTC** (`usernameNetworkTime`). It is separate from Antartical and does
not change block or transaction validation. Egypt uses the same timestamp so
the network path can be rehearsed against a matching schedule.

## Handle and binding

A Shield3 username is a 3–32 character ASCII handle, entered as `@name`. The
canonical alphabet is lowercase letters, digits, `_`, and `-`; reserved names,
Unicode, bidi controls, and ambiguous normalization are rejected. The wallet
can display a short, chain-bound handle such as `@alice#4kn7gwa` (the suffix
detects typing errors; it is not an authentication mechanism).

The directory record is signed with the owner’s ML-DSA-87 key and binds the
chain ID, canonical name, Shield3 address, hash of the full Shield3 payment
code, monotonically increasing sequence number, and lease expiry. Each local
database entry is also anchored to the canonical block hash and height at which
the node accepted it. Reads reconcile those anchors against the current chain;
records anchored to an orphaned block are deleted. A resolver
must verify the signature and the full Shield3 payment code before returning
it. The protocol permits leases of up to one year; wallet registrations use 90
days. Updates require the same owner and a higher sequence; another owner
cannot take the name until the old lease and a 30-day reclaim period have both
elapsed. Names are not written to EVM state.

## Lookup path

Before the username-network activation, the wallet uses the node-local alias
database. After activation, `tkmname_resolve` sends fixed-width requests over
TKMNet and Tor. It obtains available cover aliases from one pinned directory,
then queries the target and as many covers as there are other configured
directory operators, up to the protocol maximum of 16. At least two directory
operators are required; additional operators can be added to increase cover
separation. Each query is assigned to a different pinned directory operator.
With two operators, lookup uses one cover alias. More operators improve
separation between the target and cover queries. The wallet checks the
returned owner signature, chain ID, lease, and Shield3 code.
If Tor, TKMNet, pins, or a directory response is unavailable, resolution
fails closed rather than falling back to local lookup.

## Wallet and external-node use

The interactive wallet exposes username registration/details and renewal, and
its Send flow accepts a checksummed `@name#checksum`. Registration and renewal
cost 0 TKM; a renewal keeps the same owner and payment code, increments the
sequence, and sets a new 90–365 day lease. The wallet shows the mapped
Shield3 address, expiry, remaining lease, sequence, and renewal cost.

The `tkmname` JSON-RPC namespace provides `tkmname_status`,
`tkmname_register`, `tkmname_resolve`, and `tkmname_lookupBatch`. It is enabled
in the default and strict-privacy HTTP API module lists so the hosted wallet
can use the node's `/rpc` endpoint. External wallets may use that configured
RPC endpoint without operating directory infrastructure themselves.

An Ethereum P2P peer connection alone does **not** transfer this off-consensus
directory or the private TKMNet operator configuration. A node using its own
RPC must have the Tor, transit-relay, and pinned-directory settings above;
connecting to ordinary chain peers does not satisfy `networkReady`. The
wallet fails with a clear status in that case. We do not gossip every signed
name-to-payment-code record to all chain peers because that would publish the
mapping and defeat the private lookup design. Peer-discovered trusted operator
bootstrap is a separate network feature; it must preserve operator pinning and
per-operator query separation before it can replace explicit configuration.

This is a non-collusion design, not cryptographic PIR. The cover source sees a
request for covers; each query directory sees its one alias; colluding
operators can correlate activity. TKMNet hides the source IP from a directory
when the client has a working Tor connection, but it does not hide the alias
from the assigned directory.

The network needs a SOCKS5 proxy, two distinct transit relays, and at least
two distinct pinned directory operators. The initial deployment uses two
directory operators; more can be added without a protocol change.
Registration replicates to every configured directory and reports success
only after each one acknowledges. The two transit relays remain
separate from the directory operators and are still required for the three-hop
TKMNet privacy route. Configure `SOCKS5Proxy`, `RelayPort`, `TransitPeers`, and
`DirectoryPeers` in the `Tkmnet` TOML section. The regular
`RelayPeers` list must allow the transit hops to forward to the selected
directory. Every signing-key pin must match the SHA-256 fingerprint of that
operator's ML-DSA key. For third-party hosting, paid service scope, onboarding
deliverables, the node settings, readiness checks, and the current descriptor
tooling limitation, see [Paid third-party TKMNet operators](TKMNET_THIRD_PARTY_OPERATORS.md).

## Abuse and security limits

- The directory accepts only signed, chain-bound bindings and rejects altered
  codes, expired leases, cross-chain replay, stale sequence numbers, reserved
  labels, and aliases with Unicode confusables.
- The local directory caps at 100,000 records; each identity may hold eight
  names. New claims require an 18-bit proof of work. Batch primitives require
  exactly 16 distinct names and fail without partial results.
- Names are off-consensus records replicated to a configured operator set;
  they are not a chain-wide consensus registry.
- Records persist in each accepting node's local database, anchored to its
  canonical block hash and height. Reorgs remove records anchored to orphaned
  blocks. Registration replication copies signed bindings to configured peers;
  it is not continuous database synchronization or consensus state.
- TKMNet lookup, encrypted fixed-size replies, adaptive cover selection, per-alias
  routing, incoming signed-registration validation, and outbound replication
  are implemented. Once active, registration publishes the complete signed
  binding, including the full Shield3 payment code, to every configured
  directory operator over Tor. The RPC reports success only after all of them acknowledge;
  a partial failure is reported so the owner can retry. Exact retries are
  idempotent. Before the first activated lookup, the node republishes saved
  live bindings and checks all configured directories. `networkReady` becomes
  true only after that synchronization pass succeeds.
- Directory records remain off-consensus metadata. Each operator anchors an
  accepted record to its own current canonical block and prunes it if that
  block becomes non-canonical. Replication is not a consensus transaction:
  concurrent conflicting claims can be accepted in different arrival orders,
  and the directory set is not a consensus quorum. Do not treat a
  username as a protocol-level unique identity or send without verifying the
  returned signed Shield3 code and recipient.
