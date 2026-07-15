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

## Reward Split

The block subsidy is `200 TKM` before the first halving and is split as:

- Main King: `20 TKM` (`10%`)
- Rotating King: `80 TKM` (`40%`)
- Miner or pool wallet: `100 TKM` (`50%`)

Pool software should distribute only the miner share. The Main King and Rotating King rewards are separate block reward outputs handled by the chain.

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
