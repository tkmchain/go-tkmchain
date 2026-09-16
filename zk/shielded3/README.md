# Shield3 consensus, encryption and wallets

Shield3 activates with Antartical, scheduled on mainnet for **2026-10-01
00:00:00 UTC**. Before activation, historical Shield2 consensus and gas pricing
remain in effect. Nodes built without `shield3` and CGO reject Shield3 proofs;
they do not fall back to a remote verifier or a legacy proof system.

## Transaction and proof rules

`TKMSHIELD3` identifies the canonical RLP envelope. Transactions require the
ML-DSA-87 PQ outer transaction type, the configured chain ID and shielded-pool
recipient. A pinned Triton VM 8.0.0 program proves ownership, a depth-32 ordered
Merkle path, nullification and exact unsigned 256-bit conservation. Its native
STARK verifier is linked into the node. Proof-supplied programs, claims or
security parameters cannot replace the locally fixed relation.

Every transaction creates four commitments and three equally padded encrypted
records for each output: incoming notes, outgoing history and stamps. The last
slot is change and must belong to the proved input owner. The proof limits the
sum of the first three outputs and any public withdrawal to **5,000,000 TKM**.
Gas sponsorship is separately bounded by the transaction's maximum gas cost
and excluded from that send limit. Full-width input balances and change remain
possible; the bound is enforced inside the private relation, not inferred from
encrypted amounts. Public deposits also have the 5,000,000 TKM limit and require
a funded proof plus the normal execution transfer into the pool.

All native Tip5 digests are five canonical Goldilocks words, encoded as 40
little-endian bytes. Shield3 uses separate state domains and stores full digests
in two storage words; it never truncates them into the legacy BN254 tree.
Append operations retain known roots; path queries construct witnesses against
the current canonical tree. Nullifiers and commitments cannot be reused.
Invalid proofs leave the tree, balances and nullifiers unchanged.

The public claim has **72 words**: chain (2), asset (2), public value (8),
SHA-512 transaction intent (16 u32 words), anchor (5), nullifier (5), four
outputs (20), deposit mode (1), gas sponsorship (8 u32 words), and stamp-registry root (5). The 95 secret
words contain spending secret (5), input randomness (5), input value (8), index
(1), four owner/randomness/value openings (18 each), and four private stamp indices. The fixed path section adds 32
five-word note digests and four depth-32 stamp membership paths. Hash domains are owner 3001, commitment 3002 and nullifier
3003; stamped-owner leaves use 3004. The nullifier hashes chain, asset, owner digest, private randomness and
value, so viewing-key holders can determine spent status without learning the
spending-secret preimage.

Proof encoding is canonical and bounded to 8 MiB, with a maximum padded trace
of 32,768 rows. The verifier bounds hostile trace exponents before any shift.
Activated gas pricing charges 3,000,000 verifier gas plus one gas per proof
byte; other envelope data retains ordinary calldata pricing. Antartical enables
the existing 8 MiB encoded block cap. The miner reserves 64 KiB for block
overhead and reward records; transaction admission uses that reduced byte
budget. Block responses also respect the peer packet bound. Public node RPC
defaults to a bounded 20 MiB request limit for hex-encoded proofs, configurable
with `Node.HTTPBodyLimit`. The transaction
pool verifies native proofs after cheaper encoding, signature and gas checks.
Consensus verifies independently before state changes.

## Encryption and key separation

Suite 1 uses Go's FIPS 203 **ML-KEM-1024**, HKDF-SHA-512 and
**XChaCha20-Poly1305**. Each encapsulation and 24-byte nonce is fresh. Notes,
zero-value decoys and stamps are all padded to the same 5,785-byte ciphertext:
79-byte authenticated header, 1,568-byte KEM ciphertext, 24-byte nonce and
4,114-byte authenticated encrypted payload. The header binds version, suite,
role, chain ID and the full hiding commitment context. Decapsulation must be
followed by successful AEAD authentication.

Incoming, outgoing and stamp seeds are derived independently from the wallet
seed, scoped to chain and purpose. A receiving code authenticates its owner,
ML-KEM public keys and encrypted stamp with ML-DSA-87. Public keys allow
encapsulation, never note or stamp decryption. A viewing backup contains
incoming and outgoing private KEM seeds; scans disclose incoming/spent notes
and outgoing recipients/amounts without the spending secret. A separate stamp
seed opens the original private name and country. Random hiding commitments
prevent guessing stamps from hashes of short names or countries.

Encryption hides private note values and recipient records. The existing PQ
outer transaction still exposes its signer account, timing, gas and fees;
public funding deposits and withdrawals expose their amounts. This integration
does not establish sender anonymity. Triton VM defaults target 160 bits of
conjectured classical IOP soundness; that figure is not a claim of 160-bit
quantum security. No component guarantees permanent secrecy after private-key
exposure or absolute resistance to future attacks.

## Wallet and migration

At Antartical, **consensus requires a confirmed, immutable on-chain stamp for
all user-transaction senders**, including zero-public-value private spends and
legacy self-migrations. Public withdrawals require a stamped recipient. Every
Shield3 output owner is proved to belong to a separate depth-32 stamp registry;
indices and recipient ownership stay inside the private witness. The statement
binds a known registry root. Wallet-file stamps alone do not authorize sending.

