# Egypt network contract fixtures

Run the deterministic Egypt-network deployment test from the repository root:

```sh
go run ./cmd/egypt-contract-test
```

The command uses `params.EgyptChainConfig` and an in-memory EVM state. It
deploys:

- a mintable ERC-20-style token fixture with `mint`, `balanceOf`, `transfer`,
  and `totalSupply`; and
- a counter contract with `set(uint256)` and `get()`.

It prints the chain ID, deployed addresses, runtime bytecode hashes, deployment
gas, and the values read back from both contracts. No live wallet, node, or
private key is used, and no production network state is changed.

The token flow mints 1,000 TKM to the owner, transfers 250 TKM to a second
address, and verifies the resulting 750 TKM sender balance, 250 TKM recipient
balance, unchanged 1,000 TKM total supply, and transfer gas usage. This is a
real EVM state transition in the in-memory Egypt state, rather than a balance
calculation performed by the test harness.

The same run performs an Antartical fork rehearsal on a private copy of the
Egypt configuration. It checks the pre-fork/post-fork gate for every catalogued
feature and exercises the implemented account-abstraction signature, parallel
execution scheduler, multidimensional gas, stateless witness commitment,
deterministic randomness, oracle signature, cross-chain replay key, EOF parser,
modular precompile registry, finality certificate, alternative-engine
differential check, and zkEVM execution-claim binding. The JSON output reports
which catalog entries are marked `consensusReady`; a gate being active in the
rehearsal does not promote an unfinished implementation to consensus.

The zkEVM fields validate the claim and witness commitments used by the proof
pipeline. Full STARK proving still requires the pinned Rust/Ziren toolchain and
a real block payload and witness; this command deliberately does not mutate a
node or generate a multi-million-cycle proof.

The privacy checks reject transparent legacy and transparent PQ transactions at
Antartical, accept Shield3 and Shield4 envelopes, and verify their native proof
gas accounting. Standalone stamp and private-TVM envelopes are rejected after
the fork; those operations must be carried inside a Shield3 or Shield4
transaction.

The latest rehearsal also reports `transferGasUsed`, `tokenBalanceAfterTransfer`,
`transferRecipient`, `transferAmount`, and `recipientBalance` in its JSON
output. The full zkEVM/STARK proof generation is intentionally separate from
this fast rehearsal and can be run later on a machine with sufficient memory
and CPU.
