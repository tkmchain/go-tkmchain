# TKM Phone Service

TKM Phone is the phone-number, encrypted messaging, and WebRTC call-signaling service exposed by `gtkm` through the `tkmphone` JSON-RPC namespace. It is designed for the phone marketplace web app, but the state lives in `gtkm` so other Web3 clients can inspect and use registered numbers.

## Hardfork Activation

TKM Phone write features are gated by the `PhoneTime` hardfork. On TKMChain mainnet (`chainId 8979`), `PhoneTime` is `1784709000`, which is `2026-07-22T08:30:00Z` and corresponds to the requested 2:00pm local activation time.

Before `PhoneTime`, read-only helper RPCs such as `tkmphone_status`, prices, signing hashes, bucket listing, registered-number inspection, and WebRTC configuration remain available. State-changing phone RPCs reject with `tkm phone hardfork is not active yet`, including bucket generation, operator registration, bucket opening, number sale/transfer/revocation, device-key registration, encrypted messages, call signaling, contacts, blocking, recovery, pruning, fraud reports, and propagation import.

This keeps historical blocks and older node state from being reinterpreted by the phone feature set while letting upgraded nodes activate together at the scheduled fork time. When attached to a chain, write RPCs use the canonical head timestamp to decide activation.

Check activation with:

```sh
./build/bin/gtkm tkmphone status
```

or over HTTP:

```sh
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tkmphone_status","params":[]}'
```

Web3 clients can also call `web3.tkmphone.status()`.

## Enable RPC

Run a node with the `tkmphone` and `tkm` APIs enabled. The `tkm` namespace is needed by the web app for passphrase signing and wallet transactions.

```sh
./build/bin/gtkm \
  --http --http.addr 0.0.0.0 --http.port 8545 \
  --http.api eth,net,web3,tkm,tkmphone,mainking,miner \
  --http.vhosts '*' --http.corsdomain '*'
```

Do not expose password-capable RPC methods to untrusted networks.

## Bucket and Number Marketplace

The network supports MainKing-generated phone-number buckets and operator sales:

- MainKing generates signed number buckets with a required canonical bucket-creation transaction hash.
- The MainKing signature covers the generation round, bucket seed, bucket sizes, and creation transaction hash.
- Operators buy/open buckets and receive phone numbers.
- Operator bucket purchases are transaction-based through `paymentTx` on the operator key.
- Operators can sell single numbers to buyers.
- Buyer number purchases are transaction-based through `salePaymentTx` on the phone number.
- Buyers become the on-chain owner of the sold phone number through `tkmphone_sellNumber`.

The website at `https://phone.tkmchain.site` adds SQLite bookkeeping for orders, listings, local wallets, SIM slots, and registered-number market status.

`gtkm` persists the authoritative phone service state in the chain database and also writes a readable mirror to the node instance directory at `phone/state.json`. With the normal TKMChain datadir layout this is under `~/.tkmchain/gtkm/phone/state.json`. The mirror includes buckets, bucket `creationTx` hashes, operator bucket `paymentTx` hashes, generated numbers, number `salePaymentTx` hashes, messages, calls, device keys, notifications, contacts, recovery records, reports, and propagation records.

Ownership handoff is explicit in the phone records without adding a consensus rule. Bucket creation records `issueHash`; bucket purchase approval records `assignHash` and moves ownership from MainKing to the operator; number purchase records `transferHash` and moves ownership from operator to the buyer. Clients should call `tkmphone_numberOwnershipProof(number)` before exporting a SIM or trusting a marketplace listing. The proof contains the MainKing issue step, MainKing-to-operator bucket transfer, operator-to-user number transfer, the payment transaction hashes, and a stable `proofHash`.

## MainKing Bucket and Approval CLI

MainKing bucket generation and operator approval must be done from a private MainKing `gtkm` node, not from a public phone-market website. A hosted marketplace should only read `tkmphone_buckets`, record user payments, and display pending order data. This keeps the MainKing password and signing keys off the web server.

MainKing must provide a canonical bucket-creation transaction hash when generating buckets. The digest and signature are bound to that hash. Create a small marker transaction from MainKing to MainKing, wait until it is mined, then use that tx hash as `--creation-tx`.

