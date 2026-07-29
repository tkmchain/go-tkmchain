# TVM Institutional Suite Integration

TKMChain now has a deployed application-layer institutional suite and a TVM manifest path for explorer-visible institutional tooling.

## Mainnet Application Contract

```text
Contract: TkmInstitutionalSuite
Address:  0x43aeb055883863cfe40804e386bec801b4ca63ec
Tx hash:  0xcad679cf00644ec75008d79c2f104bde5584fe5e4f66a2987dd137d9730de12a
Block:    0x3122
Owner:    0x4441d6fEd0836B77a503e0B2788bfEd6FD8c23A8
```

The deployed contract supports:

- verified institution registration;
- institution admin rotation;
- institution suspension and revocation;
- document hash issuance and revocation;
- credential hash issuance and revocation;
- invoice issuance and TKM settlement;
- escrow with buyer, seller, and arbitrator;
- procurement record publication;
- grant and scholarship record publication;
- audit disclosures with previous-hash linking.

## TVM Position

The current TVM runtime target, `cpp-evm-v1`, is intentionally strict. It accepts bounded deterministic module forms only:

- return call input;
- return committed code hash;
- storage load;
- storage store.

Because full institutional registry execution would require more runtime opcodes, ABI dispatch, event emission, storage schemas, and gas metering, forcing all institutional logic directly into TVM today would become consensus-sensitive.

The safe implementation is therefore:

1. Deploy the institutional suite as normal application contract logic.
2. Add TVM manifest contracts that commit to the suite metadata and deployed contract address.
3. Display both the application contract and TVM manifest in the explorer.
4. Expand TVM later through a deliberate runtime version and tests when native registry execution is ready.

## TVM Manifest Fixture

Fixture:

```text
contracts/tvm/institutional_suite_manifest.cpp
```

The fixture documents the institutional modules and emits the current deterministic `ReturnCodeHash` module. The TVM envelope metadata commits to:

- manifest name;
- deployed application contract address;
- supported institutional modules;
- target runtime;
- source file.

This gives TVM a real role now: verifiable metadata, code-hash commitment, and explorer visibility without changing consensus rules.

## Focused Test

```bash
go test ./eth -run TestTVMInstitutionalSuiteManifestDeployAndRead
```

The test builds a TVM envelope for the institutional manifest, deploys it through the normal VM creation path, and verifies that `tvm_getCode` returns the expected metadata and full stored envelope.

## Future Native TVM Expansion

A future TVM runtime can make institutional modules fully native by adding a new TVM target version with:

- deterministic ABI selector dispatch;
- typed event emission;
- bounded storage read/write schemas;
- metered string and bytes handling;
- caller authorization helpers;
- safe value transfer helpers;
- conformance tests against the EVM application contract;
- explorer decoding for native TVM institutional events.

That should be done as an explicit TVM runtime upgrade, not as an accidental change hidden inside app tooling.
