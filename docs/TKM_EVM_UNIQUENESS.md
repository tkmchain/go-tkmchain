# TKM EVM profile and native asset identity

## The compatibility boundary

TKM is intentionally EVM-compatible. Existing Solidity, Vyper, Rust-EVM,
wallet, and indexer tooling can execute ordinary EVM bytecode. That lets a
developer deploy an existing contract without rewriting the virtual machine.

That compatibility has a precise limit: a chain cannot be both *byte-for-byte
Ethereum-identical* and *100% different from Ethereum*. TKM therefore uses a
layered profile. The EVM instruction and ABI surface remain familiar, while
consensus, networking, privacy, account policy, and asset identity are
TKM-native. A contract is not called TKM-native merely because it implements an
ERC interface.

This document is the implementation contract for that profile. It describes
what is available in the current code and labels future work explicitly so a
wallet or explorer never treats a proposal as a consensus rule.

## What makes TKM distinct today

The following layers are part of TKM's protocol profile or its documented fork
gates:

- **RandomX consensus and CPU mining.** Block sealing, seed epochs, difficulty,
  share verification, and the no-cgo path are TKM consensus code.
- **Post-quantum account and transaction paths.** ML-DSA account migration and
  the Shield3/Shield4 envelopes are selected by chain rules rather than by an
  Ethereum transaction type.
- **Shielded execution.** Encrypted notes, nullifiers, proof-checked value
  conservation, one-time outputs, viewing-key disclosure, and asset-specific
  replay protection are handled by the TKM privacy path.
- **Stamped accounts.** A name and country registration is a protocol record;
  the stamp key can disclose the label without becoming a spending key.
- **Onion-only transport and TKMNet.** Peer discovery, pool traffic, wallet
  relays, Phone, and EmailVM can use TKM's onion transport instead of exposing
  an IP endpoint.
- **TVM native modules.** The TKM TVM path is bounded, deterministic native
  execution behind a chain-configured precompile and fork gate.
- **Address governance.** Signed votes and unvotes are canonical state, fifteen
  active votes suspend an address, and each action burns 50 TKM.
- **TKM-native asset identity.** A token must publish the authenticated
  `TKMASSET` manifest described below to be classified as a TKM asset.

These layers are orthogonal. A normal EVM contract can continue to execute,
but it is classified as `ethereum-compatible` until it opts into the TKM asset
manifest. A manifest does not change the contract's ABI or silently grant mint,
burn, pause, upgrade, or privacy powers; those are declared capabilities that
wallets must display and contracts must enforce.

## TKM asset kinds and capabilities

The manifest has one of three accounting kinds:

- `TKM-20` (`fungible`): one divisible balance per account.
- `TKM-721` (`non-fungible`): one owner per token ID and zero decimals.
- `TKM-6909` (`multi`): many token IDs with fungible balances.

Capability bits are declarations that clients surface before a user signs:

```text
1   mintable       2   burnable       4   pausable
8   permit         16  batch transfer 32  shielded balances
64  royalty        128 soulbound     256 upgradeable
```

The declaration is not authority. For example, `mintable` tells a wallet that
minting may exist; the contract's access control, policy hash, and runtime code
still decide whether a particular mint succeeds.

## Egypt EUSD example

The repository includes a six-decimal `EUSD` fixture in
[`contracts/egypt/EUSD.sol`](../contracts/egypt/EUSD.sol). It uses chain ID
8980, the `TKM-20` kind, and an issuer-mint policy. The executable rehearsal
deploys the EVM fixture in an in-memory Egypt state, appends the `TKMASSET`
trailer, mints 1,000 EUSD, transfers 250 EUSD, and checks that:

- the sender has 750 EUSD and the recipient has 250 EUSD;
- total supply remains 1,000 EUSD;
- the runtime trailer parses back to `EUSD`, chain ID 8980, and six decimals;
- the direct asset-ID calculation equals the Antartical-gated `0x...f3`
  precompile result; and
- the output includes the manifest hash, asset ID, runtime hash, and gas used.

Run it with:

```sh
go run ./cmd/egypt-contract-test
```

When running a daemon-backed Egypt node, use
`./scripts/run-egypt.sh`. It selects `--egypt`, refuses the production
`~/.tkmchain` tree, and defaults to `~/.tkmchain-egypt`.

## Immutable manifest and asset ID

The deployed runtime code ends with a versioned `TKMASSET` trailer. It commits
to:

