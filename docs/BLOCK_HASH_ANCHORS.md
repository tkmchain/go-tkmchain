# TKMChain block-hash anchors

`contracts/TKMBlockHashAnchors.sol` is an append-only application contract for
recording canonical block hashes. It is intended to give operators, explorers,
and external auditors a small, event-indexed sequence they can compare with
their local chain.

All append methods are gated by the chain's Antartical schedule. Mainnet chain
8979 activates at `1790812800` (1 October 2026 00:00 UTC); Egypt chain 8980 is
active from genesis. Unknown chain IDs remain disabled. Calls made before the
gate revert with `AntarticalInactive`; read-only inspection remains safe.

## What the contract verifies

The contract accepts writes only from its current `owner`. Before a hash is
stored, it obtains `BLOCKHASH(height)` and requires the submitted value to
match exactly. It rejects zero hashes, future heights, and hashes outside the
EVM's 256-block `BLOCKHASH` window. After the first write, every write must be
the next height; an existing height cannot be replaced or deleted. The range
method validates the entire range before writing any element, so a bad element
cannot leave a partial append.

The useful calls are:

```text
appendVerified(height, expectedHash)
appendCanonical(height)
appendParent()
appendVerifiedRange(startHeight, expectedHashes)
anchorAt(height)
matchesCanonical(height, expectedHash)
```

`appendParent` is suitable for an operator that submits one transaction per
block. `appendVerifiedRange` is limited to 256 hashes and is useful when an
operator is catching up without missing the opcode window. Every successful
write emits `BlockHashAnchored(height, blockHash, submitter, rollingCommitment)`.
The contract also advances `rollingCommitment` with a domain-separated hash of
the previous commitment, height, and block hash. Auditors can compare that
value to detect omitted or reordered events.

## Historical blocks and the security boundary

Solidity cannot verify an arbitrary old block hash. `BLOCKHASH` intentionally
returns zero for blocks older than 256 blocks, so accepting an owner-supplied
hash for block 1 or an old live chain would only move trust into the owner key.
This contract refuses that unsafe shortcut. To anchor a chain from block 1,
deploy the contract at genesis and append continuously, or add a consensus
chain-native historical-hash oracle that supplies a verified header proof before
backfilling. Do not call a trusted off-chain list “verified” without such a
proof.

## Reorganizations

An EVM contract cannot make its own chain state survive a reorganization: a
reorg that removes the anchor transaction also removes that state. Anchors are
therefore evidence and detection, not a replacement for consensus finality.
Operators should:

1. index `BlockHashAnchored` events;
2. compare every event with the local canonical header at the same height;
3. stop serving a chain when an anchored hash conflicts; and
4. keep TKMChain's configured mandatory checkpoints and finality rules enabled.

The owner can transfer append authority, but there is no delete, overwrite,
upgrade, or self-destruct path. A compromised owner can stop future writes or
submit only hashes that the EVM itself proves; it cannot insert a different
hash for a recent height.
