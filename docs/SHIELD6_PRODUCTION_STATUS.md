# Shield6 production status

Fresh production parameters were generated on 2026-11-06 and pinned in the
mainnet configuration. This is a single-operator Groth16 setup, not an independent
multiparty ceremony or audit. The release schedule and exact public artifact
hashes are in [`params/shield6_production.json`](../params/shield6_production.json).
Installation and current release validation are described in
[`SHIELD6_PRODUCTION_PARAMETERS.md`](SHIELD6_PRODUCTION_PARAMETERS.md).

## Historical validation record — 2026-11-05

Before the production setup, Shield6 was **not ready for production activation**.
Both shipped-network activation times and both production proof keys were unset. The
candidate implements recipient output proofs, key-control registration,
journaled registry state, versioned consensus/txpool enforcement, CLI sends,
and shared desktop/Android GUI integration. The current full proof-bearing
canonical payment and loopback TCP/reorg rehearsals have now passed locally
with genuine temporary test parameters. The latest resource-controlled results
below supersede earlier timeout reports and descriptions of missing adapters.

Independent cryptographic review, authenticated multiparty setup, representative
supported-hardware runtime/latency checks, and the final hosted release/network
matrix remain release gates. Current local payment and proof-bearing TCP
measurements are recorded below, separately from earlier experimental-envelope
results. No production activation or release was performed and no live daemon
executable was replaced.

## Transaction speed and reliability changes

- Payment and withdrawal construction now resolve all input rings with three
  ring-selection RPC calls total: status, a shuffled bounded candidate batch,
  and status again. Previously each input required these three calls. At the
  16-input maximum this removes 45 sequential round trips. At 100 ms per RPC,
  that removes approximately 4.5 seconds of network waiting; this is an
  estimate, not an end-to-end benchmark.
- Decoys are still independently sampled locally per ring. The request is the
  shuffled, deduplicated union of at most 256 indices; real-member markers and
  wallet-derived key images are never sent as queries. Ring grouping is not
  included in the lookup. Root, count, chain, activation, member encoding,
  cancellation, and entropy-failure checks remain enforced.
- CLI payment and withdrawal selection now consider all scanned notes,
  choosing the smallest sufficient single note or the largest notes needed
  to cover the amount within 16 inputs. This minimizes input count and avoids
  false insufficient-funds errors caused by selecting only the oldest notes.
  Selection preserves caller-owned note order and rejects malformed,
  duplicate, or mixed-chain notes. Uniform decoy selection remains the V1
  baseline and still requires privacy review before activation.
- Shield6 CI now triggers on CLI changes and runs the wallet construction,
  scanning, selection, and CLI migration regressions.

## Local verification

All commands used `GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20` on this shared
Linux/amd64 host. These checks passed:

```sh
go test -p=1 -tags shield3 ./zk/shielded6 ./core ./eth ./core/txpool/legacypool ./params -run 'Shield6|ShieldedV6|CLSAG|ConfidentialLedger|ActivationRemainsUnset' -count=1 -timeout=10m
go test -p=1 -tags shield3 ./internal/shield3wallet ./cmd/gtkm -run 'ShieldedV6|Shield6' -skip 'MigrationNativeRehearsal' -count=1 -timeout=5m
go test -race -p=1 -tags shield3 ./internal/shield3wallet -run 'ShieldedV6|Shield6' -skip 'MigrationNativeRehearsal' -count=1 -timeout=5m
go vet -p=1 -tags shield3 ./internal/shield3wallet ./cmd/gtkm
```

A signed two-input payment regression additionally verifies that one batch
produces an envelope with two inputs, a valid stateless proof, and the correct
outer sender. Batch regressions cover all 16 inputs, overlapping rings,
malformed and reordered responses, state changes, invalid inputs,
cancellation, and entropy failure.

The maximum-envelope benchmark (16 inputs, 16-member rings, 16 encrypted
outputs) measured **2.114 seconds** for one local build and self-verification:

```sh
go test -p=1 -tags shield3 ./core -run '^$' -bench '^BenchmarkShield6MaxPaymentEnvelope$' -benchtime=1x -count=1 -timeout=5m
```

