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

## Institutional RPC

The daemon exposes a non-consensus RPC namespace named `tkminstitution`. It gives wallets, explorers, and institution portals a stable way to discover the deployed institutional suite and prepare ABI calldata without exposing admin keys in a website.

Default HTTP and WebSocket modules now include `tkminstitution`.

Useful calls:

```bash
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tkminstitution_status","params":[]}'
```

```bash
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tkminstitution_textHash","params":["Example University"]}'
```

```bash
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tkminstitution_registerInstitutionData","params":[{"admin":"0x1111111111111111111111111111111111111111","nameHash":"0x0000000000000000000000000000000000000000000000000000000000000000","institutionTypeHash":"0x0000000000000000000000000000000000000000000000000000000000000000","registrationHash":"0x0000000000000000000000000000000000000000000000000000000000000000","metadataHash":"0x0000000000000000000000000000000000000000000000000000000000000000","metadataURI":"ipfs://institution-metadata"}]}'
```

The calldata helpers do not send transactions. The returned `data` should be sent to `0x43aeb055883863cfe40804e386bec801b4ca63ec` using `eth_sendTransaction`, offline signing, or the node password RPC flow. This keeps institution admin and owner signing inside the daemon or wallet layer.

In `gtkm attach`, the web3 extension provides:

```javascript
tkminstitution.status()
tkminstitution.contractAddress()
tkminstitution.textHash("Example University")
tkminstitution.registerInstitutionData({...})
tkminstitution.issueCredentialData({...})
tkminstitution.issueInvoiceData({...})
```

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
