# TKM-native asset identity

TKM keeps the EVM execution model and the familiar ERC application surface,
but gives contracts an authenticated asset identity that Ethereum contracts do
not have by default. This lets wallets, exchanges, and explorers distinguish a
TKM asset from an ordinary ERC-compatible contract before displaying balances
or enabling transfers.

## Manifest

A token's deployed runtime bytecode ends with a `TKMASSET` v1 trailer. The
manifest commits to:

- `chainId`, so a manifest copied to another network is not native there;
- `kind`: `fungible` (`TKM-20`), `non-fungible` (`TKM-721`), or `multi`
  (`TKM-6909`);
- decimals, name, symbol, and metadata URI;
- a policy hash chosen by the issuer; and
- capability flags for minting, burning, pausing, permits, batch transfers,
  shielded balances, royalties, soulbound assets, and upgradeability.

The canonical bytes are hashed into `manifestHash`. The deployed asset ID is

```text
keccak256("TKM_ASSET_ID_V1" || chainId || contract || kind || manifestHash)
```

The contract address is therefore part of the identity, and two contracts with
the same name or symbol cannot collide as assets.

## Build a trailer

Use the `tkmasset_buildManifest` RPC method. Append the returned
`runtimeTrailer` to the runtime bytecode produced by the Solidity/Vyper/Rust
compiler before deployment. The EVM executes the normal code prefix; the
trailer is immutable metadata for clients.

Example request:

```json
{
  "chainId": "0x2313",
  "kind": "fungible",
  "decimals": 18,
  "flags": 9,
  "policyHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "name": "TKM Dollar",
  "symbol": "TKMD",
  "metadataURI": "ipfs://tkm/asset.json"
}
```

Then query `tkmasset_getAsset(address)` or `tkmasset_classify(address)`.
Manifests with the wrong chain ID are reported as non-native and their asset ID
is not trusted. Contracts without a manifest remain usable as ordinary EVM
contracts and are reported as `ethereum-compatible`.

## EVM identity primitive

After the TKM Cancun/Antartical boundary, `0x00000000000000000000000000000000000000f3`
is a deterministic, stateless precompile. Its 85-byte input is the 32-byte
chain ID, 20-byte contract address, one-byte kind, and 32-byte manifest hash;
it returns the same 32-byte asset ID used by RPC clients. The domain string is
TKM-specific, so it is not an Ethereum token identifier.

## Why this keeps compatibility

TKM does not replace ERC-20, ERC-721, or ERC-1155 execution. Those interfaces
remain available for existing tooling, while the manifest is an additional
identity and policy layer. ERC-165-style interface detection can still be used
inside contracts; the manifest is the chain-level distinction used by wallets
and explorers.

