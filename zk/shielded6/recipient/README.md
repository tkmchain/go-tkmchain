# Shield6 recipient authorization candidate

This package implements constrained recipient membership, output derivation,
private key-control registration, native statements, private payment codes and
bounded Groth16 APIs. Core, wallet, CLI and shared desktop/Android GUI adapters
are implemented. Fresh production parameters are pinned in
[`params/shield6_production.go`](../../../params/shield6_production.go), with
artifact hashes and the activation schedule in
[`params/shield6_production.json`](../../../params/shield6_production.json).
This is a single-operator setup, not an independent multiparty ceremony or
cryptographic audit. Installation and release validation are documented in
[`SHIELD6_PRODUCTION_PARAMETERS.md`](../../../docs/SHIELD6_PRODUCTION_PARAMETERS.md).
Hosted release and supported-device qualification remain separately verifiable
requirements; earlier test-only parameters are not production artifacts.

## Relations and authorization

The output relation has exactly 32 public inputs: the bytes of SHA-256 of the
caller-authenticated statement. Owner, static keys, membership indices/paths,
auxiliary curve witnesses and sender entropy remain private. No recipient
secret is needed to construct a payment.

The constrained computations for every confidential output are:

1. The unchanged V1 stamped-owner leaf is
   `Tip5_varlen(3004, chain_low_u32, chain_high_u32, owner[5])`.
   A private ordered depth-32 path authenticates its stamp root.
2. The private key commitment is
   `Tip5_varlen(6006, 2, chain_low_u32, chain_high_u32, owner[5], spend_u32_le[8], view_u32_le[8])`.
   Its authorized registry leaf is
   `Tip5_varlen(6007, 2, chain_low_u32, chain_high_u32, owner[5], commitment[5])`.
   A second private depth-32 path authenticates the binding root, using the
   **same owner**. Existing immutable stamp records are not reinterpreted.
