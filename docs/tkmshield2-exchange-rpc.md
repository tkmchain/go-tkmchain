# TKMShield2 Exchange Deposit RPC Integration

This guide is for exchanges and custodial platforms integrating native TKM
after shielded privacy activation.

## Deposit model

Native TKM deposits should use shielded V2 payment codes, not transparent
`0x...` deposit addresses.

Transparent TKM sends are disabled after privacy activation. A normal customer
deposit should therefore be treated as a shielded transaction sent to a
`tkmshield2...` payment code. Do not credit deposits from address balance
changes or explorer `transaction_count`.

The safe exchange model is:

1. Generate or assign a `tkmshield2...` payment code for the customer deposit.
2. Keep the matching view-only private key in the exchange scanner.
3. Never store the spend key, wallet passphrase, seed, or note database on the
   scanner.
4. Credit only after a mined transaction is verified against the payment code,
   view key, amount, sender, and confirmation policy.

## Generating a deposit code and view key

Create a dedicated PQ account:

```bash
gtkm account new --keystore /secure/tkm/keystore
```

The command prints:

```text
Public address:          0x...
Shielded payment code:   tkmshield2....
```

Export the matching view-only key on a trusted machine:

```bash
gtkm account shielded-view-key --keystore /secure/tkm/keystore 0xYourPQAddress
```

The output contains:

```text
TKM_SHIELDED_SETTLEMENT_ADDRESS=tkmshield2....
TKM_SHIELDED_VIEW_PRIVATE_KEY=...
```

The view key can recognize incoming encrypted notes for that payment code. It
cannot sign or spend.

## RPC calls a scanner should use

Use a normal GTKm JSON-RPC endpoint, for example:

```text
https://wallet.tkmchain.site/rpc
```

For every submitted deposit hash:

1. Fetch the transaction:

```json
{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionByHash","params":["0xTxHash"]}
```

2. Fetch the receipt:

```json
{"jsonrpc":"2.0","id":2,"method":"eth_getTransactionReceipt","params":["0xTxHash"]}
```

3. Check confirmations from the receipt block number:

```json
{"jsonrpc":"2.0","id":3,"method":"eth_blockNumber","params":[]}
```

4. Decode the shielded transaction input locally using the published TKMShield2
   note format and the configured view key.

The transaction must satisfy all of these checks before crediting:

- transaction exists;
- receipt exists;
- receipt status is success;
- receipt has a mined block;
- required confirmations are reached;
- `to` is the canonical shielded pool address;
- transparent transaction `value` is zero;
- the sender matches the customer deposit source policy, if the exchange uses
  one;
- one or more encrypted outputs decrypt with the configured view key;
- authenticated note payload hashes match the on-chain output commitments;
- total decrypted TKM amount equals the customer deposit amount;
- the tx hash has not already been credited.

## Per-user deposit codes

Exchanges may give each user their own `tkmshield2...` deposit code. This is the
preferred customer-facing model because it avoids shared-address attribution.

Operationally there are two choices:

- Per-user generated PQ accounts: each user receives a unique payment code.
  The exchange scanner stores only the matching view key for each deposit code.
- Shared settlement payment code plus internal deposit intents: simpler, but
  the exchange must require users to submit a deposit intent/hash so the amount
  can be matched.

In both designs, the scanner must credit by verified transaction hash and
decrypted note contents, not by balance delta.

## What `transaction_count: 0` means

An address can have a real balance with zero indexed normal transactions when
the balance came from genesis allocation, protocol rewards, or other
state-level credits. That is normal and must not be used as deposit evidence.

Real customer deposits produce a mined transaction hash and receipt. Shielded
deposits are still standard chain transactions, but the recipient and amount
must be detected from encrypted shielded outputs using the view key.

## Security rules

- Do not ask customers to send TKM to transparent `0x...` addresses.
- Do not credit from `eth_getBalance`.
- Do not credit from explorer transaction counts.
- Do not keep spend keys on an internet-facing scanner.
- Do not accept an unconfirmed txpool transaction as a final deposit.
- Use idempotent crediting keyed by transaction hash plus output/nullifier data.

