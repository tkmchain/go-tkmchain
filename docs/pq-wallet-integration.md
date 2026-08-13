# Post-Quantum Wallet And Explorer Integration

This document defines the wallet, explorer, and indexer surface for the
`PQTkmTxType` hardfork.

## Test Vector

Algorithm: `ML-DSA-87`

Seed:

```text
000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
```

Expected address:

```text
0x803e6EE61B7Ecba64eDF13ce0c4a8a65C495e5A5
```

Generate and inspect the vector:

```bash
printf 'test-pass\n' >/tmp/tkm-pq-pass.txt
go run ./cmd/ethkey generate \
  --pq \
  --pqseed 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f \
  --passwordfile /tmp/tkm-pq-pass.txt \
  --lightkdf \
  --json \
  /tmp/tkm-pq-key.json

go run ./cmd/ethkey inspect \
  --passwordfile /tmp/tkm-pq-pass.txt \
  --json \
  /tmp/tkm-pq-key.json
```

## Keystore Format

PQ keyfiles use JSON `version: 4`.

Required top-level fields:

- `address`: 20-byte EVM-compatible address hex.
- `algorithm`: currently `ML-DSA-87`.
- `publicKey`: canonical FIPS 204 ML-DSA-87 public key bytes.
- `crypto`: existing scrypt/AES-128-CTR/MAC keystore envelope.
- `id`: UUID.
- `version`: `4`.

The encrypted payload is the 32-byte ML-DSA seed, not the expanded private key.
Legacy ECDSA keyfiles remain `version: 3`.

## Address Derivation

PQ addresses remain 20 bytes for state and ABI compatibility.

Derivation:

```text
address = keccak256("tkmchain:pq-address:v1:" || "ML-DSA-87" || publicKey)[12:]
```

Explorers must label the account as `ML-DSA-87` when the local keystore or RPC
metadata identifies it as PQ. The chain state alone still stores a 20-byte
address.

## Transaction Format

PQ transactions use typed transaction `0x06` / `PQTkmTxType`.

JSON fields:

- `type`: `0x6`.
- `chainId`
- `nonce`
- `to`
- `gas`
- `maxFeePerGas`
- `maxPriorityFeePerGas`
- `value`
- `input`
- `accessList`
- `pqAlgorithm`
- `pqPublicKey`
- `pqSignature`

The signature is ML-DSA-87 over the transaction signing hash. There is no
recoverable `r/s/v` signature for PQ transactions.

## RPC Helpers

The node exposes helper methods in the `tkm` namespace:

- `newPQAccountWithPassphrase(passphrase)`
- `importPQSeedWithPassphrase(seed, passphrase)`
- `exportPQAccount(address, passphrase, newPassphrase)`
- `importLegacyKeyfileWithPassphrase(keyfileJSON, passphrase)`
- `accountAlgorithm(address)`
- `accountAlgorithms()`
- `pqMigrationData(publicKey)`
- `pqMigrationGas(publicKey)`
- `sendMigrationToPQWithPassphrase(args, publicKey, passphrase)`
- `preparePQMigrationWithPassphrase(address, passphrase)`
- `preparePQMigrationWithPassphrases(address, legacyPassphrase, pqPassphrase)`
- `autoMigrateToPQWithPassphrase(args, passphrase)`
- `autoMigrateToPQWithPassphrases(args, legacyPassphrase, pqPassphrase)`

`sendTransactionWithPassphrase` can sign and submit `type: 0x6` transactions
when the selected account is a PQ account.

## Migration

Before `2026-08-10 06:00:00 UTC`, users should:

1. Create a PQ account.
2. Back up the version 4 keyfile and passphrase.
3. Use `sendMigrationToPQWithPassphrase` to submit a legacy-signed value
   transfer from the old ECDSA account to the derived PQ address.
4. Verify the transaction input starts with `TKMPQMIG1` and decodes to the PQ
   address, `ML-DSA-87`, and public key.
5. Verify the PQ address appears in wallet/explorer tooling as `ML-DSA-87`.

The migration marker is normal transaction calldata:

```text
"TKMPQMIG1" || rlp([pqAddress, "ML-DSA-87", pqPublicKey])
```

Nodes validate the marker by recomputing:

```text
pqAddress = keccak256("tkmchain:pq-address:v1:" || "ML-DSA-87" || pqPublicKey)[12:]
```

From the original quantum-resistant fork until `2026-08-14 06:00:00 UTC`,
legacy-signed migration is closed. At that recovery timestamp, consensus again
accepts a normal legacy-signed transaction only when it carries a valid
recipient-bound `TKMPQMIG1` marker. Ordinary legacy transactions remain
rejected and normal user transactions must use `PQTkmTxType`.

Web wallets can pass the encrypted version 3 keyfile to
`importLegacyKeyfileWithPassphrase` on a trusted local node, then use the
prepare, export, and send helpers without exposing a raw ECDSA private key.

## Keystore-Assisted Auto Migration

For a local keystore account, `autoMigrateToPQWithPassphrase` performs the
allowed migration workflow in one call:

1. Decrypt the legacy ECDSA account to verify the passphrase.
2. Create and store a new version 4 ML-DSA-87 keyfile.
3. Build the `TKMPQMIG1` migration calldata for the new PQ public key.
4. Sign a normal legacy value transfer from the old account to the new PQ
   address.
5. Submit the transaction.

The call returns:

- `legacyAccount`
- `pqAccount`
- `pqAlgorithm`
- `pqPublicKey`
- `migrationData`
- `txHash`, when submission succeeds

If transaction submission fails after the PQ keyfile is created, the response
still includes the new PQ account metadata alongside the error. The old ECDSA
keyfile is never deleted or overwritten by auto migration.

Use `preparePQMigrationWithPassphrase` when wallet software wants to create the
PQ key and migration payload first, then estimate gas, prompt the user, or
submit the transfer later.
