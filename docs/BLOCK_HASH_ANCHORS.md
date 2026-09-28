# TKMChain block-hash anchors

`contracts/TKMBlockHashAnchors.sol` is the operator-facing append-only
application interface. The authoritative protection is consensus-native:
every Antartical RandomX block must carry a valid block-hash anchor in its
header. Nodes reject a post-fork header that omits the anchor, names a different
parent, uses the wrong height, or breaks the rolling commitment. This keeps the
rule active during import, header sync, mining, and reorganization checks.

The same interface is a deterministic protocol predeployment. At the first
Antartical state transition, the node installs the contract runtime and records
the predecessor hash before processing user transactions. The fixed address is
`0x0000000000000000000000000000000000008979` on both mainnet and Egypt. Egypt
is active from genesis, so its first mined block creates the predeployment
without a deployment transaction. Mainnet creates it at its Antartical
activation block. Miners and importers run the identical transition, keeping
the state root deterministic and preventing a transparent transaction from
weakening the privacy gate.

All append methods are gated by the chain's Antartical schedule. Mainnet chain
8979 activates at `1790812800` (1 October 2026 00:00 UTC); Egypt chain 8980 is
active from genesis. Unknown chain IDs remain disabled. Calls made before the
gate revert with `AntarticalInactive`; read-only inspection remains safe.

## Consensus header envelope

Legacy blocks retain the 32-byte extra-data limit. At Antartical the header
envelope is versioned to 128 bytes. The final 96 bytes are the fixed
`TKM_BLOCK_HASH_ANCHOR_V1` suffix:

```text
domain marker | parent height (uint64) | parent hash | rolling commitment
```

The rolling commitment is:

```text
Keccak256("TKM_BLOCK_HASH_ANCHOR_V1", previousRolling, height, parentHash)
```

The first Antartical block starts from an all-zero previous commitment. Every
later block must extend the commitment carried by its canonical parent. Existing
rotating-king or miner metadata remains in the prefix of the envelope. Egypt
uses the same consensus rule from genesis; its genesis header is the only
header without a predecessor anchor.

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

## Reorganizations and limitations

The header rule makes malformed or conflicting anchor histories invalid to all
consensus nodes. It does not claim that proof-of-work can mathematically make
an alternative branch impossible: a sufficiently powerful attacker could
recompute a valid competing branch. Mandatory checkpoints and future finality
rules are still required for economic finality. The Solidity state is an
auditable mirror and cannot survive a reorganization that removes its own
transaction.
Operators should:

1. index `BlockHashAnchored` events;
2. compare every event with the local canonical header at the same height;
3. stop serving a chain when an anchored hash conflicts; and
4. keep TKMChain's configured mandatory checkpoints and finality rules enabled.

The owner can transfer append authority, but there is no delete, overwrite,
upgrade, or self-destruct path. A compromised owner can stop future writes or
submit only hashes that the EVM itself proves; it cannot insert a different
hash for a recent height.

The consensus predeployment is the authoritative history from activation
forward. The Solidity append methods remain useful for operator-facing event
indexing and for networks that intentionally expose the application interface;
they are not a replacement for the consensus header rule or the automatic state
transition.
