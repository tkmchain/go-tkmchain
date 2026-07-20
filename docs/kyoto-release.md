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
- Operator keys require a `5000 TKM` payment record and a Main King signed operator grant. When attached to a running chain, the service verifies the payment transaction is canonical, sent by the operator, sent to Main King, and exactly `5000 TKM`.
- Number owners must sign sensitive actions: sending messages, starting calls, accepting calls, ending calls, registering device keys, transferring numbers, revoking numbers, and acknowledging delivery/read status.
- Added inbox/outbox APIs for messages and calls, device-key registration, push-style notification records, delivery/read acknowledgement, spam rate limits, payload-size limits, pruning controls, number transfer, and number revocation.
- Encrypted payload helpers use a RandomX-seed-derived service hash as the AES-256-GCM key, with nonce and route-bound authenticated data.
- TKM Phone state is persisted in the node database so operator keys, numbers, messages, calls, device keys, notifications, and counters survive daemon restart.

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