This is one measurement, not a percentile or a before/after comparison. It
excludes scanning, RPC, outer PQ signing, network propagation, and block
inclusion. Full-chain scan cost still grows with chain history. The native
legacy migration rehearsal, hosted release matrix, complete repository test
suite, and live network rehearsal were not rerun in this pass. The changes
are local source changes, not a deployed production release.

## Follow-up: real TCP network rehearsal and release validation

`eth/shielded_v6_network_test.go` now exercises two actual encrypted loopback
TCP peers on isolated chain ID 18979 with independent databases, production
transaction pools, the eth protocol, downloader, and canonical block importer.
Proof of work is deliberately faked and the sender stamp is seeded in genesis;
this is not production mining, a multi-host deployment, a stamp-registration
rehearsal, or recipient-stamp proof validation.

The test checks pre-fork pool rejection, fork-boundary deposit import, real
pool rejection of a corrupted range proof, private-spend propagation,
canonical block synchronization, rollback of accumulator outputs, key-image
indices and account nonce on a longer fork, re-inclusion of the original signed
spend, identical final state roots, and rejection of a canonically spent replay.
The cleaned final test passed within the focused `./eth` suite (4.107 seconds
for that suite); the initial complete TCP rehearsal passed in 3.11 seconds.

The normal test-chain generator previously omitted the Antartical header
anchor, its system-contract state update, and Shield6's pre-EVM state hook.
These omissions produced invalid blocks that could not exercise the actual
importer. It now applies these transitions and generates importable blocks.
The focused core/generator, network/RPC, pool, and activation checks passed:

```sh
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -p=1 -tags shield3 \
  ./core ./eth ./core/txpool/legacypool ./params \
  -run 'Shield6|ShieldedV6|CLSAG|ConfidentialLedger|ActivationRemainsUnset|GenerateChain|ChainMaker|TKMBlockHashAnchor' \
  -count=1 -timeout=10m
```

The release workflow now supports `workflow_dispatch` with `validation_only`
(default **true**) and an optional `source_ref`. `release_tag` supplies the
package version label; use a `vMAJOR.MINOR.PATCH` value for the Android matrix.
One resolver pins all packages and validation jobs to the same source commit.
Every packaging job depends on Shield6 crypto, consensus, real TCP reorg,
wallet/CLI, native migration, and wallet race checks. A source override is
rejected when publishing. Validation packages are retained as workflow
artifacts with checksums, `SOURCE_COMMIT`, and `VALIDATION_ONLY` provenance;
the GitHub release publishing step is skipped.

Validation-only Android builds use a temporary certificate labelled
"Validation Only / Do Not Distribute" and do not receive production signing
secrets. Publishing retains the existing production signing requirements.
Workflow lint passed with cached actionlint 1.7.12. Invocation guards and the
actual temporary key-generation/signing-configuration script were executed
successfully; temporary key material was removed. The full hosted packaging
matrix has **not** been executed for this worktree. Configuring a workflow is
not evidence that its builds or tests have passed remotely.

Local stripped production daemon builds with `randomx,cgo,shield3` passed for
Linux/amd64, Linux/arm64, and Windows/amd64 before the test-generator follow-up.
The Linux/amd64 artifact ran its version command. ELF/PE architecture and
runtime imports were checked; the Windows artifact imports system DLLs only.
ARM64 and Windows were cross-linked, not executed on their target operating
systems. These checks do not cover final hosted packages, macOS, Android,
browser interoperability, or receiver-stamp cryptography.

### Recipient-proof implementation remains blocked by a missing relation

The actual V1 `outputTweak` operation in `zk/shielded6/wallet_output.go` is
HKDF-SHA-512 with a 16-byte big-endian chain/position salt, a versioned domain
and a 32-bit rejection counter. It accepts the first canonical nonzero scalar.
The abbreviated `H(shared, chain, position)` in the design document therefore
requires proving HKDF, canonical scalar decoding, rejection of every earlier
counter, the ECDH multiplication, and both output-key equations. A host-side
check of those computations does not make them constraints in a proof.

