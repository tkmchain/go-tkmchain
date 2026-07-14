# Tkmchain development roadmap

Status date: July 2026

Tkmchain is being developed as a practical, EVM-compatible execution network with three defining pillars: RandomX proof-of-work, Rotating Kings governance and rewards, and a secure TVM native-contract path for deterministic C++ modules. The goal is not to replace the EVM ecosystem, but to extend it with CPU-friendly mining, transparent reward participation, and a bounded native execution layer that keeps EVM accounts, storage, ABI, logs, and tooling intact.

## Strategic direction

Tkmchain should become a chain that ordinary operators can mine, validators and governance participants can monitor, and EVM developers can use without learning a new account model. Development should therefore prioritize consensus correctness, operational clarity, stable RPC surfaces, security review, and compatibility with wallets, indexers, miners, and contract tooling.

The roadmap is organized around five workstreams:

1. Consensus and mining: harden RandomX sealing, seed handling, work submission, difficulty adjustment, and block reward finalization.
2. Governance and rewards: turn Rotating Kings into a reliable operational governance system with clear eligibility, reward accounting, checkpointing, and monitoring duties.
3. Developer platform: preserve EVM compatibility while adding TVM deployment helpers, precompile execution, and deterministic native-contract templates.
4. Node operations: improve builds, releases, observability, external mining, configuration, documentation, and safe RPC defaults.
5. Security and decentralization: expand tests, audits, generated-code checks, bad-dependency checks, fuzzing, economic simulations, and launch procedures.

## Current foundation

The codebase already contains the core foundation for the Tkmchain identity:

- `gtkm` is the main node command, with Tkmchain branding, RandomX mining flags, and JSON-RPC support.
- Chain configuration includes chain ID `8979`, RandomX configuration, RandomX transaction activation, a Main King address, and a default 100-block Rotating King interval.
- RandomX consensus code covers sealing, seal verification, seed hashes, mining work, share submission, hashrate tracking, and a no-cgo fallback path.
- Block reward logic starts at 200 TKM, targets 120-second blocks, halves over roughly four-year periods, and splits rewards 10 percent to Main King, 40 percent to Rotating King, and 50 percent to the miner.
- Rotating King APIs expose registration, status, king lists, rotation history, current/next king views, and Main King checkpoint submission.
- External miner integration exposes `miner_*` and `randomx_*` work APIs and a local stratum bridge path.
- TVM has an initial secure deployment envelope, resource limits, hashes, RPC deployment helpers, and a stateful precompile at `0x00000000000000000000000000000000000000f2`.
- Keeper provides a stateless execution validation command designed for zkVM guest environments.

## Phase 1: stabilization and launch readiness

Objective: make the current chain behavior predictable, testable, and operator-safe.

### Consensus hardening

- Freeze launch-critical chain parameters: chain ID, genesis hash, RandomX activation height, reward schedule, halving schedule, target block time, and fork schedule.
- Reconcile all RandomX difficulty documentation and code paths so operators see one canonical adjustment policy.
- Expand tests for RandomX seed hash boundaries, epoch transitions, block timestamp edge cases, invalid nonces, invalid mix digests, bootstrap behavior, and cgo/no-cgo parity.
- Verify that `Finalize` and `FinalizeAndAssemble` produce identical state roots for reward distribution, user transactions, receipts, and Rotating King state.
- Add long-run private-network simulations for difficulty stability under hashrate changes, offline miners, and delayed blocks.

### Rotating Kings hardening

- Finalize eligibility rules: required stake, registration fee, lock duration, unlock height, activation height, removal rules, and reward fallback behavior.
- Ensure documentation, code comments, RPC output, and tests agree on the stake threshold and lifecycle.
- Add tests for late registration, duplicate registration, unfunded candidates, expired locks, rotation boundary activation, empty king lists, and reward fallback to Main King or miner.
- Add structured event/log output for registration, activation, removal, rotation, and checkpoint submission.
- Define operational responsibilities for Main King and Rotating Kings, including checkpoint cadence, monitoring categories, and incident response expectations.

### Node and miner operations

- Publish reproducible Linux, macOS, and Windows builds for `gtkm` and supporting tools.
- Document CPU, memory, disk, and network requirements for full nodes, archive nodes, RPC nodes, and miners.
- Harden external mining APIs with clear local-only defaults and explicit guidance for remote stratum deployments.
- Add miner compatibility notes for RandomX seed hashes, work tuple format, nonce encoding, target comparison, and submission retry behavior.
- Provide a single-node devnet recipe and a multi-node private network recipe for testing Rotating Kings and mining.

### Security baseline

- Run the full repository checks before release: formatting, build, tests, lint, generated-code checks, and bad-dependency checks.
- Add release checklists for binaries, hashes, tags, genesis files, bootnodes, default configs, and documentation.
- Schedule focused review of RandomX consensus, reward finalization, Rotating King RPC mutability, and TVM precompile behavior.

## Phase 2: public testnet and ecosystem onboarding

Objective: move from implementation to measured network behavior with real operators.

### Testnet launch

