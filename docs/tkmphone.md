# TKM Phone Service

TKM Phone is the phone-number, encrypted messaging, and WebRTC call-signaling service exposed by `gtkm` through the `tkmphone` JSON-RPC namespace. It is designed for the phone marketplace web app, but the state lives in `gtkm` so other Web3 clients can inspect and use registered numbers.

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

- MainKing generates signed number buckets.
- Operators buy/open buckets and receive phone numbers.
- Operators can sell single numbers to buyers.
- Buyers become the on-chain owner of the sold phone number through `tkmphone_sellNumber`.

The website at `/home/mike/tkm-phone-market` adds SQLite bookkeeping for orders, listings, local wallets, SIM slots, and registered-number market status.

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

These tests cover bucket issuance, number sales, messages, calls, registered device keys, registered-number inspection, persistence, propagation, and signed action checks.
