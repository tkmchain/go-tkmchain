# Kyoto Release Notes

Release: `gtkm v1.17.6-kyoto`
Date: 2026-07-15
Network: TKMChain RandomX mainnet, chain ID `8979`
Fork name: `Kyoto`
Fork timestamp: `1784115119`

## Summary

Kyoto is the RandomX hardfork that makes post-fork proof validation strict and disconnects old clients through the fork ID schedule. Nodes should upgrade to `v1.17.6-kyoto` before syncing or mining past the Kyoto timestamp.

## Consensus Changes

- Added `KyotoTime` to the chain configuration.
- Added `IsKyoto` fork rules and config compatibility checks.
- Kyoto participates in fork ID calculation automatically because fork IDs gather timestamp forks from `ChainConfig`.
- Before Kyoto, historical RandomX compatibility paths remain enabled so old blocks can sync.
- At and after Kyoto, RandomX seal verification accepts only the canonical proof format.

## Network Security

- Added mandatory RandomX checkpoints at blocks `2370`, `6000`, and `7165` to protect nodes from syncing onto known-bad forks.
- Local startup now verifies every configured checkpoint that is already below the local head and stops the daemon if the canonical hash does not match.
- Peer connections now use the configured checkpoints as network required-block challenges. A peer that serves a different hash for a listed checkpoint is banned for 365 days and disconnected.
- Checkpoint announcements are now signed network messages. New checkpoints broadcast over the peer network must include a signature from the configured Main King address.
- Added `king_checkpointSigningHash(number, hash)` so operators can get the exact checkpoint digest to sign. The digest is bound to the TKMChain checkpoint domain, chain ID, block number, and block hash.
- Added `king_addSignedCheckpoint(number, hash, signature)` to verify, store, and broadcast a signed checkpoint. Signatures are accepted as either raw digest signatures or standard Ethereum signed-message signatures over the digest.
- Unsigned `king_addCheckpoint(number, hash)` remains local-only and no longer broadcasts unsigned checkpoint messages.
- Peers that announce conflicting, unsigned, badly signed, or locally impossible checkpoints are banned for 365 days and disconnected.

## Reward Split

The block subsidy is `200 TKM` before the first halving and is split as:

- Main King: `20 TKM` (`10%`)
- Rotating King: `80 TKM` (`40%`)
- Miner or pool wallet: `100 TKM` (`50%`)

Pool software should distribute only the miner share. The Main King and Rotating King rewards are separate block reward outputs handled by the chain.

## TKM Phone Service

- Added native `tkmphone` RPC and web3 extension support for encrypted number-based messaging and call sessions.
- Added `PhoneTime` hardfork activation at timestamp `1784701800` (`2026-07-22T06:30:00Z`, requested 12:00pm local activation). TKM Phone write RPCs are rejected before this timestamp so historical state is not reinterpreted by phone registration, bucket, device-key, message, call, contact, recovery, report, or propagation rules. Added `tkmphone_status` for activation inspection.
- Added a permanent post-PhoneTime difficulty rule: deterministic integer adjustment, 25% per-block rise/drop caps, skipped-interval reductions for long stalls, and an EDA floor above genesis difficulty so the chain can recover without falling back to difficulty `3`.
- Operator bucket keys require a `25000 TKM` payment record and a Main King signed operator grant. The `25000 TKM` bucket price is 5 numbers at `5000 TKM` each. When attached to a running chain, the service verifies the payment transaction is canonical, sent by the operator, sent to Main King, and exactly `25000 TKM`. Main King generates phone numbers only as signed buckets. Each bucket contains 5 numbers, only 5 unsold buckets can exist at a time, and a new batch of 5 can be generated only after all current buckets are bought. Each accepted operator key consumes one available bucket and receives those 5 active numbers.
- Number owners must sign sensitive actions: sending messages, starting calls, accepting calls, ending calls, registering device keys, transferring numbers, revoking numbers, and acknowledging delivery/read status.
- Added inbox/outbox APIs for messages and calls, device-key registration, push-style notification records, delivery/read acknowledgement, spam rate limits, payload-size limits, pruning controls, number transfer, and number revocation.
- Added WebSocket subscription support for new message, call update, call signaling, and notification events.
- Added eth-protocol TKM Phone propagation gossip plus export/import RPCs. Peers now relay encrypted payload records for operator keys, generated numbers, device keys, phone messages, call lifecycle updates, contacts, blocked-number updates, recovery changes, and operator reports.
- Added real voice-call signaling support for number-to-number WebRTC sessions. The node stores and propagates signed encrypted offers, answers, and ICE candidates, while mobile/web clients carry the live audio stream peer-to-peer through WebRTC using the TKM Phone number as the identity. Calls now include reject, missed-timeout, encrypted candidate listing, call-signal subscriptions, and a `webRTCConfig` RPC that exposes audio-only encrypted media requirements, signaling action names, signal size limits, candidate rate limits, and default STUN server hints.
- Added multi-device encryption helpers that produce one RandomX-seed-derived AES-256-GCM envelope per registered recipient device key.
- Added Main King bucket listing/generation APIs, operator marketplace listing, signed operator-only bucket opening, `10000 TKM` phone-number sale validation, and signed fraud-report records for operator accountability. A sold number moves from operator ownership to the buyer only after the canonical sale payment transaction is verified as buyer -> operator for exactly `10000 TKM`. Sale validation rejects forged numbers unless they carry Main King bucket provenance and belong to the operator bucket.
- Added message and call expiry timestamps, plus pruning that removes expired communication state.
- Added encrypted contact records, per-number blocking/unblocking, and recovery-key registration so a recovery address can move a number to a new owner.
- Encrypted payload helpers use a RandomX-seed-derived service hash as the AES-256-GCM key, with nonce and route-bound authenticated data.
- TKM Phone state is persisted in the node database so operator keys, numbers, messages, calls, device keys, notifications, contacts, blocked-number lists, recovery keys, reports, propagation records, and counters survive daemon restart.

