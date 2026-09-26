# TKMChain Whitepaper

**Version:** 2.0 · **Status:** implementation-aligned draft · **Updated:** September 2026

## Abstract

TKMChain is an EVM-compatible, CPU-mineable blockchain for private value transfer,
private communications, and deterministic applications. It combines RandomX
proof-of-work, Rotating King operations, post-quantum account signatures, the
Shield3 private transaction system, TKM Phone, EmailVM, and Tor-only network
profiles.

The central design choice is that financial privacy is a normal protocol
property. A payment should not publish the amount, recipient, sender account, or
personal relationship to every observer merely because settlement uses a public
ledger. Shield3 keeps the data required for consensus while encrypting the
payment details and proving validity without revealing them. Authorized viewing
keys can disclose selected information when the owner chooses.

TKMChain does not promise that a bank, exchange, wallet host, relay, browser, or
operating system can never learn anything. A bank can see what happens inside
its own account system, and an endpoint can observe requests it serves. TKMChain
reduces the information published by the ledger and provides Tor transport to
reduce network-origin exposure. Users still need a trusted wallet, secure
endpoints, and sound operational practices.

## 1. Why privacy is a protocol feature

Public ledgers are excellent for independent verification, but unrestricted
transparency creates permanent financial surveillance. A public observer can
link addresses, amounts, timestamps, counterparties, payroll, donations, and
business relationships. The data can be copied forever and correlated later.

TKMChain separates **settlement validity** from **public disclosure**:

- Every node verifies that a payment is authorized, balanced, non-replayed, and
  correctly included in a block.
- The chain stores commitments, nullifiers, encrypted envelopes, and proofs
  rather than publishing the payment's plaintext amount and recipient.
- The sender can disclose a payment to a bank, auditor, tax authority, or
  counterparty using a scoped viewing or disclosure key.
- A recipient can prove receipt without giving the world a complete account
  history.

This is useful even when a payment touches a bank. A bank may know the fiat
transfer or deposit it processed, but unrelated chain observers do not need to
learn the customer's private TKM amount, recipient, or full wallet history.
The bank receives only the disclosure required for its compliance or accounting
obligation. The bank's own records and legal duties remain outside the chain's
cryptographic privacy boundary.

## 2. Network and execution model

`gtkm` is a Go Ethereum-derived execution client. It retains the EVM account,
nonce, receipt, log, ABI, JSON-RPC, and storage model while adding TKMChain
consensus and privacy services.

The network targets 120-second blocks. RandomX proof-of-work favors general
purpose CPUs. Rotating Kings provide scheduled operational responsibility and
receive a protocol-defined reward share. TKM Phone and EmailVM use daemon-owned
state and authenticated RPC plans; private keys and plaintext content remain in
wallets.

The mainnet chain ID is **8979**. The permanent finality checkpoint is block
**41913**, whose canonical hash is:

```text
0x0be22afecd78fcfb7a9b0b3a570bb5be9e9d0c13486ea5d187c7336b28edcd74
```

Nodes reject a competing chain that replaces this checkpoint. Administrative
rewinds below it are also rejected once the checkpoint is present locally.

## 3. RandomX proof-of-work and rewards

RandomX seals each block using the height-selected seed hash. The node exposes
work through `miner_getWork` and the TKM-specific RandomX methods. Mining pools
must verify submitted work with the same seed, seal hash, nonce encoding, and
network chain rules used by `gtkm`.

The target block time is 120 seconds. Difficulty adjusts from observed block
intervals with bounded changes and emergency handling for prolonged gaps. A
network deployment must use one matching consensus implementation; a miner or
pool cannot substitute a different RandomX calculation.

The current reward model divides block subsidy and fees among:

| Recipient | Share | Role |
| --- | ---: | --- |
| Main King | 10% | protocol stewardship and checkpoint operations |
| Rotating King | 40% | scheduled network operations |
| Miner | 50% | proof-of-work security |

Exact issuance, halving, fallback, and registration parameters are consensus
configuration and must be kept identical across nodes.

## 4. Antartical activation

Antartical activates on **1 October 2026 at 00:00 UTC** (`1790812800`). It is a
scheduled consensus transition, not a client-only feature flag. Upgraded nodes
are required before activation because the Shield3 verifier, mandatory stamps,
post-quantum account rules, sponsorship paths, and private communication
permissions change together.

Historical blocks retain their historical rules. Nodes without the Antartical
verifier reject new Shield3 envelopes instead of silently interpreting them as
an older transaction format.

## 5. Shield3 private transactions

Shield3 is a canonical `TKMSHIELD3` envelope carried by a chain-bound ML-DSA-87
post-quantum transaction. The envelope contains encrypted note data and a
proof-bound intent. The native verifier checks:

