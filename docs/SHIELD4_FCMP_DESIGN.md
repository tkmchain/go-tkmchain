# Shield4 FCMP++-inspired production protocol

Shield4 is the version-4 full-chain membership protocol activated by the
existing Antartical gate. Shield3 remains valid for its existing notes; new
Shield4 spends use the separate envelope and native relation described here.

## Why this is a new version

FCMP++ is a Monero-specific construction that replaces a fixed ring with a
full-chain membership proof and combines membership, spend authorization, and
linkability. Its proof relation, commitment tree, amount proof, and nullifier
rules cannot be dropped into the Shield3 Triton relation without changing the
consensus statement and state transition.

Shield4 therefore needs a new envelope, a new native verifier entry point, new
wallet proof generation, and independent cross-language test vectors. Shield3
transactions remain valid so the migration does not reinterpret historical
state.

## Required Shield4 properties

1. A proof must show membership in the complete Shield4 note set without
   identifying the spent note.
2. Spend authorization must be bound to the hidden owner secret and the full
   transaction intent.
3. A deterministic linkability tag must prevent double spends without exposing
   the note or its recipient.
4. Amount conservation and range proofs must hide values while preventing
   inflation.
5. Proof packets, encrypted outputs, and relay requests must use fixed size
   classes and bounded verification work.
6. Wallets should use Tor and a shared relay by default; this is a wallet and
   transport policy and cannot be guaranteed by consensus alone.

## Activation rule

Shield4 uses the existing Antartical activation timestamp (`1790812800`,
2026-10-01 00:00 UTC). Before that timestamp, version-4 transactions are
rejected. No second activation fork is introduced. The native verifier is
embedded and consensus rejects a proof if that verifier is unavailable; there
is no placeholder or legacy-verifier fallback.

## Current implementation status

The repository contains a separate `TKMS4STK` native FFI entry point and a
frozen Shield4 Triton claim program. The claim program has a protocol-specific
domain assertion, so a Shield3 proof cannot be replayed under the Shield4
verifier. It uses the canonical Tip5 membership witness shape, and consensus
appends Shield4 commitments to the shared tree so the anonymity set spans both
versions.

The native layer is intentionally fail-closed until the embedded static library
is rebuilt from this source. The versioned envelope, Antartical-gated state
transition, txpool checks, shared full-chain root, link-tag replay state, RPC
status, and direct, batch, and relayed wallet constructors are wired in the
source tree. Every release must rebuild the static library and run the native
proof vectors, reorg tests, and wallet end-to-end tests together so a partially
deployed node cannot accept a proof that other nodes cannot reproduce.