The existing fixed native stamp-ownership program proves Tip5 owner-preimage
knowledge. The existing private-spend program proves Tip5 stamp membership for
Shield3 owners. Neither program constrains Edwards25519 keys, ECDH, or the V1
HKDF operation, and existing stamp leaves contain no commitment to Shield6
spend/view keys. Adding a wallet lookup, reusing an owner proof, or attaching
a signature would leave the required relation unproved.

No new recipient-binding proof backend, versioned stamp-key registration,
verifying-key/program hash, envelope format, or GUI/Android send flow was
implemented in this follow-up. Those remain required design, implementation,
cryptographic review, interoperability, and performance work. No activation
value was configured and no production release was published.

The final TCP rehearsal also passed under the Go race detector in **28.094
seconds**, and `go vet -p=1 -tags shield3 ./core ./eth` passed. The final
release/crypto workflow actionlint check and `git diff --check` passed.

```sh
GOMAXPROCS=1 GOMEMLIMIT=700MiB GOGC=20 go test -race -p=1 -tags shield3 \
  ./eth -run '^TestShield6TCPForkSpendAndReorg$' -count=1 -timeout=5m
```

## Implemented recipient-binding constraint relation

The new `zk/shielded6/recipient` package supersedes the earlier statement in
this report that no constrained relation/proof adapter existed. Its README
specifies the exact statement, witness, leaf encoding, bounded V1-compatible
output derivation and the remaining consensus/setup boundary.

The circuit privately verifies the existing depth-32 Tip5 stamp membership
and an independent new depth-32 key-binding membership with the **same hidden
owner**. It constrains canonical recipient Edwards25519 points, curve and
prime-subgroup membership, `R=rG`, `shared=rV`, exact HKDF-SHA-512 counter-zero
computation, and `P=B+tG`. The complete transaction/output context is bound by
a constrained SHA-256 statement digest. The public witness has only 32 digest
bytes; owner, recipient keys, auxiliary subgroup witnesses and paths remain
private. No recipient spending/view secret is required from the sender.

`CreateOutput` resamples sender entropy until the first V1 HKDF candidate is
canonical/nonzero, within a fixed bound and with explicit entropy errors.
The existing Shield6 note scanner decrypts these outputs unchanged.
`CreateOutputWithEphemeral` exposes the exact entropy witness to the proof
builder; it must remain private, fresh per output and cleared after use.

The proof adapter uses externally supplied Groth16/BN254 parameters and a
locally pinned verification key. It never sets up a network key automatically.
Its parser checks canonical compressed encoding, bounded count prefixes,
exact commitment count against the pinned key, and trailing bytes before
calling the underlying verifier. gnark's internal committed inputs are
accounted separately from the 32 external digest bytes.

The optimized complete R1CS has **2,344,948 constraints** and the locally
observed fingerprint:

```text
37f9d92029b781f075726d1da97114a754e282960dffe1ec58e685ef905fb0f2
```

This is a candidate constraint-system fingerprint, not an activation value or
a production verification-key pin. It matched two independent local compiles.
An earlier, larger draft setup was interrupted and is not a proof-test pass.

Native Tip5 interoperability, SHA-512/HMAC vectors, scalar bounds,
identity/torsion/off-curve rejection, fixed/variable multiplication windows,
existing-wallet scanning, entropy failure, public-witness schema, allocation
prefix guards, and a small real-proof verifier-metadata check passed.
All **22** full-relation witness/context substitution cases also passed on the
optimized circuit (221.43 seconds total on the shared host). These include
malicious auxiliary subgroup preimages as well as altered owners, paths,
keys, roots, commitments, ciphertext hash, chain, position, anchor and intent.

The full-relation **real Groth16 proof roundtrip passed** with ephemeral,
unsaved test-only parameters. It first checked a valid compiled witness and
four malicious compiled witnesses, then generated and verified two randomized
proofs. All 12 public-context replay attempts, malformed/trailing/oversized
proofs and a wrong verification-key pin were rejected. The first proof was
**196 bytes** and took **83.647 seconds** to generate on the shared host
(`GOMAXPROCS=3`, `GOMEMLIMIT=3500MiB`). One-time test setup took **25 minutes
14.486 seconds**; compilation took **44.282 seconds**. The full test passed in
**1,740.151 seconds**. An earlier 25-minute setup timeout was inconclusive;
the completed run used a 90-minute test limit.

