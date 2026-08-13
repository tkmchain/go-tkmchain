# Shielded Privacy Release Notes

Release scope: encrypted commitments and shielded transaction protocol
Date: 2026-08-10
Networks: Egypt test network and RandomX mainnet

## Summary

This release adds the foundation for a consensus-enforced shielded transaction protocol. After privacy activation, normal transparent user transactions are rejected by block processing and accepted transactions must use the shielded envelope sent to the shielded pool address.

The implementation is fail-closed: shielded spends require real zero-knowledge proof verification. Mainnet startup refuses to run an active privacy fork unless the shielded circuit ceremony output is embedded as the `shieldedGroth16VerifyingKey` chain-config artifact.

This update also adds the shielded spend circuit package, deterministic test vectors, ceremony tooling, a chain-format verifying key artifact, and the first consensus-enforced post-quantum transaction type.

## Activation

- Egypt network: privacy commitments are active from genesis for testing.
- Mainnet: `privacyCommitmentTime` is `1786341600`, which is `2026-08-10 06:00:00 UTC`.
- Mainnet: `quantumResistantTime` is also `1786341600`, so PQ-only user transaction validation activates at the same `2026-08-10 06:00:00 UTC` hardfork timestamp.
- Egypt network: quantum-resistant transaction validation is active from genesis for testing.
- Mainnet readiness now requires the embedded shielded Groth16 verifying key artifact before startup.

## Consensus Changes

- Added `PrivacyCommitmentTime` to chain configuration.
- Added `IsPrivacyCommitments` fork checks and compatibility handling.
- Added `ShieldedPoolAddress` as the consensus target for shielded transactions.
- Added shielded transaction decoding for `TKMSHIELD1` envelopes in `tx.Data`.
- Block processing now validates shielded transactions before EVM execution.
- After privacy activation, transparent non-shielded user transactions are rejected.
- After quantum-resistant activation, non-PQ user transaction types are rejected.
- Shielded transactions must target the shielded pool address.
- Shielded transactions must not expose a transparent `tx.Value`.
- Transparent-to-shielded deposits are accepted only with a deposit proof: the transaction must lock positive transparent value in `ShieldedPoolAddress`, carry one zero-nullifier/zero-anchor proof, and bind that public value to the private output commitments.
- Private spends must use a known shielded Merkle root and cannot use arbitrary local anchors.
- Duplicate nullifiers are rejected within a block.
- Already-spent nullifiers are rejected from state.
- Output commitments are stored in shielded-pool state slots.
- Existing output commitments cannot be reused.

## Zero-Knowledge Verification

- Added a production BN254 Groth16 verifier path using `gnark-crypto`.
- Added `ShieldedProofVerifier` as the consensus proof-verifier interface.
- Added `SetShieldedProofVerifier` for explicit verifier installation.
- Added `SetShieldedGroth16VerifyingKey` for installing a Groth16 verifying key.
- Added `TKMG16V1` as the shielded spend proof encoding.
- Added `TKMG16VK1` as the shielded Groth16 verifying key encoding.
- Added strict curve point decoding with subgroup checks through `gnark-crypto`.
- Added rejection for malformed proof encodings, malformed verifying keys, infinity proof points, and bad pairing checks.
- Public proof inputs are derived deterministically from chain ID, block number, split transaction-hash limbs, spend index, nullifier, anchor, balance commitment, public value, output commitment root, and binding hash.
- Consensus now rejects non-canonical BN254 field encodings for public shielded field elements before proof verification.
- Chain config can carry `shieldedGroth16VerifyingKey` as hex bytes.
- Block processing automatically loads and caches the configured Groth16 verifier when no explicit verifier override is installed.

## Shielded Spend Circuit

- Added `zk/shielded` with `TKM_SHIELDED_SPEND_V1`.
- The circuit uses BN254 Groth16/R1CS, MiMC commitments, a 32-level Merkle path, and exactly four padded output note slots.
- Public inputs are aligned with the consensus `ShieldedProofContext` order: chain ID, block number, transaction-hash high limb, transaction-hash low limb, spend index, nullifier, anchor, balance commitment, public value, output commitment root, and binding hash.
- The circuit proves note commitment derivation, Merkle membership, nullifier derivation, output commitment derivation, value conservation, output root derivation, balance commitment derivation, and transaction binding.
- Amounts, chain ID, block number, spend index, asset ID, and transaction-hash limbs are now range constrained in-circuit to avoid field-wrap money invariants and ambiguous hash reductions.
- Added the circuit specification and audit checklist in `zk/shielded/README.md`.
- Added `github.com/consensys/gnark v0.14.0` and compatible `github.com/consensys/gnark-crypto v0.19.0`.

