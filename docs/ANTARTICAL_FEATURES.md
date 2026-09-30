# Antartical protocol feature schedule

Antartical is the single activation boundary for the next TKM protocol set:
`2026-10-01 00:00:00 UTC` (`1790812800`). The schedule is deterministic from
the chain configuration; a node must never enable one of these features from a
local command-line switch.

## Feature contract

| Capability | Antartical gate | Current implementation | Consensus status |
| --- | --- | --- | --- |
| Native account abstraction (EIP-4337/RIP-7560) | `IsAccountAbstraction` | `TKM-AA-ENTRYPOINT-V1` relay envelope, nonce state, factory/paymaster EVM validation, and canonical execution | The native envelope is implemented; deployment/factory policy remains chain configuration work |
| Parallel execution (Block-STM/optimistic) | `IsParallelExecution` | `ExecuteOptimistic` runs canonical waves concurrently, validates dynamic conflicts, and retries serially | StateDB commit integration remains gated until isolated state snapshots are available |
| Alternative EVMs (Rust EVM/Revm/evmone) | `IsAlternativeEVM` | Strict process adapter plus `RegisterConformant` differential admission | No external backend is selected unless it passes canonical vectors |
| Formal verification tooling | `IsFormalVerification` | Execution witness recovery and zkEVM guest/prover host | Proof artifacts and machine-checked invariants are tooling until accepted by consensus |
| Multidimensional gas | `IsMultidimensionalGas` | Feature gate and existing gas accounting | Requires header commitment and transaction encoding for every new dimension |
| EIP-4844 blobs | `IsBlobGas` | Existing Cancun blob transactions, blob pool, and blob base fee | **Ready**; Cancun is activated at Antartical on mainnet |
| Native privacy (zkEVM/private transactions) | `IsNativePrivacy` | Shield3/Shield4, private EVM/TVM envelopes, and zkEVM execution witness path | **Ready for the implemented envelope formats** |
| Stateless clients (Verkle) | `IsStatelessVerkle` | Verkle transition storage, witness costs, and state-history recovery | Full Verkle state commitment and network witness protocol still required |
| Native randomness | `IsNativeRandomness` | RandomX mix digest remains the current consensus randomness source | A new beacon/randomness commitment must be specified before replacing it |
| Native oracles | `IsNativeOracles` | Signed `TKM-ORACLE-ENVELOPE-V1` observations persist monotonic feed rounds | Feed quorum/committee registration is still required before a value can drive bridge or pricing state |
| Cross-chain standards | `IsCrossChainStandards` | Destination-bound signed `TKM-XCHAIN-ENVELOPE-V1` messages persist replay keys | The message layer does not release value; a bridge application must consume a finalized commitment |
| EVM Object Format (EOF) | `IsEOF` | EOF-prefixed runtime code is validated and stored through the Antartical path; legacy code remains replay-compatible | Full EOF opcode-version migration is still separate from container validation |
| Modular precompiles | `IsModularPrecompiles` | Existing precompiles are statically registered | Requires an address/version registry committed by chain config |
| Deterministic gas metering | `IsDeterministicGas` | Canonical intrinsic, EIP-1559, and blob gas rules | **Ready for the current transaction formats** |
| Single-slot finality | `IsSingleSlotFinality` | Versioned header certificate envelope, chain-bound digest, active-set membership, quorum checks, and miner finality-provider hook | Once an active committee exists, blocks without a certificate are rejected and miners require a provider |

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

1. Register factory/paymaster policies for every network and publish the
   post-quantum account implementation used by the native envelope.
2. Wire isolated StateDB snapshots into `ExecuteOptimistic`; retain the serial
   executor as the deterministic fallback for unknown accesses.
3. Install Revm/evmone binaries and admit them only after canonical vectors
   pass `RegisterConformant`.
4. Commit Verkle roots/witnesses in the block header and implement snap/witness
   exchange before stateless mode can be enforced.
5. Define signed randomness/oracle feeds and modular precompile registries in
   the chain configuration; EOF container validation is now wired into
   contract deployment.
6. Configure a validator signer implementing `miner.FinalityProvider`; the
   worker now refuses to seal a post-fork block with an active committee unless
   it can attach a valid certificate.

The deterministic primitives are implemented in `consensus/antartical`:

- canonical UserOperation hashing and secp256k1 authorization;
- deterministic optimistic execution waves from access sets;
- multidimensional gas vectors;
- state-witness and zk execution claim commitments;
- signed, monotonic oracle observations and replay-bound cross-chain messages;
- EOF v1 container validation;
- modular precompile registration; and
- signed single-slot finality certificates.

The block processor currently uses the canonical Go EVM and RandomX engine;
the Antartical primitives provide the shared transition and differential-test
surface for the Rust/Revm/evmone adapters. Account-abstraction, oracle, and
cross-chain envelopes are applied by both the importer and local miner so their
state roots match. When a block carries a finality certificate,
`BlockValidator` verifies its slot, chain-bound header digest, active-validator
membership, 2/3 quorum, and metadata commitment. Once an active committee is
present, a missing certificate is rejected and the miner requires a configured
`FinalityProvider`.

## Versioned header metadata

`consensus/antartical.HeaderMetadata` is the canonical metadata envelope for
the new profile. It is encoded as:

```
TKM_ANTARTICAL_META_V1 || uint32_be(rlp_length) || rlp(version, gas_limits, commitments)
```

`gas_limits` contains independent EVM, TVM, proof-verification, and blob
ceilings. The commitments cover the canonical state witness, asset registry,
optimistic conflict transcript, randomness, oracle round, cross-chain message
set, modular precompile registry, and finality certificate. Empty domains use
domain-separated empty commitments rather than zero values. The metadata is
appended before the existing RandomX block-hash-anchor suffix, so the legacy
header and receipt RLP encodings remain unchanged. `BlockValidator` rejects a
malformed metadata record on any block and legacy blocks remain decodable.

This envelope is the wire-format foundation for the remaining activation work;
it does not by itself authorize an execution engine, oracle signer, bridge, or
finality committee. Those components must provide a valid commitment and a
consensus state transition before their catalog row can be marked
`ConsensusReady`.

## Antartical block rewards

The Antartical schedule preserves a 200 TKM initial block reward while adding
the validator share:

| Recipient | Initial share |
| --- | ---: |
| Miner | 90 TKM |
| Selected validator | 70 TKM |
| Rotating King | 35 TKM |
| Main King | 5 TKM |

Exactly one selected validator receives the 70 TKM share at each height. Every
share is halved independently at the existing RandomX halving interval, so the
total remains 100 TKM after the first halving, 50 TKM after the second, and so
on. Historical pre-Antartical reward transactions retain their existing
200-TKM percentage schedule. The selected validator address comes from the
consensus validator registration set. The registration envelope, activation
queue, persistent state registry, deterministic selection, slashing evidence,
and validator reward-marker checks are documented in
[`ANTARTICAL_VALIDATORS.md`](ANTARTICAL_VALIDATORS.md) and enforced by the
state processor after Antartical activation.