```sh
SEED=0x$(openssl rand -hex 32)
CREATION_TX=0xYourMinedMainKingCreationTransactionHash

./build/bin/gtkm tkmphone status
./build/bin/gtkm tkmphone next-round
./build/bin/gtkm tkmphone bucket-hash --seed $SEED --creation-tx $CREATION_TX
./build/bin/gtkm tkmphone generate-buckets --seed $SEED --creation-tx $CREATION_TX --mainking 0xc40F4A0b4df81F8f67A88B179a8b2271107a9ac2
./build/bin/gtkm tkmphone buckets
./build/bin/gtkm tkmphone ownership-proof --number +8979...
```

For offline signing, sign the hash returned by `bucket-hash`, then submit the signature with the same `--creation-tx` value:

```sh
./build/bin/gtkm tkmphone generate-buckets --seed $SEED --creation-tx $CREATION_TX --signature 0xMainKingSignature
```

The service allows only five unsold buckets at a time. MainKing can generate the next batch only after the current five buckets have all been bought and assigned.

When an operator buys a bucket from the website, the website records these public values for MainKing review:

- `operator`: operator wallet address
- `key_hash`: operator key hash generated by the website
- `expires_at`: operator key expiry timestamp
- `payment_tx`: canonical operator -> MainKing bucket payment transaction
- `paid_wei`: must be exactly `0x54b40b1f852bda00000` (`25000 TKM`)

MainKing verifies the payment on-chain, computes the grant hash, signs it on the private node, and registers the operator key. Example JSON-RPC flow:

```sh
RPC=http://127.0.0.1:8545
MAINKING=0xc40F4A0b4df81F8f67A88B179a8b2271107a9ac2
OPERATOR=0xOperatorAddress
KEY_HASH=0xOperatorKeyHash
EXPIRES_AT=0xExpiryTimestampHex
PAYMENT_TX=0xOperatorPaymentTx
PAID_WEI=0x54b40b1f852bda00000

GRANT_HASH=$(curl -s "$RPC" -H 'content-type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tkmphone_operatorGrantHash\",\"params\":[\"$OPERATOR\",\"$KEY_HASH\",\"$EXPIRES_AT\",\"$PAYMENT_TX\"]}" \
  | sed -n 's/.*"result":"\([^"]*\)".*/\1/p')

SIGNATURE=$(curl -s "$RPC" -H 'content-type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tkm_signHashWithPassphrase\",\"params\":[\"$MAINKING\",\"$GRANT_HASH\",\"MAINKING_WALLET_PASSWORD\"]}" \
  | sed -n 's/.*"result":"\([^"]*\)".*/\1/p')

curl -s "$RPC" -H 'content-type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tkmphone_registerOperatorKey\",\"params\":[\"$OPERATOR\",\"$KEY_HASH\",\"$EXPIRES_AT\",\"$PAYMENT_TX\",\"$PAID_WEI\",\"$SIGNATURE\"]}"
```

After this RPC succeeds, the website can detect the approval with `tkmphone_listOperators`. The operator then refreshes or opens their bucket; the website signs only with the operator wallet and calls `tkmphone_openBucket`.

### Daemon-only pending approvals

New bucket payments are self-describing. The wallet sends the `25000 TKM` bucket payment to MainKing with transaction data:

```text
TKMPHONE_BUCKET_V1 || operatorKeyHash || expiresAtUint64
```

This lets `gtkm` discover pending approvals from canonical chain data only. The website does not submit approval records and never handles MainKing credentials.

MainKing can list pending operator bucket approvals from the daemon:

```sh
./build/bin/gtkm tkmphone pending-approvals --scan-blocks 20000
```

Each row includes the operator, key hash, payment transaction, expiry, and grant hash. To approve one pending payment with an unlocked MainKing account:

```sh
./build/bin/gtkm tkmphone approve-operator   --payment-tx 0xOperatorBucketPaymentTx   --mainking 0xc40F4A0b4df81F8f67A88B179a8b2271107a9ac2
```

For offline signing, sign the listed `grantHash`, then submit:

```sh
./build/bin/gtkm tkmphone approve-operator   --payment-tx 0xOperatorBucketPaymentTx   --signature 0xMainKingSignature
```

From `gtkm attach`, the same daemon-only data is visible through Web3:

```js
web3.tkmphone.pendingOperatorApprovals(20000)
```

If you sign manually in the console, pass the signature back to the daemon RPC:

```js
web3.tkmphone.approveOperatorPayment("0xOperatorBucketPaymentTx", "0xMainKingSignature")
```

After approval succeeds, operator wallets detect it with `tkmphone_listOperators` and auto-open the assigned bucket.

## Registered Numbers

