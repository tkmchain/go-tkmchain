# Confidential value and shielded pool consensus redesign

## Status

The accepted scope is now documented in
[Confidential note ledger](CONFIDENTIAL_LEDGER.md): private note balances and
payments, opt-in note-funded fees, and explicit public deposits/withdrawals.
That implementation proposes October 4 at 10:00 UTC. The broader fully
confidential issuance/backing design below remains a separate proposal.

This is a protocol change proposal, not a claim that current Shield3 or
Shield4 deposits hide their value. In the current code, a deposit's `tx.Value`
is public, is an input to the STARK statement, and is transferred into the
pool's ordinary account balance. Hiding just one of those fields would either
leave the amount inferable or break the reserve invariant. Do not describe the
current deposit path as amount-private.

The new transition must use a new envelope and verifier domain, activated at a
separately coordinated consensus fork. Existing blocks and historical Shield3
and Shield4 transactions retain their original decoding and validation rules.
No timestamp is assigned to this full redesign. The separately implemented
[shielded-only cutoff](SHIELDED_ONLY_FORK.md) restricts transactions to existing
private notes and proposes October 4, 2026 at 10:00 UTC. The default networks
leave that cutoff disabled pending confidential funding. It does not implement
the confidential issuance/backing mechanism described here. All clients and
validators must agree on any activation before deployment.

## Required transaction relation

The new proof must bind the complete transaction intent and prove, without
publishing note values, that:

* each input commitment is a member of the accepted note tree;
* each input is authorized by its hidden spending key and has a unique
  nullifier;
* every output commitment opens to a valid owner, value, and fresh randomness;
* input value equals output value plus any explicitly public withdrawal and
  fee amounts, using range-constrained integers and no field wraparound;
* deposit and withdrawal transitions update the same asset's backing
  commitment correctly;
* encrypted recipient and change payloads open to the values committed by the
  proof; and
* the proof is bound to chain ID, fork version, asset ID, nonce/replay domain,
  stamp policy, and fee authorization.

For an ordinary private transfer, public value fields are zero. A transparent
withdrawal necessarily reveals its withdrawal amount to the public EVM account
that receives it. Gas payment is also public unless a separate fee-credit
mechanism is used.

## Consensus representation

Shielded balances must be represented by note commitments and a versioned
asset commitment root, rather than an account balance per note. Consensus
stores the note-tree root, spent-nullifier accumulator, and a hiding aggregate
backing commitment per asset. The proof demonstrates that each state update
preserves the committed aggregate and the asset's issuance/backing rules.

An ordinary EVM account balance is transparent by design. A transfer from
that account into a private pool reveals the sender's balance change even if
the transaction envelope omits `tx.Value`. Therefore full amount privacy also
requires private balances to live in the shielded asset state from issuance or
from a prior shielded balance. The legacy public account balance of the pool
cannot be treated as a confidential balance. Any bridge between transparent
EVM value and the private asset must be explicitly labeled as a public
deposit/withdrawal, or use a separately specified confidential funding
mechanism with a sound, auditable backing proof.

The implementation must not accept an unbacked note or infer backing from a
zero-valued EVM transfer. A proof of conservation alone is insufficient to
prove collateralization against a public account; the consensus relation must
bind issuance, backing, and asset supply to the chain's actual state transition.

## Daemon-side transaction construction

The daemon may expose a local, authenticated transaction-builder service and
run the native prover in a persistent worker. Wallets submit a bounded,
encrypted witness package; the daemon validates the request, constructs the
canonical envelope, proves it, verifies the result locally, and returns a
signed/broadcast transaction according to the wallet's explicit authorization.
A conventional native prover must access the plaintext witness in memory.
Running it inside a trusted local daemon can preserve local key custody;
encrypting a request to a remote prover does not hide that witness from the
remote operator. Authorization must bind the exact intended spend, and the
daemon must not create unrelated spend authorizations. A remote service that
cannot see the witness requires a different proving architecture, not merely
TLS or an encrypted request body.

Builder requirements:

* accept only local IPC or explicitly authenticated wallet sessions by default;
* never log witness data, note values, recipient secrets, or decrypted payloads;
* bind the proof to the exact transaction intent before signing;
* cache reusable proving parameters, not per-user secrets or transaction
  witnesses;
* cap input count, witness bytes, memory, and wall-clock work; and
* return structured progress/cancellation status without broadcasting a
  partially built transaction.

The prover backend must be benchmarked on supported release hardware. A
consensus proof cannot be made cheap merely by moving it into the daemon; use a
fixed, versioned circuit and measured proving parameters, and keep proof
verification deterministic and bounded for every validator.

## Rollout and acceptance gates

Before enabling the new version, implement and test the envelope, native
prover/verifier, asset commitment state, and daemon builder as one protocol
change. Require cross-language test vectors and multi-node tests for deposits,
ordinary sends, multi-input sends, withdrawals, fee sponsorship, replay,
reorgs, malformed range values, and backing shortfalls. Tests must assert that
ordinary private sends expose no amount in transaction fields, logs, receipts,
or RPC responses. They must also verify that a deliberately under-backed
state transition is rejected by every node.

Until those gates pass and a coordinated activation is configured, retain the
existing consensus behavior and clearly label existing deposit values as
public. Do not silently reinterpret existing envelopes or enable an
incompatible verifier on mainnet.
