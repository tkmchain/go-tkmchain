# Tkmchain whitepaper

Status date: July 2026

## Abstract

Tkmchain is an EVM-compatible execution network built around CPU-friendly RandomX proof-of-work, Rotating Kings governance, and a secure native-contract roadmap called TVM. It preserves the Ethereum account model, transaction model, ABI, storage semantics, logs, and developer tooling while changing the consensus and reward layer to make mining accessible and governance participation explicit.

The protocol targets 120-second blocks, RandomX mining epochs of 2,048 blocks, and a block reward model that distributes value among miners and governance operators. Each block reward is split 10 percent to the Main King, 40 percent to the current Rotating King, and 50 percent to the miner. This creates a direct economic connection between network security, operational governance, and block production.

TVM extends the roadmap beyond standard EVM bytecode by introducing a deterministic, metered, sandboxed native-contract path for audited C++ modules. TVM is designed as an execution backend for selected EVM accounts, not a second state model or a replacement for Solidity.

## 1. Motivation

Modern EVM networks often concentrate block production around specialized hardware, staking concentration, or infrastructure-heavy validator operations. Tkmchain takes a different path: it keeps EVM compatibility while using RandomX proof-of-work to favor general-purpose CPUs and broad participation.

The second design problem is governance. Many chains rely on off-chain social coordination without a protocol-visible operator role. Tkmchain introduces Rotating Kings: funded operators that rotate through a reward-bearing responsibility slot. The Main King anchors checkpoint and leadership duties, while Rotating Kings receive scheduled rewards and monitoring responsibility.

The third design problem is native execution. Some contract workloads benefit from carefully audited native implementations, but direct host execution is not acceptable in consensus. TVM addresses this by requiring deterministic envelopes, resource limits, validation, metering, and a restricted host interface while remaining compatible with EVM accounts and calls.

## 2. Design goals

Tkmchain is designed around six goals:

- EVM compatibility: existing wallets, indexers, JSON-RPC clients, ABI tooling, contracts, and account semantics should continue to work.
- Accessible mining: RandomX should allow CPU-oriented mining and reduce dependence on specialized hardware.
- Predictable rewards: block rewards and fees should be distributed transparently among the Main King, current Rotating King, and miner.
- Operational governance: Rotating Kings should create a visible set of funded operators with monitoring and checkpoint responsibilities.
- Deterministic extensibility: TVM should allow native modules only when they are bounded, deterministic, metered, and sandboxed.
- Conservative security: every consensus feature should be testable, auditable, and activated through explicit chain configuration.

## 3. System overview

Tkmchain is implemented as a Go execution client named `gtkm`, derived from the Go Ethereum architecture. It retains Ethereum's world state, transactions, receipts, logs, RLP encoding, JSON-RPC interfaces, EVM execution model, and database architecture.

The Tkmchain-specific layers are:

- RandomX consensus for proof-of-work sealing and verification.
- RandomX seed-hash and work APIs for internal and external miners.
- A reward finalization path that pays Main King, Rotating King, and miner accounts.
- Rotating King configuration, state, registration, status, rotation, and checkpoint RPCs.
- TVM envelope, validation, RPC helpers, and an initial stateful TVM precompile.
- Keeper, a stateless execution validation command intended for zkVM guest use cases.

## 4. Consensus: RandomX proof-of-work

RandomX is selected because it is optimized for general-purpose CPUs and designed to resist ASIC centralization. Tkmchain configures RandomX with a 2,048-block epoch, a default 256 MB cache, a 2 GB dataset, and a 4 GB minimum memory target. The protocol targets 120-second block intervals.

Each block header is sealed with RandomX work derived from the block seal hash and the seed hash for the relevant block height. The node exposes work through `miner_getWork` and `randomx_getWork`, returning:

```text
[sealHash, seedHash, target, blockHeight]
```

External miners compute a RandomX digest using the provided seal hash and seed-selected cache, then submit solutions through `miner_submitWork`, `randomx_submitWork`, or raw hex submission helpers.

The consensus engine verifies seals, manages RandomX caches, calculates seal hashes, exposes hashrate and share counters, and finalizes rewards. A no-cgo fallback exists for builds where the native RandomX library is unavailable, which is important for tests and development environments.

## 5. Difficulty and block timing

The target block time is 120 seconds. Difficulty adjustment is designed to respond to observed block intervals while avoiding extreme one-block swings. The implementation contains an early linear progression phase and dynamic adjustment after the initial ramp. It also includes emergency difficulty behavior for long no-block intervals.

The roadmap requires this area to be treated as launch-critical. Difficulty rules must be specified once, tested across long simulations, and documented exactly as implemented. Stable difficulty behavior is essential for miner confidence, issuance predictability, and network liveness.

## 6. Issuance and reward distribution

Tkmchain starts with a 200 TKM block reward. At 120-second block targets, the halving interval is approximately four years. Reward calculation supports up to 64 halving periods and stops paying block subsidy once the reward falls below 1 TKM.

For each finalized block, the total reward is calculated as:

```text
totalReward = blockSubsidy + transactionFees
```

The total reward is split:

| Recipient | Share | Purpose |
| --- | ---: | --- |
| Main King | 10% | Leadership, checkpointing, and protocol stewardship |
| Rotating King | 40% | Scheduled governance operator reward |
| Miner | 50% | Proof-of-work security and block production |

If the Main King or Rotating King address is unavailable, reward fallback rules prevent funds from being assigned to an empty address. The intended behavior is to keep issuance deterministic and avoid accidental reward loss.

## 7. Rotating Kings

Rotating Kings are funded governance operators. A candidate registers through RPC, locks stake, enters the active schedule at a rotation boundary, and receives the 40 percent Rotating King reward when selected.

