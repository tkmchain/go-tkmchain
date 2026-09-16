# Shield3 ML-KEM encryption and native zk-STARK spending proofs

This directory contains a **development implementation** of a new private-spend
relation and a real Triton VM zk-STARK prover/verifier. Encryption lives in
`crypto/pqcrypto/shielded_v3.go` and uses Go's FIPS 203 ML-KEM-1024 implementation.
The proof verifier uses Triton VM 8.0.0, pinned with a Cargo lockfile. It performs
native STARK verification, with no pairing-based SNARK wrapper, trusted signer,
remote verification service, or mock acceptance path.

These components are not yet selected by `core` consensus, the transaction
pool, existing wallet account creation, RPC or payout tooling. Scheduling
Antartical does not by itself switch those paths to this implementation.

## Encryption

Suite 1 is ML-KEM-1024, HKDF-SHA-512 and XChaCha20-Poly1305. Every encryption
uses a fresh encapsulation and random 24-byte nonce. All plaintexts, including
empty decoys and stamps, are padded to the same size. The wire envelope is
5,785 bytes:

| Field | Bytes |
| --- | ---: |
| Magic `TKPQ` | 4 |
| Version 3, suite 1, purpose | 3 |
| Chain ID, big-endian | 8 |
| Hiding commitment | 64 |
| ML-KEM-1024 encapsulation ciphertext | 1,568 |
| Nonce | 24 |
| Encrypted length and 4,096-byte padded payload, plus authentication tag | 4,114 |

HKDF binds the recipient public key, header and KEM ciphertext. The AEAD
additionally authenticates the full public prefix, including the nonce.
Decapsulation's implicit rejection is followed by mandatory AEAD authentication.
Decryption requires the exact expected context and private 64-byte KEM seed.
Publishing a public encapsulation key provides encryption capability only.

`DeriveShieldedV3ViewKey` derives separate keys for incoming notes, outgoing
history and stamping, scoped to a chain. A full viewing backup must contain
both incoming and outgoing seeds. A stamp seed alone cannot decrypt notes.
`SignShieldedV3ViewBinding` and `VerifyShieldedV3ViewBinding` authenticate a
canonical viewing public key with ML-DSA-87, scoped to its account, chain and
role. They protect metadata against key substitution and cross-chain replay;
these signatures are separate from the zero-knowledge spend proof.

Wallets must actually create both incoming and outgoing encrypted records to
provide full-history disclosure; this generic encryption API does not add them
to existing transactions. The commitment must include secret random blinding:
never supply a hash of a name, country, address or small amount by itself.

The primitives are distinct security components. Selecting ML-KEM-1024 does
not make the AEAD, a 32-byte wallet secret, or the STARK equally strong. There
is no claim of absolute security, permanent secrecy after private-key exposure,
or sender anonymity from adding payload encryption to an existing transaction.

## Private-spend relation

A proof attests to one nonzero private input note and four fixed output slots.
Amounts are exact unsigned 256-bit values in the smallest chain unit, encoded
as eight little-endian u32 limbs. No floating point or BN254 reduction is used.

The program constrains:

- Ownership through a private spending secret and its Tip5 owner digest.
- A note opening bound to chain ID, asset, owner, value and private randomness.
- Inclusion in the supplied root with a private index and a depth-32 Merkle path.
- A note nullifier derived from chain, asset, spending secret and randomness.
- Every output commitment, including zero-valued private decoys.
- Input value = four output values + public value, with limb carries and a
  forbidden final overflow; this is integer equality, not modular field equality.

The full transaction intent is bound as a public STARK claim. Verifying a proof
against another intent, root, nullifier, value or output commitment fails. The
verifier constructs the fixed program digest locally and uses fixed upstream
STARK parameters. Proof-supplied claims, programs and security parameters are
not accepted. Triton VM's default targets 160 bits of conjectured IOP soundness;
this number is not a guarantee of 160-bit security against quantum attacks.

