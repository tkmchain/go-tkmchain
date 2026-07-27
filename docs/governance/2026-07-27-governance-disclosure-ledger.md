# Governance Disclosure Ledger Activation

kind: governance-record
version: 1
date: 2026-07-27

## Summary

TkmChain added a non-consensus governance disclosure ledger so public governance explanations can be published as Main King signed records.

The ledger is intended for Rotating King selections, checkpoint explanations, roadmap statements, development-fund commitments, hardfork notices, emergency actions, and security announcements.

## Reason

On-chain balances and checkpoint hashes prove technical facts, but they do not preserve the public explanation behind governance decisions. Websites, forum posts, social messages, and repository files can be edited or removed. This ledger gives every official disclosure a deterministic hash, a Main King signature, an optional anchor transaction, and an append-only previous-hash link.

## Implementation

This activation adds the `tkmgov` RPC namespace and the `gtkm governance` CLI commands. Records are stored in the node database and mirrored to the node datadir governance folder.

The disclosure hash includes the domain string `TKMCHAIN_GOV_DISCLOSURE_V1`, chain ID, kind, title, version, content hash, URI, previous disclosure hash, and timestamp.

Only the configured Main King address can publish official disclosure records. Duplicate disclosure hashes are rejected. Nonzero previous hashes must refer to an existing disclosure record.

## Consensus Status

This is not a hardfork. It does not change block validation, transaction execution, header validation, checkpoint consensus, difficulty rules, rewards, chain configuration activation times, or any consensus state transition.

## Operating Policy

A Rotating King selection, checkpoint announcement, roadmap commitment, hardfork notice, development-fund commitment, emergency action, or security announcement should be treated as official only when the public document has a matching Main King signed governance disclosure record.

If a statement changes later, the old disclosure remains and the updated statement must be published as a new version linked by previous hash.