3. Recipient points have canonical coordinates, lie on Edwards25519, are
   nonidentity and belong to its prime-order subgroup. A private on-curve
   preimage `Q` with `[8]Q=P` proves subgroup membership: the group order is
   `8*l`, hence `[l]P=[8*l]Q=0`. Nonidentity excludes order one. The parameters
   are specified in [RFC 8032 §5.1](https://www.rfc-editor.org/rfc/rfc8032.html#section-5.1).
4. The sender scalar is canonical, nonzero and below `l`. The circuit proves
   `R=rG` and `shared=rV`.
5. Exact V1 HKDF-SHA-512, **counter zero**, derives the canonical nonzero tweak
   `t`. Salt is `chain_u64_be || position_u64_be`; info is
   `TKM_SHIELD6_OUTPUT_TWEAK_V1/ || 0_u32_be`. The circuit proves `P=B+tG` and
   rejects identity. `CreateOutput` resamples fresh entropy within a bound
   until the first candidate works. Existing receiver scanning is unchanged.
6. A constrained SHA-256 statement digest binds version, chain, native TKM
   asset, output position, both roots, registry anchor, transaction intent,
   output keys, commitment/limbs and ciphertext hash. The native verifier
   computes that hash from the actual encrypted-note bytes, never an RPC
   prehash or transaction-supplied statement digest.

Registration has a separate constrained relation and verification key. It
proves knowledge of **both** canonical nonzero spend/view scalars, derives
both public points, recomputes the private key commitment and binds it to the
chain, stamp owner and registration transaction intent. Core matches that
owner to the canonical stamp of the signed account before inserting the leaf.
An opaque commitment alone, an account signature alone or a wallet check does
not satisfy this key-control requirement. Keys remain hidden on-chain.

## Integer arithmetic and performance

Tip5 is bit-for-bit compatible with the pinned twenty-first 1.1.0 reference,
including its Montgomery-byte S-box, MDS and fixed/variable domains. Optimized
reductions constrain an integer quotient/remainder equation inside BN254.
Quotients have at most 128 bits, remainders have two bounded 32-bit limbs, and
the exact Goldilocks modulus bound is enforced. Every intermediate integer is
below 2^193, so native-field equality cannot wrap. S-box byte hints are bound
by exact reconstruction and membership in the fixed 256-entry lookup table.
No unchecked hint replaces a hash, range bound or curve equation.

Tests compare the implementation against native Tip5 and the previous
emulated permutation, and override reduction/byte hints maliciously. The
optimized output circuit has 1,870,422 constraints, down from 2,373,927 in the
initial authorized-registry candidate. This crosses from a 2^22 to a 2^21
Groth16 domain. This is **not** an end-to-end speed measurement. The earlier
83.6-second proof measurement belongs to an older circuit. Initial shared-host
setup attempts timed out. With a controlled CPU/memory allocation and the
standalone miner paused, current output setup completed in 14m46s, and the
current payment plus change was constructed in 84.24s. These are local
measurements, not production latency percentiles or block-confirmation times.

`PreparedProver` authenticates circuit, proving key and verification key from
locally installed release pins and retains them across sends. It does not
compile or generate setup on the send path. Proof work is serialized per
loaded prover to bound memory. Cancellation is checked before and after the
backend operation; it does not promise mid-proof interruption.

The proving-key file is hashed before decoding and again during decoding.
Under the authenticated release-artifact contract, its decoder uses gnark's
trusted-source API instead of repeating subgroup checks on millions of SRS
points at every start. This applies only to the pinned proving key. Received
proofs and verification keys retain full validation. The measured output-key
load fell from 5m21s to 3m06s on this host; it remains a substantial startup
cost, separate from setup and per-payment proving.

Current candidate R1CS SHA-256 fingerprints:

- Output: `a3426a12b2e63f581f4d792662d7dd2d1e3d6d1fa8b5b6dbca174d2110d2a50b`
- Registration: `0040720811ae44399e005fe617b7f6673fea63c458fdaf55eacf5b410eaa53c3`

These fingerprints are not production verification-key pins or setup approval.

## Consensus and clients

`Shield6RecipientTime` gates the new protocol independently of the historical
experimental V6 path. After that fork, envelope version 7 requires exactly one
proof and authenticated registry-root pair for every confidential output;
version 8 handles key-control registration. Version 6 is rejected. Production
planning must enable recipient authorization together with Shield6, never
activate the proofless experimental path on a public network.

Both verification keys, their SHA-256 pins and the separate proving-key
SHA-256 pins live in chain configuration. Missing, malformed or mismatched
keys fail closed. Active key metadata cannot be changed without a
configuration rewind. No shipped network supplies these keys or an
activation timestamp.

Registry nodes, owner lookup, count, commitment, current root and known-root
markers use the normal state journal. Reorgs remove their writes. Historical
canonical roots remain usable during slow proof construction; output and
change may use different snapshots. The registry anchor is Keccak-256 of the
versioned domain, chain and both roots; consensus requires both known-root
markers and the exact anchor. Rings and key images are separately resolved
against canonical Shield6 state. Gas prices verification, proof bytes and
worst-case registry/output state writes.

Intent includes actual fields and root references and excludes recipient and
registration proof bytes to avoid circular dependencies. The final PQ account
signature authenticates the complete encoded transaction. Txpool admission,
canonical processing and reorg rechecks enforce their corresponding checks.

A `tkmshield6r.` payment code carries the receiver's public keys and private
membership witnesses for the payer. It contains no spend/view secret. Senders
do not query recipient-owner associations through public RPC. Wallet builders
require local parameters matching the chain pin and recheck canonical roots
after proving, before signing. All confidential change uses the same rule.
Full public withdrawals use the new envelope with no output proofs.

New envelopes carry an intent-bound expiry at most one hour after canonical
header time. Wallets check it after proving; consensus and pool head pruning
reject expired transactions. The GUI retains uncertain requests until inclusion,
nonce consumption or canonical expiry, then permits a fresh request. A deep
reorg can revert canonical inclusion/time, as with other chain confirmations.
Recipient anonymity depends on the population and selected registry snapshots;
early roots with few bindings provide a small anonymity set. Zero knowledge
alone does not hide information already implied by that public set.

The CLI exposes recipient-key registration, payment codes, payments, deposits,
withdrawals and change. The shared desktop/Android GUI uses authenticated
loopback HTTP, local proving and exact signed-transaction persistence before
broadcast. A stable request ID retries the same bytes after a lost reply;
status requests contain no seed. Private witnesses never enter public RPC.

## Setup and release artifacts

Two independently reviewed circuit-specific multiparty setups are required.
The extended ceremony tool accepts either `shield6-recipient-v2` or
`shield6-registration-v2`, as well as the legacy default:

```sh
go run ./cmd/shielded-ceremony -circuit shield6-recipient-v2 compile -out recipient.r1cs
# Run the existing init/contribute/verify phase1 and phase2 commands with the
# same -circuit value, independently contributed transcripts and final beacon.
go run ./cmd/shielded-ceremony -circuit shield6-recipient-v2 finalize \
  -commons commons.bin -beacon 0x... -pk recipient.pk -vk recipient.vk \
  -vk-hex recipient.hex -manifest manifest.json phase2-1.bin phase2-2.bin
# Repeat independently for shield6-registration-v2 with registration.* outputs
# and registration-manifest.json.
```

Do not treat a local setup or a self-calculated key hash as production
provenance. Authenticate the manifests through the reviewed release. Install
`recipient.r1cs`, `recipient.pk`, `recipient.vk`, `manifest.json`, and the
corresponding `registration.*` / `registration-manifest.json` locally.
Set `TKM_SHIELD6_RECIPIENT_PARAMETERS` to that directory. Android's local
parameter directory is `<node-data-directory>/shield6-recipient`. The app's
**Import Shield6 wallet files** button imports all eight files from a selected
release folder, checks bounded sizes and manifest hashes, and replaces the
managed directory after validation. Restart the node after replacing files.
This file-integrity check does not establish ceremony provenance.

Manifests encode `version`, `r1csSHA256`, `provingKeySHA256`, and
`verifyingKeySHA256` as generated by the ceremony tool. The local loader checks
all pins, fixed candidate circuit fingerprints, bounded regular files and
trailing bytes. CLI and GUI adapters compare both proving-key and
verification-key hashes against the chain's authenticated configuration
before decoding artifacts; a self-selected manifest cannot authorize a key.
Consensus trusts only the separately configured verification-key pins.
`groth16.Setup` is used only for temporary test keys; no production material
has been generated here. An opt-in integration-test cache uses separate
`test-*` filenames and a `NOT_FOR_PRODUCTION` manifest domain so a later
rehearsal failure does not require another expensive setup. It is not a
reviewed wallet parameter release.

## Validation

```sh
./scripts/shield4-build.sh
GOMAXPROCS=2 GOMEMLIMIT=4GiB go test -short -p=1 -tags shield3 \
  ./zk/shielded6/recipient -count=1 -timeout=20m
# Use an isolated runner, not a host running a node and miner:
# Choose an unused fixture path; an existing file is rejected.
GOMAXPROCS=2 GOMEMLIMIT=4GiB SHIELD6_RECIPIENT_INTEGRATION=1 \
  SHIELD6_RECIPIENT_TEST_PARAMETERS=/tmp/shield6-test-parameters \
  SHIELD6_RECIPIENT_NETWORK_FIXTURE=/tmp/shield6-recipient-network.json \
  go test -p=1 -tags shield3 ./core -run '^TestShield6RecipientEndToEnd$' \
  -count=1 -timeout=100m -v
SHIELD6_RECIPIENT_NETWORK_FIXTURE=/tmp/shield6-recipient-network.json \
  go test -p=1 -tags shield3 ./eth -run '^TestShield6RecipientTCPSpendAndReorg$' \
  -count=1 -timeout=10m -v
```

The real integration gate covers key-control registration, canonical deposit,
payment/change, receiver scans, proof/nonce/root tampering, two state replicas
and rollback/replay, with no mock proof verifier. Only after those checks pass,
the optional handoff writes public test state and the exact signed payment;
it contains no proving key or witness and refuses to overwrite an old file.
State export reconstructs the exact trie root before writing the fixture.
The TCP gate seeds that validated state into two isolated nodes and checks
signed proof substitutions, real pool propagation, canonical import, reorg
and exact replay. Native stamp enrollment and proof of work are separate gates.
The fixture uses isolated chain ID 18979 and rejects production chain IDs;
using mainnet's ID would correctly activate mandatory mainnet checkpoint
challenges against its synthetic genesis. Dedicated/release workflows require
both tests on an isolated runner. Both passed locally on the current circuit;
workflow source is not evidence of a hosted pass. Exact local outcomes and
unresolved release gates are recorded in `docs/SHIELD6_PRODUCTION_STATUS.md`.
