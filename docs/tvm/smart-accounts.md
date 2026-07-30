# TKMChain Smart Accounts

TKMChain smart accounts provide multisignature authorization, spending limits, restricted session keys, delayed guardian recovery, deterministic account creation, and allowlisted gas sponsorship. They are implemented entirely with ordinary contracts and non-consensus RPC helpers.

## Hardfork And Consensus Status

No hardfork is required.

The implementation does not modify block validation, transaction formats, gas rules, RandomX, Rotating King selection, rewards, fork activation, or state-transition semantics. Smart-account operations are submitted as ordinary contract transactions and are validated by every node using existing execution rules.

The `tkmaccount` RPC only constructs calldata and hashes. It does not sign, unlock accounts, submit transactions, or modify state.

## Components

Source: `contracts/smartaccount/TKMSmartAccounts.sol`

- `TKMEntryPoint` verifies operation nonce, expiry, account authorization, and optional sponsorship before execution.
- `TKMAccount` holds TKM, executes calls, and enforces owners, thresholds, spending limits, sessions, locking, guardians, and delayed recovery.
- `TKMAccountFactory` deploys deterministic accounts with `CREATE2`.
- `TKMAllowlistPaymaster` accepts short-lived sponsor signatures only for approved target/selector pairs.
- `tkmaccount` exposes pure node-side hash and ABI builders.
- `gtkm smartaccount` exposes the helpers through IPC or an explicitly supplied endpoint.

No contract address is embedded before deployment. `tkmaccount_status` reports `deployed: false` and zero addresses until an audited deployment is intentionally configured in a later release.

## Build Contracts

The source is pinned to Solidity 0.8.24 compatibility and artifacts are generated with compiler 0.8.30:

```bash
cd ~/go-tkmchain
./contracts/smartaccount/compile.sh
```

The script writes reproducible ABI and bytecode files under `contracts/smartaccount/artifacts/`. Do not deploy artifacts produced by a different compiler without reviewing the bytecode and recording the compiler settings.

## Deployment Order

Deploy through a secured offline or hardware-backed owner:

1. Deploy `TKMEntryPoint`.
2. Deploy `TKMAccountFactory(entryPoint)`.
3. Optionally deploy `TKMAllowlistPaymaster(sponsorSigner)`.
4. Fund the paymaster operator outside the account contracts as required by the transaction relayer model.
5. Configure only the exact application target/selector pairs eligible for sponsorship.
6. Publish addresses, bytecode hashes, compiler version, deployment transactions, and audit report.
7. Update status metadata only after bytecode at every address is verified.

EntryPoint versions are immutable. A future upgrade should deploy a new EntryPoint and factory instead of changing the behavior of already-created accounts.

## RPC Setup

`tkmaccount` is a safe builder namespace and is enabled in default HTTP and WebSocket module lists. It can also be selected explicitly:

```bash
gtkm --http --http.addr 127.0.0.1 --http.api eth,net,web3,tkmaccount
```

Keep transaction submission, signing, keystore, and password RPC namespaces private. The smart-account helper does not require any of them.

Check status:

```bash
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tkmaccount_status","params":[]}'
```

CLI equivalent:

```bash
gtkm smartaccount status ~/.tkmchain/gtkm.ipc
```

## RPC Methods

```text
tkmaccount_status()
tkmaccount_operationHash(operation, entryPoint, chainId)
tkmaccount_createData({owners,threshold,salt})
tkmaccount_executeData(target,value,data)
tkmaccount_setOwnersData(owners,threshold)
tkmaccount_setLimitsData(dailyLimit,highValueThreshold)
tkmaccount_setGuardianData(guardian,enabled)
tkmaccount_setRecoveryPolicyData(threshold,delaySeconds)
tkmaccount_recoveryHash(owners,threshold)
tkmaccount_approveRecoveryData(ownerHash)
tkmaccount_cancelRecoveryData()
tkmaccount_completeRecoveryData(owners,threshold)
tkmaccount_setSessionData(session)
tkmaccount_revokeSessionData(key)
tkmaccount_ownerAuthorization(signatures)
tkmaccount_sessionAuthorization(signature)
tkmaccount_sponsorshipHash(operationHash,expiry,paymaster,chainId)
tkmaccount_sponsorshipData(expiry,signature)
tkmaccount_predictAddress(factory,salt,initCodeHash)
```

All numeric JSON fields use standard hex quantity encoding when called through JSON-RPC.

## Operation Format

```json
{
  "account": "0xAccount",
  "target": "0xTarget",
  "value": "0x0",
  "data": "0x12345678",
  "nonce": "0x0",
  "validUntil": "0x0",
  "gasLimit": "0x30d40",
  "paymaster": "0x0000000000000000000000000000000000000000",
  "paymasterData": "0x"
}
```