### TKM Phone Web3 Examples

Generate an owner-signed action digest before sending a message:

```javascript
const digest = web3.tkmphone.ownerActionHash(fromNumber, "send-message", payloadHex)
const signature = await wallet.signMessage(web3.utils.hexToBytes(digest))
const msg = web3.tkmphone.sendEncryptedMessage(fromNumber, toNumber, ciphertextHex, nonceHex, signature)
```

Send an expiring message and watch events over WebSocket RPC:

```javascript
const expiresAt = Math.floor(Date.now() / 1000) + 3600
web3.tkmphone.sendEncryptedMessageWithExpiry(fromNumber, toNumber, ciphertextHex, nonceHex, expiresAt, signature)
web3.currentProvider.send({jsonrpc: "2.0", id: 1, method: "tkmphone_newMessages", params: []})
```

Use the added management APIs from web3 as `status`, `bucketPrice`, `operatorKeyPrice`, `mainKingNumberPrice`, `numberSalePrice`, `nextBucketRound`, `bucketGenerationHash`, `generateBuckets`, `buckets`, `listOperators`, `openBucketHash`, `openBucket`, `operatorInventory`, `sellNumber`, `deviceKeySigningHash`, `transferNumberSigningHash`, `webRTCConfig`, `rejectCall`, `expireRingingCalls`, `callCandidateHash`, `callCandidateSigningHash`, `callCandidateListHash`, `callCandidateListSigningHash`, `addCallCandidate`, `callCandidates`, `reportOperator`, `addContact`, `contacts`, `blockNumber`, `unblockNumber`, `registerRecovery`, `recoverNumber`, `propagationQueue`, and `importPropagation`.

### TKM Phone CLI Examples

The `gtkm tkmphone` command attaches to the local IPC endpoint by default, or to the endpoint passed as the last argument. The `--seed` value is a fresh 32-byte random value chosen by MainKing for each bucket generation round. It is not the chain RandomX seed. Generate one with `openssl rand -hex 32` and prefix it with `0x`. MainKing can generate the next batch of five signed buckets with an unlocked account:

```bash
./build/bin/gtkm tkmphone prices
./build/bin/gtkm tkmphone next-round
SEED=0x$(openssl rand -hex 32)
./build/bin/gtkm tkmphone generate-buckets --seed $SEED --mainking 0xc40F4A0b4df81F8f67A88B179a8b2271107a9ac2
./build/bin/gtkm tkmphone buckets
```

For offline signing, get the digest first and pass the resulting signature manually:

```bash
./build/bin/gtkm tkmphone bucket-hash --seed 0x1111111111111111111111111111111111111111111111111111111111111111
./build/bin/gtkm tkmphone generate-buckets --seed 0x1111111111111111111111111111111111111111111111111111111111111111 --signature 0xSignature
```

Operators can open only their assigned bucket; if the operator account is unlocked, the CLI signs `tkmphone_openBucketHash` automatically:

```bash
./build/bin/gtkm tkmphone open-bucket --operator 0xOperator --bucket 1
```

### TKM Phone Tests

Run the focused phone regression suite with:

```bash
go test ./eth -run TkmPhone -count=1
```

The client-facing Web3/RPC message and voice-call path is covered directly by:

```bash
go test ./eth -run TestTkmPhoneAPIEncryptedMessageAndVoiceCallFlow -count=1 -v
```

This test exercises MainKing bucket generation, operator bucket opening, number sale, device-key registration, encrypted message delivery, private inbox/notification lookup, encrypted WebRTC offer/answer signaling, ICE candidate exchange, call listing, WebRTC config checks, and signed call termination.

## Mining Notes

Mining work requires an etherbase. If you see:

```text
Refusing to generate mining work without etherbase
```

restart the node with a reward address, for example:

```bash
./build/bin/gtkm --randomx --miner.etherbase 0xYourPoolWallet ...
```

For pool mining, do not use `--mine` for local solo sealing. Use the external miner/pool mode that serves `miner_getWork` with `--miner.etherbase` set to the pool wallet.

## Upgrade Steps

1. Pull or deploy the Kyoto source.
2. Build the daemon:

```bash
go build -o build/bin/gtkm ./cmd/gtkm
```

3. Confirm the version:

```bash
./build/bin/gtkm version
```

Expected:

```text
Version: 1.17.6-kyoto
```

4. Restart all validators, RPC nodes, pool nodes, and seed nodes with the new binary.
5. Make sure pool nodes include `--miner.etherbase 0xYourPoolWallet`.

## Compatibility

Nodes that do not know the Kyoto timestamp should be treated as stale after the fork. Updated nodes advertise the Kyoto fork through fork ID and should reject incompatible peers once the local head passes the Kyoto timestamp.

## Operational Checks

After restart, check:

- The node reports `Gtkm/v1.17.6-kyoto` in `web3_clientVersion`.
- The daemon syncs historical pre-Kyoto blocks without `invalid proof` failures.
- New post-Kyoto blocks are produced only by canonical RandomX proof validation.
- Pool blocks credit `100 TKM` to the pool wallet, while chain reward outputs account for the additional `20 TKM` Main King and `80 TKM` Rotating King rewards.

## TKM Phone Service

Added network-aware phone-number registration, device-key inspection, encrypted message signaling, and WebRTC call signaling through the `tkmphone` RPC namespace. Full operational details are in [TKM Phone Service](./tkmphone.md).