- the ML-DSA-87 transaction signature and chain binding;
- stamped account and recipient authorization;
- note and Merkle-root membership;
- distinct, nonzero nullifiers;
- output commitment correctness and value balance;
- proof program, domain separation, version, and size limits;
- replay, time, and activation rules.

A Shield3 send supports one to four private inputs owned by the same hidden
owner. Multiple inputs allow a wallet to consolidate notes without exposing
which historical notes were selected. Each output uses a fresh one-time
recipient key and encrypted payload. The public chain sees only data required by
consensus, including the transaction hash, block position, commitments,
nullifiers, gas fields, and any public deposit or withdrawal value.

The combined value covered by a Shield3 operation is limited to **5,000,000
TKM**. The proof and native relation also enforce bounded input count, bounded
trace size, bounded envelope size, and fixed field encodings. These limits reduce
resource-exhaustion risk and make consensus behavior reproducible.

### 5.1 What is private

For a normal Shield3 private send, the public ledger does not reveal the
plaintext amount, recipient payment code, note owner, viewing key, or complete
wallet history. A commitment hides the note contents while allowing a node to
verify the proof. A nullifier prevents double spending without identifying the
spent note to ordinary observers.

Shield3 does not make every field disappear. Transaction hashes, block timing,
gas, commitments, nullifiers, proof sizes, and relay behavior remain observable.
Public deposits and transparent withdrawals necessarily reveal their public
value and destination. A direct send can expose the payer's account; a shared
relay can hide that account from the chain but introduces an operator that may
observe timing or packets.

### 5.2 Viewing and disclosure keys

Viewing permissions are explicit:

| Key | Can disclose | Cannot do |
| --- | --- | --- |
| Incoming | incoming notes and received totals | spend or identify later spends |
| Full viewing | incoming and authorized outgoing history | spend |
| Payment disclosure | one selected payment and amount | scan the wallet or spend |
| Stamp disclosure | the stamped name/country record | reveal payment notes or spend |

A disclosure is a deliberate capability grant. Users should share the smallest
scope that meets the recipient's need. A viewing key is sensitive information,
even though it is not a spending key.

### 5.3 Stamps, registration, and sponsorship

After Antartical, a new private account begins by creating a stamp containing a
user-selected name and country label. The stamp is cryptographically bound to
the account and is visible only through the stamp permission. Shield3 sends and
value recipients must satisfy the registered-stamp rule; attempts to bypass it
are invalid consensus transactions.

A sponsored registration lets an authorized sponsor pay the registration cost
without receiving the user's spending seed. The sponsor signs an offer and the
user authorizes the exact action. Sponsorship changes who pays the fee; it does
not grant the sponsor spending authority or viewing access.

### 5.4 Relay and network privacy

Shield3 supports user-selected relays, Dandelion-style stem/fluff
propagation, bounded delays, and fixed relay packet handling. Relays can hide
the payer account from the on-chain transaction while preserving an exact,
idempotent request so a timeout cannot create a second spend.

Tor onion-only mode routes configured peer traffic and remote relay HTTP through
an explicit SOCKS5 proxy. It disables clearnet discovery, NAT advertising, and
clearnet fallback. The node advertises its `.onion` hostname and refuses to
start rather than advertising `127.0.0.1` when onion-only configuration is
incomplete. Local loopback RPC and prover calls remain local by design.

Tor reduces direct IP exposure; it does not erase timing, endpoint, relay, or
operating-system metadata. Users who require network anonymity must run Tor,
configure an onion service, use onion peers, and avoid clearnet RPC fallbacks.
See `docs/TOR_INSTALLATION.md` and `docs/PRIVACY_MODE.md`.

## 6. TKM Phone

TKM Phone is a daemon-backed phone identity and communications service. Its
canonical state includes:

- Main King-issued number buckets and operator approvals;
- ownership transfers and device-key registration;
- encrypted messages with expiry and recovery controls;
- encrypted WebRTC call signaling, contacts, blocking, and notifications;
- ownership proofs and propagation records.

Phone numbers are issued in bounded batches. Operators purchase buckets through
canonical payments, and Main King approval remains on a private daemon rather
than a hosted website. Device keys authorize use without transferring account
ownership. Messages and call signaling are encrypted payloads; the chain does
not store message plaintext or audio. WebRTC media is an endpoint-to-endpoint
concern and should use the application's authenticated signaling and privacy
configuration.

Phone privacy protects content and authorization. It cannot make a compromised
phone, browser, notification service, or recipient device trustworthy.

## 7. EmailVM and private messages