The operation hash commits to every field, the EntryPoint address, and chain ID. Owner and session-key signatures use the standard Ethereum signed-message prefix over this hash, preventing reuse on another chain or EntryPoint.

Authorization byte `1` selects a session-key signature. Authorization byte `2` selects an ABI-encoded `bytes[]` of owner signatures. Owner signatures must be ordered by recovered signer address and must not contain duplicates.

## Account Creation

Example calldata request:

```bash
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{
    "jsonrpc":"2.0","id":1,"method":"tkmaccount_createData","params":[{
      "owners":["0x1111111111111111111111111111111111111111"],
      "threshold":1,
      "salt":"0x0000000000000000000000000000000000000000000000000000000000000001"
    }]
  }'
```

Submit the returned calldata to the deployed factory using the user's normal wallet signer.

## Spending Policy

`dailyLimit` caps native TKM value sent during each UTC-sized on-chain day (`block.timestamp / 1 days`). `highValueThreshold` determines when the full owner signature threshold is required. Calls below that threshold still require at least one valid owner signature.

Policy changes use self-calls: the smart account must call itself through EntryPoint. An externally owned account cannot directly bypass account policy setters.

## Session Keys

A session contains:

- one authorized target;
- one four-byte function selector;
- maximum TKM value per call;
- total remaining TKM value;
- start and expiry timestamps;
- an active flag.

Session keys cannot call policy functions unless the owner explicitly makes the account itself the target and chooses that selector. Wallets should refuse such unsafe sessions. Each successful session validation decrements its remaining value before execution; a reverted operation rolls the decrement back atomically.

## Guardian Recovery

1. Owners add guardians through account self-calls.
2. Owners configure guardian threshold and a delay from one hour to 30 days.
3. Guardians approve the hash of the replacement owner array and threshold.
4. The first approval starts the delay; subsequent approvals for the same hash accumulate.
5. Existing owners may cancel through an authorized self-call.
6. After the delay and threshold, anyone may submit the exact replacement owner set.

Recovery does not grant Main King, Rotating Kings, node operators, or application administrators control over user accounts.

## Sponsorship

The initial paymaster is intentionally restrictive:

- only owner-approved target and selector pairs are eligible;
- the operation must name that paymaster;
- the sponsor permit includes operation hash, expiry, paymaster, and chain ID;
- permits expire within one day;
- the sponsor signer can be rotated;
- the owner can pause sponsorship immediately.

Relayers should add off-chain quotas, rate limits, reputation controls, and cost accounting. Those controls improve abuse resistance but never replace the on-chain selector and signature checks.

## CLI Builder Calls

The CLI `call` subcommand only permits known `tkmaccount` helper methods:

```bash
gtkm smartaccount call \
  --method setGuardianData \
  --params '["0x2222222222222222222222222222222222222222",true]' \
  ~/.tkmchain/gtkm.ipc
```

The result is calldata. Sign and submit it through the account workflow; the CLI does not unlock or send from a wallet.

## Testing

```bash
cd ~/go-tkmchain

# Compile Solidity and refresh checked artifacts
./contracts/smartaccount/compile.sh

# RPC hashing, validation, ABI, bytecode, and security-invariant tests
go test ./eth -run SmartAccount -count=1

# CLI and Web3 integration compilation
go test ./cmd/gtkm ./internal/web3ext -count=1

# Broader affected packages
go test ./eth ./node ./cmd/gtkm ./internal/web3ext -count=1
```

Before mainnet, run an independent contract audit, public testnet deployment, invariant fuzz campaign, and bug bounty. Successful local tests are not a substitute for an audit.

## Security Boundaries

- No `tx.origin`, `delegatecall`, or `selfdestruct` is used.
- EntryPoint has a reentrancy lock and per-account nonces.
- ECDSA rejects invalid `v` and high-`s` signatures.
- Operations and sponsorship permits are chain-bound.
- Owner signatures are recovered on-chain; claimed signer addresses are never trusted.
- Only EntryPoint can validate and execute normal account operations.
- Account policy changes require authorized self-calls.
- Recovery is delayed, threshold-based, cancellable, and account-specific.
- Paymaster eligibility is restricted by target and selector.

## Phone And Institution Integration

The contracts do not automatically give control to phone-number owners or institutional administrators. A registered phone device key may be added explicitly as an owner or restricted session key by a future wallet integration. Phone-number transfer must not transfer smart-account ownership automatically.

Institutions may create threshold accounts or operate paymasters for narrowly approved calls. Institutional credentials may inform an application policy, but they do not bypass signatures or account authorization.