This proving time does **not** meet the fast-transaction goal. It is one local
measurement under concurrent node load, not an isolated hardware benchmark.
No independent full-relation verification timing was captured in this run;
the test now logs that timing separately for subsequent validation. Proof
generation needs further performance work before a production speed claim.
The focused proof-parser/verifier race check also passed (5.396 seconds), as
did existing Shield6 cryptography, wallet/CLI regressions, vet, formatting
and workflow validation.

The 2.11-second maximum transaction-envelope measurement earlier in this
report does **not** include this recipient proof. Compile and setup are
one-time preparation costs; actual proving and verification costs must be
measured separately. No speed claim for proof-bearing production transactions
is established by the existing envelope benchmark.

The new dedicated CI workflow runs the relation tests, real proof roundtrip,
existing Shield6 crypto regressions and vet on an isolated runner. Release
validation also explicitly tests the new package. Workflow configuration is
local source work, not evidence that the hosted runs have executed.

Versioned key-binding registration, canonical new-registry state/paths and
reorg handling, all-output proof requirements in a new envelope, gas pricing,
setup provenance, authenticated production key pinning, and wallet UI
integration remain required. No production setup material was generated,
no new consensus acceptance path was enabled, and activation remains unset.


## Integrated recipient authorization candidate (latest worktree)

The new protocol uses envelope 7 for proof-bearing transactions and envelope
8 for recipient key registration, gated by `Shield6RecipientTime`. After that
fork, version 6 is rejected. Missing per-output proofs, unknown/reorganized
registry roots, invalid anchors and unpinned verification keys fail closed.
The root and proof context is per output so payment and change can use
independent canonical historical registry snapshots without re-proving after
every append. The registry is append-only, owner bindings are immutable and
all nodes/lookups/root markers use journaled state.

Registration has a separate constrained key-control proof. It proves
knowledge of both hidden spend/view scalars and binds their key commitment to
the stamped account owner, chain and signed transaction intent. Consensus
matches that owner to canonical account state before insertion. The registry
leaf additionally wraps the owner and commitment; an unchecked opaque
commitment cannot authorize someone else's stamp or substitute for key control.
Both verification keys have separately configured SHA-256 pins, as do the two
proving keys. This metadata becomes immutable at the recipient fork. No
production key material is provided here. CLI and GUI loaders check the
proving-key and verification-key hashes against chain metadata before
artifact decoding; a locally supplied manifest alone cannot authorize them.

The optimized output circuit uses constrained integer Goldilocks reductions
inside BN254. Native/emulated Tip5 comparisons and malicious quotient,
remainder, limb and byte-hint overrides passed. Pair-hash cost dropped from
23,332 to 13,754 constraints. The complete output circuit dropped from
2,373,927 to **1,870,422 constraints**, crossing the Groth16 domain boundary
from 2^22 to 2^21. Its new candidate fingerprint is:

```text
a3426a12b2e63f581f4d792662d7dd2d1e3d6d1fa8b5b6dbca174d2110d2a50b
```

The key-control registration circuit has **367,322 constraints** and fingerprint:

```text
0040720811ae44399e005fe617b7f6673fea63c458fdaf55eacf5b410eaa53c3
```

These fingerprints identify candidate constraints, not ceremony approval or
production verification-key pins. Current circuit compilation accepted the
valid output witness and rejected the compiled owner/scalar/context changes.
Its first fingerprint-check run deliberately failed against the previous pin;
the pin was then updated to the measured new circuit. The final matching
compiled-relation run **passed in 90.562 seconds**, including valid and hostile
compiled witnesses. The current primitive/constraint/verifier test partition
also **passed in 406.318 seconds**, including all 22 output substitutions,
registration secret/context attacks and malicious reduction/byte hints.

