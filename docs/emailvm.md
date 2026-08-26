# TKM EmailVM and Domain Service

EmailVM is a canonical encrypted-mail index and domain/mailbox registry built
on TKM shielded transactions. It exposes the `tkmdomain` and `emailvm` JSON-RPC
namespaces and the `domain` and `emailvm` objects in `gtkm attach`.

Private keys, PQ seeds, passphrases, shielded notes, witnesses, and proofs stay
in the client. The daemon returns action plans; the wallet constructs the
shielded proof, signs locally, and broadcasts the raw transaction.

## Economics

- The first canonical PQ-signed shielded action that creates `@tkm` becomes
  the permanent super address. Canonical transaction order resolves races.
- One mailbox/subscriber unit costs exactly **1 TKM**.
- A custom operator domain has a fixed **2,500 TKM** registration fee.
- Operators pre-purchase capacity at **1 TKM per subscriber unit**.
- A 1,000-unit custom domain therefore costs `2500 + (1000 * 1) = 3500 TKM`.
- Every custom-domain registration and capacity expansion transfers its full
  fee to the super address.
- Each mailbox sold under a custom domain transfers 1 TKM to that domain's
  configurable payout address. A mailbox under `@tkm` transfers 1 TKM to the
  super address.

Names are lowercase and canonical. Custom domains contain letters, digits, and
interior hyphens. Mailbox usernames also permit dots and underscores. `tkm` is
reserved and cannot be registered as a custom operator domain.

Every domain and full mailbox address has a deterministic Keccak-256 registry
hash. Version-3 actions commit both that hash and the readable canonical
`domain`/`username` in the shielded transaction's proof-bound application data.
The first valid registration in canonical block order is stored permanently;
later purchases of the same canonical string or registry hash are rejected.
Version-1 and version-2 registrations are rebuilt into the same hash registry
when a node replays the chain.

See [EmailVM Permanent Name Registry](./emailvm-name-registry.md) for the exact
hash preimage, test vectors, version-3 blockchain payloads, duplicate/race
rules, RPC responses, and compatibility behavior.

## Shielded installment orders

The current shielded proof exposes at most a `uint64` public release per
withdrawal, approximately 18.446 TKM including gas sponsorship. Large domain
fees are therefore split into many proof-backed withdrawals. Every part carries
the same proof-bound EmailVM `applicationData` instruction.

The registry records partial canonical payments but does not activate a domain,
capacity expansion, or mailbox until the exact required total has been mined.
Overpayments are ignored and cannot accidentally change the quoted price.
`tkmdomain_pending` shows installment progress and transaction hashes.

The action plan returns:

- `orderId`: deterministic hash of the operation;
- `withdrawalRecipient`;
- `totalWithdrawalAmountWei` and the human TKM amount;
- `maximumPartWei` and `partCount`;
- `applicationData`, as `0x` bytes;
- client-side construction instructions.

The web wallet should broadcast all independent parts without waiting for each
previous part to confirm. Canonical block ordering determines activation.

## `gtkm attach`

Enable `tkmdomain` and `emailvm` when using an explicit API list:

```bash
./build/bin/gtkm --http --http.api eth,net,web3,tkmprivacy,tkmdomain,emailvm
```

Then attach:

```bash
./build/bin/gtkm attach
```

Create `@tkm` once. The PQ signer of the first claim mined on the canonical
chain becomes the permanent super address:

```javascript
domain.claimSuper()
domain.status()
domain.superAddress()
```

This claim plan must be attached to a small locally signed shielded spend. Paid
operator and mailbox actions remain unavailable until the claim is canonical.

Prepare a 1,000-subscriber `@john` operator registration:

```javascript
domain.quote(1000)
domain.operator(1000, "3500", "john")
domain.operatorWithPayout(1000, "3500", "john", "0xOperatorPayout")
domain.pending()
domain.get("john")
domain.hash("john")
domain.mailboxHash("alice", "john")
domain.registration(domain.mailboxHash("alice", "john"))
domain.setPayout("john", "0xNewOperatorPayout")
```

Buy mailboxes:

```javascript
domain.buy("alice", "john")  // alice@john, pays the john operator
domain.buy("alice", "tkm")   // alice@tkm, shared network namespace
domain.mailbox("alice@john")
domain.mailboxes("john")
```

If the console does not install module globals, use `web3.tkmdomain.*` and
`web3.emailvm.*` with the same arguments.

The returned plan is not a signed transaction. Pass its recipient, total,
parts, and application data to the local PQ wallet/prover workflow.

## Command-line helpers

The same plans are available outside the JavaScript console:

```bash
./build/bin/gtkm domain quote 1000
./build/bin/gtkm domain claim-tkm
./build/bin/gtkm domain operator 1000 3500 john
./build/bin/gtkm domain operator --payout 0xOperatorPayout 1000 3500 john
./build/bin/gtkm domain set-payout john 0xNewOperatorPayout
./build/bin/gtkm domain buy alice john
./build/bin/gtkm domain buy alice tkm
./build/bin/gtkm domain hash john
./build/bin/gtkm domain mailbox-hash alice john
./build/bin/gtkm domain registration 0xRegistryHash
./build/bin/gtkm domain pending
./build/bin/gtkm domain list
./build/bin/gtkm domain mailbox alice@john
```

