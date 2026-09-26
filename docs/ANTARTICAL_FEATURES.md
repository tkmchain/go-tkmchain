# Antartical protocol feature schedule

Antartical is the single activation boundary for the next TKM protocol set:
`2026-10-01 00:00:00 UTC` (`1790812800`). The schedule is deterministic from
the chain configuration; a node must never enable one of these features from a
local command-line switch.

## Feature contract

| Capability | Antartical gate | Current implementation | Consensus status |
| --- | --- | --- | --- |
| Native account abstraction (EIP-4337/RIP-7560) | `IsAccountAbstraction` | Smart-account operation hashing, owner/session authorization APIs | EntryPoint/UserOperation execution still needs a consensus transaction format and deployed code |
| Parallel execution (Block-STM/optimistic) | `IsParallelExecution` | Canonical serial executor remains the reference result | Requires conflict scheduler plus serial equivalence tests before consensus use |
| Alternative EVMs (Rust EVM/Revm/evmone) | `IsAlternativeEVM` | The Go EVM remains canonical | Alternative engines must pass byte-for-byte state/receipt differential vectors |
| Formal verification tooling | `IsFormalVerification` | Execution witness recovery and zkEVM guest/prover host | Proof artifacts and machine-checked invariants are tooling until accepted by consensus |
| Multidimensional gas | `IsMultidimensionalGas` | Feature gate and existing gas accounting | Requires header commitment and transaction encoding for every new dimension |
| EIP-4844 blobs | `IsBlobGas` | Existing Cancun blob transactions, blob pool, and blob base fee | **Ready**; Cancun is activated at Antartical on mainnet |
| Native privacy (zkEVM/private transactions) | `IsNativePrivacy` | Shield3/Shield4, private EVM/TVM envelopes, and zkEVM execution witness path | **Ready for the implemented envelope formats** |
| Stateless clients (Verkle) | `IsStatelessVerkle` | Verkle transition storage, witness costs, and state-history recovery | Full Verkle state commitment and network witness protocol still required |
| Native randomness | `IsNativeRandomness` | RandomX mix digest remains the current consensus randomness source | A new beacon/randomness commitment must be specified before replacing it |
| Native oracles | `IsNativeOracles` | No consensus oracle feed is installed | Needs signed, replay-protected feed format and quorum rules |
| Cross-chain standards | `IsCrossChainStandards` | No bridge messages are accepted by consensus | Needs a replay-protected message envelope and finality proof rules |
| EVM Object Format (EOF) | `IsEOF` | Legacy EVM bytecode remains canonical | Requires EOF validation, code storage rules, and opcode versioning |
| Modular precompiles | `IsModularPrecompiles` | Existing precompiles are statically registered | Requires an address/version registry committed by chain config |
| Deterministic gas metering | `IsDeterministicGas` | Canonical intrinsic, EIP-1559, and blob gas rules | **Ready for the current transaction formats** |
| Single-slot finality | `IsSingleSlotFinality` | TKM remains RandomX proof-of-work | Requires a separate validator/finality consensus engine; it cannot be enabled by a flag on PoW |

The `params.Rules` object exposes all gates, and
`ChainConfig.AntarticalFeatureCatalog()` gives clients the same machine-readable
schedule. The `ConsensusReady` field is intentional: a scheduled feature is
not treated as consensus code until its deterministic implementation and test
vectors are present.

Running nodes expose the same view through the read-only `tkmprotocol` RPC
namespace:

```sh
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  --data '{"jsonrpc":"2.0","id":1,"method":"tkmprotocol_antarticalFeatures","params":[]}'
```

Include `tkmprotocol` in `--http.api` (or `--ws.api`) when using a custom API
allowlist.

## Integration requirements before marking the remaining rows ready

1. Add consensus transaction/envelope formats and replay protection for account
   abstraction and cross-chain messages.
2. Build a deterministic parallel executor that proves serial equivalence for
   every conflict set; retain the serial executor as the fallback.
3. Add Revm/evmone differential vectors and reject any engine result that differs
   from the canonical Go EVM.
4. Commit Verkle roots/witnesses in the block header and implement snap/witness
   exchange before stateless mode can be enforced.
5. Define signed randomness/oracle feeds, EOF validation, and modular precompile
   registries in the chain configuration.
6. Specify a validator finality protocol and a migration from RandomX before
   enabling single-slot finality.

The deterministic primitives are implemented in `consensus/antartical`:

- canonical UserOperation hashing and secp256k1 authorization;
- deterministic optimistic execution waves from access sets;
- multidimensional gas vectors;
- state-witness and zk execution claim commitments;
- signed oracle observations and replay-bound cross-chain messages;
- EOF v1 container validation;
- modular precompile registration; and
- signed single-slot finality certificates.

The block processor currently uses the canonical Go EVM and RandomX engine;
the Antartical primitives provide the shared transition and differential-test
surface for the Rust/Revm/evmone adapters.
