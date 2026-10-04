# Stamp an unfunded address with an external sponsor

An ML-DSA-87 account can create its encrypted stamp and `tkmshield3.` receiving
address without any TKM. Registering the stamp on-chain costs gas. The interactive
wallet's **12) Stamp sponsorship** lets a different, funded, already-stamped
ML-DSA account pay that gas. This uses the existing Antartical sponsorship rules;
it does not change consensus or introduce a new fork.

Both parties run a node on the same chain and open:

```sh
./build/bin/gtkm wallet interactive
```

Use mainnet chain ID 8979 or Egypt chain ID 8980 consistently. A test network
must use its own data directory, for example `--datadir ~/.tkmchain-egypt`.

## Exchange the codes

1. **Recipient:** choose **12 → 1) Prepare my receiving address**. Select the
   local ML-DSA account and unlock it. If needed, enter a private name and country.
   The exact encrypted stamp is saved in the keyfile. Save the receiving address
   to a new text file and give that public file to the sponsor. This step costs
   no gas. An old ECDSA account must first migrate to ML-DSA-87.
2. **Sponsor:** choose **12 → 2) Create an offer code**. Select and unlock the
   funded, registered sponsor account. Import the recipient address by entering
   `@/path/to/recipient.txt`. Review the recipient, maximum gas fee and expiry,
   then type `OFFER`. Save and send the `tkmstamp1.` offer code to the recipient.
   An offer does not spend any funds or register the recipient.
3. **Recipient:** choose **12 → 3) Enter offer code**. Select the same recipient
   account and import the offer with `@/path/to/offer.txt`. Review the sponsor
   and fee terms, then type `AUTHORIZE`. The wallet builds the ownership proof
   locally and creates an authorization code. Return that code file to the sponsor.
4. **Sponsor:** choose **12 → 4) Pay registration**. Import the authorization
   file, review the verified recipient and maximum fee, and type `PAY`. The wallet
   validates the proof, signs the transaction, saves an exact signed copy, and
   broadcasts it. It shows the transaction hash and waits for a successful receipt
   and matching on-chain stamp registry entry.

The final return step is required: the sponsor's transaction signature covers
the recipient's completed proof. An offer alone is not a signed payment. Neither
party gives the other its seed, password, keyfile, viewing key or stamp key.

Codes and Shield3 addresses are long; use public text files rather than pasting
them into terminals with input limits. Export refuses to overwrite an existing
file. Keep the recipient's complete encrypted keyfile backup: the seed alone
does not reproduce the original randomized stamp.

## Fees, expiry and retry

The sponsor needs a confirmed stamp and sufficient **public** TKM for the
maximum registration fee. The recipient can have zero public TKM. Sponsorship
pays gas only; it does not add a spendable balance to the recipient. Once the
stamp is confirmed, a separate Shield3/Shield4 payment can fund that address.

Offers expire within one hour of the chain head timestamp. If the sponsor uses
the offered nonce for another transaction, the wallets reject the stale offer;
create a new offer. Keep one outstanding offer per sponsor nonce. An unchanged
or stale node head is not evidence that the network is progressing.

Before broadcast, the sponsor wallet saves the signed transaction under
`<keystore>/.stamp-submissions/<transaction-hash>.txt` and prints the full path.
If RPC disconnects or confirmation times out, use **12 → 5) Retry a saved signed
registration**, enter `@/path/to/the/saved/file.txt`, and type `RETRY`. This resends
the exact transaction, preserving its hash, proof and nonce. An already-confirmed
matching stamp is reported without submitting another registration.

If a transaction stays pending, check that blocks are advancing, the node is
synced, and the sponsor has no earlier pending nonce. Do not create repeated
payments simply because a receipt has not arrived. The wallet reports pending
or failed confirmation separately from successful registration.
