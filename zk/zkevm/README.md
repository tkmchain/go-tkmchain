# Generalized zkEVM execution proof

The keeper guest is the generalized execution circuit for TKM. It runs the
same go-ethereum stateless block processor used by the node, so the proof
covers the chain's complete implemented EVM semantics: opcode execution, gas,
calls, memory, storage, precompiles, logs, receipts, and the resulting state
and receipt roots. The block and witness are private zkVM inputs.

The guest commits one fixed statement. Ziren exposes its SHA-256 digest as the
proof public value:

```text
version || chain_id || block_number || block_hash || state_root || receipt_root
```

The verifier hashes that exact little-endian byte layout and compares the
result to the proof's 32-byte public value.

The `ziren` build commits this statement only after `ExecuteStateless` has
recomputed both roots and matched them to the block header. Any execution
error or root mismatch exits without a valid proof.

## Build the guest

Install the pinned Ziren toolchain, then build the existing keeper guest for
MIPS32 little-endian:

```bash
zk/zkevm/build_keeper.sh build/keeper-mipsle
```

The build script overlays the standard library's kernel-version and hostname
probes, patches the Go runtime's early kernel-version probe, and stages the
selected `x/sys` module with its MIPS `uname` wrapper replaced. It also
disables host epoll, VMA naming, and entropy syscalls in the guest overlay.
Ziren guests do not have a host kernel, so this prevents Linux/MIPS syscalls
from entering the deterministic guest ABI. The entropy replacement is scoped
to this guest build and is only safe because the keeper executes a supplied
block and witness; it never generates keys, signatures, transaction nonces,
or other security material. The daemon and wallet use the normal operating
system cryptographic randomness.

The guest input is the RLP encoding of `main.Payload`:

```go
type Payload struct {
    ChainID uint64
    Block   *types.Block
    Witness *stateless.Witness
}
```

To create a payload from a running local node, build `cmd/fetchpayload` and
use its IPC endpoint. A pruned node may need to replay stored blocks to rebuild
the parent state; opt into that expensive recovery explicitly:

```bash
go build -o build/fetchpayload ./cmd/fetchpayload
./build/fetchpayload \
  --rpc "$HOME/.tkmchain/gtkm.ipc" \
  --block latest \
  --recover-state \
  --out "$PWD"
```

The recovery RPC is IPC-only in the recommended deployment. It never changes
the canonical head, but it can consume substantial CPU, disk, and time when
the requested block is far behind the head.

## Prove and verify

The host uses the Ziren STARK mode. It executes the guest, generates a proof,
verifies it locally, writes the proof-with-public-values artifact, reloads it,
and verifies it a second time:

```bash
cargo run --release --manifest-path zk/zkevm/keeper-host/Cargo.toml -- \
  --elf build/keeper-mipsle \
  --input block-payload.rlp \
  --proof keeper-proof.bin \
  --vk keeper-vk.bin
```

This uses Ziren's native STARK verifier. Groth16 and PLONK wrapping are not
used here because those wrappers are not post-quantum.

The verifier can run independently from the prover using only the proof and
the pinned program verifying key:

```bash
cargo run --release --manifest-path zk/zkevm/keeper-host/Cargo.toml -- \
  --verify keeper-proof.bin \
  --vk keeper-vk.bin \
  --chain-id 8979 \
  --block-number 43947 \
  --block-hash 0x... \
  --state-root 0x... \
  --receipt-root 0x...
```

The five statement flags are optional for inspection, but a production verifier
must provide all five together. This checks that the proof is for the exact
chain, block, and roots the caller intended; proof validity alone does not
authorize a different block.

## Consensus status

The proof pipeline is deliberately separate from ordinary block processing
until the chain embeds a pinned verifier key and proof artifact format. A node
must never accept a proof merely because a host command returned success. The
next consensus integration step is to vendor the exact Ziren verifier crate,
pin its verifier key hash, and call that verifier from the node's native
proof backend. Until that artifact is pinned, transparent execution remains
governed by the existing Antartical rules.
