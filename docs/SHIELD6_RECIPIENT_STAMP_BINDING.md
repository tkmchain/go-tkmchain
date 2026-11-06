# Shield6 recipient stamp binding

## Status

The candidate constrained relation now exists in
[`zk/shielded6/recipient`](../zk/shielded6/recipient/README.md). It proves
hidden membership in the existing stamp registry and a separate versioned
key-binding registry, then constrains the exact V1 output-key derivation.
The package documents its precise leaf encoding, proof API, setup boundary
and tests. Registration key-control proofs, canonical binding-registry state, versioned
consensus enforcement and client adapters are implemented. Fresh single-operator
production parameters are pinned with the activation schedule in
[`params/shield6_production.json`](../params/shield6_production.json).
The fork requires proof-bearing envelopes and rejects the older proofless V1
envelope. This setup is not an independent ceremony or cryptographic audit.
Parameter installation is described in
[`SHIELD6_PRODUCTION_PARAMETERS.md`](SHIELD6_PRODUCTION_PARAMETERS.md).
Measurements and exact validation scope are recorded in
[`SHIELD6_PRODUCTION_STATUS.md`](SHIELD6_PRODUCTION_STATUS.md).

Shield6 V1 does not prove that a private output belongs to an Antartical-
stamped receiver. Its consensus transition checks the outer sender's stamp and
checks public withdrawal recipients, but a private output contains only a
one-time Edwards25519 key, an ephemeral key, a Pedersen commitment, and
encrypted note data. The transaction does not carry a consensus-verifiable
relation from those fields to a registered stamp.

This was a protocol blocker requiring the versioned recipient-binding proof
and its consensus hardfork. Independent review remains outstanding. Do not treat a public
Merkle-path check or an RPC stamp lookup as a substitute.

## Why the current primitives do not compose

The Antartical stamp accumulator commits to a Shield3 Tip5 owner digest. The
Shield6 payment code instead contains independent Edwards25519 spend and view
public keys. Shield6 forms an output key as

```text
R = rG
shared = r * recipient_view_public
P = recipient_spend_public + H(shared, chain_id, output_index)G
```

Consensus can validate the curve points and later validate CLSAG spends, but
it cannot infer which registered stamp owns `recipient_spend_public` or
`recipient_view_public`. Revealing either static key together with a public
stamp-membership path would make the receiver identity linkable. A recipient
signature over `P` would authorize those bytes but would not prove that `P`
was derived from the signer's registered Shield6 keys.

## Required protocol change

Add a new Shield6 envelope/proof version and a versioned stamp-key binding
commitment. The consensus proof must establish all of the following in one
zero-knowledge statement:

1. A hidden stamp leaf is a member of the canonical Antartical stamp root at
   the transaction's state anchor.
2. The leaf's Shield6 key-binding commitment opens to valid, non-identity
   Edwards25519 spend and view public keys.
3. The output's ephemeral key and one-time key satisfy the Shield6 derivation
   above for the exact chain ID and output position in the signed envelope.
4. Every private output has such a proof. Public deposit or withdrawal
   recipients continue to follow their explicit stamped-account rule.
5. The proof is bound to the transaction intent, asset ID, output commitment,
   encrypted-note hash, chain, and proof version, preventing output or chain
   substitution.

The binding commitment must be added through a versioned stamp-registration
operation authorized by the stamped identity. Existing immutable V1 stamp
records cannot be silently reinterpreted. Wallets need a migration or
re-registration flow that proves control of both the existing stamp and the
Shield6 keys. The new proof system, parameter-generation or transparent-setup
story, verifying-key hash, proof byte limits, gas schedule, and state-root
encoding must be fixed in the protocol specification before implementation.

## Consensus acceptance gates

The new envelope must be rejected unless its proof verifies against the
canonical stamp root and the exact output fields. Tests must show rejection
when each of the stamp leaf, spend key, view key, ephemeral key, one-time key,
output position, chain ID, encrypted-note hash, and state anchor is changed.
Tests must also show that a valid receiver can be proven without publishing
its stamp owner, stable Shield6 keys, or membership path, and that two outputs
to the same receiver do not expose a common receiver identifier.

Before activation, run wallet interoperability tests, proof malleability and
malformed-point tests, transaction replay and reorg tests, two-node consensus
tests, gas and maximum-envelope measurements, and the release artifact matrix.
Only then may a new activation timestamp be configured. Shield6 V1 remains
unfit for production activation because it cannot meet the recipient-stamp
consensus rule while preserving the intended receiver privacy.

## What does not close the gap

- Wallet-side or RPC-side checks can be bypassed by a direct transaction.
- A public stamp membership path reveals a stable recipient association.
- A signature over an output key does not prove the key-derivation relation.
- Adding static Shield6 keys to the public stamp record creates a public link.
- Treating sender stamping as proof that all private recipients are stamped
  is logically invalid.

These are rejected as consensus designs for Shield6 recipient authorization.
