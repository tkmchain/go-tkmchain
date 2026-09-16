# Shield3 mined-node test report

## Outcome and purpose

On September 16, 2026, an isolated node built from `artartical` mined real
RandomX blocks, funded new wallets entirely from rewards, and confirmed stamp,
deposit, direct-send and relay transactions. This exercises the node's HTTP RPC,
transaction pool, miner, block importer, native verifier and canonical wallet
scanner together. It uses `eth.New`, not the simulated backend or fake sealing.

The initial run tested commit `7284ba20a`. Reviewing that run exposed an
unnecessary rebroadcast on confirmed relay retries. The follow-up changes fix
that behavior and add the reusable runner at
[`scripts/shield3-testnode/main.go`](../scripts/shield3-testnode/main.go).
The fresh-chain rerun and its evidence are recorded below after completion.

## Private-chain configuration

| Setting | Value |
| --- | --- |
| Chain/network ID | 8980, used only by this isolated private chain |
| Starting account allocation | Empty; each test wallet starts with zero public TKM |
| Antartical and earlier supported forks | Active from genesis on the private chain |
| RandomX transaction and Monero proof modes | Active from block zero |
| Sealing | Real native RandomX, one mining thread, cache/light mode |
| Public RPC | Loopback HTTP, dynamically assigned port |
| Peers and discovery | Disabled |
| State | Dedicated directory, hash scheme, no pruning |
| Genesis block gas limit | 30 million |
| Genesis base fee/miner minimum gas price | 1 wei, intentionally small test fees |
| King reward addresses | Empty in this test configuration |

The network ID reuses Egypt's offline mining readiness path. This is **not**
the public Egypt genesis or network. The runner constructs and starts the node
services directly; it does not change or bypass the daemon's mainnet checkpoint
configuration. Mandatory mainnet checkpoints do not establish the identity of
this private chain. No production database, peer, key or mining process is used.

Mainnet activation remains **October 1, 2026, 00:00 UTC**. Setting local fork
activation to zero changes only the test genesis. Low fees and rapid low-height
mining make this a functional test, not a mainnet throughput or fee benchmark.

## Initial run

Three initial RandomX blocks gave the sender **300 TKM** in public rewards.
Additional blocks were mined to confirm each transaction. The old report's
`minedBlocks: 3` describes initial funding, not the final height; the chain ended
at block 11. A mining worker can complete another empty block while stop is
being processed, so inclusion heights can differ between runs.

| Transaction | Included block | Gas used | Receipt status |
| --- | ---: | ---: | ---: |
| Sender stamp registration | 4 | 1,914,808 | 1 |
| Sponsored recipient stamp | 5 | 2,032,072 | 1 |
| Sponsored relay-operator stamp | 6 | 2,026,140 | 1 |
| Shield 10 TKM into sender notes | 8 | 4,948,032 | 1 |
| Direct private send of 2 TKM | 9 | 4,956,452 | 1 |
| Relay batch totaling 6 TKM | 11 | 4,953,788 | 1 |

The relay's three paid outputs were 1 TKM to the recipient, 2 TKM to the
operator's private wallet, and 3 TKM back to the sender. The fourth output was
change. Confirmed private balances were:

| Wallet | Balance in TKM |
| --- | ---: |
| Sender | 4.999999999986 |
| Recipient | 3 |
| Operator | 2 |

The sender's difference from 5 TKM is the quoted gas reserve of 14,000,000 wei
(`7,000,000 gas * 2 wei`). Relay mode reserves the full quote, not just actual
gas consumed. Public balance rewards and gas refunds are separate from these
private balances. These intentionally tiny test fees are not production estimates.

[Initial JSON evidence](testdata/shield3-live-20260916-initial.json) contains
all six complete transaction hashes and checked balances. These are private-chain
hashes and have no public explorer receipts.

## Findings and corrections

### Confirmed relay retries incorrectly reported uncertainty

The original relay saved the exact signed transaction and returned its correct
hash on retry. However, it always called `eth_sendRawTransaction` again. After
confirmation the node returned `nonce too low`, causing the relay to mark
`submissionUncertain: true` even though the transaction was already canonical.
No second payment was created, but the result was misleading.

The relay now calls `eth_getRawTransactionByHash` first. This node RPC serves
canonical or pooled transactions. If its returned bytes exactly equal the
saved, authenticated transaction, the relay returns that transaction and hash
without another broadcast or uncertainty. If a reorganisation/eviction removes
the transaction, lookup fails, or returned bytes differ, it retransmits only the
original durable signed bytes. It never proves or signs another payment for the
same request. The wallet still confirms against its own node; a remote relay's
status is not trusted as proof of inclusion.

