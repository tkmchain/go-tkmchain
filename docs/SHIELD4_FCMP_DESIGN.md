# Shield4 FCMP++-inspired design boundary

Shield4 is reserved for a future full-chain membership proof system. It is not
accepted by current consensus. Shield3 remains the active Antartical private
transaction format.

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

Any future Shield4 activation on this chain must use the existing Antartical
activation timestamp (`1790812800`, 2026-10-01 00:00 UTC). If the complete
implementation is not ready at that timestamp, Shield4 remains disabled; no
second activation fork is silently introduced. A version-4 transaction must be
rejected until the complete verifier and wallet implementation are present. No
placeholder verifier may accept a Shield4 proof, and Shield3 validation must
never fall back to Shield4 or a legacy verifier.

## Current status

The repository contains no FCMP++ implementation. The current Shield3 relation
already hides Merkle paths and recipient amounts, uses recipient-secret
nullifiers, and supports fixed outputs, relays, Tor, and Dandelion propagation.
Implementing Shield4 requires first selecting and implementing a concrete
post-quantum full-chain membership relation and its proof vectors.
