# TKM Pool Treasury Smart Account

## Purpose

`TKMPoolTreasurySmartAccount` is a non-consensus smart account for the mining pool treasury. It is initialized with the pool's existing public wallet:

```text
0x4441d6fEd0836B77a503e0B2788bfEd6FD8c23A8
```

Only the public address was read from `/home/mike/pool/config.json`. No password, private key, token, or other credential is embedded in the source or artifacts.

## Source

```text
contracts/smartaccount/TKMPoolTreasurySmartAccount.sol
```

The contract inherits `TKMAccount`, including:

- replay-protected EntryPoint execution;
- owner and threshold signatures;
- high-value multisignature policy;
- daily native-TKM spending limits;
- target/selector/value/time-restricted payout session keys;
- emergency locking;
- threshold guardian recovery with a mandatory delay;
- owner-controlled policy changes through authorized self-calls.

The initial threshold is one because only one pool owner address is presently configured. The initial owner can later add additional owners and raise the threshold through an authorized smart-account self-call.

## Build

```bash
cd /home/mike/go-tkmchain
./contracts/smartaccount/compile.sh
```

Artifacts:

```text
contracts/smartaccount/artifacts/TKMPoolTreasurySmartAccount.abi
contracts/smartaccount/artifacts/TKMPoolTreasurySmartAccount.bin
```

## Deployment

Deploy the shared `TKMEntryPoint` first. Then deploy the pool account with the EntryPoint address as its only constructor argument:

```text
constructor(address entryPoint)
```

Do not send pool funds to a predicted or newly deployed address until all of the following have been verified:

1. The deployment transaction succeeded.
2. Runtime bytecode matches the locally compiled audited artifact.
3. `INITIAL_POOL_OWNER()` returns the expected pool wallet.
4. `entryPoint()` returns the intended audited EntryPoint.
5. A low-value deposit and withdrawal succeeds.
6. Replay of the same operation fails.
7. Locking, session revocation, and recovery cancellation have been tested.

Deployment is an on-chain action and is intentionally not performed automatically by the build process.

## Recommended Initial Policy

Before moving material pool funds, use authorized self-calls to:

1. Add at least two separately secured owner addresses.
2. Set an owner threshold of two.
3. Add independent recovery guardians.
4. Set a recovery threshold of at least two and a delay of 48 hours or longer.
5. Set a conservative daily payout limit.
6. Set a high-value threshold that requires all configured owner signatures.
7. Create a restricted payout session key only if automated payouts require it.

Do not use the same private key for the primary owner, payout session, and recovery guardian.

## Automated Payout Session

If the pool needs automated payouts, create a session key with:

- the payout contract as the only target;
- the payout function selector as the only selector;
- a maximum value per payout;
- a total remaining payout budget;
- a short validity period;
- no permission to call the treasury account itself.

This prevents the payout process from changing owners, guardians, recovery configuration, limits, or other sessions. Revoke and rotate the session immediately if the payout host is compromised.

## No Hardfork

This contract uses existing EVM-compatible contract creation and calls. It does not change consensus, RandomX, Rotating Kings, block validation, transaction encoding, rewards, gas rules, or fork configuration. No hardfork is required.