Use `--rpc https://host/rpc` for a remote endpoint. Local IPC is the default.

## Prover integration

`/build-withdrawal` and `/build-transfer` accept the optional field:

```json
{
  "requestId": "local-unique-request-id",
  "applicationData": "0x544b4d454d41494c564d31..."
}
```

`requestId` remains a short local idempotency/note-tracking value.
`applicationData` is independently size-checked, placed in the shielded spend
metadata, committed by the shielded intent hash, and consequently covered by
both the proof and local PQ transaction signature.

For operator, expansion, and mailbox purchases, construct a shielded withdrawal
for every returned part. For key publication or encrypted email, attach the
application data to a shielded spend; a small self-transfer is sufficient.

## Encrypted EmailVM

Publish a 32-byte X25519 mailbox encryption public key:

```javascript
emailvm.publishKey("alice@john", "0x...")
emailvm.key("alice@john")
```

Construct an encrypted message action:

```javascript
emailvm.send("alice@john", "bob@tkm", "0xCiphertext", "0x12ByteNonce")
```

After the carrier shielded transaction is canonical:

```javascript
emailvm.inbox("bob@tkm")
emailvm.outbox("alice@john")
emailvm.message("0xMessageId")
```

`emailvm.key` returns the canonical mailbox, owner, public key, publication
transaction, and block. `gtkm emailvm key alice@john` exposes the same record
from scripts. A publication plan is rejected until the mailbox itself is
canonically registered, and the index accepts the key only when the canonical
PQ transaction sender owns that mailbox.

The web wallet can export a password-encrypted portable EmailVM mail keyfile.
It contains the X25519 decryption private key, owner address, mailbox names,
and public key authenticated with XChaCha20-Poly1305; its encryption key is
derived with scrypt. It never contains the ML-DSA-87 seed, shielded viewing
key, note openings, or spending key. Import therefore requires opening the
separate owning PQ wallet and verifying every mailbox and published public key
against the canonical network index.

### Consecutive message submission

Each message remains a normal locally signed, proof-backed shielded
transaction and must spend a distinct available note. The durable daemon
bucket permits up to ten one-transaction EmailVM message batches from the same
sender at consecutive nonces. This lets the browser encrypt, prove, and submit
different messages without waiting for each preceding message to confirm. The
wallet serializes local note selection and reserves a note as soon as its raw
transaction is accepted, preventing duplicate-nullifier and nonce races.

The bucket submits each valid carrier to the txpool immediately and the miner
rebuilds work on the normal new-transaction event. Inclusion is in the next
eligible block subject to canonical nonce order, block gas capacity, and
consensus validation; neither the RPC nor the wallet can guarantee a block
before one is produced. Ordinary large shielded batches remain exclusive so a
deposit or withdrawal cannot overlap another reserved nonce range.

The node verifies that the canonical PQ transaction sender owns the `from`
mailbox. EmailVM v1 publishes X25519 encryption keys; transaction authorization
remains ML-DSA-87/PQ, while X25519 content encryption itself is not
post-quantum-secure. A future ML-KEM profile can be introduced as a new key
algorithm without changing mailbox ownership. Mail bodies are client-encrypted and limited to 8 KiB; nonces are
12-32 bytes. Sender and recipient mailbox names, transaction hashes, timing,
and ciphertext sizes are public routing metadata. Clients must never submit
plaintext email content.

## Canonical state and reorgs

Every upgraded node reconstructs the first canonical `@tkm` claimant, domain,
mailbox, key, and encrypted-message
state by scanning canonical `TKMSHIELD1` transactions for the EmailVM marker.
The local index is cached in the chain database and mirrored at:

```text
~/.tkmchain/gtkm/emailvm/state.json
```

The indexed block hash is checked before extending the index. A detected reorg
discards the cache and deterministically rebuilds from canonical blocks.

This design does **not** require a consensus hardfork or a new Groth16 ceremony:
it uses an existing proof-bound opaque shielded metadata field. Nodes without
the feature still validate the transactions normally but do not expose the
EmailVM/domain index.

## Security rules

- Never expose password-based signing RPC methods on a public endpoint.
- Construct proofs and sign PQ transactions locally in the browser, Clef, or a
  trusted exchange client.
- Recheck domain availability and capacity immediately before broadcasting a
  large installment batch.
- Treat a submitted `@tkm` claim as provisional until it is canonical; another
  claimant mined first wins, and the super address cannot later be changed.
- Do not consider an order active until `domain.get`, `domain.mailbox`, or the
  canonical index reports it.
- Encryption keys are public; PQ seeds, viewing keys, note openings, and
  decryption private keys remain secret.
