# Antartical consensus primitives

This package is the shared deterministic implementation surface for the
Antartical fork. It contains:

- EIP-4337/RIP-7560 UserOperation hashing and secp256k1 or ML-DSA-87
  authorization;
- optimistic Block-STM wave execution with dynamic conflict retry;
- multidimensional gas accounting;
- stateless state-witness and zk execution claim commitments;
- signed oracle observations and cross-chain replay keys;
- EOF v1 container validation;
- versioned modular precompile registration;
- differential execution engine comparison; and
- quorum-checked single-slot finality certificates.

The core state transition consumes the corresponding Antartical envelopes:
`TKM-AA-ENTRYPOINT-V1` is executed through the reserved native entry point,
`TKM-ORACLE-ENVELOPE-V1` persists monotonic feed rounds and values, and
`TKM-XCHAIN-ENVELOPE-V1` persists destination-bound replay keys and payloads.
The miner runs the same envelope transitions as the importer. A configured
`miner.FinalityProvider` attaches a certificate after execution; once an
active committee exists, the block validator and miner both reject a missing
certificate.

`ProcessExecutionEngine` is a strict, time-limited adapter for a separately
built Revm/evmone process. It is never selected automatically: callers must
admit it with `EngineRegistry.RegisterConformant`, using canonical vectors.

The profile also includes the Antartical asset and stateless execution
primitives in `profile.go`:

- `TypedTransactionDomain` and `TypedTransaction` bind signatures to the
  chain, contract, operation type, and payload. ML-DSA-87 is verified against
  the sender address, while secp256k1 remains the compatibility path.
- `TokenPolicy` and `TokenState` enforce mint, burn, pause, unpause, royalty,
  and shielded capability rules. `TokenPolicy.Commitment` and
  `ValidateManifest` bind executable policy to the `TKMASSET` manifest hash;
  manifest flags alone do not grant authority.
- `AssetRegistryCommitment` produces an order-independent registry root.
  `AttachAssetRegistryCommitment` and `AssetRegistryCommitmentFromHeaderExtra`
  use a versioned header-extra suffix so old header RLP remains decodable.
- `ShieldedAssetNullifierBinding` binds chain, asset, token ID, and nullifier,
  preventing a proof for one token from being replayed as another asset.
- `ProtocolGasVector` charges EVM, TVM, proof, and blob dimensions with
  overflow checks. `ConflictTranscript` records deterministic execution waves
  and produces receipt metadata without changing legacy receipt RLP.
- `StateWitness.CanonicalCommitment` and `StatelessLightClientProof` provide
  a sorted witness commitment and a finality-certificate verification path for
  light clients. The historical `Commitment` encoding is retained for replay
  compatibility.
- `EngineRegistry.RegisterConformant` requires every alternate EVM backend to
  match the canonical interpreter on supplied vectors before registration.

`params.Rules.TKMProfileVersion` is the activation selector shared by node
code: version `0` keeps historical metadata encodings, and version `1` is
selected at Antartical (genesis on Egypt, the configured timestamp on
mainnet). The `AtVersion` helpers reject typed transactions, policy updates,
registry suffixes, receipt transcript sidecars, and canonical witness output
before version `1`.

All hashes include a protocol domain tag and chain identifiers where applicable.
The host EVM, zkEVM guest, and private transaction envelopes use these same
commitments when the Antartical rules are active.