EmailVM provides canonical domain, mailbox, key publication, and encrypted mail
actions through the `tkmdomain` and `emailvm` namespaces. Domain names and
mailbox ownership are registered by deterministic hashes and canonical block
order. Payment plans may be split into multiple Shield3 withdrawals when a
single proof value limit would otherwise be exceeded.

Mailbox encryption keys are published as X25519 public keys. Mail content is
encrypted by the sender for the recipient key before it reaches the daemon.
Wallets keep PQ seeds, passphrases, viewing keys, witnesses, and proofs locally.
The daemon returns an action plan; the wallet builds, proves, signs, and submits
the final transaction.

The public chain can verify that a mailbox action was authorized and paid. It
does not need the mail body, private key, or plaintext attachment. A mailbox
provider or recipient endpoint can still observe local metadata such as login,
connection time, or the fact that it received a message.

## 8. EVM and TVM privacy boundary

Transparent EVM and TVM execution is retained only before Antartical for
compatibility and migration. At the Antartical timestamp (`1790812800`), the
consensus and txpool paths reject transparent EVM transfers, contract calls,
deployments, and TVM calls. The only user transaction envelopes admitted after
that boundary are the consensus-verified Shield3/Shield4 private envelopes.
Synthetic block rewards remain protocol-generated. Stamp registration and
private TVM actions must be carried inside one of those shielded envelopes;
standalone protocol envelopes are rejected.

This boundary is intentionally fail-closed. Shield3/Shield4 prove private note
ownership and value conservation. The bounded `TKMPRIVATEVM1` relation remains
available only as shielded payload logic and is not a standalone transaction
type. It does not claim arbitrary zkEVM
compatibility: public EVM bytecode, logs, and unrestricted host calls remain
disabled. Encrypted calldata is never treated as a substitute for a state
transition proof.

## 9. Security boundaries

TKMChain's protections are layered:

1. RandomX proof-of-work secures block production.
2. ML-DSA-87 protects account authorization against currently known classical
   and quantum attacks at the chosen security level.
3. Shield3 commitments, nullifiers, authenticated encryption, and proofs hide
   payment details while preserving double-spend prevention.
4. Stamps and scoped disclosure keys reduce unauthorized account use and
   over-sharing.
5. Tor and onion-only operation reduce direct network-origin exposure.
6. Daemon-owned Phone and EmailVM state prevents websites from becoming the
   authority for ownership or approval.
7. Checkpoint finality prevents rollback through block 41913.

No cryptographic design can promise that nobody will ever compromise an
endpoint, steal a key, correlate metadata, or discover a future cryptographic
weakness. The correct security claim is narrower: without the relevant spending,
viewing, or disclosure secret, a normal chain observer cannot derive the private
Shield3 amount and recipient from the consensus data alone.

## 10. Operating guidance

For a private node, install and verify Tor before starting `gtkm`, configure a
hidden service for the P2P listener, use onion bootnodes, and set:

```sh
./build/bin/gtkm \
  --privacy.onion-only \
  --p2p.tor-socks5=socks5://127.0.0.1:9050 \
  --p2p.onion-hostname=<this-node>.onion \
  --http --http.addr=127.0.0.1 --http.port=8545 \
  --http.api=eth,net,web3,tkm,tkmprivacy,tkmphone,tkmdomain,emailvm
```

Keep wallet, prover, relay, and node RPC on loopback or a protected onion
endpoint. Do not expose password-capable APIs to the public internet. A desktop
wallet uses its local node. The Android package keeps the node alive in the
background, but a production Android build must bundle and start a Tor runtime
before enforcing onion-only mode; an external Android Tor service is otherwise
required.

## 11. Roadmap and verification

Before a production Antartical activation, operators must deploy matching
binaries, proving keys, Tor configuration, and wallet versions. Verification
includes RandomX vectors, Shield3 native and wallet vectors, stamp and
sponsorship tests, relay idempotency and restart tests, Phone ownership and
message tests, EmailVM registry tests, checkpoint tests, and cross-platform
release builds.

The project should publish implementation notes and incident reports when
behavior changes. Privacy claims should always identify what is hidden, what is
public for consensus, and what metadata remains outside the cryptographic
boundary.

## Conclusion

TKMChain uses privacy to make ordinary digital money safer to use. Shield3
prevents the ledger from becoming a permanent public history of every amount
and relationship, while scoped disclosures preserve auditability when a user
chooses it. TKM Phone and EmailVM extend the same principle to communications.
Tor reduces network-origin exposure, RandomX keeps block production accessible,
and checkpoint finality protects the history that users rely on.

The result is not a promise of magic anonymity. It is a system where validity is
public, private details are encrypted and proof-checked, disclosure is selective,
and users can choose the trust and network exposure appropriate to each payment.