Tip5 digests remain five canonical Goldilocks field words, encoded as 40 bytes.
They cannot be silently inserted into the legacy 32-byte BN254 commitment tree.
Hash inputs use distinct domains: owner 3001, note 3002, nullifier 3003. Owner,
note and nullifier hashing use Tip5 variable-length padding; internal Merkle
nodes use its fixed-length ordered pair hash.

The public claim has 58 field words: chain (2), asset (2), public value (8),
intent (16 u32 words from its 64-byte digest), anchor (5), nullifier (5), and
four outputs (20). The 91 secret words contain the input secret (5), randomness
(5), value (8), index (1), and four owner/randomness/value openings (18 each).
The path has 32 additional private five-word digests.

## Build and test

Install Rust 1.89 with rustfmt and clippy, then run from the repository root:

```sh
cargo build --release --locked --manifest-path zk/shielded3/stark/Cargo.toml
TKM_SHIELD3_TESTDATA=/tmp/tkm-shield3-vector cargo test --release --locked \
  --manifest-path zk/shielded3/stark/Cargo.toml
TKM_SHIELD3_STARK_BIN="$PWD/zk/shielded3/stark/target/release/tkm-shield3-stark" \
  TKM_SHIELD3_TESTDATA=/tmp/tkm-shield3-vector \
  go test ./crypto/pqcrypto ./zk/shielded3
```

The Rust tests independently compute note commitments, verify valid full-width
amounts, reject a fully committed 256-bit overflow attempt, alter every witness
word and every Merkle path word, generate a genuine randomized proof, and reject
modified proofs and all public-input replays. The Go interoperability test uses
the resulting proof, computes wallet commitments through the native helper,
then generates and verifies another randomized proof through the Go API. It is
skipped unless the native binary and real fixture directory are supplied.
The fixture secrets are deterministic test data, never wallet secrets.

The Go backend exposes `Describe`, `Prove` and `Verify`. `GenerateSecret` samples
fresh uniform canonical secrets and randomness with `crypto/rand`. `Describe`
only computes openings and a candidate root; it does not verify spending rights.
Secret request data goes through stdin, not command-line arguments. Requests,
responses, proof sizes and trace heights are bounded; malformed input, native
panics, absent binaries and cancellation cannot result in proof acceptance.

The binary protocol begins with `TKMS3STK`, followed by 58 little-endian u64
words. `prove` and `describe` receive 91 secret words and 32 private digests.
`verify` receives a u32 proof-word count and exactly that many canonical u64
words. A successful verification emits exactly `OK\n`; proving emits a u32
count and proof words. Maximum proof size is 8 MiB plus four bytes; maximum
padded trace height is 16,384. These are development resource limits, not yet
transaction gas or production consensus limits.

## Required integration before Antartical can use V3

1. Define canonical V3 transaction encoding and intent hashing; bind all fees,
   ciphertexts, sponsorship and withdrawals while excluding spend proofs.
2. Add a separate V3 commitment tree and nullifier storage. Consensus must
   authenticate anchors and atomically reject/record repeated nullifiers.
3. Implement deposit and V2-to-V3 migration relations. This private-spend
   relation cannot prove a legacy note opening or create money from a deposit.
4. Embed the identical verifier on every node platform and account for its
   work in consensus limits. Process availability must not decide block validity.
5. Wire wallet keys, backup, incoming/outgoing disclosures, encrypted stamps,
   address metadata authentication, RPC, transaction pool and activation gates.
6. Test real end-to-end transfers, migrations and fork boundary behavior.

The current account transaction still exposes its signing identity and fees.
Achieving anonymous V3 transfers also requires the new outer transaction and
funding path; ciphertext confidentiality alone does not remove that metadata.

References:

- [NIST FIPS 203](https://doi.org/10.6028/NIST.FIPS.203)
- [Go ML-KEM documentation](https://pkg.go.dev/crypto/mlkem)
- [Triton VM 8.0.0](https://docs.rs/triton-vm/8.0.0/triton_vm/)
- [Triton VM specification](https://triton-vm.org/spec/)