```text
chainId | kind | decimals | capability flags | policyHash |
name | symbol | metadataURI
```

All fields have bounded lengths and canonical big-endian encoding. The chain
does not trust a client-supplied name or symbol. It parses the trailer from the
canonical runtime code and recomputes the manifest hash.

For a deployed contract, the asset identity is:

```text
keccak256(
    "TKM_ASSET_ID_V1" ||
    uint256(chainId) ||
    address(contract) ||
    uint8(kind) ||
    bytes32(manifestHash)
)
```

The contract address is included, so two deployments with the same name,
symbol, and source still have different identities. A manifest with a chain ID
that does not match the node's chain is reported but is not marked native and
does not receive a trusted asset ID.

## Build a manifest before deployment

Add the `tkmasset` namespace to the node's HTTP API list. The command below
builds canonical bytes and a trailer; the trailer must be appended to the
compiled **runtime** bytecode, not the constructor/init bytecode.

```bash
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  --data '{
    "jsonrpc":"2.0", "id":1,
    "method":"tkmasset_buildManifest",
    "params":[{
      "chainId":"0x2313",
      "kind":"fungible",
      "decimals":18,
      "flags":9,
      "policyHash":"0x0000000000000000000000000000000000000000000000000000000000000000",
      "name":"TKM Dollar",
      "symbol":"TKMD",
      "metadataURI":"ipfs://tkm/asset.json"
    }]
  }'
```

`0x2313` is decimal 8979, the TKM mainnet chain ID. Egypt uses 8980, so a
manifest must be built with `0x2314` there. The response contains:

```json
{
  "standard": "TKM-20",
  "manifest": "0x...",
  "runtimeTrailer": "0x...",
  "manifestHash": "0x..."
}
```

If a contract address is included in the request, the response also contains
the deterministic `assetId`. The address is normally unknown until CREATE or
CREATE2 is evaluated, so computing it after deployment is the usual workflow.

## Classify an existing contract

Wallets, exchanges, and explorers should call the chain-level classifier rather
than guessing from `symbol()` or bytecode selectors:

```bash
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  --data '{
    "jsonrpc":"2.0", "id":2,
    "method":"tkmasset_getAsset",
    "params":["0x43aeb055883863cfe40804e386bec801b4ca63ec", "latest"]
  }'
```

A native result includes `native:true`, `network:"tkm"`, `standard`,
`manifestHash`, `assetId`, the policy hash, capabilities, and the runtime code
hash. A normal ERC contract is returned successfully as:

```json
{
  "native": false,
  "network": "ethereum-compatible",
  "standard": "unknown",
  "classificationNote": "runtime code has no TKM asset manifest"
}
```

This is deliberately not an error: compatibility contracts remain usable. A
wrong-chain manifest is also returned as `native:false` with a chain-mismatch
classification note.

## Contract-facing identity primitive

After the TKM Cancun/Antartical precompile gate, address
`0x00000000000000000000000000000000000000f3` accepts exactly 85 bytes:

```text
bytes 0..31   uint256 chain ID
bytes 32..51  contract address
byte  52      kind (1, 2, or 3)
bytes 53..84  manifest hash
```

It returns the 32-byte asset ID and has no state. The checked-in Solidity helper
is [contracts/tkm/TKMAsset.sol](../contracts/tkm/TKMAsset.sol):

```solidity
bytes32 id = TKMAssetIdentity.compute(
    bytes32(uint256(block.chainid)),
    address(this),
    1,                 // TKM-20
    manifestHash
);
```

The precompile is unavailable before its fork gate. A deployment tool must
check the active fork or handle the call failure instead of assuming it exists
on every historical block.

## A complete token deployment checklist

1. Write the token's policy and choose one of the three TKM kinds.
2. Generate the canonical manifest with `tkmasset_buildManifest`.
3. Compile the contract and append `runtimeTrailer` to the runtime code.
4. Deploy through the normal EVM transaction path, then record the receipt and
   runtime code hash.
5. Call `tkmasset_getAsset` at the deployment block and at the current head.
6. Have the contract expose `tkmAssetManifest`, `tkmAssetKind`, and
   `tkmAssetPolicyHash` for human-readable tooling, while treating the runtime
   trailer and RPC classification as the consensus-facing identity.
7. Publish the policy document and metadata URI. Never use a mutable web page
   as the only description of minting, burning, upgrade, or privacy behavior.

## Antartical protocol profile

