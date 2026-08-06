# Shielded Privacy Release Notes

Release scope: encrypted commitments and shielded transaction protocol
Date: 2026-08-06
Networks: Egypt test network and RandomX mainnet

## Summary

This release adds the foundation for a consensus-enforced shielded transaction protocol. After privacy activation, normal transparent user transactions are rejected by block processing and accepted transactions must use the shielded envelope sent to the shielded pool address.

The implementation is fail-closed: shielded spends require real zero-knowledge proof verification. Mainnet startup refuses to run an active privacy fork unless the shielded circuit ceremony output is embedded as the `shieldedGroth16VerifyingKey` chain-config artifact.

This update also adds the shielded spend circuit package, deterministic test vectors, ceremony tooling, and a chain-format verifying key artifact.

## Activation

- Egypt network: privacy commitments are active from genesis for testing.
- Mainnet: `privacyCommitmentTime` is `1786010400`, which is `2026-08-06 10:00:00 UTC`.
- Mainnet readiness now requires the embedded shielded Groth16 verifying key artifact before startup.

## Consensus Changes

- Added `PrivacyCommitmentTime` to chain configuration.
- Added `IsPrivacyCommitments` fork checks and compatibility handling.
- Added `ShieldedPoolAddress` as the consensus target for shielded transactions.
- Added shielded transaction decoding for `TKMSHIELD1` envelopes in `tx.Data`.
- Block processing now validates shielded transactions before EVM execution.
- After privacy activation, transparent non-shielded user transactions are rejected.
- Shielded transactions must target the shielded pool address.
- Shielded transactions must not expose a transparent `tx.Value`.
- Output-only shielded deposits are rejected; shielded transactions must spend at least one private note.
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
verifying.hex: a307f78a326e1a6fc70ada418f906d94e52c43aa5ebc0c962daa12ff6eae567e
verifying.key: c5cfb0c58b1a9a6823e8b4973dc122590b6568253d4152a7ac928cce8f157d79
proving.key: 7220670143963d8ebf26c1ffb74797f2ef657c6cee63f64c7b0b409137043b1d
```

- The proving key is not embedded in node code and must remain restricted to
  prover infrastructure.

## Internal Security Review Fixes

- Replaced consensus Keccak output-root derivation with the same fixed-slot MiMC output-root derivation used by `TKM_SHIELDED_SPEND_V1`.
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

## Operational Notes

- Mainnet nodes must not ship with a placeholder shielded verifying key.
- The circuit and ceremony output are generated outside the node binary, encoded with the `TKMG16VK1` format, and embedded into the `MainnetShieldedGroth16VerifyingKey` artifact slot.
- With the artifact present, all mainnet nodes must run the same binary/config before `2026-08-06 10:00:00 UTC`.
- Shielded proof verification is consensus-critical. Any circuit, key, or encoding change requires all nodes to use exactly the same artifact and public input mapping.

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
go test ./core -run ^$
go test ./eth/downloader -run ^$
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

## Remaining Production Requirements

- Release wallet/client support for note creation, note scanning, proof generation, encrypted note backup, and relayer or sender-hiding transaction submission.
- Distribute the exact same binary/config to every mainnet validator before activation.
- Keep the proving key outside the node binary and restrict it to prover infrastructure.
- Preserve the circuit hashes, ceremony transcript hashes, beacon values, proving key hash, and verifying key hash in the release archive.
