# Shielded Groth16 recovery — 2026-08-20

Mainnet switches to this verifier at Unix timestamp `1787209200`, which is
`2026-08-20 07:00:00 UTC`. Blocks before the timestamp use the historical
verifier rules; blocks at or after it accept proofs for the recovery key only.

Git tag: `shielded-groth16-recovery-20260820`

Default proving-key URL:

```text
https://raw.githubusercontent.com/tkmchain/go-tkmchain/shielded-groth16-recovery-20260820/prover-keys/shielded-groth16-recovery-20260820/proving.key
```

Artifact SHA-256 hashes:

```text
7c3dc3b9f33e522e84665189fa02c08299d209daaa80f96d2dfa6ad43dc2be40  proving.key
24a3dcf939acc41bc236c628e556ad80fb0a8e381f8f93a095fcc44196fcea9b  verifying.key
214b4671b3110d14936117b92cb3a4266895afd7d0725fe1099377c02bbc0fef  verifying.hex
```

The key pair passed a full Groth16 proof-generation and verification round
trip against `TKM_SHIELDED_SPEND_V1` before packaging.

## Ceremony record

This recovery ceremony was performed by one operator with two local
contributions in each phase. It is not an independently operated
multi-participant ceremony. A later ceremony should collect contributions from
independent people or organizations before it is described as multi-party.

```text
phase1-0.bin  00a93fb5c1c128e9f0fbb2df4cca5bbab5304d77271e47a3fed4d292ac8d7f7f
phase1-1.bin  dc8877e7f1184d4063bc10259eb1296e7d619848119a971b2dd823829a663b8f
phase1-2.bin  1cedbe3be4fec5af3472e0c6b3d14ef72567cbd53ee8bb70cda6afbae0943b9d
phase1 beacon cab9e6876b73fd6449a9677895a0f5de36b08fca391b8a20ad4728e10f5bcfdb
commons.bin   310773f805513072306407558b3cf58ebcf47a462bc5f67170e600ca1cfd6df1
phase2-0.bin  e752c8b30dc73ef0edb59ab1fa3e5085591deb8d9b65bb703e0da9fd76c72e3b
phase2-1.bin  2c126c01f55a63748965aef0718b4a1484fe5400ba80577538f33e257ac94d73
phase2-2.bin  6e8dfe4bced036ebe97c501c3378a4390bc9a35258a59ec1352e049e8d74ed5a
phase2 beacon 2ba74357f1deed002dfefd2183128f17e95f8913fa57dadff227f72e652c26e5
```

The proving and verifying keys are public. Private note witnesses, signing
keys, and any retained ceremony randomness are secrets.