## Test Vectors

- Added deterministic vector generation in `zk/shielded/vectors.go`.
- Added committed fixtures in `zk/shielded/testdata/vectors.json`.
- Added `cmd/shielded-vectors` to regenerate fixtures:

```bash
go run ./cmd/shielded-vectors -out zk/shielded/testdata/vectors.json
```

- Included one valid witness and invalid cases for wrong nullifier, unbalanced output value, and wrong Merkle anchor.
- Added a drift test that fails if the committed fixture diverges from the deterministic generator.

## Ceremony Tooling And Artifact

- Added `cmd/shielded-ceremony` with file-based MPC setup commands:
  `init-phase1`, `contribute-phase1`, `verify-phase1`, `init-phase2`,
  `contribute-phase2`, `finalize`, and `encode-vk`.
- Added `cmd/shielded-setup` for local development key generation.
- Embedded the generated chain-format mainnet verifying key in
  `mainnetShieldedGroth16VKHex`.
- Recorded artifact hashes:

```text
stored-config verifying.hex: a307f78a326e1a6fc70ada418f906d94e52c43aa5ebc0c962daa12ff6eae567e
stored-config verifying.key: c5cfb0c58b1a9a6823e8b4973dc122590b6568253d4152a7ac928cce8f157d79
upgraded verifier verifying.hex: 71850b9114e51f7290d2f04a2583df5bea7e142afd7457ecde72e49c7945d0cd
upgraded verifier verifying.key: b42cf88c36107d34fec8bacda545cab3cb17da52ac9d20674d553b69deda40a7
upgraded prover proving.key: 2c094442e02f1d39cc7ac47e213815b1a24f50decc58bd473258357605d9db72
```

- The proving key is not embedded in node code and must remain restricted to
  prover infrastructure.

## Internal Security Review Fixes

- Replaced consensus Keccak output-root derivation with the same fixed-slot MiMC output-root derivation used by `TKM_SHIELDED_SPEND_V1`.
- Added deposit mode to `TKM_SHIELDED_SPEND_V1`: zero nullifier and zero anchor prove `sum(outputs) == public value` for transparent-to-shielded deposits.
- Added the shielded commitment Merkle tree root registry, per-commitment witness storage, and root checks for spends.
- Split the transaction hash into two 128-bit public inputs instead of reducing the full 256-bit hash into one field element.
- Required exactly four padded outputs in shielded transaction envelopes so consensus matches the circuit.
- Added verifier-side public-context validation before pairing checks.
- Tightened mainnet ceremony-key readiness checks to decode BN254 points and reject malformed, trailing-byte, or infinity encodings.
- Fixed privacy RPC nullifier bookkeeping so it no longer assumes a nullifier is also the commitment map key.

## Mainnet Ceremony Gate

- Added `MainnetShieldedGroth16VerifyingKey` as the audited ceremony artifact slot.
- The slot is now populated with the `TKMG16VK1` shielded Groth16 verifying key artifact.
- Added `CheckMainnetShieldedPrivacyReady`.
- Mainnet CLI startup fails with a clear error if privacy is active but the ceremony verifying key is missing or malformed.
- Egypt/dev configurations remain usable for test verifier work without requiring the mainnet artifact.

## Privacy RPC And Storage

- Added `privacy` RPC support for encrypted commitment workflows.
- Added privacy activation time and activation status APIs.
- Added privacy-focused defaults API.
- Added encrypted commitment registration.
- Added shielded note registration with payload hash, ephemeral public key, view tag, encrypted payload, and nonce.
- Added commitment status and commitment listing APIs.
- Added nullifier spend/status APIs.
- Added encrypted payload, nonce, and view-tag length validation.
- Added raw database storage for privacy activations, commitments, and nullifiers.
- Legacy address-based privacy registration is disabled after encrypted commitment activation to avoid a public address registry side channel.

## RandomX Cleanup

