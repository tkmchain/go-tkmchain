# Antartical validator consensus

Antartical validators are consensus state, not an optional EVM contract. A
validator registers with a PQ transaction (`PQTkmTxType`) carrying the
`TKMVALREG1` envelope and an ML-DSA-87 public key. The transaction value is the
500,000 TKM bond and the protocol burns a 100 TKM registration fee. The
transaction sender, reward address, and ML-DSA public key must derive the same
post-quantum account address.

## Activation queue

Registrations are written to the reserved consensus registry at
`ShieldedPoolAddress`. The record contains the public key, bond, reward
address, activation height, exit height, jail height, and slash count. The
registry index is append-only and its count is part of the state root. A
registration activates no sooner than 720 blocks after it is included, so all
nodes have the same queue and activation boundary. A record with a zero bond,
an exit height, or an active jail interval is excluded from selection.

## Deterministic selection

At each block, clients sort active records by their canonical 20-byte address
and select one record using `SHA-256("TKM_VALIDATOR_SELECT_V1" || parentHash ||
height) mod totalStake`. With the fixed bond this gives a deterministic,
stake-weighted rotation and does not depend on map iteration order. The
selected record's reward address is the only address allowed to receive that
block's validator reward marker.

## Rewards and halving

After Antartical, the base 200 TKM schedule is split into four independent
shares: miner 90 TKM, selected validator 70 TKM, rotating king 35 TKM, and main
king 5 TKM. Each share is halved at the existing four-year RandomX interval.
The validator marker is a synthetic `BlockRewardValidator` transaction and is
validated against the state-selected address and the exact halved amount before
the block is accepted. Historical reward marker kinds and historical blocks are
unchanged.

## Slashing

Anyone can submit a `TKMVSLASH1` PQ envelope containing two ML-DSA-87
signatures by the same validator over different block hashes at one height.
The signatures use the domain `TKM_VALIDATOR_ATTEST_V1` plus the height and
block hash. Nodes verify the public key/address binding, require different
hashes, and replay-protect the evidence digest in state. Valid equivocation
evidence burns the validator's remaining bond, sets its exit height, and jails
the record for 21,600 blocks. A repeated evidence digest is rejected.

The registry, queue, selection, slashing state, and reward marker checks run in
the normal state transition, so bypassing them in a wallet or EVM backend does
not create a valid block.
