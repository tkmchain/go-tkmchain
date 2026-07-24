# Tkmchain whitepaper

Status date: July 2026

## Abstract

Tkmchain is an EVM-compatible execution network built around CPU-friendly RandomX proof-of-work, Rotating Kings governance, network-native phone infrastructure, and a secure native-contract roadmap called TVM. It preserves the Ethereum account model, transaction model, ABI, storage semantics, logs, and developer tooling while changing the consensus and reward layer to make mining accessible and governance participation explicit.

The protocol targets 120-second blocks, RandomX mining epochs of 2,048 blocks, and a block reward model that distributes value among miners and governance operators. Each block reward is split 10 percent to the Main King, 40 percent to the current Rotating King, and 50 percent to the miner. This creates a direct economic connection between network security, operational governance, and block production.

TKM Phone extends the network beyond balances and contracts by making phone-number identity, encrypted messaging, and call signaling part of the chain's RPC-backed service layer. TVM extends the roadmap beyond standard EVM bytecode by introducing a deterministic, metered, sandboxed native-contract path for audited C++ modules. TVM is designed as an execution backend for selected EVM accounts, not a second state model or a replacement for Solidity.

## 1. Motivation

Modern EVM networks often concentrate block production around specialized hardware, staking concentration, or infrastructure-heavy validator operations. Tkmchain takes a different path: it keeps EVM compatibility while using RandomX proof-of-work to favor general-purpose CPUs and broad participation.

The second design problem is governance. Many chains rely on off-chain social coordination without a protocol-visible operator role. Tkmchain introduces Rotating Kings: funded operators that rotate through a reward-bearing responsibility slot. The Main King anchors checkpoint and leadership duties, while Rotating Kings receive scheduled rewards and monitoring responsibility.

The third design problem is identity and communication. Wallet addresses are powerful but not human-friendly, and off-chain messaging/call systems introduce separate trust, billing, and identity layers. TKM Phone addresses this by issuing phone numbers from Main King generated buckets, binding number ownership to chain accounts, requiring device-key registration for use, and using RandomX-derived encryption/signaling primitives for messages and calls.

The fourth design problem is native execution. Some contract workloads benefit from carefully audited native implementations, but direct host execution is not acceptable in consensus. TVM addresses this by requiring deterministic envelopes, resource limits, validation, metering, and a restricted host interface while remaining compatible with EVM accounts and calls.

## 2. Design goals

Tkmchain is designed around seven goals:

- EVM compatibility: existing wallets, indexers, JSON-RPC clients, ABI tooling, contracts, and account semantics should continue to work.
- Accessible mining: RandomX should allow CPU-oriented mining and reduce dependence on specialized hardware.
- Predictable rewards: block rewards and fees should be distributed transparently among the Main King, current Rotating King, and miner.
- Operational governance: Rotating Kings should create a visible set of funded operators with monitoring and checkpoint responsibilities.
- Native communications: phone numbers, SIM device keys, encrypted messages, and call signaling should be network-visible while keeping private content encrypted.
- Deterministic extensibility: TVM should allow native modules only when they are bounded, deterministic, metered, and sandboxed.
- Conservative security: every consensus feature should be testable, auditable, and activated through explicit chain configuration.

## 3. System overview

Tkmchain is implemented as a Go execution client named `gtkm`, derived from the Go Ethereum architecture. It retains Ethereum's world state, transactions, receipts, logs, RLP encoding, JSON-RPC interfaces, EVM execution model, and database architecture.

The Tkmchain-specific layers are:

- RandomX consensus for proof-of-work sealing and verification.
- RandomX seed-hash and work APIs for internal and external miners.
- A reward finalization path that pays Main King, Rotating King, and miner accounts.
- Rotating King configuration, state, registration, status, rotation, and checkpoint RPCs.
- TKM Phone buckets, operator approvals, phone-number ownership, SIM/device keys, encrypted message storage, call signaling, and phone state persistence.
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
- `tkmphone`: phone buckets, pending operator approvals, registered numbers, device keys, encrypted messages, call signaling, contacts, blocking, recovery, notifications, and propagation.

Remote RPC exposure should be conservative. Mining and governance namespaces are operationally powerful and should be exposed only to trusted networks or protected infrastructure.


## 9. TKM Phone infrastructure

TKM Phone is a network-native communications layer built into `gtkm`. It provides phone-number identity, a transaction-based number marketplace, SIM/device registration, encrypted messages, and WebRTC call signaling without making the website or marketplace the source of authority. The authoritative state is held by the daemon and exposed through the `tkmphone` namespace.

The phone-number supply model is bucket based. Main King generates signed bucket batches. Each batch contains five buckets, and each bucket contains five phone numbers. A new batch can be generated only after the previous five buckets are bought. This prevents unbounded number issuance and makes number provenance auditable.

The operator purchase flow is transaction based:

1. An operator pays exactly `25,000 TKM` to Main King for one bucket.
2. The payment transaction carries a `TKMPHONE_BUCKET_V1` data marker, the operator key hash, and the expiry timestamp.
3. `gtkm` scans canonical Main King payments and derives pending operator approvals from chain data only.
4. Main King approves from the daemon through `gtkm tkmphone approve-operator` or the `tkmphone_approveOperatorPayment` RPC.
5. The operator wallet detects approval with `tkmphone_listOperators`, opens the assigned bucket, and reveals its five phone numbers.