`TestKnownTransactionRetrySkipsBroadcast` covers a known payment, loss of that
payment, a lookup outage, and nonmatching returned bytes. Existing lost-reply,
restart, request/nonce reservation and real native batch-vector tests also pass.

### Test coverage needed strengthening

The first live relay send used one input, and checked exact retries without
restarting the service. The new runner splits the sender's remaining 8 TKM into
four 2 TKM notes before relaying 6 TKM plus a positive gas reserve. Three notes
cannot cover that total, so the spend must consume **all four inputs**. It checks
all four nullifiers against the actual signed transaction hash.

After confirmation the runner stops the relay, opens the same durable state in
a new relay instance, and retries through its HTTP endpoint. It requires identical
bytes/hash, no uncertainty, and exactly one relay broadcast across the restart.
It also checks each selected-payment disclosure, a separate stamp-only disclosure,
receive-only permissions, unchanged payer public nonce, omitted payer public key,
and a stamped operator starting without public funds.

The updated report distinguishes `initialFundingBlocks` from `finalHeight`.
The runner requires a fresh output directory and does not write spending seeds
or viewing keys to disk. Reusing an existing directory fails before modifying it.

### Expected log messages and validation limits

After canonical inclusion the pool can briefly retain a transaction while its
head reset is running. Its stale-state pruning then reports `stamp already
registered`, `commitment already exists` or `nullifier already spent`. In the
initial run these messages refer to transactions with successful canonical
receipts; they are pool cleanup, not failed payments. Pruning remains enabled.

An attempted public 1 TKM transfer was rejected with `transparent transactions
are disabled after privacy activation`. That checks the transparent-transfer
rule; it does **not** independently demonstrate the unstamped-recipient rule.
The follow-up runner separately checks the wallet rejects a Shield3 deposit to
an unregistered stamp. Consensus-level stamp enforcement and illegal proof
mutations are covered by the focused consensus/native tests described in the
[implementation guide](SHIELD3_ANTARTICAL.md#verification-map).

The initial setup used an oracle ignore price of zero, which the node sanitized
to 2 wei and logged. The runner now supplies 2 wei explicitly.

## Fresh-chain rerun

The corrected runner completed on a new genesis from **23:13:55 to 23:18:56 UTC**.
It mined three initial funding blocks and ended at block 14. The dynamic port was
`35971`; it was loopback-only and closed during shutdown. The run passed every
assertion and reported exactly one relay broadcast, including the post-restart
retry.

The corrected run confirmed sender and sponsored stamps, the 10 TKM deposit, a
2 TKM direct send, a split that produced four 2 TKM inputs, and the four-input/
three-recipient 6 TKM relay batch. It verified all selected payment disclosures,
the separate stamp disclosure, receive-only scan isolation, unchanged payer
public nonce, omitted payer public key, zero-public-balance operator funding,
and rejection of a zero mix digest mutation. Its final balances and transaction
hashes are in [the rerun JSON evidence](testdata/shield3-live-20260916-rerun.json).

The corrected transaction hashes are intentionally different from the initial
run because every key, genesis and state was fresh. Both runs used the same
production consensus code and native verifier.

## Reproduce

Run from the repository with the matching native libraries and normal Linux
build tools installed:

```sh
make gtkm
GOFLAGS=-p=2 GOMEMLIMIT=768MiB go build -tags randomx,cgo,shield3 \
  -o ./build/bin/shield3-testnode ./scripts/shield3-testnode
RAYON_NUM_THREADS=2 GOMEMLIMIT=768MiB ./build/bin/shield3-testnode
```

The default creates a new private `/tmp/tkm-shield3-live-*` directory. To choose
an output path, pass `--output /path/to/a/new-directory`; its parent must exist
and the chosen directory must not exist. No command connects to production.

The runner exits successfully only after receipts and assertions pass and the
node shuts down cleanly. Evidence includes `genesis.json`, `node.log`, signed
public transaction `.bin` files and `report.json`. A failure produces a nonzero
exit and records the error in the report when setup reached the output directory.
The RPC port is temporary and is no longer serving after the runner exits.

## What this establishes

This establishes real mining, transaction admission, canonical block inclusion,
private balance conservation, four-input batch handling, scoped disclosures and
durable relay retry behavior for the tested local configuration. It does not
prove permanent cryptographic security, network anonymity, public-network sync,
reorganisation behavior under live peers, physical Android performance, wallet
GUI interaction or relay TLS/proxy deployment. Those need separate checks.
The previous browser/Windows/Android checks remain separate evidence.

Existing broad repository test failures and 67 baseline lint issues remain
reported separately; they do not turn this live test into a clean full-suite run.
