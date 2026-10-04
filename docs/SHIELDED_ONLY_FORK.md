# Shield3/Shield4 private-payment cutoff

## Implementation and proposed schedule

**Superseded for the current rollout scope:** the accepted
[confidential ledger proposal](CONFIDENTIAL_LEDGER.md) allows explicit public
deposits and withdrawals. This older, stricter cutoff remains disabled and
must not be enabled alongside the ledger.

The working-tree implementation uses a separate `shieldedOnlyTime` fork.
The proposed time is **October 4, 2026, 10:00 UTC** (`1791108000`). Live-network
defaults leave this field unset until confidential funding and protocol
operations work. Tests explicitly configure the cutoff on independent copies
of the network configuration. No live activation, release, or deployment is
established by this document.

This cutoff restricts user transactions to existing private notes. It does
**not** implement confidential issuance or migration of public account balances.
Those are separate protocol work described in
[Confidential value consensus](CONFIDENTIAL_VALUE_CONSENSUS.md).

## Consensus rules

At or after the configured block timestamp, a user payment must:

1. Be a post-quantum transaction addressed to the shielded pool.
2. Carry a canonical Shield3 or Shield4 envelope and zero outer `value`.
3. Spend existing notes, with no public deposit or withdrawal.
4. Have zero withdrawal recipient, withdrawal value, and gas sponsor reserve.
5. Pass the existing native proof verifier, ownership/stamp checks, membership,
   range, asset, nullifier, and value-conservation checks.

A zero-value stamp registration remains permitted and must pass its own
ownership/proof validation. Prefixes alone do not authorize a transaction.
The policy applies during transaction admission, shielded processing, and EVM
execution. It also rejects pre-fork deposits left in the mempool when a new
block reaches activation.

The existing native spend relation already supports zero public value. This
change constrains its accepted public inputs; it does not replace the STARK
circuit or reinterpret historical proofs. Historical blocks retain their
original rules. Configuration compatibility rejects silently changing the
timestamp once the fork has been reached. Fork-ID negotiation includes the new
timestamp.

## What remains visible

Payment principal is represented by encrypted outputs and commitments. A
private spend under this policy has zero public principal and does not add to
or subtract from the public pool backing balance. However, the transaction hash,
block position, outer signing account or relay account, gas, fees, ciphertexts,
and existing public protocol data remain visible. This is not transaction
invisibility or a rewrite of historical data.

The aggregate legacy pool balance remains public. Protocol-generated mining,
validator, and king rewards still use their existing public reward rules.

## Funding and operational consequences

Public balances cannot be turned into new private notes after the cutoff.
Deposits before activation reveal their amounts permanently. Funds already in
private notes can continue to move privately; they cannot be withdrawn to a
public account under this policy.

In particular, a mining pool cannot replenish private payout liquidity from
new public mining rewards after activation. It needs previously funded notes.
Public-value governance votes, validator registration/bond withdrawal, and
other non-note user protocol envelopes are also rejected. This cutoff must not
be described as a complete replacement for those operations. Confidential
funding, issuance, and protocol operations must be implemented before a rollout
that needs those functions to continue.

## Fees and explicit relay sponsorship

Previously, a note could release a public gas reserve, with unused gas refunded
to a public account. Requiring zero `GasSponsorValue` closes that public value
release path. The sender must have enough public balance for gas, or use an
operator that explicitly agrees to pay fees without reimbursement from notes.

Wallet APIs provide:

- `BuildFeeSponsoredRelayOffer`: signed version-2 fee quote, bound to the
  operator, chain, nonce, fees, and expiry.
- `BuildFeeSponsoredRelaySubmission`: verifies the native proof and requires
  an explicit maximum fee in wei before the operator signs.
- Authenticated local wallet service `/shield3/fee-sponsored-relay-offer` and
  `/shield3/submit-relay` with `relayFeeLimitWei` for the same deliberate opt-in.

Existing `BuildRelaySubmission` callers cannot silently start spending an
operator's public funds under the new policy. Existing relay integrations need
an explicit fee sponsorship policy. Receiving a remote payment request alone
does not authorize operator spending.

Wallet deposit and withdrawal builders query the active policy before entering
the proving queue, so an unsupported operation does not waste a native proof.
RPC status includes `shieldedOnly` and `shieldedOnlyTime` for client handling.

## Automatic private note preparation

The daemon-managed payout prover scans the configured signing account's
confirmed private notes when handling an authenticated payout. If total private
funds suffice but the largest four notes cannot cover the payment, it builds a
real Shield3/Shield4 self-payment combining up to four inputs. Subsequent
payout retries rescan after confirmation and repeat only if more combining is
needed. No manual note records or owner secrets need to be entered.

Preparation spends public gas and requires a public reserve for both preparation
and payout. It never falls back to a public deposit, releases a private gas
reserve, or invents notes from a displayed public balance. It only uses the
configured signer and its own receiving code.

The signed preparation transaction is journaled before broadcast separately
from payout records and bound to the chain and signing account. Retries
rebroadcast those exact bytes while pending and
check the receipt against the canonical block before rescanning. A preparation
hash is returned as `preparationTxHash`, with `status: preparing-notes`; it is
never returned as the miner's payout `txHash`.

Authenticated health reports `maxImmediatePayoutWei` separately from
`maxPayoutWei`. The latter includes consolidatable private funds only when the
public gas reserve covers the necessary merge steps and payout. This lets the
pool request automatic preparation instead of unnecessarily requesting another
public deposit.

This implements automatic consolidation of existing private funds. Automatic
**confidential creation from public funds is not implemented**: the existing
public-balance funding protocol still reveals its conversion amount.

## Verification

Fast checks:

```sh
go test -p 1 ./params ./core/forkid ./core/txpool ./internal/shield3wallet ./internal/gui
go test -p 1 ./core -run 'TestShieldedOnly|TestPrivateExecution|TestProcessShielded|TestShieldedV[34]|TestAntarticalStamp' -count=1
```

With a compatible native library already built:

```sh
RAYON_NUM_THREADS=2 GOMAXPROCS=2 go test -p 1 -tags shield3 \
  ./internal/shield3wallet -run '^TestShieldedOnlyNativePayments$' \
  -count=1 -timeout=20m -v
```

The native test uses real stamp/payment proofs, pre-fork backing, two independent
in-memory state replicas, a 1,000 TKM Shield4 payment, a 500 TKM Shield3 return,
and a 100 TKM sponsored relay payment, followed by a three-input 1,600 TKM
private self-consolidation. It checks rejection at activation,
replay, restore/re-execution, proof public values, emitted application logs,
recipient decryption, and unchanged public pool backing. These are not live
Tor/P2P nodes or a full chain-reorganization rehearsal.

Test coverage is not a claim of independent cryptographic review or completion
of the confidential-backing redesign.

### Local results, October 3, 2026

- The focused package suites above passed, as did
  `go test -p 1 ./cmd/shielded-payout-prover -count=1`.
- `TestShieldedOnlyNativePayments` passed with the three-input consolidation
  included (430 seconds on this server).
- Native-tag builds of `cmd/gtkm` and `cmd/shielded-payout-prover` succeeded.
- No running mainnet daemon was replaced or restarted, and no release/tag was
  published for this change. Full repository release checks and live network
  rehearsal have not been run for this patch.