- Launch a public testnet with published genesis, bootnodes, explorer configuration, RPC endpoints, faucet, mining guide, and Rotating King registration guide.
- Track block interval distribution, orphan rate, difficulty response, miner diversity, RPC latency, peer count, chain growth, and state growth.
- Run controlled hashrate shock tests to validate difficulty behavior and emergency adjustment behavior.
- Create public dashboards for chain height, hashrate estimate, difficulty, active peers, current Rotating King, next rotation height, and reward distribution.

### Governance operations

- Enroll a first cohort of Rotating King operators with documented responsibilities.
- Validate registration, lock, unlock, rotation, and reward flows on testnet.
- Publish a transparent incident process for missed checkpoints, stale nodes, RPC failures, and suspected consensus issues.
- Define Main King succession, emergency checkpointing, and parameter-upgrade procedures before mainnet.

### Developer onboarding

- Publish examples for deploying ERC-20, ERC-721, and ERC-1155 contracts using standard EVM tools.
- Publish JSON-RPC examples for RandomX mining, Rotating King status, Main King checkpointing, and TVM deployment preparation.
- Provide SDK snippets using existing Ethereum clients where possible, avoiding Tkmchain-only wrappers unless they add real value.
- Add compatibility matrices for wallets, block explorers, indexers, contract frameworks, and mining tools.

## Phase 3: TVM developer preview

Objective: prove native deterministic modules can coexist with EVM contracts without weakening consensus safety.

### TVM specification

- Complete the TVM envelope specification: magic, version, target, module hash, metadata hash, exported selectors, resource limits, and optional attestations.
- Specify the deterministic target accepted by the chain and ban nondeterministic behavior such as syscalls, threads, wall-clock time, filesystem access, network access, unsafe floating point behavior, inline assembly, and undefined memory behavior.
- Define a consensus gas schedule for module validation, CPU steps, memory, storage, logs, calls, and contract creation.
- Document how TVM calls map to EVM `CALL`, `STATICCALL`, `DELEGATECALL`, return data, revert data, logs, and storage keys.

### Runtime and tooling

- Extend the current conformance runtime beyond storage and hash-return opcodes toward a validated deterministic module format.
- Add compiler tooling that emits the accepted TVM target with metadata and resource declarations.
- Add `tvm_validateDeployment` and `tvm_buildDeployment` examples with realistic module bytes, metadata, limits, and failure cases.
- Add differential tests comparing TVM templates against equivalent Solidity contracts.
- Prepare audited native templates for ERC-20, ERC-721, ERC-1155, access control, pausing, and reentrancy protection.

### Safety gates

- Keep TVM opt-in by chain configuration and fork activation.
- Require conformance tests before accepting new runtime versions.
- Require independent review before enabling production TVM contract deployment beyond constrained templates.

## Phase 4: mainnet release candidate

Objective: make the network launchable and maintainable.

- Freeze genesis, bootnodes, binaries, default config, RPC defaults, and chain parameters.
- Complete third-party audits for RandomX consensus integration, reward accounting, Rotating Kings, TVM precompile surface, and release process.
- Run multi-week public testnet with stable metrics and published postmortems for any incidents.
- Prepare exchange, explorer, wallet, miner, and infrastructure operator documentation.
- Publish Main King and Rotating King operator keys, procedures, backup policies, and monitoring requirements.
- Publish final economic parameters: premine, block reward, halving, fee handling, reward fallback, registration fee, stake lock, and governance scope.

## Phase 5: post-mainnet expansion

Objective: scale the ecosystem without compromising the consensus base.

- Add more Rotating King automation: alerts, dashboards, rotation proofs, reward accounting exports, and historical reports.
- Add miner pool support, pool operator documentation, and share-accounting reference integrations.
- Improve state growth controls, archival workflows, pruning guidance, and snapshot distribution.
- Expand TVM from preview to production templates after audits and testnet validation.
- Investigate zkVM-backed stateless validation using Keeper for light clients, fraud proofs, or independent execution verification.
- Continue upstream compatibility work with modern Ethereum fork rules where it is compatible with Tkmchain's RandomX execution model.

## Success metrics

The roadmap should be measured by concrete outcomes:

- Consensus: no known state-root divergence across supported platforms and build modes.
- Mining: stable 120-second target behavior across changing hashrate conditions.
- Decentralization: growing count of independent miners, nodes, RPC operators, and Rotating Kings.
- Governance: clear rotation history, predictable rewards, and auditable checkpoint operations.
- Developer experience: standard EVM deployments work without custom account or ABI assumptions.
- Security: release candidates pass full tests, lint, generated-code checks, bad-dependency checks, fuzz targets, and independent review.
- Operations: operators can build, sync, mine, register, monitor, and recover nodes using published documentation.

## Near-term priorities

The next development cycle should focus on the smallest set of work that turns the existing implementation into a credible public testnet:

1. Reconcile parameters and documentation for Rotating King eligibility and RandomX difficulty.
2. Expand consensus and reward tests around edge cases and fork boundaries.
3. Publish a complete testnet operator guide with genesis, bootnodes, mining, RPC, and Rotating King registration.
4. Add chain dashboards and structured metrics for RandomX, rewards, rotations, and RPC health.
5. Run a public testnet with documented incidents, fixes, and measurable stability targets.