- Removed stale Ethash references from the touched code paths.
- Removed stale TerminalTotalDifficulty/Merge-era fields from the touched code paths.
- Downloader fixture setup now uses RandomX faker consensus instead of Ethash.
- Obsolete downloader tests are gated behind `legacy_downloader_tests` so the active downloader package compiles cleanly.
- Added mandatory mainnet checkpoints for blocks `20141` through `20146`, `20173`, `20179`, and `20261`.
- Blocks `20142` through `20145` use a bounded, checkpoint-only stored `MixDigest` compatibility path because those already-mined canonical blocks carry stored proof values that cannot be reproduced by the strict verifier. Normal RandomX proof validation resumes at block `20146`.
- Blocks `20146` and `20173` use checkpoint-only historical PQ receipt-root compatibility because those already-mined canonical PQ transaction blocks were sealed before `PQTkmTxType` receipt trie encoding was finalized. New PQ transaction blocks use the normal typed receipt encoding.
- Blocks `20179` through `20261` use a second exact-hash stored `MixDigest` compatibility path after the long-gap difficulty transition at block `20179`. The mandatory checkpoint set anchors the start and end of the range, and the verifier keeps exact per-block hashes for the historical segment so new blocks cannot use stored-result proof acceptance.

## Quantum-Resistant Transactions

- Raised the module requirement to Go `1.25.0`.
- Added `github.com/emmansun/gmsm v0.44.1` as the ML-DSA dependency.
- Added `crypto/pqcrypto` with ML-DSA-87 key generation, signing, verification, and domain-separated address derivation.
- Added typed transaction `PQTkmTxType` (`0x06`).
- Added `PQTkmTx` fields for `pqAlgorithm`, `pqPublicKey`, and `pqSignature`.
- Added `SignPQTkmTx` and `SignNewPQTkmTx` helpers.
- Added `NewQuantumSigner` and sender validation that verifies real ML-DSA signatures instead of ECDSA public-key recovery.
- Added `QuantumResistantTime` to chain config, fork ordering, compatibility checks, and runtime rules.
- Mainnet `QuantumResistantTime` is `1786341600`; Egypt activates from genesis.
- Txpool and block processing reject non-PQ user transactions after the fork.
- Pre-fork block processing rejects PQ transactions unless the quantum signer is active for that timestamp.
- PQ addresses remain 20-byte EVM addresses for state/ABI compatibility, but use a `tkmchain:pq-address:v1:` domain separator so they are not legacy secp256k1-derived addresses.
- Added `postQuantumMainKingAddress`; `mainKingAddress` remains the pre-fork
  Main King address and rewards/control APIs switch to the PQ address at
  `quantumResistantTime` when the post-quantum address is configured.

## PQ Wallet And RPC Support

- Added version 4 PQ keystore JSON for ML-DSA-87 accounts.
- PQ keystore files encrypt the compact ML-DSA seed using the existing
  scrypt/AES-CTR/MAC envelope.
- Existing version 3 ECDSA keystore files are unchanged and remain available for
  pre-fork migration.
- Added PQ account creation, seed import, encrypted export, unlock, delete, and
  transaction signing to `accounts/keystore`.
- Added algorithm metadata lookup so clients can distinguish `ECDSA-secp256k1`
  and `ML-DSA-87` accounts.
- Added `tkm` RPC helpers:
  `newPQAccountWithPassphrase`, `importPQSeedWithPassphrase`,
  `exportPQAccount`, `importLegacyKeyfileWithPassphrase`, `accountAlgorithm`, `accountAlgorithms`,
  `pqMigrationData`, `pqMigrationGas`, `sendMigrationToPQWithPassphrase`,
  `preparePQMigrationWithPassphrase`, `preparePQMigrationWithPassphrases`,
  `autoMigrateToPQWithPassphrase`, and `autoMigrateToPQWithPassphrases`.
- `SendTransactionWithPassphrase` can sign `PQTkmTxType` when the selected local
  account is a PQ account.
- Legacy hash/text signing remains ECDSA-only; PQ signatures are not returned as
  recoverable `r/s/v` signatures.
- Pre-fork migration is performed by creating a PQ account and sending a
  legacy-signed value transfer from the old ECDSA account to the derived PQ
  address before activation.
- Added `TKMPQMIG1` migration calldata that binds the transfer recipient to the
  ML-DSA-87 public key so wallets and explorers can verify migration intent.
