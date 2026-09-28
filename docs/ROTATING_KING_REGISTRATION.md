# Rotating-king registration

Rotating-king registration is checked against chain state and a block-height
activation schedule. The RPC path and the consensus manager now use the same
minimum stake: **100,000 TKM**. A registration also requires the existing
1-TKM registration fee reserve.

## RPC

The `rk` namespace exposes:

```text
rk_add(address)                 register a funded address
rk_list()                       list registrations and lock metadata
rk_status(address)              inspect one registration
rk_getKingStats()               inspect the rotation schedule
```

`rk_add` rejects the zero address, duplicate registrations, and accounts with
less than the stake plus fee reserve. The registration is persisted in the
rotating-king database and announced to peers. Once an account falls below the
100,000-TKM stake, it is removed from the schedule on the next state check;
this prevents an underfunded address from continuing to receive king rewards.

The RPC registration metadata is a reservation and eligibility record. It is
not an arbitrary balance mutation performed outside a block. A wallet that
spends the reserved balance therefore becomes ineligible and is pruned, while
the normal transaction remains subject to the chain's state-transition rules.

## Deterministic consensus rules

The consensus manager's `RegisterKingAt` method validates:

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