The requested TKM-specific execution profile is implemented in
`consensus/antartical/profile.go` and is exercised by the Egypt rehearsal. It
is activated by the existing Antartical feature gate; Egypt activates that
gate at genesis, while mainnet uses the configured Antartical timestamp.
`params.Rules.TKMProfileVersion` is `0` before that gate and `1` at or after
it. The `AtVersion` helpers reject profile operations while version `0` is
active, so nodes cannot accidentally emit the new metadata early.

### Typed transactions and account abstraction

`TypedTransactionDomain` hashes the chain ID, receiving contract, operation
type, and payload. A signature made for one contract or network therefore
cannot be replayed at another. `TypedTransaction.Verify` supports the legacy
secp256k1 compatibility path and ML-DSA-87, where the public key is checked
against the post-quantum sender address. `UserOperation` in `account.go`
provides the TKM-specific EIP-4337/RIP-7560 operation hash and the same ML-DSA
sender policy.

### Executable token policy

`TokenPolicy.Commitment` is the canonical policy hash and
`TokenPolicy.ValidateManifest` requires the manifest's capability bits and
policy hash to match it. `TokenState.Apply` then enforces administrator-only
minting, burn authority, pause/unpause, maximum supply, royalty accounting,
and shielded capability checks. A `mintable` or `shielded` bit by itself is
never authority. The Egypt test covers every operation and rejects a mutated
manifest capability set.

### Asset registry and shielded assets

`AssetRegistryCommitment` sorts `(assetID, manifestHash, runtimeCodeHash)`
records and builds a domain-separated Merkle commitment. The current safe
integration stores the versioned commitment as a suffix in `Header.Extra`,
which preserves legacy header RLP decoding while allowing stateless clients to
extract and verify the root. `AttachAssetRegistryCommitmentAtVersion` leaves
the old header metadata unchanged at profile version `0` and appends the
versioned suffix at version `1`. `ShieldedAssetNullifierBinding` includes chain,
asset ID, token ID, and nullifier in one domain-separated digest, so a valid
Shield3/Shield4 proof cannot be retargeted to another token.

### Resource accounting and deterministic execution

`ProtocolGasVector` charges EVM, TVM, proof verification, and blob resources
independently and rejects counter overflow. `ConflictTranscript` commits the
canonical optimistic execution waves and access sets. Its
`ReceiptTranscript` binds that commitment to a receipt index without changing
the legacy receipt RLP or receipt root before the network-wide receipt-format
upgrade. The metadata is therefore safe to carry in the fork-specific receipt
sidecar while old blocks remain replayable. `ReceiptMetadataAtVersion` rejects
version `0` and emits the sidecar metadata only at version `1`.

### Stateless clients and alternate EVMs

`StateWitness.CanonicalCommitment` is the versioned, order-independent witness
format for the Verkle/stateless path. `StatelessLightClientProof` verifies the
witness root and commitment together with a quorum finality certificate,
without opening the full state database. `CommitmentAtVersion` selects the old
encoding at version `0` and the canonical encoding at version `1`. The old
witness commitment remains available for historical blocks.
`EngineRegistry.RegisterConformant` admits an
alternate EVM implementation only after it matches the canonical interpreter
on every supplied execution vector; a backend cannot be selected by name alone.

These are consensus primitives and conformance checks, not a claim that a
Rust/Revm or evmone binary is shipped in every release. A release that adds an
alternate backend must provide vectors covering state roots, receipts, proof
digests, gas dimensions, and rejection cases before registering it.

## Security invariants for clients

- Never classify an asset from `name`, `symbol`, or a contract's claimed
  `tkmAssetId()` alone.
- Bind cache keys to chain ID, contract address, kind, manifest hash, and
  runtime code hash.
- Display capability flags as warnings and verify the contract's authorization
  rules before signing.
- Treat a chain-mismatch manifest as untrusted; do not reuse an asset ID across
  TKM mainnet, Egypt, or another network.
- Keep private Shield3/Shield4 note data, owner secrets, viewing keys, and
  disclosure capsules out of RPC logs and indexer databases.
- Re-run classification after an upgrade or code change; the runtime trailer is
  immutable for a given deployment, but a proxy's implementation policy can
  change and must be surfaced to users.

The implementation and unit tests live in `core/tkmasset`, `core/vm`, and
`eth/api_tkmasset_test.go`. The shorter binary-format reference is in
[TKM_ASSET_STANDARD.md](TKM_ASSET_STANDARD.md).
