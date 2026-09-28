# Rotating-king registration

Rotating-king registration is checked against chain state and a block-height
activation schedule. Existing pre-Antartical registrations keep the legacy
minimum of **50,000 TKM**. At Antartical activation the minimum becomes
**100,000 TKM**, matching the consensus manager. A registration also requires
the existing 1-TKM registration fee reserve.

## RPC

The `rk` namespace exposes:

```text
rk_add(address)                 register a funded address
rk_list()                       list registrations and lock metadata
rk_status(address)              inspect one registration
rk_getKingStats()               inspect the rotation schedule
```

`rk_add` rejects the zero address, duplicate registrations, and accounts with
less than the active fork's stake plus fee reserve. The registration is
persisted in the rotating-king database and announced to peers. Once an
account falls below the active stake, it is removed from the schedule on the
next state check; this prevents an underfunded address from continuing to
receive king rewards.

## Interactive wallet

The console wallet provides the same operations without manually constructing
an RPC request. Start `gtkm` with the intended data directory and then run:

```bash
./gtkm wallet interactive
```

Choose **Kings** from the dashboard:

1. Press `r` to register a local account. Select the account, review the
   active stake requirement and registration fee reserve, then type `REGISTER`.
2. Press `s` to query one address. Enter either a local account number or a
   full address to see its registration hash, locked amount, unlock height,
   added height, and whether it is current or next in the rotation.
3. The Kings screen automatically lists registrations and recent rotation
   history below the schedule.

The wallet signs locally through the node's IPC endpoint. It never sends a
password or private key to `rk_add`. Save the registration hash shown after a
successful call; it is the chain-bound commitment used to compare the
registration across nodes. Registration does not create an out-of-band
balance change: spending the reserved stake makes the account ineligible and
the consensus manager prunes it on the next state check.

The RPC registration metadata is a reservation and eligibility record. It is
not an arbitrary balance mutation performed outside a block. A wallet that
spends the reserved balance therefore becomes ineligible and is pruned, while
the normal transaction remains subject to the chain's state-transition rules.

## Deterministic consensus rules

The consensus manager's `RegisterKingAt` method validates the Antartical
policy:

1. a non-zero address;
2. a non-negative balance at least equal to the 100,000-TKM minimum;
3. no duplicate address;
4. a two-block activation delay with overflow rejection; and
5. a deterministic registration commitment containing the address, stake,
   added height, activation height, and (when configured) chain ID.

Rotation refuses to advance when no candidate has enough stake. It never
selects an underfunded candidate and records a false eligible result.

## Egypt rehearsal

Egypt (chain ID `8980`) exercises underfunded and duplicate rejection,
chain-bound registration hashing, activation at a rotation boundary, and
eligible selection:

```bash
go run ./cmd/egypt-contract-test
```

The JSON output includes `rotatingKingChecks`,
`rotatingKingRegistration`, and `rotatingKingActivation`.
