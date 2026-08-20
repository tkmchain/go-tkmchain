# TKM Shielded Payout Prover

`shielded-payout-prover` is the private HTTP service used by the mining pool after privacy commitments are active. The pool calls this service at `shieldedPayoutProverURL`; the prover builds a `TKMSHIELD1` envelope, creates a Groth16 shielded spend proof, signs a TKM transaction, submits it to the node, and returns the transaction hash.

The service fails closed. If the proving key, PQ signer, node RPC, or spendable notes are missing, it returns an error and the pool keeps the miner balance owed.

## Build

```sh
cd /home/mike/go-tkmchain
go build -o /home/mike/shielded-prover/shielded-payout-prover ./cmd/shielded-payout-prover
```

## Runtime Files

Recommended private runtime directory:

```text
/home/mike/shielded-prover
```

Required files:

```text
/home/mike/shielded-prover/config.json
/home/mike/shielded-prover/proving.key
/home/mike/shielded-prover/notes.json
/home/mike/shielded-prover/requests.json
```

Permissions:

```sh
chmod 700 /home/mike/shielded-prover
chmod 600 /home/mike/shielded-prover/config.json
chmod 400 /home/mike/shielded-prover/proving.key
chmod 600 /home/mike/shielded-prover/notes.json /home/mike/shielded-prover/requests.json
```

## Config

Example:

```json
{
  "listen": "127.0.0.1:8787",
  "bearerToken": "same-value-as-pool-shieldedPayoutProverToken",
  "nodeRPC": "http://127.0.0.1:8545",
  "keystoreDir": "/home/mike/.tkmchain/keystore",
  "signerAddress": "0xYourPostQuantumSignerAddress",
  "signerPassphraseFile": "/home/mike/shielded-prover/signer.pass",
  "signMode": "pq",
  "provingKeyPath": "/home/mike/shielded-prover/proving.key",
  "notesPath": "/home/mike/shielded-prover/notes.json",
  "requestsPath": "/home/mike/shielded-prover/requests.json",
  "gasLimit": 3000000,
  "submitSync": true,
  "receiptTimeoutMs": 20000
}
```

For production after the quantum fork, keep `"signMode": "pq"` and use an ML-DSA-87 keystore account. Legacy signing is only for pre-fork or private testing.

## Pool Wiring

The pool config must point at the prover:

```json
{
  "shieldedPayoutProverURL": "http://127.0.0.1:8787/payout",
  "shieldedPayoutProverToken": "same-value-as-prover-bearerToken"
}
```

The pool sends:

```json
{
  "requestId": "64-hex-idempotency-key",
  "poolWallet": "0xPoolWallet",
  "to": "0xMinerPayoutWallet",
  "amountAntd": 5,
  "amountWei": "0x4563918244f40000",
  "payoutTxType": "0x6",
  "privacyCommitmentTime": 1786341600,
  "quantumResistantTime": 1786341600
}
```

The prover returns:

```json
{ "txHash": "0x..." }
```

Repeated `requestId` values are idempotent. If the request was already sent, the same transaction hash is returned.

## Funding Notes

Do not fund the prover by sending TKM to mainking. Mainking payments are for privacy activation/metadata, not spendable shielded liquidity.

Create real prover-controlled shielded notes with the authenticated deposit endpoint:

```text
POST http://127.0.0.1:8787/deposit
Authorization: Bearer <shieldedPayoutProverToken>
Content-Type: application/json
```

Example body:

```json
{
  "requestId": "pool-liquidity-001",
  "amountAntd": 5
}
```

The prover builds a `TKMSHIELD1` deposit proof, sends a positive-value transaction to `params.ShieldedPoolAddress`, waits for the receipt, reads `tkmprivacy_commitmentPath`, and writes the resulting note into `notes.json`.

Deposit `requestId` values are idempotent too. If the same deposit is retried after an RPC timeout and a transaction hash was already recorded, the prover returns the recorded hash instead of creating another deposit.

This endpoint requires a TKM node binary that includes the deposit-capable shielded circuit and the matching embedded verifier artifact. A node running the older shielded verifier rejects deposit proofs.

## Note Inventory

`notes.json` stores explicit spendable shielded notes. Deposit notes are imported automatically by `/deposit`; manual imports should only be used after their commitment is on chain and their Merkle witness is known.

Schema:

```json
{
  "notes": [
    {
      "id": "note-001",
      "ownerSecret": "11",
      "noteRandomness": "22",
      "noteValueWei": "5000000000000000000",
      "assetId": "1",
      "commitment": "0x...",
      "merkleRoot": "0x...",
      "createdTxHash": "0x...",
      "merklePath": ["32 decimal field elements"],
      "merklePathIndex": ["32 values, each 0 or 1"],
      "status": "available"
    }
  ]
}
```

Values must fit in unsigned 64-bit integers because the current circuit constrains note values with `api.ToBinary(..., 64)`. A single note can therefore carry at most `18446744073709551615` base units.

For pool operation, use notes no larger than `0xffffffff22e4c000` wei, which is `18.44674407` TKM at the pool's 8-decimal payout precision. A miner owed more than that is paid across multiple payout cycles.

When a payout succeeds, the input note is marked `spent` in `notes.json`, change is added as a new note when present, and the request is recorded in `requests.json`.

If `/deposit` returns that a transaction was added to the transaction pool but was not processed before the receipt timeout, wait for the receipt and retry the same `requestId`. The retry finalizes the Merkle witness and marks the note `available`; it does not create another deposit.

## Replacement Overrides

`/deposit` and `/payout` accept optional authenticated maintenance fields:

```json
{
  "nonce": "0x3",
  "gasPriceWei": "0x12a05f200"
}
```

Use these only to replace a stuck nonce or underpriced pending transaction. For payout replacement, keep the same `requestId`; the prover reuses the already-spent note only when the stored request record matches the note's `spentRequestId`.

## Block Number Behavior

The prover builds durable shielded proofs with public `BlockNumber = 0`. Upgraded nodes accept both historical block-bound proofs and the new zero-block proofs. This prevents otherwise valid payouts from becoming invalid just because they were mined after the next block.

## Run

```sh
/home/mike/shielded-prover/shielded-payout-prover \
  -config /home/mike/shielded-prover/config.json
```

Health:

```sh
curl -sS http://127.0.0.1:8787/healthz
```

The health response separates service readiness from payout liquidity:

```json
{
  "ok": true,
  "payoutReady": false,
  "hasSpendableNotes": false,
  "availableNoteCount": 0,
  "availableNoteMaxWei": "0"
}
```

If `ok` is true but `payoutReady` is false, the prover is running but cannot pay because `notes.json` has no spendable shielded note large enough for the request. Do not create synthetic notes to clear this error; import only real notes whose commitment exists on chain and whose Merkle witness is known.

The pool error `no spendable shielded note is available for this amount` means the prover needs more real shielded liquidity. Add inventory through `/deposit`; do not fund mainking and do not edit fake notes into `notes.json`.

Start it in the background:

```sh
nohup /home/mike/shielded-prover/shielded-payout-prover \
  -config /home/mike/shielded-prover/config.json \
  >> /home/mike/shielded-prover/prover.log 2>&1 &
```

## Security

- Bind to `127.0.0.1` or a private network only.
- Never expose the prover directly to the internet.
- Keep the bearer token out of git.
- Treat `proving.key` as a public, hash-verified circuit artifact. Keep note witnesses, bearer tokens, signer keys, and ceremony toxic-waste material private.
- Persist `requests.json`; it protects against double-send retries.
- Back up `notes.json` after every successful payout.