- Added keystore-assisted auto migration that creates a version 4 PQ keyfile,
  builds the migration marker, signs the legacy transfer, and submits it while
  migration is enabled.
- The recovery fork at `2026-08-14 06:00:00 UTC` permits valid `TKMPQMIG1`
  transfers after `quantumResistantTime`; all other post-fork legacy user
  transactions remain rejected.
- Added `ethkey --pq` tooling for generating and inspecting version 4 PQ
  keyfiles.
- Added the deterministic PQ wallet/explorer integration vector:
  `0x803e6EE61B7Ecba64eDF13ce0c4a8a65C495e5A5`.
- Added `docs/pq-wallet-integration.md` for external wallet, explorer, and
  indexer integration.

## Operational Notes

- Mainnet nodes must not ship with a placeholder shielded verifying key.
- The circuit and ceremony output are generated outside the node binary, encoded with the `TKMG16VK1` format, and embedded into the `MainnetShieldedGroth16VerifyingKey` artifact slot.
- With the artifact present, all mainnet nodes must run the same binary/config before `2026-08-10 06:00:00 UTC`.
- Shielded proof verification is consensus-critical. Any circuit, key, or encoding change requires all nodes to use exactly the same artifact and public input mapping.
- The stored chain config keeps the original verifier artifact for database compatibility. Upgraded nodes also embed `MainnetShieldedGroth16UpgradedVerifyingKey` and try it before falling back to the stored-config verifier, so transparent-to-shielded deposits can activate without rewriting the already-stored chain config.

## Test Coverage

Focused checks run for this release:

```bash
go test ./params -run 'ShieldedPrivacy|PrivacyCommitment'
go test ./cmd/utils -run ^$
go test ./params ./core -run Shielded
go test ./params ./core/rawdb ./eth -run TestPrivacy
go test ./zk/shielded
go test ./cmd/shielded-vectors
go test ./cmd/shielded-ceremony ./cmd/shielded-setup
go test ./accounts/keystore ./internal/ethapi -run 'PQ|^$'
go test ./cmd/ethkey -run PQ
go test ./crypto/pqcrypto ./core/types ./params ./core/txpool -run 'PQTkm|Quantum|PrivacyCommitment'
go test ./core -run ^$
go test ./eth/downloader -run ^$
go test ./eth ./internal/ethapi -run ^$
```

Covered behavior includes:

- privacy fork scheduling;
- mainnet ceremony key requirement;
- malformed verifying key rejection;
- well-formed verifying key envelope acceptance;
- Egypt/dev verifier-key exemption;
- shielded envelope round trip;
- canonical public field input enforcement;
- padded output-slot enforcement;
- consensus/circuit public-input vector alignment;
- transparent transaction rejection after activation;
- Groth16 proof/VK parser behavior;
- configured Groth16 verifier usage during shielded block processing;
- verifier-unavailable fail-closed behavior;
- shielded spend circuit compilation;
- valid shielded witness proving;
- invalid shielded witnesses failing proof generation;
- deterministic vector fixture drift detection;
- ceremony CLI compile health;
- development setup key generator compile health;
- nullifier and commitment state updates;
- duplicate nullifier rejection;
- transparent-value rejection.
- ML-DSA-87 PQ transaction signing and sender derivation;
- PQ transaction tamper rejection;
- pre-fork PQ transaction rejection by non-quantum signers;
- post-fork non-PQ user transaction rejection;
- post-fork PQ transaction txpool acceptance.
- PQ keystore create/export/import;
- PQ raw seed import;
- PQ unlock and transaction signing;
- PQ delete;
- PQ account algorithm metadata.
- PQ migration payload derivation and address binding.
- keystore-assisted PQ migration preparation.
- `ethkey` PQ deterministic seed generation and inspection.

## Remaining Production Requirements

- Release wallet/client support for note creation, note scanning, proof generation, encrypted note backup, and relayer or sender-hiding transaction submission.
- Update external wallets, explorers, and operational scripts to use PQ account metadata and `PQTkmTxType` before mainnet activation.
- Distribute the exact same binary/config to every mainnet validator before activation.
- Keep the proving key outside the node binary and restrict it to prover infrastructure.
- Preserve the circuit hashes, ceremony transcript hashes, beacon values, proving key hash, and verifying key hash in the release archive.