The current implementation exposes a default rotation interval of 100 blocks. At 120-second blocks, this is approximately 3 hours and 20 minutes per slot. RPC methods expose current king, next king, registered kings, status, lock fields, rotation history, and aggregate stats.

The Rotating King system has four purposes:

- decentralize operational responsibility beyond a single privileged operator;
- align governance operators with network uptime and monitoring;
- make reward allocation auditable at the protocol level;
- create a path for checkpoint and incident-response duties without changing EVM transaction semantics.

Main King RPC methods expose the configured Main King address and checkpoint submission. Checkpoints are expected to be used as operational safety anchors, not as a substitute for full block verification.

## 8. JSON-RPC surface

Tkmchain keeps standard Ethereum-style JSON-RPC behavior and adds Tkmchain-specific namespaces:

- `miner`: mining work, seed hash, and proof submission.
- `randomx`: RandomX-specific work, seed, height, hashrate, and raw submission helpers.
- `king`, `rk`, and `rotatingking`: registration, status, schedule, rotation history, and king stats.
- `mainking`: Main King address and checkpoint submission.
- `tvm`: TVM deployment validation and envelope construction.

Remote RPC exposure should be conservative. Mining and governance namespaces are operationally powerful and should be exposed only to trusted networks or protected infrastructure.

## 9. TVM: deterministic native contracts

TVM is a proposed native-contract layer for deterministic C++ modules that coexist with EVM bytecode contracts. It is not a second account model, not a new token standard, and not arbitrary host binary execution.

A TVM deployment is wrapped in an envelope containing:

- magic bytes identifying TVM code;
- version;
- deterministic target, currently `cpp-evm-v1`;
- code hash;
- metadata hash;
- memory page, stack slot, and call depth limits;
- module bytes and metadata bytes.

The current envelope limits are intentionally bounded: 24 KB maximum module size, 8 KB maximum metadata size, 256 memory pages, 1,024 stack slots, and 1,024 call depth. These limits make validation and execution easier to reason about while the system matures.

The initial TVM precompile is registered at:

```text
0x00000000000000000000000000000000000000f2
```

The current runtime supports a small deterministic conformance instruction set: return input, return code hash, load storage, and store storage. Static execution rejects storage writes. Future TVM work should compile safe templates to this bounded target rather than executing arbitrary native binaries.

## 10. EVM compatibility

Tkmchain's compatibility principle is simple: EVM users should not need to know whether a counterparty account uses EVM bytecode or a future TVM backend. Accounts, nonces, balances, value transfers, storage keys, logs, ABI encoding, return data, and revert data remain EVM-compatible.

TVM token contracts must therefore expose standard selectors and emit standard events. An ERC-20, ERC-721, or ERC-1155 implemented through TVM should be indistinguishable to wallets and indexers from an equivalent Solidity contract when viewed through ABI and event interfaces.

## 11. Security model

Tkmchain's security rests on four layers:

- Proof-of-work security from RandomX miners.
- Economic accountability from Main King and Rotating King reward roles.
- EVM compatibility and inherited testing depth from the Go Ethereum architecture.
- Deterministic validation and sandboxing for TVM modules.

The highest-risk areas are consensus seal verification, difficulty adjustment, reward finalization, registration and lock lifecycle, RPC exposure, and TVM runtime expansion. These areas require focused tests, fuzzing, simulation, independent review, and conservative activation.

TVM specifically must reject nondeterminism. A production TVM target must forbid unmanaged syscalls, threads, filesystem access, network access, wall-clock time, undefined memory behavior, unsupported floating point behavior, inline assembly, and any import that bypasses the metered host interface.

## 12. Keeper and stateless validation

Keeper is a specialized command for validating stateless execution of Ethereum-style blocks. It reads an RLP payload containing a block, witness data, and chain ID, executes the block without relying on local full state, and checks that computed state and receipt roots match the header.

This creates a future path for zkVM guest execution, independent block verification, and stateless validation research. Keeper is not required for base chain operation, but it can become an important verification tool as Tkmchain matures.

## 13. Economics

The economic model connects three participants:

- Miners provide work and receive 50 percent of block rewards.
- The Main King receives 10 percent for leadership and checkpoint duties.
- The current Rotating King receives 40 percent for scheduled operational governance.

This model intentionally pays both security providers and governance operators from the block reward. Its long-term health depends on broad miner participation, transparent Rotating King eligibility, predictable reward distribution, and clear operator responsibilities.

Before mainnet, the project should publish final supply, premine, block reward, halving, registration fee, stake lock, unlock, reward fallback, and governance parameters in one canonical document.

## 14. Roadmap

The development path should proceed in five stages:

1. Stabilization: reconcile parameters, expand consensus tests, harden RandomX and Rotating King edge cases, and complete operator documentation.
2. Public testnet: launch with mining, Rotating King registration, dashboards, faucet, explorer, and published incident reports.
3. TVM preview: complete the specification, build compiler tooling, expand conformance tests, and audit template contracts.
4. Mainnet release candidate: freeze genesis and binaries, complete audits, publish launch procedures, and run multi-week testnet validation.
5. Post-mainnet expansion: grow miner tooling, Rotating King automation, TVM templates, state management, and zkVM/stateless validation research.

## 15. Conclusion

Tkmchain combines the proven EVM execution model with a distinct consensus and governance design. RandomX makes block production CPU-oriented. Rotating Kings make governance operations visible and reward-bearing. TVM creates a path for deterministic native modules without abandoning EVM compatibility.

The project should now focus on disciplined stabilization: parameter reconciliation, test coverage, public testnet operations, security review, and clear documentation. If those foundations are completed carefully, Tkmchain can offer a credible EVM-compatible network with accessible mining, transparent governance rewards, and a conservative path toward native contract execution.

