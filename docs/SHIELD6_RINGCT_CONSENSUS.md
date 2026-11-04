# Shield6 classical ring-confidential transaction fork

## Scope and activation

Shield6 is a proposed consensus upgrade for private spends. It replaces the
Shield3/4/5 STARK spend proof with fixed-size linkable rings and confidential
amount commitments. The choice of classical ring signatures explicitly gives
up post-quantum security for Shield6 spend authorization. It does not change
the security of ML-DSA account transactions or ML-KEM-encrypted wallet
records.

This document is a protocol specification target, not an activation notice.
No Shield6 activation timestamp is configured, and nodes must not accept a
Shield6 transaction until the complete implementation and chain migration are
ready and a fork time is set consistently.

## Transaction relation

Each input names exactly 16 distinct eligible output public keys and their
amount commitments. Wallets shuffle the final ring order locally after
selecting candidates; consensus requires unique accumulator indices but does
not sort them. This keeps the real member's array position independent of its
age rank among decoys. One member is controlled by the spender; the ring
signature proves that fact without revealing which member. Each input carries
a linkable key image. Consensus rejects a repeated key image, including
duplicates within a transaction and across assets. The signature challenge
binds the chain ID, Shield6 intent domain, asset ID, ring indices, range-proof
bytes, output keys, output commitments, encrypted-note bytes, and outer
transaction fee fields. The CLSAG challenge separately binds each key image
and pseudo-output; those fields are omitted from the signing intent to avoid a
circular dependency because CLSAG signing derives them together. CLSAG
signature bytes are excluded from the intent, while the completed range proof
is included because it exists before signing. This initial envelope has no
expiry field and supports only native TKM.

Amounts use Pedersen commitments. Each 128-bit atomic-unit amount is split into
two 64-bit limbs; a canonical RLP proof bundle contains one aggregate
Bulletproofs+ proof for all low limbs and a second for all high limbs. Each
aggregate is bounded to 16 commitments, matching the pinned backend's
1,024-generator table. The committed output value is
`C_low + 2^64*C_high`, including the correspondingly weighted mask. This avoids
the legacy `uint64` atomic-unit ceiling. A private transfer proves that the
sum of pseudo-output commitments equals the sum of output commitments. A
public deposit proves output commitments equal the outer transaction's
visible deposit amount; the aggregate output mask is disclosed to verify that
boundary. A public withdrawal proves input commitments equal confidential
change plus the visible withdrawal amount. The destination and amount of both
boundaries are public by design. The outer account pays gas separately; there
is no private fee commitment. An ordinary private transfer cannot create or
destroy value.

Consensus derives the Bulletproofs+ verifier's internal batch-check coefficient
stream deterministically from a domain-separated hash of the ordered public
commitments and canonical proof bytes. The upstream verifier accepts a random
reader for this coefficient; a node-local cryptographic RNG is not suitable
for consensus because it could make nodes disagree on the same proof. Wallet
proving still uses fresh cryptographic randomness.

The output record contains a one-time public key, amount commitment, and an
authenticated encrypted note. The wallet derives the one-time key from an
ephemeral Edwards25519 ECDH secret and the transaction-local output position;
XChaCha20-Poly1305 protects the amount, asset ID, and commitment openings.
Consensus assigns the separate global accumulator index only after inclusion.
The wallet scanner checks the one-time key before decryption, authenticates
the envelope fields, checks the amount and both limb openings, and recovers the
one-time spend scalar.

## Decoys and ring construction

Ring size is fixed at 16 for this version. Wallets should sample decoys from
the consensus-eligible output set using a versioned selection algorithm and
fresh wallet randomness. The transaction includes selected ring indices;
consensus checks that every member exists in the canonical pre-transaction
accumulator state and is unique. The real member's position is hidden by the
linkable ring signature. The wallet library implements unbiased uniform
sampling without replacement from a caller-provided, sorted candidate set,
then Fisher-Yates shuffles the 16 selected members before signing. The
transaction preserves this shuffled order through RPC and consensus
verification. The selector does not implement an age-weighted distribution.
Wallet integrations must fetch candidates and choose locally, without
disclosing the real output index to a remote server.

Decoy selection must not create fake ledger outputs or mark decoys spent.
Only the key image is added to spent state. Ring members are not removed from
the anonymity set when another transaction references them as decoys.

## Output accumulator and migration

The current Tip5 note commitments do not contain the public curve keys needed
for a classical ring. They cannot be reinterpreted as ring members. Shield6
therefore uses a separate append-only output accumulator whose leaves contain
the new one-time key and amount commitment. Shield3/4/5 history and roots
remain unchanged for replay compatibility.

Existing Shield3/4/5 notes use an explicit two-transaction migration after
Shield6 activation: withdraw a legacy note to the same stamped account, then
make a public Shield6 deposit to a wallet-generated Shield6 address. The
withdrawal amount and destination and the following deposit amount are public;
this is not a private cross-system proof. Consensus permits the legacy
self-exit only after Shield6 activation, only to the signed sender, and only
for native TKM with no note-funded fee. The ordinary legacy proof and stamp
checks still apply. The CLI waits for 12 canonical confirmations of the legacy
withdrawal, rechecking both its receipt and block hash each poll, before it
offers the separate deposit confirmation. It applies the same confirmation
check to the Shield6 deposit before reporting migration complete. If either
receipt disappears or its block is reorganized, it reports both relevant
transaction hashes and does not claim the migration is complete. This
confirmation depth reduces reorg risk but is not a finality guarantee.
Never reinterpret Tip5 commitments as Edwards25519 ring
outputs or claim that they link cryptographically.

Legacy spends remain valid during a versioned migration interval; a separately
scheduled fork may sunset new legacy spends only after a published migration
plan. Migration proof generation may be slower than normal Shield6 sends.
Wallets should batch migration locally, and consensus must bound batch size
and verifier cost. A hardfork must not strand unmigrated balances or silently
convert opaque legacy commitments into spendable keys.

## State transition and resource limits

Consensus state stores the Shield6 output accumulator, key images, asset
commitments, and any public supply counters needed by each asset policy. State
changes occur only after all signatures, range proofs, commitment-balance
equations, asset rules, and fee checks pass. Invalid transactions must leave
all Shield6 state unchanged. Reorgs revert output leaves and key images using
the normal state journal. Intrinsic gas includes a conservative 20,000-gas
charge for each possible zero-to-nonzero state slot write: up to 32 accumulator
frontier slots plus four per-output indexes, two transaction-level accumulator
slots, two per-input key-image indexes plus the shared count, and the three
confidential-backing counters for public deposits or withdrawals. Frontier
cost uses the tree-depth worst case instead of relying on the average number of
writes. This prices state growth even though the consensus processor performs
the writes outside EVM bytecode.

The fork defines maximum ring count, output count, proof bytes, and verifier
gas before activation. Verification limits are derived from measured worst
case execution on supported release platforms. The two-minute target is a
wallet-construction benchmark on a named reference machine, not a consensus
guarantee for every device. Release tests must include proof generation,
verification, tampering, replay, reorg, multi-input, migration, and maximum
size cases.

Latest one-iteration benchmarks after moving secret operations to the
constant-time backend (Go 1.26.6, Linux amd64, Intel Core Processor Haswell
without TSX) measured 2.324 seconds for proving and verifying a 16-output
paired 128-bit range proof, and 0.731 seconds for signing and verifying all 16
inputs with 16-member rings. Isolated verification measured 0.173 seconds for
the paired range proof and 0.189 seconds for all 16 CLSAG inputs. These
primitive figures are from single local runs and should be treated as
reference measurements, not platform guarantees.

Three consecutive maximum configured private-envelope runs on the same host
measured 3.10, 4.63, and 4.62 seconds, with 35.4–35.6 MB allocated, for 16
inputs with 16-member rings and 16 encrypted outputs. The spread shows why
supported-device measurements are still needed. It includes range-proof
generation and verification, output encryption, CLSAG signing and
self-verification, commitment conservation, and envelope encoding. It
excludes note scanning, the outer ML-DSA account signature, RPC, transaction
propagation, and block inclusion. These local measurements are below the
two-minute construction target, but do not establish a universal send-time
guarantee. Release CI should repeat the benchmark on each supported
architecture; slower-device and end-to-end measurements remain required.

## Cryptographic implementation requirement