This design deliberately keeps Main King custody out of websites. Public websites and wallets can display payments, pending state, buckets, numbers, and market actions, but Main King approval remains a private daemon operation. The daemon can list pending approvals with `gtkm tkmphone pending-approvals`, showing operator address, key hash, payment transaction, expiry, and grant hash.

Operators can sell individual numbers for the default `10,000 TKM` sale price. A valid sale requires a canonical buyer-to-operator payment transaction and transfers on-chain ownership of the number to the buyer. Once sold and registered, a number is permanently controlled by its owner and should not return to the market unless ownership is explicitly transferred by the owner.

SIM registration separates account ownership from device use. A phone number owner registers one or more device keys through owner-signed actions. Messages, call signaling, contacts, blocking, and recovery actions require the number owner or an active registered device key. This allows a downloaded SIM file or wallet-integrated SIM slot to operate a number while retaining owner-level control.

Messages are encrypted payload records stored through `tkmphone_sendEncryptedMessage` and related expiry-aware methods. Calls use browser WebRTC for audio transport and `gtkm` as encrypted signaling storage for offers, answers, and ICE candidates. The chain does not expose plaintext messages or audio; it stores encrypted metadata and verifies ownership/device signatures.

`gtkm` also writes a readable phone-state mirror under the node phone directory, normally `~/.tkmchain/gtkm/phone/state.json`, for operational inspection and recovery. The canonical source remains the chain database and `tkmphone` service state.

## 10. TVM: deterministic native contracts

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

## 11. EVM compatibility

Tkmchain's compatibility principle is simple: EVM users should not need to know whether a counterparty account uses EVM bytecode or a future TVM backend. Accounts, nonces, balances, value transfers, storage keys, logs, ABI encoding, return data, and revert data remain EVM-compatible.

TVM token contracts must therefore expose standard selectors and emit standard events. An ERC-20, ERC-721, or ERC-1155 implemented through TVM should be indistinguishable to wallets and indexers from an equivalent Solidity contract when viewed through ABI and event interfaces.

## 12. Security model

Tkmchain's security rests on five layers:

- Proof-of-work security from RandomX miners.
- Economic accountability from Main King and Rotating King reward roles.
- EVM compatibility and inherited testing depth from the Go Ethereum architecture.
- Daemon-owned TKM Phone approval, number ownership, device-key verification, encrypted message storage, and call signaling controls.
- Deterministic validation and sandboxing for TVM modules.

The highest-risk areas are consensus seal verification, difficulty adjustment, reward finalization, registration and lock lifecycle, RPC exposure, TKM Phone approval/signature flows, phone state propagation, and TVM runtime expansion. These areas require focused tests, fuzzing, simulation, independent review, and conservative activation.

TVM specifically must reject nondeterminism. A production TVM target must forbid unmanaged syscalls, threads, filesystem access, network access, wall-clock time, undefined memory behavior, unsupported floating point behavior, inline assembly, and any import that bypasses the metered host interface.

## 13. Keeper and stateless validation

Keeper is a specialized command for validating stateless execution of Ethereum-style blocks. It reads an RLP payload containing a block, witness data, and chain ID, executes the block without relying on local full state, and checks that computed state and receipt roots match the header.

This creates a future path for zkVM guest execution, independent block verification, and stateless validation research. Keeper is not required for base chain operation, but it can become an important verification tool as Tkmchain matures.

## 14. Economics

The economic model connects three participants:

- Miners provide work and receive 50 percent of block rewards.
- The Main King receives 10 percent for leadership and checkpoint duties.
- The current Rotating King receives 40 percent for scheduled operational governance.
- Phone operators buy Main King generated buckets for `25,000 TKM` and may sell individual numbers at the default `10,000 TKM` sale price.

This model intentionally pays both security providers and governance operators from the block reward. Its long-term health depends on broad miner participation, transparent Rotating King eligibility, predictable reward distribution, and clear operator responsibilities.

Before mainnet, the project should publish final supply, premine, block reward, halving, registration fee, stake lock, unlock, reward fallback, and governance parameters in one canonical document.

## 15. Roadmap

The development path should proceed in six stages:

1. Stabilization: reconcile parameters, expand consensus tests, harden RandomX and Rotating King edge cases, and complete operator documentation.
2. Public testnet: launch with mining, Rotating King registration, dashboards, faucet, explorer, and published incident reports.
3. Phone infrastructure: harden daemon-only approvals, bucket issuance, SIM/device keys, encrypted messages, WebRTC signaling, explorer views, and wallet automation.
4. TVM preview: complete the specification, build compiler tooling, expand conformance tests, and audit template contracts.
5. Mainnet release candidate: freeze genesis and binaries, complete audits, publish launch procedures, and run multi-week testnet validation.
6. Post-mainnet expansion: grow miner tooling, Rotating King automation, phone marketplace tools, TVM templates, state management, and zkVM/stateless validation research.

## 16. Conclusion

Tkmchain combines the proven EVM execution model with a distinct consensus and governance design. RandomX makes block production CPU-oriented. Rotating Kings make governance operations visible and reward-bearing. TVM creates a path for deterministic native modules without abandoning EVM compatibility.

The project should now focus on disciplined stabilization: parameter reconciliation, test coverage, public testnet operations, security review, and clear documentation. If those foundations are completed carefully, Tkmchain can offer a credible EVM-compatible network with accessible mining, transparent governance rewards, and a conservative path toward native contract execution.

