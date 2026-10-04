# Confidential note ledger and explicit public boundaries

## Activation and implementation status

The requested target is **October 4, 2026 at 10:00 UTC**, Unix
`1791108000`. The consensus setting is `confidentialLedgerTime`.
The draft is implemented in source; default mainnet, RandomX and Egypt
configurations still leave this setting unset. Updating the proposed constant
does not update running nodes or schedule a live network fork.

This replaces the earlier shielded-only cutoff proposal for this scope.
`confidentialLedgerTime` and `shieldedOnlyTime` cannot both be configured:
the ledger permits explicit public deposits and withdrawals.
Historical blocks retain their rules. Activating or removing a fork behind
the chain head is detected by configuration compatibility checks.

## Privacy model

| Operation | What remains public |
| --- | --- |
| Deposit / backed note issuance | Funding account and deposited amount |
| Shield3/Shield4 payment | Transaction hash, commitments, nullifiers or link tags, outer signer/relay, gas parameters and fee |
| Private balance | Individual note values are encrypted; aggregate pool backing is public |
| Withdrawal / privacy exit | Withdrawal account and amount |
| Mining subsidy | Existing public rewards and halving rules |

“Issuance” here means issuing private notes against a verified deposit.
It does **not** mean confidential mining rewards or minting additional TKM.
A deposit remains observable through both transaction value and account
balance changes. Public exits are deliberate boundaries, not hidden transfers.
Direct sends expose the outer signer; an explicitly fee-sponsored relay can
separate it from the note owner. This implementation does not hide transaction
existence or promise anonymity against every observer.

## Consensus accounting and proofs

The existing native STARK proves membership, note ownership, ranges and
conservation. The relation binds deposit backing, withdrawal value, and the
authorized fee through the complete transaction intent. Optional envelope
`FeeMode` is also intent-bound. Mode zero preserves historical RLP when omitted;
mode one is rejected before the ledger fork.

Versioned, asset-scoped storage tracks:

- opening backing;
- cumulative public deposits and withdrawals;
- cumulative prepaid fees burned; and
- remaining backing.

The invariant is:

`backing = opening + deposits - withdrawals - prepaid fees`.

Counters commit only after proof verification. Nullifiers prevent replay.
Storage and fee debits participate in StateDB rollback and block re-execution.
Validator bonds, including unwithdrawn exit bonds, and pTKM backing are excluded
from native note reserves. Existing V2 migration withdrawals remain explicit
public exits and debit the ledger. They cannot issue new legacy notes.

Opening backing is an upper bound on historical note liabilities, because
unsolicited legacy transfers can leave excess funds in the pool. It is not
presented as an exact count of all historical outstanding note values.

## Fees without public refunds

After activation, a Shield3/4 spend has two fee choices:

1. Public gas: sender or explicitly consenting relay pays ordinary gas.
   `GasSponsorValue` must be zero.
2. Prepaid note fee: native TKM notes prove a debit equal to
   **7,000,000 gas × gas fee cap**. Fee cap and tip cap must be equal.
   The entire authorized fee is burned; there is no unused-gas refund,
   public sender credit, or miner priority-fee credit for this mode.

The second choice is a new, opt-in consensus fee policy. Wallets must authorize
a maximum in wei before proving. The actual fixed charge can be less than that
maximum, but is never partially refunded. It supports direct native TKM spends
and explicit native withdrawals. pTKM and relayed prepaid fees are not supported.

A transaction-specific fee ticket is created only by a successful native proof
debit and consumed once during gas purchase. Supplying a fee mode in an RPC
message does not create gas credit.

Wallet APIs:

- `BuildPrepaidPayment(..., version, maxFeeWei)` for version 3 or 4.
- `BuildPublicWithdrawal(..., version, maxFeeWei)` for an explicit exit.
  A nil fee limit selects public gas.
- Authenticated wallet `send` requests may supply `prepaidFeeLimitWei`;
  that selects Shield4 and is included in the retry request digest.
- `tkmprivacy_shieldedV3Status` and `tkmprivacy_shieldedV4Status` expose
  activation and fixed fee gas. `tkmprivacy_confidentialLedger` takes an asset
  ID and returns public counters, never individual decrypted notes.

## Automatic daemon note preparation

The payout prover scans canonical notes and consolidates small notes on demand.
Preparation uses a separate journal from payouts: its hash is never reported
as payment to the recipient. Signed bytes are persisted before broadcast and
reused after a restart. A canonical successful receipt permits the next step.

An operator can additionally authorize public funding from its configured
signing account. For example, these fields in the existing authenticated prover
configuration permit at most 1,000 TKM per funding transaction:

```json
{
  "autoPublicFunding": true,
  "autoPublicFundingLimitWei": "1000000000000000000000"
}
```

This funds only the missing principal, sends the note to the same account, and
retains enough public gas for preparation and payout. It operates on payout
requests, not as an unattended background sweep. It requires the account's
original registered stamp and signing key. The default is disabled.
The limit is per funding transaction, not a lifetime spending allowance.

After the ledger fork, an operator may also explicitly configure
`prepaidFeeLimitWei` as its per-payment fee budget. Otherwise payouts use public
gas. Funding deposits and consolidation still require a public gas reserve.
No additional legacy V2 proof builder is used.

Pending preparation returns HTTP 202 with `status: "preparing-notes"`,
`preparationTxHash`, and `preparationPrivacyBoundary`. Public funding is
labeled `public-deposit`. Consumers must wait for completion rather than marking
the preparation hash as a successful recipient payout.

Authenticated health separates current `hasSpendableNotes` and
`maxImmediatePayoutWei` from eventual `maxPayoutWei`, `payoutReady`, and
`publicFundingAvailable`. A client must not interpret eventual funding
capacity as an already confirmed note balance.

## Verification

Local verification on October 3, 2026: the native rehearsal passed in
442 seconds. Targeted consensus, wallet, payout-prover, GUI, RPC, fork-ID and
transaction-admission checks passed, and both `gtkm` and
`shielded-payout-prover` built with `-tags shield3`.
The first rehearsal attempt stopped because the test RPC omitted the existing
Shield3 activation timestamp; the fixture was corrected and the complete
native rehearsal rerun successfully.

Targeted tests cover activation boundaries, malformed fee authorization,
fee-ticket forgery and replay, reserve overdraw, counter overflow, asset
isolation, and state rollback. The native rehearsal command is:

```sh
RAYON_NUM_THREADS=2 GOMAXPROCS=2 go test -p 1 -tags shield3 \
  ./internal/shield3wallet -run '^TestConfidentialLedgerNativePayments$' \
  -count=1 -timeout=20m -v
```

It exercises real native stamp/deposit/spend/withdrawal proofs and two
independent in-memory state replicas. It is not a live Tor multi-node deployment
or a release-platform compatibility test. Proving takes measurable CPU time;
automatic note management does not make proof construction instantaneous.