The revised emulated output relation's real-proof run was killed during setup
on the shared node/miner host after 4,791 seconds. The subsequent optimized
canonical integration run hit its 100-minute timeout in Groth16 G1 setup,
before proving. **Neither is a pass or a current speed measurement.** The
successful 196-byte / 83.647-second proof earlier in this report belongs to the
older circuit and does not validate the current circuit or integration.
Setup and send-time proving remain separate costs. No production speed claim
is made for the current proof-bearing path.

Prepared-prover loaders authenticate and reuse local circuit/proving/key files
across sends, bound file sizes and proof concurrency, and check cancellation
before/after backend work. Wallets recheck canonical recipient roots after
proving, before signing. Versioned private payment codes contain paths for the
payer and no recipient secret. Payers do not query recipient owner associations
through public RPC.

CLI registration/send/deposit/withdrawal/change and shared desktop/Android GUI
routes are implemented. The GUI preserves authenticated loopback boundaries,
saves exact signed public transactions before broadcasting, and retries by
stable request ID after lost replies. Status polling carries no seed. The
shared engine's **23 tests passed**, including exact retry identity and
seed-free status requests, and its production bundle rebuilt successfully.
Current focused core registry/config and GUI boundary/persistence tests also
passed. Later validation results must be recorded separately as they complete.

The ceremony tool now supports independent output and registration circuits,
streamed artifact I/O, candidate fingerprints, manifests and full artifact
roundtrip checks. No ceremony was conducted, no production key was generated
and no setup material was installed into a node. Dedicated and release CI now
require a real proof-bearing canonical registration/deposit/payment/change
rehearsal on an isolated runner. The workflows have not been executed for this
uncommitted source tree.


The one-time registration **real proof test passed** on the current key-control
circuit: temporary compile/setup took 9 minutes 13.305 seconds; proving took
33.007 seconds and produced a 196-byte proof. Changed owner, chain, commitment
and intent were rejected. Total test time was 586.805 seconds. This is not a
payment speed measurement and these unsaved keys are not production material.

The current focused core, config, RPC, wallet, GUI, CLI and ceremony compilation
partition passed after key-control integration. Additional bounds now reject
oversized RLP lists before allocation and cap Shield6 payloads at 256 KiB;
a maximum-configured-fields decode test ensures legitimate field bounds fit.
New envelopes expire within one hour of canonical header time, with wallet,
canonical execution and txpool pruning checks. The GUI releases retired
requests only after canonical inclusion, nonce consumption or expiry and scans
durable history with bounded memory.

### Final source validation

The final focused regression run passed for core (16.646 s), params (0.011 s),
RPC/network (7.187 s), wallet (3.224 s), GUI (0.334 s), CLI (0.320 s), and
legacy pool (0.256 s); the ceremony command compiled. This includes rejection
of an unapproved local proving-key manifest before artifact decoding.
Verifier encoding/allocation tests passed under the race detector (9.331 s).
Static analysis passed for the changed protocol, wallet, GUI, RPC and command
packages. The dependency check found no forbidden dependencies; `go.mod` and
`go.sum` are unchanged.

All 23 shared wallet-engine tests passed again, and the production bundle
rebuilt. The standalone browser scripts passed syntax checks. All Android
Java sources compiled with Java 17 against Android API 34. The Android app now
imports the eight public parameter files through the system folder picker,
with streaming size/storage bounds, SHA-256 checks and staged replacement.
The native loader still authenticates both key hashes against chain metadata.
Java compilation is not an APK/device proof or performance test.

Full payment proving and proof-bearing TCP network validation remain open.
The passing TCP test is still the earlier experimental envelope rehearsal.
The new isolated full-proof CI gate has source coverage but no hosted result
for this uncommitted worktree. No production setup, independent audit,
activation, daemon replacement or release publication was performed.

The real-proof canonical rehearsal now optionally exports its public state,
verification keys and exact signed payment only after successful validation.
An exact-root snapshot check catches missing account or storage preimages.
The new TCP rehearsal consumes that fixture, rejects four signed proof/context
substitutions in the real pool, propagates the genuine payment between two
peers, imports it, rolls it back on a longer branch and re-includes the exact
same signed bytes. Both release workflows require this handoff without
repeating setup. The fixture contains no proving key or private witness; it is
ephemeral test data, not a production provenance record. This new real-proof
TCP test remains unexecuted because its upstream full-proof setup did not
complete here.

