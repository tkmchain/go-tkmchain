# Shielded V2 recipient binding — 2026-08-20

Mainnet activates Shielded V2 at Unix timestamp `1787832000`, or
`2026-08-27 12:00:00 UTC`. Blocks before the timestamp require V1 envelopes;
blocks at or after it require V2 envelopes.

Source and proving-key branch: `shielded-v2-recipient-binding-20260820`

```text
https://raw.githubusercontent.com/tkmchain/go-tkmchain/shielded-v2-recipient-binding-20260820/prover-keys/shielded-v2-recipient-binding-20260820/proving.key
```

## Protocol changes

- The recovered ML-DSA-87 sender address is the twelfth Groth16 public input.
- V2 note commitments bind the recipient account address, asset, value, and
  randomness under domain `2001`.
- V2 commitments, nullifiers, output roots, balances, bindings, and intent
  hashes are domain-separated from V1.
- An official V1 self-note can migrate only when its legacy owner field equals
  the recovered PQ sender. The output is always V2.
- `gtkm --tkmprover` downloads, verifies, caches, and serves both keys without
  loading a signer or requiring a funded prover account.
- The wallet signs only after independently recomputing the requested V2
  recipient and change commitments from the returned output openings.

## Artifact hashes

```text
248d2a299233c0d57e5a03d30cba62d4dde8f716594e67585842065b5eebd626  proving.key
de7585bcaea8bbf14fbd7e7a42aa2724e6e1ee925f62fa507a4d38403ed9d62b  verifying.key
f244511eee64c0af44c97dd2fef4e2158fe52690e4c1c0cd03d2113907be6924  verifying.hex
```

The pair passed a full Groth16 proof-generation and verification round trip
against the 37,808-constraint V2 circuit with 12 public inputs.

## Ceremony record

Phase 2 reused the sealed Phase 1 common reference string recorded by the
2026-08-20 recovery ceremony. It then received two sequential local
contributions and the beacon
`TKM_SHIELDED_V2_RECIPIENT_BINDING_2026-08-20`.

```text
phase2-0.bin  ef7b759422d81186a34b8af43fb427244c3feec9440b2825027cccbfc11b176f
phase2-1.bin  655df55f31f28f65760ede906306a8f52eede8efa442733da362bb497f818be0
phase2-2.bin  d25d9d8e0b4340da771aa0dae7785be61c9f960cd8540de0356ca048110d58c3
```

This was one operator performing local contributions, not an independently
operated multi-party ceremony. The keys are internally consistent, but the
ceremony should not be described as independently multi-participant or audited.