A phone number is treated as registered when it has at least one active device key in `gtkm`. Registration is done with:

- `tkmphone_deviceKeySigningHash(number, device, publicKey)`
- owner signs that hash
- `tkmphone_registerDeviceKey(number, device, publicKey, signature)`

Once registered, signed network actions require an active device key. The following signed RPC actions reject unregistered numbers:

- `tkmphone_sendEncryptedMessage`
- `tkmphone_sendEncryptedMessageWithExpiry`
- `tkmphone_startCall`
- `tkmphone_startCallWithExpiry`
- `tkmphone_acceptCall`
- `tkmphone_rejectCall`
- `tkmphone_endCall`
- `tkmphone_addCallCandidate`
- `tkmphone_callCandidates`

The local marketplace also marks registered numbers as permanently off market. After device registration or SIM import, the website writes status `registered` and refuses resale.

## Registered Number RPC

`gtkm` exposes registered-number details directly:

| Method | Params | Result |
| --- | --- | --- |
| `tkmphone_registeredNumber` | `number` | number metadata, registered flag, active device count, active device keys |
| `tkmphone_registeredNumbers` | none | all numbers with at least one active device key |
| `tkmphone_deviceKeys` | `number` | all device keys for a number |

Example:

```sh
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tkmphone_registeredNumbers","params":[]}'
```

The Web3 extension also exposes:

- `web3.tkmphone.registeredNumber(number)`
- `web3.tkmphone.registeredNumbers()`
- `web3.tkmphone.deviceKeys(number)`
- `web3.tkmphone.deviceKeySigningHash(number, device, publicKey)`
- `web3.tkmphone.transferNumberSigningHash(number, newOwner)`

## SIM Slots and Import

The website keeps local SIM slots in SQLite. A SIM slot is the local operational profile for one phone number and stores:

- phone number
- owner address
- device id
- public key
- private key backup metadata
- bucket/listing references

SIM slots are created automatically for owned numbers, but they cannot be used on the network until a device key is registered in `gtkm`. The `Register device key` button registers the device on `gtkm`, updates the local SIM slot, and marks the number permanently off market.

A downloaded SIM/number JSON can be restored with the website's `Import downloaded number` control. Import recreates the local SIM slot and marks the number registered/off-market locally. If the number is not owned by the logged-in wallet, the server rejects the import.

## Encrypted Messages

Messages use the `tkmphone` RPC as encrypted storage/signaling:

1. Select a registered SIM slot.
2. Encrypt payload with `tkmphone_encryptPayload`.
3. Sign `tkmphone_sendMessageSigningHash` with the owner wallet.
4. Submit `tkmphone_sendEncryptedMessage`.
5. Recipient loads private inbox through the website, which calls `tkmphone_messagesForNumber` for the selected SIM slot.

## Voice Calls

Voice calls use browser WebRTC for audio and `gtkm` as encrypted call signaling.

Website call buttons:

- `Dial`: creates local microphone stream, creates WebRTC offer, encrypts it, signs it, and stores it with `tkmphone_startCall`.
- `Receive selected`: loads the selected incoming call, decrypts the offer, creates a WebRTC answer, encrypts it, signs it, and stores it with `tkmphone_acceptCall`.
- `Connect answer`: caller loads/decrypts the answer and sets it as the remote description.
- `Poll candidates`: exchanges encrypted ICE candidates through `tkmphone_addCallCandidate` and `tkmphone_callCandidates`.
- `End`: signs and submits `tkmphone_endCall`, then stops local audio tracks.

The page includes local and remote audio elements. Browser microphone permission is required. For non-local network access, the browser may require HTTPS or a secure context for reliable microphone/WebRTC behavior.

## Tests

The phone service is covered by focused tests:

```sh
go test ./eth -run TkmPhone -count=1
```

For the public Web3/RPC message and call path, run:

```sh
go test ./eth -run TestTkmPhoneAPIEncryptedMessageAndVoiceCallFlow -count=1 -v
```

That regression test covers the complete client-facing flow: MainKing bucket generation, operator registration, bucket opening, number sales, device-key registration, encrypted `hello` delivery to the recipient inbox, notification creation, encrypted WebRTC offer/answer signaling, ICE candidate exchange, call listing, WebRTC config validation, and signed call termination.

The broader `TkmPhone` test group also covers bucket issuance, number sales, messages, calls, registered device keys, registered-number inspection, persistence, propagation, and signed action checks.
