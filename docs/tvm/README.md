# TVM secure native contract architecture

TVM is a proposed native-contract layer that lets audited C++ contract modules coexist with EVM bytecode contracts without changing EVM account, transaction, token, or ABI semantics. TVM code must be deterministic, metered, sandboxed, and callable through the same message-call rules used by EVM contracts.

## Goals

- Support native C++ implementations for contract logic that can create and manage EVM-standard tokens, including ERC-20-compatible fungible tokens and ERC-721/ERC-1155-compatible NFTs.
- Preserve coexistence with EVM contracts by using EVM addresses, ABI encoding, logs, revert data, storage keys, account balances, and call/create semantics.
- Keep consensus safety first: every TVM operation must be deterministic, gas-metered, bounded, and independent of host-specific behavior.
- Make TVM opt-in at the chain-configuration and deployment layers so existing EVM contracts continue to execute unchanged.

## Non-goals

- TVM does not execute arbitrary host binaries directly inside consensus.
- TVM does not introduce a second account model, token standard, or transaction type.
- TVM does not bypass EVM authorization, nonce, balance, gas, or state-transition rules.

## Execution model

A TVM contract is deployed as EVM account code with a TVM envelope. The envelope identifies the TVM version, compiler target, code hash, metadata hash, exported ABI selectors, and declared resource limits. Calls to a TVM account use normal EVM `CALL`, `STATICCALL`, `DELEGATECALL`, and contract-creation entry points, then dispatch through the TVM runtime instead of the EVM interpreter.

The TVM runtime must expose only a small deterministic host interface:

- read and write contract storage;
- read call context, block context, and transaction context already available to EVM contracts;
- emit EVM logs;
- perform metered calls and contract creation through the EVM state transition;
- return or revert with ABI-compatible data;
- charge gas for CPU, memory, storage, calls, logs, and validation.

## Deployment structure

A TVM deployment is valid only when all fields are present and validated before the account code is accepted:

| Field | Purpose |
| --- | --- |
| `magic` | Distinguishes TVM envelopes from EVM bytecode. |
| `version` | Selects the consensus TVM runtime and ABI rules. |
| `target` | Names the deterministic C++ compilation target accepted by the chain. |
| `codeHash` | Commits to the compiled TVM module bytes. |
| `metadataHash` | Commits to source metadata, compiler settings, and declared interfaces. |
| `exports` | Lists callable selectors and mutability for ABI dispatch. |
| `limits` | Declares maximum stack, memory, call depth, code size, and validation budget. |
| `signatureSet` | Optional governance or allow-list attestations for permissioned deployments. |

## Token and NFT compatibility

TVM token contracts must use EVM-compatible ABIs and event topics. A TVM ERC-20, ERC-721, or ERC-1155 implementation is therefore indistinguishable to wallets, indexers, and EVM contracts from an equivalent Solidity implementation when it exposes the required selectors and emits the required events.

Security-critical token behavior must be implemented through shared TVM libraries or audited templates where possible:

- supply accounting must check overflow and underflow before state writes;
- transfers must update balances before external calls;
- approvals must follow the relevant EVM token standard exactly;
- NFT ownership and operator approvals must remain canonical in EVM storage;
- metadata and royalty extensions must be explicit opt-ins, not implicit runtime behavior.

## Security requirements

TVM consensus execution must reject any module that depends on undefined, platform-specific, or non-deterministic C++ behavior. The accepted target must forbid or replace features that cannot be made deterministic, including unmanaged system calls, threads, wall-clock time, filesystem access, network access, floating-point behavior without a fixed specification, inline assembly, and undefined memory behavior.

The runtime must enforce the following controls:

1. **Sandboxing:** TVM modules run without direct host access. All state, call, and log operations go through metered host functions.
2. **Deterministic validation:** module validation checks code size, imports, memory layout, exported selectors, and banned instructions before deployment.
3. **Gas metering:** every instruction class and host function has a consensus gas schedule. Execution stops before resource limits are exceeded.
4. **Memory safety:** linear memory is bounds-checked, initialized deterministically, and capped by the declared deployment limit.
5. **Reentrancy visibility:** TVM follows EVM call ordering, so templates should expose standard guards for token minting, transfers, and callbacks.
6. **Static calls:** `STATICCALL` mode forbids storage writes, value transfers, contract creation, and log emission exactly as required by EVM semantics.
7. **State compatibility:** storage keys, logs, revert data, and return values remain byte-for-byte compatible with EVM tooling.
8. **Upgrade control:** runtime versions are activated by fork rules only; deployed contracts keep the version declared in their envelope unless governance defines an explicit migration.

## Coexistence with EVM

EVM and TVM contracts share the same world state and can call each other through standard ABI calls. The caller does not need to know whether the callee is EVM bytecode or TVM code. Gas, return data, reverts, logs, and value transfers propagate through the existing EVM call frame rules.

This keeps TVM an execution backend for selected contract accounts, not a separate chain environment. Existing EVM tooling can continue to inspect accounts, decode calls, index events, and verify token behavior using the same standards it already supports.

## Implementation phases

1. Define the TVM envelope, validation rules, gas schedule, and host interface as a specification.
2. Add chain-configuration gates and fork activation rules.
3. Implement a deterministic TVM validator and runtime behind the existing EVM call dispatcher.
4. Add conformance tests for ABI compatibility, token standards, gas accounting, reverts, logs, storage, and cross-calls.
5. Add audited ERC-20, ERC-721, and ERC-1155 C++ templates that compile to the accepted TVM target.
6. Add differential tests against equivalent EVM contracts to prove coexistence and tool compatibility.

## GTKM RPC integration

GTKM exposes TVM preparation helpers through the `tvm` JSON-RPC namespace:

- `tvm_validateDeployment` checks that a compiled deterministic C++ module is non-empty, bounded by TVM size limits, and uses safe resource limits.
- `tvm_buildDeployment` validates the same input and returns EVM account deployment code containing the TVM envelope and module bytes.

The RPC input accepts compiled module bytes as `code`, optional ABI/compiler metadata as `metadata`, and explicit `memoryPages`, `stackSlots`, and `callDepth` limits. The output includes the TVM version, accepted target, code hash, metadata hash, and deployable envelope bytes.

## Runtime and precompile integration

The initial TVM runtime is exposed through the TVM precompile at `0x00000000000000000000000000000000000000f2`. The precompile accepts a validated TVM envelope, charges deterministic gas based on input size, decodes the envelope, and executes the bounded TVM runtime through a restricted host environment.

The host environment currently exposes storage load and storage store operations scoped to the TVM precompile account. Static execution rejects storage writes, preserving EVM `STATICCALL` semantics. The first runtime target supports deterministic conformance opcodes for returning call input, returning the code hash, reading storage, and writing storage; future C++ tooling should compile safe contract templates to this bounded target rather than executing arbitrary native binaries.