The sole unstamped-sender exception is `TKMSTAMP1`: a zero-value PQ registration
to the reserved code-free pool. It authenticates the encrypted stamp under the
beneficiary's ML-DSA key and proves knowledge of the owner preimage with a fixed
native STARK, bound to the complete registration intent. A copied proof cannot
register another owner or account. Registrations cannot replace an existing
address or owner. They count against block gas/byte limits and pay ordinary gas
fees, paid either by the registering address or by an already stamped sponsor.
A sponsored envelope adds the beneficiary ML-DSA public key, expiry (at most
one hour from block time), and beneficiary signature. Both that signature and
the owner STARK bind the complete unsigned fee-paying transaction: chain,
sponsor public key, sponsor nonce, fee caps, gas, target, zero value, original
encrypted stamp and beneficiary key/expiry. The sponsor signs the completed
PQ transaction. Only the sponsor balance and nonce change; no beneficiary
balance or nonce is needed. Expired, substituted, reused-owner and already
registered-beneficiary transactions are invalid. Self-funded encoding is
unchanged because sponsorship fields are optional trailing RLP fields. Protocol block
rewards and pre-execution system calls remain distinct from user transactions.

Validators reject invalid registration proofs and unstamped transactions as
consensus errors, making blocks containing them invalid. A node with removed
checks follows incompatible rules; it cannot make enforcing nodes accept these
blocks. Name and country are self-declared encrypted labels, not verified legal
identity. Public stamp registrations reveal an address, owner digest and hiding
commitment, but never the private label plaintext.

The shared Android/Windows wallet requires name/country stamping before new
PQ account creation at Antartical. Existing accounts can add their original
stamp once. Keyfile export/import and password changes retain the authenticated,
encrypted stamp; keep an encrypted backup because recovery words alone do not
retain the original randomly blinded stamp record.

Both Send and Receive begin with **First: stamp your address**. Create or reuse
the original encrypted stamp, register it, then check the displayed transaction
hash for confirmation. Sending, private receiving-code sharing, funding and
migration controls remain disabled until the on-chain stamp is confirmed.
Registration retries retain the same signed bytes and request ID after a timeout
or GUI restart. Keep an encrypted backup before registering: a newly blinded
stamp from recovery words does not replace the original registered commitment.

Private identity, viewing-key scans, proof construction and submission run only
through authenticated loopback `/shield3/` endpoints. Public `tkmprivacy` RPC
provides activation status, bounded canonical encrypted-output batches, current
paths and canonical/pending nullifier status. It does not receive spending
secrets or viewing seeds. Viewing scans check the starting head's canonical hash
before returning balances and exclude pending or already spent notes.

The wallet builds one input per proof and selects a confirmed note covering the
amount and fees. It reports when balances are split across smaller notes that
cannot cover a single requested send. Funding creates a suitable note. Private
sends, funding and legacy migrations display their transaction hash as
unconfirmed until a canonical receipt appears. Shield3 signed transaction bytes
and request digests are saved locally before broadcast; retrying a stable
request ID retains the exact transaction after an RPC timeout or GUI restart.

Legacy Shield2 funds remain accessible through a restricted bridge: one entire
V2 note may be withdrawn to its own PQ signer with no sponsorship or private
change. All four zero-output openings are checked against their commitments.
The inherited V2 Groth16 verifier is used only for this migration. The wallet's
**Migrate one Shield2 note** action reveals that withdrawal's value; wait for
confirmation before migrating another note or using **Shield public funds**.
Private post-fork V2 transfers and V2 deposits are rejected.

## Builds and verification

Rust 1.89 and CGO are required. `make gtkm`, `make gtkm-gui`,
`make shielded-payout-prover` and production targets build and embed the static
library. Windows GUI builds add the `x86_64-pc-windows-gnu` Rust target; Android
adds `aarch64-linux-android` and uses the NDK linker. `release.yml` installs those
toolchains and packages the embedded verifier with each supported binary.

```sh
./scripts/shield3-build.sh
TKM_SHIELD3_TESTDATA=/tmp/tkm-shield3-vector cargo test --release --locked \
  --manifest-path zk/shielded3/stark/Cargo.toml -- --test-threads=1
TKM_SHIELD3_STARK_BIN="$PWD/zk/shielded3/stark/target/release/tkm-shield3-stark" \
  TKM_SHIELD3_TESTDATA=/tmp/tkm-shield3-vector \
  go test -tags shield3 ./crypto/pqcrypto ./zk/shielded3 ./internal/shield3wallet
```

Rust tests generate genuine randomized proofs, check private send boundaries,
full-width conservation/overflow, change ownership, deposits and sponsorship,
and reject modified witnesses, proofs and public-input replays. Go tests execute
wallet deposits/spends through consensus and normal transaction execution,
check viewing-only histories, pending/reorg handling, failed-proof atomicity,
stamp backup authentication and private-endpoint access boundaries. Fixtures
contain deterministic test secrets, never wallet secrets. The cryptography CI
runs native tests, interoperability, race checks and lint with the build tag.

### Sponsored stamp registration

In desktop and Android Send/Receive, open **Register with a sponsor**. The new
wallet creates its private stamp and shares **My stamp sponsorship request**.
This is a signed public receiving code, usable for registration only until its
stamp is confirmed. A stamped sponsor creates a one-hour fee offer for that
request. The new wallet imports the offer and authorizes its original stamp
with an owner STARK and ML-DSA signature; it returns the authorized JSON packet.
The sponsor imports it, reviews the verified beneficiary and maximum fee, then
explicitly confirms and submits. Packet metadata is derived from the verified
transaction, not trusted from imported JSON fields. No step transfers funds to
the beneficiary. Activity shows the hash; the beneficiary checks its canonical
stamp status before sending or sharing a payment receiving code. Keep the
original encrypted keyfile backup.

The fee offer uses the sponsor's pending nonce. A competing sponsor transaction
requires a new offer and fresh beneficiary authorization. Completed signed
submissions use durable stable request IDs and preserve exact raw bytes on
uncertain RPC responses. There is no automatic unlimited sponsor service or
free gas; sponsors explicitly approve each registration. These rules activate
at the existing Antartical timestamp, October 1, 2026 00:00 UTC.