Use a maintained, independently reviewed implementation of CLSAG and
Bulletproofs+ with fixed parameters and canonical encodings. Do not implement a
new ring signature or range proof from scratch, accept caller-supplied
parameters, or use decoy metadata as a substitute for proof verification.
Monero's reference code documents CLSAG over ring public keys and commitment
points and its Bulletproofs+ range-proof interface; the chain integration must
still define its own TKM domains, transaction encoding, asset rules, and test
vectors ([CLSAG/RingCT implementation](https://github.com/monero-project/monero/blob/master/src/ringct/rctSigs.cpp),
[Bulletproofs+ interface](https://github.com/monero-project/monero/blob/master/src/ringct/bulletproofs_plus.h)).

## Implementation status (2026-11-03)

The repository now contains an initial, inactive Shield6 implementation:

* `zk/shielded6` uses the pinned `git.gammaspectra.live/P2Pool/consensus/v5`
  `v5.0.10` CLSAG verifier and Bulletproofs+ implementation (MIT license),
  strict point/scalar parsing, an amount-width adapter for signing aggregate
  128-bit commitments, paired range proofs, and XChaCha20-Poly1305 wallet
  output creation/scanning. Secret-scalar multiplication in key derivation,
  note creation/scanning, commitment construction, range proving, key-image
  derivation, and CLSAG signing uses that backend's `ConstantTimeOperations`
  implementation; public consensus verification uses the variable-time
  implementation only on public transcript data. Signing clears decoded spend
  keys, masks, nonces, and intermediate scalars on return. This is a source-level
  mitigation, not a claim of complete side-channel resistance or independent
  cryptographic audit. Tests cover 16-output proofs, CLSAG interoperability
  above 64 bits, wallet output round trips, and tamper rejection.
* `core/shielded_v6.go` defines private-transfer and explicit public-boundary
  envelopes, transaction intent, range-proof verification, and commitment
  conservation checks.
* `core/shielded_v6_state.go` stores an append-only output accumulator and
  key-image replay state in the normal journaled state database; ring members
  are resolved against pre-transaction canonical state.
* `zk/shielded6/wallet.go` derives chain-separated spend/view keys and encodes
  a checksummed chain-bound payment code. `wallet_output.go` creates and scans
  encrypted native-TKM outputs, recovers one-time spend keys, and derives the
  CLSAG key image used for spent-note checks. Core builders
  support private sends, public deposits, change-bearing withdrawals, and full
  exits. `internal/shield3wallet.ScanShieldedV6` scans bounded RPC pages locally,
  excludes confirmed and pending key-image spends, and rejects a scan if the
  head or accumulator changes during retrieval. `SelectShieldedV6Ring` samples
  canonical members using wallet randomness, randomizes lookup order, fetches
  all 16 members in one order-preserving bounded RPC call, and checks the
  accumulator root before returning. `BuildAndSignShieldedV6Payment`
  composes the stamped-sender check, active-chain check, spend-status checks,
  canonical ring selection, envelope construction, intrinsic-gas check, and
  ML-DSA-87 account signature. Neither ring selection nor payment construction
  sends a signer position to RPC. The scanner returns decrypted notes to the
  caller, which must clear them after use. Tests verify the deposit mask
  equation, tampering rejection, full-exit conservation, stateful ring
  validation, scanning, pending-spend exclusion, and reorg detection. The
  interactive CLI can display a chain-bound Shield6 payment code, scan
  spendable notes, select enough inputs up to the consensus maximum, create
  private change, build and sign with `BuildAndSignShieldedV6Payment`, and
  broadcast through the node's normal raw-transaction RPC. It also provides a
  staged legacy-note migration: build a legacy self-withdrawal to the same
  stamped account, wait for 12 canonical confirmations, then request a separate
  confirmation before submitting the public Shield6 deposit. The CLI waits for
  12 canonical confirmations of that deposit before reporting completion. If
  the deposit is canceled or fails, the withdrawn value remains in the
  account's public balance and the CLI reports the withdrawal hash. The flow requires
  an active Shield6 fork and a stamped sender account. GUI and Android send
  flows are not wired yet. Recipient stamp
  authorization also has no Shield6 consensus representation: the current
  private output contains only the Shield6 one-time key, commitment, and
  encrypted note. Do not infer recipient stamp enforcement from the sender's
  stamp check. These are release-blocking gaps.
* The `tkmprivacy_shieldedV6Status`, `tkmprivacy_shieldedV6Outputs`,
  `tkmprivacy_shieldedV6OutputsByIndex`, and
  `tkmprivacy_shieldedV6WalletOutputs` RPC methods expose activation and bounded
  canonical accumulator pages. Per-key-image lookup RPCs were removed because
  they accepted a wallet-derived key image and revealed that query to the RPC
  operator. Wallets instead download bounded pages from
  `tkmprivacy_shieldedV6SpentKeyImages` and the public pending set from
  `tkmprivacy_shieldedV6PendingKeyImages`, then match locally. The wallet-output
  method resolves encrypted records through canonical transaction lookups; it
  never decrypts or returns note amounts. Ring candidate selection is performed
  locally by the wallet library, and indexed candidate batches preserve request
  order while rejecting duplicates and oversized batches.
* The current wallet samples decoys uniformly from the accumulator and
  shuffles selected ring order independently of accumulator age. Output
  creation heights are not committed as a separate accumulator field, and the
  protocol has no age-weighted decoy distribution or minimum ring age. This
  keeps every recorded output eligible, but it does not model real-wallet
  spend-age patterns; transaction timing can therefore weaken anonymity in
  statistical analyses. A production privacy claim requires a specified,
  tested selection distribution and a coordinated consensus rule if output
  eligibility is changed. Do not add an arbitrary maturity delay as a local
  wallet-only fix.
* The legacy transaction pool reserves Shield6 key images across both queued
  and executable transactions. A second transaction attempting the same
  pending spend is rejected before pool eviction; same-sender/same-nonce fee
  replacements may retain their own images. Eviction, removal, reorg reinjection,
  and nonce demotion release reservations through the transaction lookup
  lifecycle. This is a local anti-churn policy; canonical state validation
  remains the consensus double-spend guard.
* The fork timestamp is optional and remains unset on mainnet. No node should
  accept Shield6 transactions until an activation is explicitly configured.

This is not yet a complete wallet release. The CLI migration wizard has not
been exercised against a live multi-node fork rehearsal. GUI and Android have
no Shield6 send or migration flow. The RPC scanner downloads the public spent
key-image index in bounded pages and the public pending set, then checks owned
notes locally. It does not send derived key images to the node.
There is no private bridge proof converting legacy Tip5 Shield3/4/5 notes into
curve-based RingCT outputs. Migration is therefore explicit and public:
legacy self-withdrawal followed by Shield6 deposit. Token asset domains and
private fee handling need separate consensus rules before activation.

The automated checks cover primitive signing/proof generation and verification,
128-bit amount commitments, wallet output encryption/scanning and note signing,
wallet-built envelope validation, tamper rejection, wire encoding, accumulator
append/revert, fork gating, stateful ring membership, key-image replay rejection,
and core/txpool compilation. The native confidential-ledger wallet test contains
a migration scenario across two state replicas: it creates a legacy Shield4
note, withdraws to the same stamped account, deposits into a Shield6 wallet,
scans the resulting note, and checks ledger counters and replica agreement.
However, its latest local run was interrupted during an earlier high-memory
legacy Shield3 proof, before reaching that migration scenario, so the scenario
is implemented but is not counted as a passing end-to-end test. The isolated
Shield6 deposit builder and migration-policy tests pass. A separate deterministic
two-state rehearsal applies public deposits and a two-input private spend,
compares accumulator and backing-ledger results, rejects a replay, then restores
the pre-spend state and reimports the spend. These exercise state-transition and
rollback logic in-process; they are not live P2P tests or canonical-chain
reorganization rehearsals.

The Linux release workflow now runs the native migration rehearsal as a
separate, visible 30-minute gate after the bounded wallet tests. This avoids
silently counting an interrupted run as coverage and makes the real legacy
proof-to-Shield6 path part of release validation. The updated workflow has not
yet run on GitHub, so it provides a gate but not a passing result.
The Shield3/Shield6 cryptography workflow also runs the same rehearsal on its
clean Ubuntu runner with a 45-minute test timeout; it has not yet produced a
hosted result either.

On 2026-11-03, a stripped production Linux/amd64 daemon build succeeded, and a
full Windows/amd64 daemon cross-build also succeeded from Linux using MinGW,
including the Windows RandomX archive and target-specific Shield3/4 native
archive. The resulting 70 MB PE binary imports only Windows system DLLs; the
MinGW C++ and pthread runtimes were statically linked. The Windows artifact was
validated structurally with `file` and `objdump`, not executed on Windows. The
Shield6 crypto test package separately cross-compiles for Linux ARM64, Linux
ARMv7, Windows/amd64, and macOS/ARM64. Full daemon artifacts for macOS and ARM
targets, plus runtime testing on those platforms, still require their release
builders or matching native toolchains.

These tests do not establish production readiness, complete cross-platform
release artifacts, full CLI operation against a live node, migration of legacy
notes across multiple live nodes, or the two-minute end-to-end send target.
Shield5 remains unchanged. Keep Shield6 inactive until those gaps are closed
and the consensus change receives independent review.

### Verification and distribution update on 2026-11-04

The focused `zk/shielded6` cryptography tests passed, and the core two-state
deposit/spend/replay/reorg rehearsal passed. The isolated wallet private-send
and public-deposit builders passed, as did the shipped-network activation
guard, txpool pending-key-image reservation tests, and Shield6 RPC tests. The
broader wallet test selection
`go test -p=1 -tags shield3 ./internal/shield3wallet -run 'ShieldedV6|Shield6'`
did not complete on this shared 7.6 GiB host: legacy proof generation consumed
most available memory while the live daemon and miner were running. The test
processes were stopped before an assertion result; this is an inconclusive
resource-limited run, not a passing migration rehearsal. The production daemon
and miner were left running. Run the full migration test on a dedicated CI
runner with sufficient memory before treating that gate as passed.

The release now carries `THIRD_PARTY_NOTICES`, including the full MIT notice
for `git.gammaspectra.live/P2Pool/consensus/v5 v5.0.10`, in Linux, macOS,
Windows, and ARM release archives, the Windows wallet archive, and Debian
packages. The release workflow parsed as YAML, the Debian packaging script
passed `bash -n`, and `git diff --check` passed after the packaging changes.
This validates packaging metadata only; the hosted release workflow has not
been run here.

### Verification rerun on 2026-11-03

The focused suites were rerun from the current worktree with Go 1.26.6 and one
Go worker per command. All passed:

* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./zk/shielded6 -count=1`
* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./core -run '^TestShield6TwoNodeDepositSpendReplayAndReorgRehearsal$' -count=1`
* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./internal/shield3wallet -run 'ShieldedV6' -count=1`
* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./core/txpool/legacypool -run 'ShieldedV6' -count=1`
* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./eth -run 'ShieldedV6' -count=1`

The maximum-capacity envelope gate passed again on 2026-11-04:
`GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3
./core -run '^TestShield6MaxPaymentEnvelopeUnderTwoMinutes$' -count=1 -v`
reported 5.225 seconds per build for the configured 16-input/16-output case.
This remains a local envelope-construction measurement, not end-to-end wallet
send latency. The multi-input gas schedule also now charges each repeated
warm key-image-count update at 2,900 gas after the slot's first 20,000-gas
write; the two-input delta has a regression assertion and the full Shield6
core-focused suite passes with the corrected formula.

The current source also rebuilt `cmd/gtkm` successfully on Linux/amd64 on
2026-11-04 using `GOMAXPROCS=1 GOMEMLIMIT=1300MiB GOGC=30 go build -tags
shield3 -trimpath -ldflags='-s -w'`. The resulting binary reports version
`1.21.82-shielded-gas`, Go 1.26.6, and has SHA-256
`dd78eff4627b9fe76de94ca5ba7fc8e90580ceaaa584d2cb88a2b1d58372ed9d`.
After the multi-input gas correction, the daemon was rebuilt again under
`/tmp/gtkm-shield6-gate-final`; it reports the same version and has SHA-256
`0a4623c2df9a6866265f56c895214971c367553d044842d8410f9a8ec6000420`. Neither
artifact was used to start or replace a node. These are Linux daemon compile
checks, not packaged releases or cross-platform release builds.

After that rerun, CLSAG intent construction was tightened to include the
finalized range-proof bytes. A regression test verifies that changing the
proof changes the signing intent while changing only the CLSAG signature does
not. The intent-binding regression and two-state deposit/spend/replay/reorg
rehearsal both pass after this change.

The following isolated gates also passed after the intent change:

* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./core -run '^TestShield6LegacyMigrationAllowsOnlySelfExitAfterActivation$' -count=1 -v`
* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./internal/shield3wallet -run '^TestBuildAndSignShieldedV6Deposit$' -count=1 -v`
* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./internal/shield3wallet -run '^TestBuildAndSignShieldedV6Payment$' -count=1 -v`
* `GOMAXPROCS=1 go test -p=1 -tags shield3 ./cmd/gtkm -run '^TestWaitWalletCanonicalConfirmations$' -count=1 -v`

The last command passed. It exercises confirmation-depth handling, cancellation,
receipt disappearance, and canonical-block hash changes in the CLI's migration
waiter. The Linux release test selection includes the timed maximum-payment
gate; the workflow also compiles the pure-Go Shield6 crypto test package for
each Linux ARM target and limits Cargo to one job for the memory-intensive
native proof-library build. The workflow file passes local YAML syntax parsing;
the GitHub-hosted matrix itself has not yet run.

### Verification rerun on 2026-11-04 (latest)

The focused suites were rerun from the current worktree with Go's test
parallelism limited to one and a 700 MiB Go memory target:

* `go test -p=1 -tags shield3 ./zk/shielded6 -count=1` passed (6.1 s).
* Core deposit validation, the two-replica deposit/spend/replay/reorg
  rehearsal, and the maximum-capacity payment timing test passed together
  (15.0 s).
* `go test -p=1 -tags shield3 ./eth -run 'ShieldedV6' -count=1 -timeout=5m`
  passed (0.4 s).
* `go test -p=1 -tags shield3 ./internal/shield3wallet -run
  'ShieldedV6|Shield6' -count=1 -timeout=5m` passed (198.9 s), including the
  native Shield6 migration rehearsal. The test process peaked at about 3.5 GiB
  resident memory despite the Go memory target; the shared host had less than
  1 GiB free disk during the run. This is a successful local test result, but
  it confirms that the migration suite belongs on an isolated, adequately
  provisioned CI runner rather than alongside a production node.
* `git diff --check` passed after these runs.

The follow-up static and activation checks also passed: `go vet -p=1 -tags
shield3 ./zk/shielded6 ./core ./eth ./internal/shield3wallet`, and the shipped
network test `go test -p=1 ./params -run
'^TestShield6ActivationRemainsUnsetOnShippedNetworks$' -count=1`.

These results supersede earlier notes in this document that called the local
Shield6 migration rehearsal interrupted or inconclusive. They do not establish
the full release matrix, a live P2P fork/reorg rehearsal, or platform-wide
latency and resource bounds.

The maximum configured payment builder is now enforced by
`TestShield6MaxPaymentEnvelopeUnderTwoMinutes`, which runs the benchmark once
and fails if a maximum-capacity envelope build exceeds two minutes. The
Linux/amd64 rerun on 2026-11-03 passed at 3.53 seconds per build with
`GOMAXPROCS=1`, `GOGC=20`, and a 700 MiB Go memory limit. A direct one-iteration
benchmark immediately before adding the timed test measured 3.07 seconds and
35,255,992 bytes allocated. The test and benchmark cover range
proof generation, output encryption, CLSAG signing and self-verification, and
commitment checks, but excludes the outer account signature and RPC latency.

A fresh one-iteration maximum-envelope benchmark on 2026-11-03 measured 3.97
seconds and 35,445,464 bytes allocated on the same Haswell host with
`GOMAXPROCS=1`, `GOGC=20`, and `GOMEMLIMIT=700MiB`. This is an additional
primitive-builder measurement; it still excludes wallet scanning, account
signature latency, RPC, and network propagation.

The updated production daemon was then rebuilt from this worktree for Linux
amd64 into `/tmp/gtkm-shield6-linux-amd64-recheck`; the stripped ELF reports
version `1.21.82-shielded-gas-7f3624b9-20261031`. This verifies compilation
and CLI version startup only; the artifact was not used to start or replace a
live node.

On 2026-11-03, a second unstripped Linux/amd64 build from the current worktree
completed with `GOMAXPROCS=1 GOMEMLIMIT=1400MiB go build -tags shield3
-trimpath -o /tmp/gtkm-shield6-finalcheck ./cmd/gtkm`. The resulting ELF ran its
`version` command and reported `1.21.82-shielded-gas-7f3624b9-20261031`. This
confirms the current CLI and consensus integration compile; it is not a
packaged release or a multi-platform artifact test.

A subsequent rerun of the full native confidential-ledger wallet test did not
finish. It passed the initial replicated deposit and one legacy Shield4 spend,
then reached a high-memory legacy Shield3 proof step. The test process reached
about 3.3 GiB RSS while swap was exhausted, so it was interrupted before the
Shield6 migration section. The live daemon's PID changed during this pressure
event; its local RPC subsequently responded at block 62062, reported
`eth_syncing=false`, and had one peer. The miner and pool processes remained
running. This rerun is inconclusive for end-to-end migration and is not
recorded as a pass. Repeat it on an isolated, higher-memory runner before the
production migration gate.

The dedicated `TestShield6MigrationNativeRehearsal` was also attempted on
2026-11-03 with one Go worker and a 1.4 GiB Go memory limit. The test process
reached approximately 3.3 GiB RSS after 146 seconds, while the live miner and
daemon shared the host. It was interrupted to protect the node and is not a
pass. The supervised daemon restarted during the memory-pressure period; after
the test stopped, its local RPC answered `eth_blockNumber` at `0xf2a6`. Run this
migration rehearsal on a dedicated runner with sufficient memory and no live
node workload before treating the release gate as passed.

The local full Linux/ARMv7 daemon build remains unverified. Its Rust proof
library compilation was killed by the operating system under memory pressure
while this host's live node and miner were running. This is not evidence of a
source error; the release workflow's dedicated ARM runners still need to
complete the full ARMv7 build and publishable artifact checks. The ARM64 and
Windows artifacts noted above were cross-built, but neither was executed on
native hardware. No live multi-node Shield6 P2P fork rehearsal has been run.

On 2026-11-04, the Shield6 cryptography test binary was cross-compiled from the
current worktree for Linux/ARM64, Linux/ARMv7, Windows/amd64, and macOS/ARM64.
`file` confirmed the expected ELF, PE32+, and Mach-O formats. These four test
binaries are architecture-compatibility checks only; they are not full `gtkm`
release artifacts and were not executed on their target operating systems.
`go vet -tags shield3` also passed for `zk/shielded6`, `core`, `eth`,
`internal/shield3wallet`, `core/txpool/legacypool`, and `cmd/gtkm` with one Go
worker. This static-analysis result does not replace the native migration
rehearsal or hosted release-artifact checks.

### Withdrawal wallet and local release checks on 2026-11-04

The wallet now has a direct Shield6 public-withdrawal builder and interactive
wallet option 18. It verifies the sender's registered stamp, requires a stamped
public destination, checks that public account funds cover the transaction fee,
selects local spendable notes, and returns any remainder as a confidential
Shield6 change output. The destination and withdrawal amount are public by
design. The native wallet builder, payment/deposit builders, and wallet
language-catalog tests passed:

```sh
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 \
  ./internal/shield3wallet ./cmd/gtkm \
  -run 'TestBuildAndSignShieldedV6(Withdrawal|Deposit|Payment)|TestWalletLanguage' \
  -count=1 -timeout=15m
```

A fresh Linux/amd64 production daemon build succeeded. Its SHA-256 is
`7b2771ff8a500a2ba89f6f13b363c7e6cba38b9e64584fc9ced96a6895583e26`.
This local binary links against the host's GLIBC 2.39 and GLIBCXX 3.4.32, so it
is a build check and must not be distributed as the portable Linux release.
The release workflow's Debian 11 builder is the portability gate and still
needs a passing hosted artifact run.

A fresh Windows/amd64 production cross-build succeeded as a 71 MB PE32+
executable, SHA-256
`6f9d8ba029e3a469dcd9f526e3d0ce36f4178b380c1e0eba1667346fce266d91`.
`objdump` shows only Windows system DLL imports; MinGW runtime libraries are
statically linked. It was not executed on Windows. These two builds do not
replace native macOS/ARM builds, target-platform runtime tests, a live P2P
fork rehearsal, or the hosted release workflow. Shield6 activation remains
unset.

### Focused verification rerun on 2026-11-04

The complete `zk/shielded6` cryptography and wallet package suite passed in
4.809 seconds. The wallet builder/scanner selection (private payment, deposit,
withdrawal, local ring selection, spendable-note scanning, reorg detection, and
secret cleanup) passed in 1.905 seconds.

The CLSAG all-signer-position, 128-bit range-proof, and wide-witness cleanup
regressions passed together with Go parallelism set to one and a 700 MiB Go
memory target. The shipped-network activation test passed and confirms
`Shield6Time` remains unset on the canonical network configurations. The
two-state deposit/spend/replay/rollback rehearsal passed in 4.316 seconds.
The maximum configured private-payment envelope test passed in 3.002 seconds,
well below its two-minute construction threshold. `git diff --check` passed.
These local checks do not replace the hosted release matrix, a full legacy-note
migration run on an isolated runner, or a live multi-node fork/reorg rehearsal.

The focused commands were:

```sh
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 ./zk/shielded6 -count=1 -timeout=5m
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 ./internal/shield3wallet \
  -run '^(TestBuildAndSignShieldedV6Payment|TestBuildAndSignShieldedV6Deposit|TestBuildAndSignShieldedV6Withdrawal|TestSelectShieldedV6RingUsesCanonicalCandidates|TestScanShieldedV6FindsSpendableNoteAndDetectsReorg|TestClearShieldedV6SpendCandidatesZeroizesPageNotes|TestScanShieldedV6MatchesPublicSpentKeyImagesLocally)$' \
  -count=1 -timeout=5m
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 ./zk/shielded6 \
  -run 'TestShield6(CLSAGAllSignerPositions|WideAmountRangeProof128|WideWitnessCleanup)' -count=1
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 ./params \
  -run '^TestShield6ActivationRemainsUnsetOnShippedNetworks$' -count=1
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 ./core \
  -run '^TestShield6TwoNodeDepositSpendReplayAndReorgRehearsal$' -count=1 -timeout=3m
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 ./core \
  -run '^TestShield6MaxPaymentEnvelopeUnderTwoMinutes$' -count=1 -timeout=3m -v
```

### Current CLI path

After a coordinated Shield6 activation, run `gtkm wallet interactive`, choose
`15) Show Shield6 payment code`, and share the resulting `tkmshield6:` code.
Choose `2) Send`, select the stamped ML-DSA-87 account, paste that code, enter
the amount, and confirm with `SEND`. The wallet scans canonical outputs,
selects confirmed notes locally, returns excess value as a private change
output, signs the spend and outer account transaction, and submits it to the
connected node. The payment code is derived from the selected account seed, so
the account keyfile and its recovery backup are required to scan and spend the
funds. Choose `18) Withdraw Shield6 to stamped address` for an explicit public
exit. The destination and amount are visible, the destination must already be
stamped, the outer account pays gas, and any unwithdrawn value is returned to
the same wallet as a confidential change note. The wallet scans spendable
notes and builds a CLSAG withdrawal locally. Choose `16) Migrate legacy notes to Shield6` to migrate a Shield3/4/5
note. The CLI asks for the legacy version and amount, then requires `MIGRATE`
to submit a self-withdrawal to the same account. After successful confirmation,
it asks separately for `DEPOSIT`; the deposit amount and destination are
public. If the second step is canceled or cannot pay its fee, the withdrawn
funds remain at the account's public address and the CLI reports the withdrawal
hash. This process is not atomic. Use it only after Shield6 activation and with
a node that has a reliable canonical chain view.

### Maximum-output proof bundle

The initial one-proof encoding exceeded the pinned backend's 1,024-point
generator table at 16 outputs. It is replaced with a canonical RLP pair of
separate low-limb and high-limb proofs. Both producer and verifier cap each
aggregate at 16 commitments and the encoded pair at
`MaxRangeProofBundleSize`. Consensus checks the complete bundle before curve
verification, and Shield6 gas charges every encoded proof byte. The regression
test proves and verifies all 16 outputs and independently tampers with each
limb proof, checks malformed bundle arity, and rejects trailing bytes. Keep
this test in the release gate; never increase output capacity without deriving
and testing the corresponding backend generator capacity first.

The release workflow's Linux ARM test-binary checks were run locally as well:
`GOOS=linux GOARCH=arm64 go test -tags shield3 -c -o /tmp/shield6-arm64.test
./zk/shielded6` produced an AArch64 ELF, and the equivalent command with
`GOARCH=arm GOARM=7` produced an ARM EABI5 ELF. These binaries were not run on
ARM hardware. On the current source, the primitive, core replay/reorg,
migration-policy, wallet, txpool, RPC, and CLI confirmation-waiter test suites
passed with one Go worker per command.

The migration CLI now preflights the account's pending public balance against
the maximum fee reserves for both the legacy self-withdrawal and the follow-up
Shield6 deposit before it broadcasts the withdrawal. If the reserve cannot be
calculated or the balance is insufficient, it stops before step one. The
focused fee-reserve and canonical-confirmation tests pass. This avoids knowingly
starting a migration that cannot fund its second transaction at the current
fee estimate, but cannot guarantee future fee-market conditions or block
inclusion.

After the 12-confirmation wait, immediately before building the deposit, the
CLI obtains a fresh gas-price estimate and pending account balance. It requires
the account to cover both the public deposit amount and the full deposit gas
limit at the current fee cap. If fees rose during confirmation and the reserve
is no longer sufficient, it leaves the withdrawal amount public and reports
the shortfall without submitting the deposit. The release workflow runs tests
for both preflight calculations and the canonical confirmation waiter.

`TestShield6MigrationNativeRehearsal` now isolates a full native Shield4
self-withdrawal followed by a Shield6 public deposit on two independent state
replicas. It checks both public-boundary transaction types, replica state-root
agreement, output accumulator agreement, local note decryption, backing
conservation, and the two-minute Shield6 deposit-build limit. The test compiles
and is included in the Linux release test selection. A local execution attempt
was interrupted after 83 seconds at about 2.2 GiB RSS because the production
daemon and miner were active on the same 7.6 GiB host and available memory fell
to about 230 MiB. No test assertion had failed before interruption; the run is
inconclusive. Run this gate on an isolated runner before considering migration
validated.

The focused retest on 2026-11-03 passed the Shield6 key/payment-code and
malformed-input crypto tests, accumulator append/revert and frontier checks,
stateful CLSAG ring verification and replay rejection, fork ordering and
pre-activation rejection, legacy self-exit migration policy, pending key-image
reservation, and bounded Shield6 RPC API tests. The high-memory native migration
rehearsal was not rerun; its status remains inconclusive.
A second attempt on 2026-11-03 used `GOMEMLIMIT=1500MiB`; after 29 seconds the
test process had reached about 1.0 GiB RSS while the daemon and miner left only
274 MiB available and swap use was increasing. It was interrupted before any
assertion result to protect those live services. Run the rehearsal on an
isolated host with sufficient memory; do not treat either attempt as a pass.

The subsequent focused verification on 2026-11-03 passed `./zk/shielded6`,
the core Shield6 accumulator, CLSAG, replay, fork-gate, and two-state
deposit/spend/reorg tests, the Shield6 wallet construction/scanning/ring tests,
the bounded RPC and transaction-pool key-image tests, and the CLI migration
fee-reserve checks. The maximum configured private-envelope builder measured
about 3.0 seconds under a 700 MiB Go memory limit on this host. The primitive
package test binary also cross-compiled as Linux AArch64 and ARM EABI5. These
checks do not validate the native migration rehearsal or the full node release
artifacts. Recipient stamp authorization remains a protocol gap: the Shield6
output does not prove that a receiver controls a registered stamp, and no
wallet-only preflight can replace that consensus check. Keep activation unset.

An additional bounded rerun on 2026-11-03 passed the primitive crypto suite,
the core two-state deposit/spend/replay/reorg and accumulator rollback tests,
the shipped-network activation guard, the wallet Shield6 tests, txpool
key-image tests, the Shield6 RPC tests, and `git diff --check`. A broad initial
wallet filter also selected the separate native migration rehearsal; it was
stopped at 61 seconds to avoid repeating the known memory pressure on this
shared node host. It produced no migration result and is not counted as a pass.

The wallet scan privacy path was then tightened: consensus now keeps an
append-only, journaled index of canonical Shield6 key images, and RPC exposes
bounded pages of that public set plus a bounded sorted set of pending pool key
images. The wallet matches owned notes against those public sets locally; it
does not send key images derived from owned notes to the RPC server during
scanning or payment construction. The txpool remains the final pending-spend
race check. This adds per-input state writes within the existing Shield6 input
verification charge. Focused tests passed for key-image index append,
duplicate rejection and rollback, RPC page order and bounds, scanner matching
of confirmed and pending images, and the two-state spend/reorg rehearsal. The
implementation remains inactive and still requires the native migration
rehearsal, full release builds, multi-node testing, and recipient-stamp proof
design before activation.

The Linux release workflow now selects the renamed local key-image matching
test instead of the removed owner-specific status-query test. The exact wallet
test selection from that workflow passed on 2026-11-03, and the updated
`release.yml` parsed successfully. A fresh daemon binary build was not run on
this host because only about 1.3 GiB memory was available while the production
daemon and miner were using over 5 GiB combined; rely on the isolated release
runner to validate the full binary build.

The final privacy pass removed the per-key-image status RPC methods. Those
methods accepted a wallet-derived image and exposed wallet ownership/spend
queries to the node operator. Wallet scanning uses only bounded public index
pages and local matching. The focused crypto, core, RPC, wallet, and txpool
test selections passed after this change. A broad wallet selector was
interrupted after 111 seconds when its native legacy-migration case reached
about 3.3 GiB RSS; that run is inconclusive and is not counted as a pass.

### Bounded verification on 2026-11-04

On the shared production host, the following bounded test selections passed
with `GOMAXPROCS=1` and one package worker: `go test -p=1 -tags shield3
./zk/shielded6 -count=1`; the Shield6 consensus/state tests in `./core`; the
Shield6 payment, deposit, ring-selection, scan/reorg, and key-image scan tests
in `./internal/shield3wallet`; and the Shield6 RPC and txpool tests in `./eth`
and `./core/txpool/legacypool`. These are unit and in-process state tests; they
do not constitute a live multi-node rehearsal.

On the same Haswell host, three repeated samples of three builds each, with
`GOMAXPROCS=1`, measured 2.165–2.799 seconds per 16-output 128-bit aggregate
Bulletproofs+ proof (median 2.551 seconds; 33,633,664–33,634,176 bytes
allocated) and 0.916–1.140 seconds per signing and self-verification of 16
maximum-size CLSAG inputs (median 0.959 seconds; 655,648–655,664 bytes
allocated). These isolate cryptographic construction; they exclude accumulator
scanning, RPC, the outer transaction signature, and propagation, and do not
represent a multi-platform performance study.

The CLSAG adversarial test now also substitutes a different valid key image,
valid pseudo-output, and reordered valid ring. Each must fail verification,
which exercises transcript binding instead of only malformed-point parsing.
The focused CLSAG, wide-amount, all-signer-position, and malformed-input test
selection passed after these cases were added.

The native Shield4-to-Shield6 migration rehearsal was intentionally not run
again on this host. Previous attempts exceeded 3 GiB RSS while the live node
and miner were active and were stopped before completion. Its dedicated
60-minute GitHub Actions job is the required place to complete that test. The
job must pass before this migration is considered validated.

Shield6 remains unactivated. Production readiness still requires the isolated
native migration test, full release artifact matrix, live multi-node/reorg
rehearsal, and a consensus-verifiable recipient-stamp authorization design.
Do not set an activation timestamp or describe the fork as ready until those
gates pass; public deposits and withdrawals remain explicit value-disclosure
boundaries in this protocol version.

The same day, a single-iteration verifier benchmark on the Haswell host
measured 148.5 ms for verification of the maximum 16-output 128-bit aggregate
range proof and 147.9 ms for verification of 16 maximum-size CLSAG inputs
(16-member ring per input), with `GOMAXPROCS=1`. These are direct verifier
measurements, distinct from the earlier proof-generation and signing samples.
They show the isolated crypto checks fit comfortably within a two-minute
wallet-side construction target on this CPU, but do not establish block-level
throughput, gas calibration, or performance on slower architectures. The
focused primitive, core state/replay/reorg, wallet, RPC, txpool, and CLI
migration fee-preflight test selections were rerun on 2026-11-04 and passed;
the native migration and live network gates remain outstanding.

The Shield6 cryptography test package was also cross-compiled on 2026-11-04
with `CGO_ENABLED=0` for Linux/ARM64, Linux/ARMv7, Windows/amd64, and
macOS/ARM64. The outputs were confirmed as AArch64 ELF, ARM EABI5 ELF, PE32+,
and ARM64 Mach-O binaries. These checks establish Go source portability for
the primitive package only; they do not substitute for full daemon release
builds, platform-native artifact validation, or runtime tests on those systems.

A further local attempt ran the isolated native migration test with
`GOMEMLIMIT=1200MiB`, `GOGC=40`, and one Go worker while the production daemon
and miner remained active. It initialized both replicas, registered the stamp,
and built and applied the legacy Shield4 deposit in 47.704 seconds. Before the
withdrawal proof completed, the test reached 3.34 GiB RSS, available system
memory fell below 900 MiB, and the live daemon's RSS dropped. The test was
interrupted to protect the daemon; it is inconclusive, not a test failure or a
pass. The daemon recovered to 3.1 GiB available memory after interruption. Run
this end-to-end gate on the isolated GitHub runner defined in
`shield3-crypto.yml` before treating migration as validated.

### Migration rehearsal and gas-accounting follow-up (2026-11-04)

The isolated native migration rehearsal subsequently completed successfully
on this host in 185.00 seconds with one Go package worker and bounded Go heap
settings (`GOMEMLIMIT=1300MiB`, `GOGC=20`). It used two independent in-process
state replicas. Both replicas accepted the same stamped legacy Shield4 deposit
and same-account public withdrawal, then accepted a Shield6 public deposit;
both ended at the same state root and Shield6 accumulator root. The destination
wallet scanned the migrated note at the expected amount, and the confidential
ledger's deposited, withdrawn, and backing totals matched the pool balance.
The legacy deposit proof took 54.027 seconds, the legacy self-withdrawal proof
took 1m53.486s, and the Shield6 migration deposit proof took 336ms. This is a
passing migration-path test, but it is not a live P2P multi-node or canonical
chain reorganization test. It also confirms that the migration bridge is
deliberately transparent: the legacy withdrawal and Shield6 deposit disclose
their boundary amounts.

The rehearsal was run once, not concurrently with another test process. During
the run, the test process reached roughly 3.1 GiB RSS and available memory
briefly fell to about 1.4 GiB; it recovered after completion. The daemon,
miner, and pool were left running. Earlier entries in this document describing
the migration test as unrun or inconclusive are superseded by this completed
run; they remain as an audit trail of prior attempts.

A final source review found that the Shield6 gas estimate charged the shared
key-image-count slot as a fresh zero-to-nonzero write for every input. Actual
subsequent input updates that same nonzero slot, so the prior estimate
undercharged multi-input transactions. The formula now adds 2,900 gas for each
input after the first, while retaining worst-case 20,000-gas charges for each
new key-image and lookup slot. A regression assertion checks the exact
two-input increment. The focused envelope/gas test, the complete Shield6 core
test selection, a fresh Linux `gtkm` build, and the activation-unset parameter
test all passed after the correction. This remains a conservative EVM-storage
style estimate and should be compared with the exact host-state write model
before activation.

The remaining production gates are unchanged: recipient-stamp authorization
has no consensus-verifiable Shield6 representation; GUI/Android integration,
live P2P and chain-reorg rehearsal, the full release artifact matrix, and
cross-platform runtime/performance validation remain incomplete. Shield6 stays
inactive. The successful in-process migration rehearsal does not clear these
gates or authorize setting an activation timestamp.

The Shield6 crypto test package was also cross-compiled again on this host
for Linux ARM64, Linux ARMv7, Windows x64, and macOS ARM64. The resulting
artifacts were identified as AArch64 ELF, ARM EABI5 ELF, PE32+, and ARM64
Mach-O, respectively. These are crypto-package portability checks only; the
full daemon and native Shield3 libraries still need the release matrix.
Finally, a fresh run of `TestShield6MaxPaymentEnvelopeUnderTwoMinutes` measured
4.181 seconds per maximum-size envelope build on this host. This confirms the
local envelope-construction target for this case, not end-to-end network
confirmation latency or performance on all release devices.

An attempt to run the full Linux ARM64 release target on this shared node
rebuilt the target RandomX and Shield3 native libraries, then entered the Go
daemon compile. It was interrupted before the daemon artifact completed when
the live node's RSS rose above 3.5 GiB and available host memory reached about
1.1 GiB. The daemon, miner, and pool remained running. Do not count this as an
ARM64 daemon build; the complete ARM64/ARMv7/Windows/macOS release artifact
matrix still requires a dedicated runner or a host with more spare memory.

A later attempt to rerun the two-state deposit/spend/replay/reorg rehearsal
was interrupted during Go package compilation, before the test started. The
live daemon reached about 4.3 GiB RSS and available memory fell to roughly
220 MiB; the test process was stopped and the services were left running.
This attempt was inconclusive at the time and did not replace the earlier
passing two-state test result. A later bounded local run passed the strengthened
rehearsal; see “Bounded local verification (2026-11-04)” below. Hosted CI must
still run the gate on the release revision.

Both release workflows now run that isolated migration test with one Go
worker, `GOMEMLIMIT=1300MiB`, and `GOGC=20`, matching the settings used for the
passing local rehearsal. Both workflow YAML files parse successfully. The
hosted workflows still need to run on the release revision; syntax and local
test success do not prove the complete artifact matrix.

The two-state rehearsal assertion has since been strengthened to compare the
replicas' complete indexed output sets, each output's canonical ring member and
transaction hash, and every indexed key image. This makes the test detect
replica divergence that could otherwise preserve the same accumulator root or
aggregate counts. The release workflow and Shield3/Shield6 crypto workflow
also run this consensus rehearsal as a separate bounded step, so its result is
visible independently of the broader test selection.

### Bounded local verification (2026-11-04)

The following checks completed on the current worktree with the live daemon,
miner, and pool left running. Go test processes were serialized and limited to
`GOMEMLIMIT=1300MiB`, `GOGC=20`, and `GOMAXPROCS=1` where applicable:

* `go test -p=1 -tags shield3 ./zk/shielded6 -count=1` passed.
* `go test -race -p=1 -tags shield3 ./zk/shielded6 -count=1` passed in
  49.111 seconds with a bounded Go memory limit.
* `TestShield6TwoNodeDepositSpendReplayAndReorgRehearsal` passed; it compared
  both replicas' indexed output records, creation transaction hashes, key
  images, accumulator root/count, and confidential backing state after deposit,
  multi-input spend, replay rejection, rollback, and canonical re-import.
* The six focused Shield6 wallet tests passed: private-payment signing, deposit
  construction, canonical ring selection, reorg-aware note scanning, secret
  clearing, and local key-image matching.
* After adding randomized ring ordering, `TestSelectRingShufflesIndicesAndIncludesRealOutput`,
  the six focused wallet tests, and
  `TestBuildShieldedV6EnvelopeFromScannedNote` passed. The core test shuffled
  ring members and accumulator indices before signing, then verified both the
  stateless proof and stateful CLSAG against canonical state.
* `TestShield6MigrationNativeRehearsal` passed on two independent state
  replicas. The real legacy Shield4 self-withdrawal took 1m57.649s; the Shield6
  migration deposit builder took 338ms. Both transitions were confirmed on
  both replicas and the destination wallet scanned the expected note amount.
* `TestShield6MaxPaymentEnvelopeUnderTwoMinutes` passed at 5.548s per
  maximum-size envelope build. This measures local construction, not network
  confirmation time.
* The targeted `./core ./eth` Shield6/CLSAG tests, including the Ethereum API
  package, passed. `TestShield6ActivationRemainsUnsetOnShippedNetworks` passed.
* `git diff --check` passed and both modified GitHub Actions workflow files
  parsed as YAML.

These results clear the local crypto, wallet, in-process consensus/migration,
and construction-performance checks. Hosted CI still must complete the release
workflow and full published-platform artifact matrix. A live multi-node P2P
fork/reorg rehearsal is also outstanding; do not configure a Shield6 activation
timestamp until those gates pass.

### Linux daemon link verification (2026-11-04)

A current stripped Linux production daemon was built with the repository's
`make production-gtkm` target into `/tmp/shield6-production-bin/gtkm`, leaving
the running daemon binary untouched. The first build exposed that Goja's JIT
assembly caused GNU ld to mark the process stack executable. The Linux host
and ARM RandomX link flags now include `-Wl,-z,noexecstack`, and the Linux amd64
and ARM release jobs assert that the built daemon's `GNU_STACK` flags are
exactly `RW`.

The rebuilt Linux amd64 daemon is a stripped x86-64 ELF executable with
`GNU_STACK RW`; its `version` command runs. SHA-256:
`9f68223cae67409e0131fe979b57bfd01ae9cd7118166eaacb21c6d0bd6baad9`.
`ldd` reports the expected glibc, libstdc++, and libm runtime dependencies.
Both workflow YAML files parse and `git diff --check` passes. ARM, macOS, and
Windows release binaries still require their hosted build matrix; this Linux
build does not substitute for those platform checks.

### Follow-up local verification (2026-11-04)

After the ring-order hardening, a fresh bounded rerun passed the complete
`./zk/shielded6` package, focused `./core` Shield6 proof/state tests, the six
`./internal/shield3wallet` Shield6 send/deposit/ring/scan tests, and
`TestShield6ActivationRemainsUnsetOnShippedNetworks`. The core wallet-built
transaction test also confirms shuffled ring order is accepted and duplicate
accumulator indices are rejected. The CLI migration fee-reserve and canonical
confirmation tests passed after compiling the current `cmd/gtkm` dependency
graph. `gofmt -d` and `git diff --check` reported no issues.

The pure-Go `zk/shielded6` test package also cross-compiled successfully on this
host for Linux AArch64, Linux ARM EABI5, Windows x86-64, macOS ARM64, and macOS
x86-64. `file` confirmed ELF AArch64, ELF ARM EABI5, PE32+, and both Mach-O
formats. This verifies Go source and test-binary portability for the crypto
package; it does not validate native Shield3/Rust linking, full daemon builds,
or runtime behavior on those platforms.

This does not close the activation gate: GUI and Android Shield6 send flows
remain unwired, recipient-stamp authorization has no Shield6 consensus proof,
and hosted multi-platform release CI has not been run on this worktree. Keep
`Shield6Time` unset until these protocol and release gates are resolved.

### Canonical block-processor integration check (2026-11-04)

Added `TestShieldedV6DepositUsesCanonicalBlockProcessor` in
`core/shielded_v6_rehearsal_test.go`. It submits a signed Shield6 deposit
through `StateProcessor.Process`, the same execution path used by block import,
then checks the successful receipt, output-accumulator append, and sender nonce.
The paired negative case signs an envelope with a corrupted range proof and
confirms block processing rejects it without changing the output accumulator
or sender nonce. The focused test passes with:

```sh
go test -p=1 ./core -run '^TestShieldedV6DepositUsesCanonicalBlockProcessor$' -count=1
```

This closes the gap between the previous direct-helper rehearsal and the real
serial block processor for Shield6 deposits. It does not replace full block
import, P2P propagation, platform release, GUI/Android, or activation testing.

The same file also contains `TestShieldedV6PrivateSpendUsesCanonicalBlockProcessor`.
It processes a 16-output deposit and wallet-built CLSAG spend through
`StateProcessor.Process`, snapshots the pre-spend state, replays the same block
after rollback, checks deterministic accumulator state, and rejects a duplicate
key-image inclusion. On this host, the Shield6 crypto package passed in 5.870s
and the three focused core rehearsal tests passed in 4.567s. The command was:

```sh
go test -p=1 ./zk/shielded6 -count=1
go test -p=1 ./core -run '^(TestShieldedV6DepositUsesCanonicalBlockProcessor|TestShieldedV6PrivateSpendUsesCanonicalBlockProcessor|TestShieldedV6TwoNodeDepositSpendReplayAndReorgRehearsal)$' -count=1 -timeout=5m
```

This verifies canonical state processing for deposits and private spends in an
in-process deterministic harness. It is not a networked fork/reorg test.

The release-workflow Shield6 filter was then run locally with the same build
tag against both packages:

```sh
go test -tags shield3 -p=1 ./core ./eth -run 'ShieldedV6|Shield6|CLSAG' -count=1 -timeout=10m
```

Both packages passed (`core` 11.479s; `eth` 0.302s), including the new
canonical processor spend/replay case. This remains test evidence, not a
completed multi-platform release matrix or live-network hardfork rehearsal.

### Local Linux production daemon build (2026-11-04)

The current source also built successfully with production RandomX and
Shield3/cgo tags. The output was directed under `/tmp` so it did not replace
the repository's existing `build/bin/gtkm` or the running node:

```sh
CARGO_BUILD_JOBS=1 make production-gtkm GOBIN=/tmp/shield6-release-check GO_BUILD_P=1
```

The Rust verifier library built successfully (43,358,214 bytes), followed by
the stripped `gtkm` binary. It reports version `1.21.82-shielded-gas`, commit
`7f3624b9fc088a89b2e7f1a277c2253e0d9de5a3`, Go 1.26.6, Linux amd64. Its GNU
stack is `RW` (non-executable), and SHA-256 is
`ba3b03a3b2337422f45b12f5e11f1761163ef11f8bbd53a2361348314e597e1e`.
This proves the local Linux production daemon builds; Windows, macOS, and ARM
release artifacts and hosted release CI remain unverified on this worktree.

The migration-policy test now covers all legacy versions accepted by the CLI
(Shield3, Shield4, and Shield5). It verifies that only a public withdrawal
back to the transaction's own sender is recognized as the Shield6 migration
exit; a withdrawal to another address or a legacy deposit is rejected. The
test passed both standalone and inside the full release-workflow Shield6
filter. That filtered core/eth run passed again after the change (`core`
12.220s; `eth` 0.243s):

```sh
go test -p=1 ./core -run '^TestShield6LegacyMigrationAllowsOnlySelfExitAfterActivation$' -count=1
```

### Bounded continuation check (2026-11-04)

The current worktree passed the shipped-network activation guard, the
`zk/shielded6` crypto suite, the filtered core/eth consensus and RPC suites, the
focused wallet send/deposit/ring/scan tests, and the CLI migration confirmation
tests. The maximum configured payment-envelope test was rerun with one Go
worker and a 700 MiB Go memory limit; it measured 3.007 seconds per build and
passed the two-minute bound.

The native legacy-proof migration rehearsal was not repeated in this session.
At the time of verification, the live daemon and miner together used about
5.3 GiB RSS and the host had about 1.2 GiB available memory. The existing
bounded two-replica migration pass recorded above remains the migration test
evidence; do not run the high-memory native rehearsal alongside those live
services. No live node was stopped or replaced. `git diff --check` passed.

`TestShield6TransactionP2PPropagation` was added to `eth/handler_eth_test.go`
and passed. It builds a range-proven Shield6 deposit, signs the outer typed
transaction with ML-DSA-87, propagates it over the real `eth` protocol message
handler to two peers, and checks byte-identical transaction encoding and
preservation of the decoded Shield6 envelope at both receivers. The test uses
the protocol test harness's accepting transaction pool, so it verifies the
wire path rather than receiver consensus execution; canonical block processing
is covered by the core tests above. This does not substitute for a live
multi-node fork/reorg rehearsal.

On 2026-11-04, the focused crypto, core, eth, and params Shield6/CLSAG suites
were rerun with one Go worker and passed, including transaction propagation and
the shipped-network activation guard. A wider wallet-filter run was stopped
after it reached roughly 2 GiB RSS and reduced host memory availability to
about 227 MiB; that run is not counted as a pass. The earlier focused wallet
and native migration test results recorded above remain the available wallet
evidence. No live services were changed. Full P2P block-sync/reorg behavior,
the cross-platform release artifact matrix, and live fork rehearsal remain
release gates; keep `Shield6Time` unset until they pass.

### Shield6 amount-boundary follow-up (2026-11-04)

Removed the inherited Shield3 5,000,000 TKM ceiling from Shield6 public
consensus deposits, public withdrawals, and wallet envelope builders. Shield6
amounts remain constrained by the protocol's 128-bit encoding and the
confidential balance/range-proof rules. Regression coverage now builds and
validates deposits and signed-note withdrawals just above the old Shield3 cap,
and confirms a deposit above the Shield6 128-bit range is rejected.

The bounded focused command passed after these changes:

```sh
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 ./zk/shielded6 ./core ./eth ./params -run 'Shield6|ShieldedV6|CLSAG|ConfidentialLedger|ActivationRemainsUnset' -count=1 -timeout=10m
```

The `zk/shielded6`, `core`, `eth`, and `params` packages all passed. The
attempted in-process downloader/reorg test did not shut down its background
sync cycle reliably and was removed; no block-sync/reorg success is claimed.
The passing P2P test covers signed transaction propagation only. Mainnet
`Shield6Time` remains unset, and complete network reorg, platform artifact,
and live fork rehearsal gates remain outstanding.

### Wallet, migration, and release rebuild follow-up (2026-11-04)

The focused Shield6 wallet tests passed for payment signing, public deposit
construction, canonical ring selection, and reorg-aware output scanning. The
real two-replica native migration rehearsal also passed: both replicas accepted
a legacy Shield4 deposit and owner-directed withdrawal, accepted the Shield6
deposit, appended identical accumulator state, and the destination wallet
scanned the migrated note. The Shield6 migration deposit proof took 173 ms.
The legacy Shield4 withdrawal proof took 2m2s; this is legacy migration cost,
not Shield6 payment construction time.

After the amount-boundary changes, `make production-gtkm` successfully rebuilt
the stripped Linux amd64 daemon under `/tmp/shield6-release-check/gtkm`. It
reports version `1.21.82-shielded-gas`, commit `7f3624b9`, and has a non-
executable `RW` GNU stack. SHA-256:
`1da4b56913fc9c40179dc53718b0b7504f4940f1a0e983aa7ec920ce9c20f935`.
This is a local Linux build only, not proof of the release matrix.

The focused wallet command completed successfully for all four tests (payment,
deposit, ring selection, and reorg-aware scanning). The isolated native
migration rehearsal completed successfully in 174.85 seconds; within it, the
Shield6 migration deposit build took 173 ms. Shield6 crypto test binaries also
cross-compiled successfully for Linux arm64 and armv7. These cross-compiles
cover `./zk/shielded6` only and do not substitute for complete ARM daemon builds
or macOS/Windows release artifacts.

A full Linux ARM64 `gtkm` cross-build was attempted with the available
AArch64 compiler and the prebuilt RandomX/Shield3 libraries. Go package
compilation reached the final external link, which failed with `ld: No space
left on device`; the root filesystem was 99% full with 838 MiB free. This is
not counted as a successful ARM daemon build. No files outside the task's
`/tmp` output path were removed. Retry the full link only after storage is
freed; preserve the cross-compile and local Linux build results above.

The temporary Linux amd64 production binary and ARM64/armv7 Shield6 test
executables were removed after recording their verification results to return
space to the nearly full host filesystem. Rebuild them before packaging or
publishing release artifacts.

### ARM64 production cross-build (2026-11-04)

The full stripped production daemon cross-compiled successfully for Linux
AArch64 after setting `TMPDIR=/dev/shm` so the external linker did not exhaust
the nearly full root filesystem. The verified artifact was
`/tmp/gtkm-linux-arm64-shield6` (ELF AArch64, GNU_STACK `RW`, SHA-256
`9d1e48d18a4deb16d6c10dbcd8defa047057890f0c87177b759bad30f3bb51e1`). That
temporary binary has since been removed to recover disk space. This was an
untagged local build, not a published release.

An ARMv7 Shield3/4 native archive build was also attempted in `/dev/shm`, but
its optimized `triton-vm` Rust compilation exceeded 3 GiB RSS while the local
miner was running. It was stopped to avoid host memory exhaustion. The partial
tmpfs workspace was removed. ARMv7 daemon artifacts remain unverified.

### Replay/reorg and payment-speed rerun (2026-11-04)

The Shield6 two-replica deposit/spend/replay/reorg rehearsal passed again in
5.60 seconds. The maximum configured Shield6 payment-envelope construction
test passed at 4.83 seconds per build, below its two-minute regression bound.
These results validate the in-process consensus rehearsal and envelope build;
they do not establish live P2P synchronization or stamp authorization for
recipients.

### Release-matrix resource check (2026-11-04)

A Windows amd64 production cross-build was started after the Linux checks, but
Go's cross-compilation cache filled the root filesystem before the build
completed. The build was stopped; no Windows artifact was produced and no
Windows build is claimed. Temporary Shield6 validation binaries were removed
to restore 836 MiB of free space. The root filesystem remains near capacity,
so any further platform build needs an isolated cache and temporary directory
with enough reserved space. Do not retry the matrix build against the live host
without that space budget.

The activation blockers are unchanged: recipient stamp ownership is not yet
proven by the Shield6 consensus relation, GUI/Android send flows are not wired,
and live multi-node block synchronization/reorg and the complete release
platform matrix have not passed. Keep `Shield6Time` unset until these are
resolved and a coordinated chain migration is ready.

### Interactive wallet first-deposit path (2026-11-04)

The interactive wallet now exposes `Deposit TKM to Shield6` as a direct public
entry path, in addition to the two-step legacy migration. It refuses to build
unless the node reports Shield6 active on the selected chain, requires an
ML-DSA-87 account with its registered stamp, checks the public balance against
the deposit plus maximum gas reserve, and verifies the returned transaction
hash before reporting broadcast. After inclusion it prints the transaction
hash and the chain-bound Shield6 payment code. The amount and sender are
explicitly disclosed by the deposit boundary; the resulting note is encrypted
to the wallet. The focused `cmd/gtkm` Shield6 migration and wallet test filter
passed after wiring this menu action.

This closes the first-note usability gap for CLI users; it does not close the
separate GUI/Android integration or recipient-stamp consensus gaps above.

### Canonical withdrawal rehearsal coverage (2026-11-04)

The bounded Shield6 core tests now exercise public withdrawal through the
canonical block transaction processor, in addition to envelope-level checks.
The single-state test verifies that an unstamped destination is rejected
without consuming a key image, while a stamped destination is credited and the
pool backing ledger is reduced by exactly the public withdrawal amount.

The two-replica deposit/spend/replay/reorg rehearsal now also submits a signed
public withdrawal to both state replicas. Both reject an unstamped destination
without changing key-image state, then accept the stamped destination and
record the same withdrawn amount in their confidential backing ledgers. The
focused two-replica test passed in 8.7 seconds. This is an in-process
deterministic state-replica test; it is not a live P2P/network or hardfork
rehearsal.

### Bounded wallet and daemon verification (2026-11-04)

The Shield6 wallet builder, deposit, withdrawal, ring selection, scanner,
reorg-detection, and local key-image matching tests passed. The interactive
wallet's migration fee-reserve and deposit-balance guard tests also passed.
The current daemon compiled successfully on Linux/amd64 from this worktree
(`go build -p=1 -tags shield3 ./cmd/gtkm`); this is a host build, not a
portable release artifact.

The native legacy migration test was deliberately interrupted after 27.7
seconds because this host is running the daemon and miner. That interrupted
run is not a pass; migration remains a dedicated-runner verification gate.
Windows cross-build evidence and a prior successful native migration
rehearsal are recorded above, but this session did not execute the full release
matrix or live-network fork test. Keep activation unset.

The Shield6 cryptography test package was also cross-compiled successfully in
this worktree for Windows/amd64, macOS/arm64, Linux/arm64, and Linux/armv7.
These are compile checks only; they do not substitute for running the tests on
native hosts or building the complete daemon/release packages for each target.

Maximum-load single-iteration benchmarks on this host (Intel Core Processor,
Haswell-class virtual CPU, one Go scheduler thread) measured 3.67 seconds for
generating and verifying paired 128-bit range proofs over 16 outputs, 0.24
seconds to verify that maximum-output proof, 0.26 seconds to verify sixteen
16-member CLSAG rings, and 4.14 seconds to build and verify a maximum 16-input,
16-output payment envelope. The envelope benchmark includes proof generation,
output encryption, CLSAG signing and verification, commitment conservation,
and encoding. It excludes RPC scanning/decoy retrieval, the outer ML-DSA
signature, transaction propagation, and block inclusion. The Linux release job
now records these same maximum-load benchmark results for each release source
revision; they are not a promise of end-to-end confirmation latency.

An interoperability test now constructs a CLSAG with the pinned upstream
wallet signer and verifies its fixed-width transaction encoding through
Shield6's consensus-facing decoder and verifier. This exercises the reverse
direction from the Shield6 wallet signer, which already verifies generated
signatures using the upstream verifier. The test passed locally and is now run
by the Linux release job as part of the Shield6 cryptography suite.
After adding this test, the complete Shield6 crypto test binary cross-compiled
again for Windows/amd64, macOS/arm64, Linux/arm64, and Linux/armv7; `go vet`
also passed for `zk/shielded6`.

The output-accumulator test suite now recomputes the sparse Merkle root from
the ordered leaf set at counts around binary frontier carries (1, 2, 3, 4, 5,
7, 8, 15, 16, 17, 31, 32, 33, 39, and 40). It passed, in addition to the
append-only and snapshot-revert checks.

### Focused readiness rerun (2026-11-04)

The complete `zk/shielded6` Go test package passed. A bounded cross-package
selection also passed for the CLSAG upstream interoperability vector, 128-bit
range proofs, sparse accumulator recomputation, ring membership and replay
rejection, activation and legacy self-exit policy, bounded/activation-gated
RPC reads, wallet payment/deposit/withdrawal construction, canonical ring
selection, scanning/reorg detection, and migration fee preflight. These tests
ran with one Go scheduler thread and a 700 MiB Go memory limit while the host
daemon and miner remained online.

The default command sandbox could not start Go because its loopback network
namespace setup failed; the same bounded test commands completed outside that
sandbox. Shield6 activation remains unset. The open gates remain
recipient-stamp ownership enforcement in Shield6 consensus, GUI/Android send
integration, a hosted native migration run, full platform release artifacts,
and live multi-node fork/reorg rehearsal.

The two-replica deposit/spend/replay/reorg/withdrawal rehearsal passed again
on this revision. The canonical block-processor deposit test now also signs a
valid Shield6 deposit from an account whose stamp was removed from state; the
block processor rejects it and leaves the output accumulator, nonce, and
confidential backing unchanged. The accepted-deposit case also asserts that the
pool's actual TKM balance and the confidential ledger's deposited and backing
totals each increase by exactly the outer transaction amount, guarding against
double credit and unbacked Shield6 outputs. The test passed.

A prior cold-cache daemon build attempt was stopped before linking when free
root-disk space fell below 900 MiB. On 2026-11-04, after reclaiming the
rebuildable Go cache, a second bounded Linux/amd64 build progressed through
dependency compilation to the large `eth` package. It was stopped when host
memory available fell to about 250 MiB with the node and miner online. It
produced no artifact and is not a build pass. The earlier successful Linux
host build recorded above remains the local compile evidence; a clean release
build must still run on an adequately provisioned hosted runner.

The Shield6 fork-schedule tests now cover the exact activation boundary
(inactive one second before, active at the configured timestamp), reject a
schedule before Antartical or without the required quantum-resistant fork, and
assert that the shipped mainnet activation remains nil. The focused `params`
tests passed on 2026-11-04.

The Shield6 wallet scan cleanup was tightened on 2026-11-04: decrypted note
fields are now wiped even when authenticated plaintext fails decoding or
domain validation, and rejected scan amounts have their backing words cleared.
`go test -tags shield3 ./zk/shielded6` passed after the change. This is a
wallet memory-hygiene improvement and does not close the activation blockers
listed above.

The Shield6 crypto/wallet test package also cross-compiled successfully on
2026-11-04 for Linux/arm64, Linux/armv7, macOS/arm64, and Windows/amd64 using
Go 1.24.6. These checks establish package portability only; they do not replace
the tagged release workflow's full `gtkm` builds, native-library link checks,
or runtime tests on those platforms.
`GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go vet -tags shield3 ./zk/shielded6`
also passed on this revision.

The amount-proof API now clears generated per-output blinding witnesses when
the caller requests only commitments/proof, and clears them on every error
return. The wallet-facing witness API retains them only on success, where the
caller remains responsible for clearing them. The regression suite checks the
wide-witness wipe helper, and the focused Shield6 crypto tests passed again.

### Bounded consensus and speed rerun (2026-11-04)

With `GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20`, the fresh-cache core package
build completed and `TestShield6TwoNodeDepositSpendReplayAndReorgRehearsal`
passed in 5.08 seconds. The same bounded invocation ran
`TestShield6MaxPaymentEnvelopeUnderTwoMinutes`; the maximum configured
16-input, 16-member-ring, 16-output wallet builder completed in 3.596 seconds.
This is local envelope construction plus self-verification, not end-to-end
RPC, network propagation, outer ML-DSA signing, or block-inclusion latency.
Full daemon release builds and the native legacy-to-Shield6 migration rehearsal
were not rerun on this host; the full build remains a hosted release gate.

### Hosted migration and production-build gate (2026-11-04)

GitHub Actions run [41803812832](https://github.com/tkmchain/go-tkmchain/actions/runs/41803812832)
passed the isolated native migration rehearsal on commit `a6f19341c`. CI built
the production native Shield4 verifier, migrated Shield4 state to Shield6 on
two independent state replicas, and verified that Shield6 remains inactive on
the shipped networks. This closes the hosted migration-test gate for that
revision; it is not a live peer-to-peer fork or reorganization rehearsal.

At the time of this update, the run's `native-proof` job is still executing its
Go/native interoperability checks. Its production Linux GTKM daemon build and
version check are queued later in that job, so neither is recorded as passed
yet. Full macOS, Windows, and ARM daemon release artifacts and a live multi-node
fork/reorg rehearsal also remain outstanding. Shield6 activation remains
unset pending those checks and the other activation requirements above.