The handoff also exposed an activation-order issue: placing the independent
recipient fork before the confidential-ledger timestamp in the generic
chronology list rejected an already-active confidential ledger. Recipient
activation now uses its explicit Shield6 prerequisite and pinned key checks,
with regression coverage for the staggered schedule. Existing fork ordering
is unchanged.

After that correction, focused core snapshot/recipient checks passed (1.063 s),
params passed (0.031 s), and RPC plus the existing TCP rehearsal passed
(8.202 s). The new proof-bearing TCP test compiled and skipped without its
required real-proof fixture; that skip is not a validation pass. Static
analysis and workflow lint passed again after the handoff changes.
Wallet (3.895 s), GUI (0.600 s), CLI (0.753 s), and pool (0.303 s)
regressions also passed after the fork-order correction, and the dependency
check again reported no forbidden dependencies.

The final `make all` passed with `GOBIN=/tmp/shield6-build-validation`, including
the RandomX/cgo/native-Shield3 node build and all nine standard commands.
The ceremony command and keeper `example` build passed separately against the
same final source. Outputs stayed in that isolated temporary directory; no
running executable was overwritten. Final `gofmt`/`goimports`, browser syntax,
workflow lint and `git diff --check` checks passed. This is local build evidence,
not the unrun cross-platform release/runtime matrix or full repository suite.

## Current full payment setup and TCP verification — completed

The earlier local setup timeout is resolved. The current, unchanged output
circuit completed genuine `groth16.Setup` in **14m46.185s**; its registration
circuit completed setup in **3m18.911s**. Output compilation took 25.534s and
registration compilation 8.936s. These are temporary test parameters, not a
production ceremony. Both candidate R1CS fingerprints remain unchanged.

The successful resource-controlled run used `GOMAXPROCS=3`, a 300% CPU quota,
and a 3 GiB Go heap budget, with a 3500 MiB resident-memory cap and 1536 MiB swap cap,
and paused the competing standalone XMRig process. The node remained running.
Recovery was armed before each miner pause, the original miner executable,
arguments, environment and configuration were restored afterward, and recovery
timers were disarmed after restoration was observed. Source/build changes did
not replace the live node executable or alter its chain configuration.

The first full canonical rehearsal passed in 1323.28s including both setups,
registration, deposit, payment/change, four signed substitution attacks,
receiver scans and rollback/replay. Its deposit construction took 50.290s and
payment plus change 92.039s. Its initial TCP handoff correctly failed against
mandatory mainnet checkpoints: the synthetic fixture had reused chain ID 8979.
The producer now uses isolated chain **18979**, the reader rejects other chain
IDs, and the TCP test asserts that production checkpoint challenges are absent.
Production checkpoint enforcement was not changed. The chain-bound witnesses
and signed payment were regenerated using the same genuine setup.

Parameter reload exposed a separate startup bottleneck. For an SRS whose full
file hash has already been authenticated against release/chain metadata, the
loader now uses gnark's documented trusted-source proving-key decoder. The
complete pre-decode hash, decode-time hash, byte/size bounds and trailing-data
checks remain enforced. Received transaction proofs and verification keys
retain full point/subgroup validation. Unauthenticated files are rejected
before the proving-key decoder is called. Regression tests cover wrong pins,
file changes during decoding and incompletely consumed artifacts.

On this host, output-parameter loading fell from **321.004s to 186.405s** and
registration-parameter loading from **65.555s to 39.074s**. Loading remains a
substantial startup cost. Prepared parameters are retained across sends;
compilation/setup are not on the send path. The separately named, opt-in test
cache permits recovery without repeating setup and cannot be mistaken for a
reviewed production wallet manifest by its filename/domain.

The final isolated-chain canonical rehearsal **passed in 374.40s**, including
loading both cached setups. With parameters loaded:

