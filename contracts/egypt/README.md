# Egypt network contract fixtures

## Isolated node data directory

Egypt must never share the production node database. Use the checked-in
launcher, which selects chain ID `8980`, refuses `~/.tkmchain`, and defaults to
`~/.tkmchain-egypt`:

```sh
./scripts/run-egypt.sh --port 3001 \
  --http --http.addr 127.0.0.1 --http.port 8645 \
  --http.api eth,net,web3,tkmasset,tkmprivacy,randomx \
  --http.vhosts localhost
```

To use another isolated location, set `TKM_EGYPT_DATADIR` to a directory that
is not a production datadir. The in-memory fixture below does not connect to a
node and never writes chain data.

## EUSD

[`EUSD.sol`](EUSD.sol) is the Egypt six-decimal test token. It provides
issuer-controlled minting, burning, allowances, transfers, and the TKM asset
kind/policy view methods. A deployment must append the runtime trailer produced
by `tkmasset_buildManifest` with:

```text
chainId: 8980 (0x2314)
kind: fungible / TKM-20
decimals: 6
flags: mintable
symbol: EUSD
```


Run the deterministic Egypt-network deployment test from the repository root:

```sh
go run ./cmd/egypt-contract-test
```

The command uses `params.EgyptChainConfig` and an in-memory EVM state. It
deploys:

- the EUSD runtime fixture with `mint`, `balanceOf`, `transfer`, and
  `totalSupply`, including a valid `TKMASSET` trailer; and
- a counter contract with `set(uint256)` and `get()`.

It prints the chain ID, deployed addresses, runtime bytecode hashes, deployment
gas, and the values read back from both contracts. No live wallet, node, or
private key is used, and no production network state is changed.

The EUSD flow mints 1,000 EUSD (1,000,000,000 base units) to the owner,
transfers 250 EUSD to a second address, and verifies the resulting 750 EUSD
sender balance, 250 EUSD recipient balance, unchanged 1,000 EUSD total supply,
and transfer gas usage. It also parses the deployed trailer, recomputes the
chain-bound asset ID, and runs the Antartical-gated TKM asset-ID precompile;
the direct and precompile identities must match. This is a real EVM state
transition in the in-memory Egypt state, rather than a balance calculation
performed by the test harness.

The same run performs an Antartical fork rehearsal on a private copy of the
Egypt configuration. It checks the pre-fork/post-fork gate for every catalogued
feature and exercises the implemented account-abstraction signature, parallel
execution scheduler, multidimensional gas, stateless witness commitment,
deterministic randomness, oracle signature, cross-chain replay key, EOF parser,
modular precompile registry, finality certificate, alternative-engine
differential check, and zkEVM execution-claim binding. The JSON output reports
which catalog entries are marked `consensusReady`; a gate being active in the
rehearsal does not promote an unfinished implementation to consensus.

The profile portion of the same output additionally verifies:

- chain- and contract-bound typed transaction signatures, including ML-DSA-87;
- an executable token policy commitment bound to manifest capabilities, with
  mint, burn, pause, royalty, and shielded operation checks;
- the order-independent asset registry root carried in a versioned header-extra
  suffix;
- a Shield3/Shield4 asset-ID and token-ID nullifier binding;
- independent EVM, TVM, proof, and blob gas dimensions with overflow rejection;
- deterministic parallel conflict waves and receipt-index metadata;
- the canonical sorted stateless witness commitment and a quorum finality
  light-client verification path; and
- alternate-engine admission through differential conformance vectors.

The rehearsal is intentionally isolated and deterministic. It does not deploy
EUSD to a live node, modify `~/.tkmchain`, or replace the historical receipt
RLP/witness encoding. Those wire-format changes require a coordinated network
upgrade; preserving the old encoding here prevents accidental chain splits.

The zkEVM fields validate the claim and witness commitments used by the proof
pipeline. Full STARK proving still requires the pinned Rust/Ziren toolchain and
a real block payload and witness; this command deliberately does not mutate a
node or generate a multi-million-cycle proof.

The privacy checks reject transparent legacy and transparent PQ transactions at
Antartical, accept Shield3 and Shield4 envelopes, and verify their native proof
gas accounting. Standalone stamp and private-TVM envelopes are rejected after
the fork; those operations must be carried inside a Shield3 or Shield4
transaction.

The latest rehearsal also reports `tokenName`, `tokenSymbol`, `tokenDecimals`,
`tokenStandard`, `tokenManifestHash`, `tokenAssetId`,
`tokenPrecompileAssetId`, `tokenManifestVerified`, `transferGasUsed`,
`tokenBalanceAfterTransfer`, `transferRecipient`, `transferAmount`, and
`recipientBalance` in its JSON output. The full zkEVM/STARK proof generation is
intentionally separate from this fast rehearsal and can be run later on a
machine with sufficient memory and CPU.
