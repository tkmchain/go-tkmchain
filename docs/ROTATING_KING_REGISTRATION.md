# Rotating Kings: operator guide

Rotating Kings (RKs) are a scheduled reward role alongside the RandomX miner
and the configured Main King. The node derives the king for a block from the
registered list, the activation height of each entry, and the chain's rotation
interval. That same selection is used when the block's reward transactions are
validated.

This guide describes the current implementation. In particular, RK
registration currently starts through the node RPC (`rk_add`); it is not a
signed EVM transaction. The RPC checks account balance, stores registration
metadata in the node's rotating-king database, announces the update to peers,
and writes a marker into miner header extra data for reconstruction from the
canonical chain. The one-TKM registration amount shown in status is a required
balance cushion, not a separately charged or burned registration transaction.

## Requirements and timing

- The address must be nonzero, not already registered, and funded with at least the
  active threshold plus the fee cushion.
- The minimum is **50,000 TKM before Antartical** and **100,000 TKM after
  Antartical**. The node also checks for a **1 TKM balance cushion**.
- RK funds are not placed in a consensus escrow by `rk_add`. The address must
  keep at least the active minimum in its ordinary account balance. Spending
  below the threshold makes it ineligible; nodes prune underfunded entries
  during RK state checks.
- Each registration is assigned the next rotation boundary as its activation
  height. Mainnet's configured interval is **100 blocks**, so a registration
  made during a slot waits until the next height divisible by 100.
- The registration metadata carries a **30-day lock/unlock horizon**. The
  node represents that horizon as an unlock time and approximate block height;
  the entry is removed when its unlock height is reached. This metadata does
  not prevent an ordinary account transaction from spending the balance.

## How the rotation is selected

At each rotation boundary, the node builds the ordered set of registered
addresses that have activated and have not expired or been pruned for low
balance. It advances to the next address in that order. The same address
receives the RK share for the blocks in its interval; it does not receive a
share for every miner or every registered king. If no eligible RK is selected,
the RK share follows the protocol's fallback reward handling.

The interval is a chain parameter (`RotatingKingRotationInterval`); the RPC
reports the current value. For the usual 100-block interval, boundaries are
heights 100, 200, 300, and so on. `rk_getKingStats` reports the current block,
current and next king, next boundary, blocks remaining, interval, and per-RK
status. `rotatingking_getRotationHistory` reports recent boundary changes.

### Rewards

Before Antartical, the current implementation splits the block subsidy as
10% Main King, 40% Rotating King, and 50% miner. After Antartical, fixed
per-block shares begin at 5 TKM Main King, 35 TKM Rotating King, 90 TKM miner,
and 70 TKM for the selected validator. The Antartical shares halve together
on the configured halving schedule. Only one RK receives the RK share at a
given block. These are block reward amounts, not a payment made at RK
registration.

## Register from the interactive wallet

Start the node with the wallet's data directory and RPC/IPC enabled, then run:

```bash
./build/bin/gtkm wallet interactive
```

From the wallet dashboard:

1. Open **Kings**.
2. Choose **Register** (or `r`) and select the funded local account.
3. Review the required balance and confirm by typing `REGISTER`.
4. Check the returned registration hash, activation/rotation status, and
   unlock height. Use the Kings screen again to refresh the status.

The wallet uses the local node's IPC connection. `rk_add` accepts an address;
the node does not receive the wallet password or private key. Registration is
not complete merely because the wallet submitted a normal payment—the local
node must accept the RPC request and share the registration update with peers.

## RPC reference

Expose the `rk` or `rotatingking` namespace on a trusted local IPC endpoint or
appropriately protected RPC endpoint. Relevant methods include:

```text
rk_add(address)                         register a funded address
rk_list()                               list registered addresses/status
rk_status(address)                      inspect one address
rk_getKingStats()                       current schedule and all RK status
rotatingking_getRotationHistory(limit)  recent rotation boundaries
```

Example read-only calls using `curl` against a local HTTP RPC endpoint:

```bash
curl -s http://127.0.0.1:8545 \
  -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","id":1,"method":"rk_getKingStats","params":[null]}'

curl -s http://127.0.0.1:8545 \
  -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","id":2,"method":"rk_status","params":["0xYourAddress"]}'
```

`rk_status` includes whether the address is registered, current, or next;
the displayed balance threshold, registration fee cushion, added height,
activation/rotation information, unlock height/time, and registration hash.
`rk_list` and `rk_getKingStats` refresh local expiry and balance eligibility
before returning their result.

## Persistence and peer propagation

The node stores RK records in its `rotatingking` database. It broadcasts new
records to connected Ethereum-protocol peers, and records a compact `RK`
marker in header extra data so nodes can recover registration and activation
metadata by scanning canonical headers after restart or database recovery.
The node also checks balance eligibility and expiry when updating RK state.

The `hash` returned in status is an identifier derived from registration
metadata; it is not a signed registration transaction or a standalone
consensus proof. Verify the address and schedule fields (`addedHeight`,
`activationHeight`, and `unlockHeight`) across nodes instead of treating this
hash alone as proof of registration.

For reliable operation, register while the node is connected to peers, keep
the node synchronized, and verify `rk_status`/`rk_getKingStats` on more than
one node. A disconnected node cannot announce an update until peers reconnect.

## Egypt test

The Egypt rehearsal covers RK eligibility checks and rotation behavior:

```bash
go run ./cmd/egypt-contract-test
```

Its JSON result includes `rotatingKingChecks`, `rotatingKingRegistration`,
and `rotatingKingActivation`.

## Current design boundary

RK records are propagated and recoverable from canonical block headers, but
the registration request itself is not currently an EVM transaction with a
consensus escrow, an on-chain fee burn, or an independently verified stake
lock. The actual stake check is an account-balance threshold, and the current
lock metadata is a schedule/eligibility record. Registration propagation also
depends on peers receiving the update and the relevant header metadata being
included. Treat these distinctions as important when describing the feature
or relying on it for custody/security.