| Measured operation | Local elapsed time |
| --- | ---: |
| One-output proof-bearing deposit construction | 42.732s |
| One-input, 16-member-ring payment plus confidential change | 84.243s |
| Source-pool validation of that genuine payment | 42.903ms |
| Second-pool TCP propagation, including source validation | 603.843ms |
| Block construction, canonical import and second-peer sync | 197.368ms |
| Longer-branch construction, reorg and second-peer sync | 22.963ms |

These are individual local measurements on the shared Linux/amd64 four-vCPU,
8 GB host with the standalone miner paused, not production latency percentiles.
Construction excludes wallet-history scanning, external RPC and outer account
signing. TCP uses two real encrypted loopback peers, independent databases,
production pools, downloader and importer. Proof of work is faked, so the block
sync measurement does not predict live mining or confirmation time. A payment
with change currently takes about **84 seconds to construct** on this profile;
it is not an instant-send result. Local proving dominates propagation.

The proof-bearing TCP rehearsal **passed in 1.17s**, including four signed
substitution rejections, propagation, matching canonical state, longer-fork
rollback, exact signed-transaction re-inclusion and spent-replay rejection.
An earlier immediate probe of the same valid fixture also passed in 1.73s
while the artifact check was running. The independent saved-artifact check
**passed in 228.95s**: it loaded both actual circuit/proving/verifying-key sets,
matched their pins to the canonical rehearsal and verified both payment proofs
through the loaded verifier. This uses genuine proofs, not a mock verifier or
dummy setup.

Logs and public test artifacts are retained in the private local directory
`/tmp/shield6-full-validation-20261105/`: `payment.log` records initial setup;
`payment-final.log`, `cache-final.log` and `tcp-final.log` record the final
passes; `result-final.json` records both validation success and miner restoration.
The test-only proving files are not production material. Hosted release jobs,
cross-platform/device runtime qualification, independent review and production
multiparty setup remain open. Activation stays unset on shipped networks.

Post-change focused regressions passed: core 10.803s, params 0.012s, eth 4.830s,
wallet 2.146s, GUI 0.235s and CLI 0.338s. The pinned-artifact authentication and
proof-decoder regression partition passed in 1.116s. `go vet` passed for the
recipient package, core, eth and the shared fixture package. Changed Go files,
workflow lint and `git diff --check` passed; dependency files are unchanged.
The private miner environment/restart snapshot was removed after restoration
was verified. The live node continued to respond and its head advanced after
mining resumed.

## Exact production-parameter verification — 2026-11-06

The fresh single-operator setup completed in 997.52 seconds for output proofs
and 231.633 seconds for registration. These parameters, rather than the retired
ephemeral test setup, match the mainnet verification and proving-key pins.
All ten published HTTPS artifacts were checked byte-for-byte by size and SHA-256.

The canonical production-key rehearsal passed in 623.32 seconds. Deposit
construction took 82.734 seconds; payment plus private change took 149.225
seconds while other repository checks were running. These are single shared-host
measurements, not unloaded-hardware percentiles. Key loading is included in the
full rehearsal duration and is additional to the reported construction times.

The same signed proof-bearing payment passed real encrypted TCP propagation,
canonical import, rollback and exact replay on two peers: source-pool validation
42.472 ms; propagation including source validation 605.029 ms; block construction,
import and second-peer sync 214.631 ms; longer-branch reorg and sync 27.341 ms.
The TCP test passed in 1.40 seconds. Both rehearsals use isolated chain 18979,
fake proof of work and genesis-seeded stamps; they do not submit a live mainnet
payment or qualify all supported devices.

Full local repository tests, lint, generated-code and module-hygiene checks are
release prerequisites. The execution-spec downloader now retains every case in
the three fixture directories used by the Go suite, excluding unused upstream
engine/sync data to keep validation within this host's disk capacity.
The repository already skips Ethereum execution-spec block/state/transaction
vectors as inapplicable to TKM. A passing full command does not qualify those
Ethereum vectors or remove that existing coverage limitation.
Independent review, an independent ceremony and representative device runtime
qualification have not been performed. The exact scheduled timestamp is in
[`params/shield6_production.json`](../params/shield6_production.json); all
operators must upgrade before that boundary. Hosted build results must be
checked separately from these local verification results.
