# TKMChain Smart Accounts: Pre-Deployment Security Review

Date: 2026-07-30  
Target network: TKMChain mainnet, chain ID 8979  
Status: **DO NOT DEPLOY — independent audit required**

## Scope

- `contracts/smartaccount/TKMSmartAccounts.sol`
- `contracts/smartaccount/TKMPoolTreasurySmartAccount.sol`
- generated ABI and bytecode under `contracts/smartaccount/artifacts/`
- `tkmaccount` RPC hash and calldata builders
- `gtkm smartaccount` CLI integration

This is an internal pre-deployment review. It is not an independent audit and must not be described publicly as one.

## Toolchain

- Solidity compiler: `0.8.30+commit.73712a01`
- Slither: `0.11.6`
- Go focused tests: `go test ./eth -run SmartAccount -count=1`
- Reproducible build: `./contracts/smartaccount/compile.sh`

## Reviewed Source Hashes

```text
2fa1f44c217a959540fc6a23945666ff506bc397a7210ecbc53d6e344cfa94b5  TKMSmartAccounts.sol
7a0c23beeff0fffab918e2d83de1819e24dcbc75551a9d0be0b7ce88745d9dbc  TKMPoolTreasurySmartAccount.sol
```

## Reviewed Artifact Hashes

```text
3a8c9f447c4b156c5b165e90d2f83c304b4dc4bd3fae01e5d438e65878134381  TKMEntryPoint.bin
6a83c9deddf87349578332be66f19780fa6dd27377047fe6e2518dff4e6ec6f6  TKMPoolTreasurySmartAccount.bin
```

Any source or artifact change invalidates this review and requires all analysis and tests to be rerun.

## Material Findings Fixed

### SA-01: Policy Changes Could Use One Signature

Severity: High  
Status: Fixed

Account self-calls used `value == 0`, so they previously required only one owner even when the configured owner threshold was greater than one. This could allow one compromised owner to replace all owners, weaken recovery, raise spending limits, or create an unrestricted session.

The authorization rule now requires the full owner threshold whenever the operation target is the account itself. High-value operations also continue to require the full threshold.

### SA-02: Recovery Did Not Invalidate Sessions

Severity: High  
Status: Fixed

A compromised session key could remain active after guardian recovery. The account now maintains a session epoch. Every session records its creation epoch, and successful recovery increments the global epoch, invalidating all earlier sessions without iteration.

### SA-03: Recovery Proposal Griefing

Severity: Medium  
Status: Fixed

One guardian could previously replace an active recovery proposal with a different owner hash, resetting approvals and delaying recovery indefinitely. While a proposal is active, approvals must now reference the same owner hash. A different recovery requires cancellation or completion first.

### SA-04: Unbounded Return-Data Copy

Severity: Medium  
Status: Fixed

EntryPoint and account execution returned arbitrary target returndata. A malicious target could create excessive return data and consume relayer gas during decoding. Account execution no longer returns target data, and unsuccessful target calls return a bounded error.

### SA-05: Missing Paymaster Validation And Audit Events

Severity: Low  
Status: Fixed

The paymaster constructor now rejects a zero signer, explicitly implements `ITKMPaymaster`, and emits events for ownership, signer, and pause changes.

### SA-06: Missing Explicit Execution Target Check

Severity: Low  
Status: Fixed

Account validation already rejected a zero target. Execution now repeats the zero-address check at its own trust boundary.

## Remaining Static-Analysis Findings

### Reentrancy Warnings

Disposition: Guarded, requires independent verification

Slither reports state writes after external calls in EntryPoint. `handleOperation` sets `entered = true` before validation and rejects every nested `handleOperation` call. A revert restores the lock and nonce atomically. Nonce increments before target execution. Independent reviewers must verify this reasoning and test malicious account, paymaster, and target contracts.

### Event After External Call

Disposition: Expected

Execution events are emitted only after successful target execution. Account spending state is updated before the call. A failed call reverts state and emits no success event.

### Timestamp Comparisons

Disposition: Expected

Timestamps enforce operation expiry, session validity, daily spending windows, recovery delays, and sponsorship expiry. These checks do not depend on exact sub-minute timing. Independent review should confirm accepted miner timestamp tolerance.

### Low-Level Target Call

Disposition: Required design primitive

The account must call arbitrary owner-authorized targets. Target, selector, value, calldata, nonce, chain ID, EntryPoint, expiry, gas limit, paymaster, and paymaster data are committed by the signed operation hash.

### Hash Equality In Recovery

Disposition: Required

Strict equality verifies that finalized owners and threshold exactly match the guardian-approved recovery commitment.

### Inline Assembly In ECDSA Recovery

Disposition: Requires independent verification

Assembly reads the standard 65-byte signature fields. The implementation checks signature length, valid `v`, and low `s` before `ecrecover`.

## Required Tests Before Deployment

- malicious account reentrancy into EntryPoint;
- malicious paymaster reentrancy and revert behavior;
- malicious target reentrancy into account and EntryPoint;
- owner signature ordering, duplication, high-`s`, bad-`v`, and malformed ABI fuzzing;
- nonce replay and cross-chain/EntryPoint replay;
- full-threshold enforcement for every account self-call;
- session per-call, cumulative value, selector, target, validity, revocation, and recovery-epoch invalidation;
- daily-limit rollover and limit reduction below already-spent value;
- guardian threshold, duplicate approval, proposal conflict, cancellation, delay, and finalization;
- paymaster selector allowlist, expiry, chain binding, signer rotation, pause, and malformed data;
- deterministic factory address and duplicate deployment;
- gas-bound execution, revert behavior, and denial-of-service fuzzing;
- invariant tests showing unauthorized actors cannot change owners or move value;
- testnet deployment with a funded relayer and no material funds.

## Independent Audit Gate

Before mainnet deployment, an independent Solidity security auditor must receive:

1. the exact source hashes in this report;
2. compiler version and build script;
3. ABI and bytecode artifacts;
4. threat model and trust assumptions;
5. internal findings and Slither JSON output;
6. full test and fuzz results;
7. proposed deployment and verification procedure.

The auditor must publish a signed report listing scope, commit/source hashes, findings, fixes, retest results, and deployment recommendation. All auditor findings must be resolved or explicitly accepted with documented risk.

## Deployment Gate

Mainnet deployment is blocked until all conditions below are true:

- independent audit report received and verified;
- audited source hashes equal deployment source hashes;
- all high and medium findings closed;
- tests and fuzzing pass on frozen source;
- testnet deployment and operational rehearsal completed;
- deployment wallet, nonce, gas settings, and constructor arguments reviewed by two people;
- deployed runtime bytecode matches the audited artifact;
- EntryPoint and pool owner getters verified on-chain;
- deployment transaction hashes and addresses published;
- only a low-value canary deposit is used initially;
- emergency lock, recovery cancellation, and session revocation rehearsal completed.

No mainnet deployment transaction was submitted during this review.
