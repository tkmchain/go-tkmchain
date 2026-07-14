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

## How to create a TVM smart contract

A TVM smart contract is created with the same account model as an EVM contract: the deployment transaction has no `to` address, consumes gas, increments the sender nonce, and returns a `contractAddress` in the transaction receipt. The difference is the deployment bytecode. For TVM, the transaction data must be a validated TVM envelope, not EVM initcode.

### What is needed

A deployer needs the following inputs before sending the deployment transaction:

| Input | Required | Description |
| --- | --- | --- |
| `code` | yes | Compiled deterministic TVM module bytes. In the current implementation this is the bounded TVM instruction/module payload accepted by `core/tvm`. Future C++ tooling should emit these module bytes. |
| `metadata` | no | ABI, compiler settings, source metadata, or verification information. Metadata is stored inside the envelope and committed by `metadataHash`. |
| `memoryPages` | yes | Maximum TVM linear memory pages. Must be in `[1, 256]`. |
| `stackSlots` | yes | Maximum TVM stack slots. Must be in `[1, 1024]`. |
| `callDepth` | yes | Maximum nested TVM call depth. Must be in `[1, 1024]`. |
| funded deployer account | yes | The account sending the creation transaction must have enough TKM to pay gas and any value sent with the contract. |
| enabled RPC namespaces | yes | Use `eth` for sending/checking transactions and `tvm` for building or viewing TVM envelopes. Start HTTP with `--http.api eth,net,web3,tvm` or include `tvm` in any custom API list. |

The TVM envelope contains:

- `magic`: `TVM\0`, used by contract creation to detect a TVM deployment;
- `version`: currently `1`;
- `target`: currently `cpp-evm-v1`;
- `codeHash`: Keccak-256 hash of the module bytes;
- `metadataHash`: Keccak-256 hash of metadata bytes;
- resource limits;
- module bytes and metadata bytes.

### Step 1: build the deployment envelope

Use `tvm_buildDeployment` to validate the module and wrap it in a deployable envelope. The `code` and `metadata` fields are hex bytes.

```sh
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"tvm_buildDeployment",
    "params":[{
      "code":"0x01",
      "metadata":"0x",
      "memoryPages":1,
      "stackSlots":16,
      "callDepth":4
    }],
    "id":1
  }' | jq '.'
```

The response includes `deploymentCode`. This is the exact byte string that must be sent as the contract creation transaction `data`/`input`.

Example response shape:

```json
{
  "version": 1,
  "target": "cpp-evm-v1",
  "codeHash": "0x...",
  "metadataHash": "0x...",
  "deploymentCode": "0x54564d00..."
}
```

The `deploymentCode` starts with `0x54564d00`, which is `TVM\0` in hex. If the deployment data does not start with this magic prefix, normal EVM contract creation rules apply instead.

### Step 2: send a contract creation transaction

Send an Ethereum-style contract creation transaction with:

- `from`: deployer address;
- no `to` field;
- `data`: the `deploymentCode` returned by `tvm_buildDeployment`;
- enough `gas` to pay base creation cost and code storage cost;
- optional `value` if the contract account should be funded at creation.

Example with an unlocked local account:

```sh
DEPLOYMENT_CODE="0x54564d00..."
FROM="0xYourDeployerAddress"

curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"method\":\"eth_sendTransaction\",
    \"params\":[{
      \"from\":\"$FROM\",
      \"data\":\"$DEPLOYMENT_CODE\",
      \"gas\":\"0x100000\"
    }],
    \"id\":1
  }" | jq '.'
```

For production deployments, sign the transaction offline and submit it with `eth_sendRawTransaction`.

### Step 3: confirm the receipt

After the transaction is mined, check the receipt:

```sh
TX="0xYourDeploymentTransactionHash"

curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"method\":\"eth_getTransactionReceipt\",
    \"params\":[\"$TX\"],
    \"id\":1
  }" | jq '.'
```

A successful deployment has:

- `status: "0x1"`;
- `contractAddress` set to the new TVM contract account;
- gas used for transaction execution and code storage.

### Step 4: verify that code was stored

The chain stores the full TVM envelope as the account code. `eth_getCode` therefore returns the stored envelope bytes:

```sh
CONTRACT="0xYourContractAddress"

curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"method\":\"eth_getCode\",
    \"params\":[\"$CONTRACT\",\"latest\"],
    \"id\":1
  }" | jq -r '.result'
```

The result should be non-empty and should begin with `0x54564d00`.

For a decoded TVM-specific view, use `tvm_getCode`:

```sh
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"method\":\"tvm_getCode\",
    \"params\":[\"$CONTRACT\",\"latest\"],
    \"id\":1
  }" | jq '.'
```

`tvm_getCode` returns the decoded module bytes as `code`, metadata as `metadata`, the raw stored envelope as `envelope`, and the hashes/limits declared at deployment.

If the caller only needs the module bytes, use `tvm_getWasm`:

```sh
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"method\":\"tvm_getWasm\",
    \"params\":[\"$CONTRACT\",\"latest\"],
    \"id\":1
  }" | jq '.'
```

### Contract creation behavior inside the VM

During contract creation, the VM checks the creation data before running EVM initcode:

1. If the data starts with `TVM\0`, the VM treats it as a TVM deployment envelope.
2. The envelope is decoded and validated with `tvm.UnmarshalBinary`.
3. The VM charges normal contract code storage gas for the full envelope size.
4. The original envelope is stored with `StateDB.SetCode(contractAddress, envelope)`.
5. The transaction receipt reports the created `contractAddress` just like an EVM contract deployment.

If the data does not start with `TVM\0`, creation continues through the normal EVM initcode path. This keeps existing EVM contract deployment behavior unchanged.

### Common errors

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `tvm_getCode` returns `method does not exist` | The node was started without the `tvm` RPC namespace, or the running binary is old. | Rebuild/restart and include `tvm` in `--http.api`, for example `--http.api eth,net,web3,tvm`. |
| Receipt has `status: "0x1"` but `eth_getCode` returns `0x` | The node is running a binary without TVM envelope storage support, or the contract was deployed before the fix. | Rebuild/restart and redeploy. Existing empty-code contracts are not repaired automatically. |
| Deployment fails with invalid target/version/hash | The `deploymentCode` is not a valid TVM envelope or was modified after `tvm_buildDeployment`. | Rebuild the envelope with `tvm_buildDeployment` and send the returned `deploymentCode` unchanged. |
| Deployment runs as EVM instead of TVM | The data does not start with the TVM magic prefix `0x54564d00`. | Use `deploymentCode` from `tvm_buildDeployment`, not raw module bytes. |
| Out of gas during deployment | Gas limit does not cover contract creation and envelope code storage. | Increase the transaction gas limit. |

### Minimal deployment checklist

1. Compile or prepare deterministic TVM module bytes.
2. Call `tvm_validateDeployment` or `tvm_buildDeployment` with safe limits.
3. Send a contract creation transaction with `data` equal to `deploymentCode` and no `to` field.
4. Wait for a receipt with `status: "0x1"` and `contractAddress`.
5. Confirm `eth_getCode(contractAddress, "latest")` returns bytes beginning with `0x54564d00`.
6. Use `tvm_getCode` or `tvm_getWasm` to inspect the decoded TVM contract code.
