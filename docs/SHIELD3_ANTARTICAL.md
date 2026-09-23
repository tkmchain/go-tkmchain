# Antartical implementation guide

## Activation and scope

Mainnet Antartical activates on **October 1, 2026, 00:00 UTC**
(`1790812800`). Shield3, mandatory stamps, sponsored stamp registration and
shared-relay rules use this existing activation. Historical blocks retain their
historical rules. Update every validator, miner and wallet node before activation:
the native spending relation now supports four inputs and has a different fixed
program digest. Pre-activation experimental Shield3 proofs must be rebuilt.
Nodes without the embedded verifier reject Shield3; they cannot silently use V2.

This guide describes implemented code in `artartical`. It does not announce a
mainnet deployment. Pool, exchange and web-wallet integrations require separate
updates; this change integrates the desktop and Android embedded wallets.

## Consensus transaction model

Shield3 uses a canonical `TKMSHIELD3` RLP envelope inside a chain-bound
ML-DSA-87 PQ transaction sent to the reserved, code-free shielded pool. Consensus
checks the outer signer, registered stamps, known note and stamp roots, fixed
native proof, bounds, nullifiers and commitments before updating state.

A private spend consumes **one to four notes of the same hidden owner**. Each
opening proves its ordered depth-32 membership under one known anchor and its
correct domain-separated nullifier. Nullifiers must be nonzero and distinct;
inactive extra slots must be zero. Checked 256-bit addition rejects overflow.
The proof conserves the aggregate inputs against outputs, any public withdrawal
and private gas reserve. If any input is already spent, the entire spend fails:
no other nullifier, commitment, balance or root is consumed. Successful inputs
all record the same actual signed transaction hash.

There are always four output commitments. Each has incoming, outgoing and stamp
ciphertexts. Slot 3 is change and must have the proved input owner. The aggregate
of slots 0–2 plus public withdrawal cannot exceed **5,000,000 TKM**. Gas reserve
and change are excluded from this limit. Public deposits also have the 5 million
TKM limit, zero private inputs, and require the ordinary pool value transfer.
Encrypted padding hides output values, but input count is observable.

The public proof statement has 108 words; private openings have 137 words and
256 five-word path digests. Native digests are full 40-byte Tip5/Goldilocks values,
with owner/note/nullifier/stamp/nullifier-key hash domains 3001/3002/3003/3004/3005. No BN254
reduction or legacy commitment is accepted. The pinned Triton VM 8.0.0 verifier
accepts only the locally fixed program and security settings. Maximum padded
trace is 65,536 rows; proofs are capped at 8 MiB. Antartical's encoded block
limit is 8 MiB, with transaction admission reserving 64 KiB overhead. Native
verification costs 3 million gas plus one gas per proof byte; ordinary encrypted
envelope calldata retains its pricing. Wallets budget 7 million gas per send.

The SHA-512 proof intent binds the complete unsigned outer transaction and
proof-free envelope: chain, operator public key, nonce, gas, fee caps, target,
value, access list, all ciphertexts, commitments, input count/nullifiers, relay
mode and expiry. Altering any authorized payment or fee invalidates the proof.

## Encryption and keys

Each fixed 5,785-byte record uses **ML-KEM-1024**, **HKDF-SHA-512** and
**XChaCha20-Poly1305** with fresh encapsulation and nonce. The authenticated
header binds version, suite, purpose, chain and full hiding commitment context.
KEM implicit rejection is followed by mandatory AEAD authentication. Note
values, recipient information, private labels and decoys share fixed padding.

Independent chain-scoped purposes derive incoming (1), outgoing (2), stamp (3)
and selective-disclosure auditor (4) seeds. Publishing an encapsulation key
allows encryption, not decryption. Incoming/outgoing viewing keys disclose
wallet history but do not authorize spending. Stamp keys disclose labels only.
Auditor keys decrypt explicitly addressed disclosure capsules only.

STARKs avoid the inherited elliptic-curve proof system for V3. Triton defaults
claim 160-bit conjectured **classical** soundness; this is not 160-bit quantum
soundness. ML-KEM and ML-DSA address known quantum attacks on older public-key
systems. Neither these primitives nor this integration promises permanent
unhackability, anonymity after secret exposure, or immunity to future research.

## Carrot-inspired output privacy

Antartical Shield3 keeps its existing PQ envelope and STARK relation while
adding privacy ideas inspired by Carrot:

- every output receives a fresh 40-byte one-time output key (the canonical five-word Tip5 digest) derived from the
  hidden owner, note randomness and commitment; it is never reused;
- every new note carries a random 32-byte payment tag inside the authenticated
  incoming/outgoing ciphertexts; it is not exposed as a public payment ID;
- change already receives fresh randomness and therefore gets a new output key
  instead of reusing an input or recipient identifier;
- wallets can export incoming-only, outgoing-only, or full viewing keys; the
  outgoing tier can inspect outgoing records without receiving or spending funds;
- the complete proof-free envelope remains bound by the Shield3 intent digest,
  so output keys, ciphertexts, relay fields and payment metadata cannot be
  swapped after proof creation;
- existing multi-input spends, selective disclosure capsules and relay
  propagation continue to operate over this format.

These additions do not import Monero's FCMP++ or Ed25519 assumptions. They are
metadata and key-hierarchy extensions tied to the existing Shield3 commitment
and PQ proof system.

## Stamps and account creation

At Antartical, creating a PQ account in the desktop/Android UI requires creating
its private name/country stamp first. Existing accounts can add their original
stamp once. The encrypted record is authenticated by the account's PQ key.
Labels are self-declared, not verified legal identity. Name/country remain
private unless the holder supplies the separate stamp key.

`TKMSTAMP1` registers an immutable address, owner digest and hiding commitment.
An owner STARK proves knowledge of the owner preimage and binds the full
registration intent. Reusing an owner or replacing an address's registered stamp
is forbidden. Keep the original encrypted keyfile: recovery words alone do not
restore its randomly blinded stamp. Export/import and password changes preserve
that original record.

Every user transaction signer requires a confirmed consensus stamp; public
value recipients/withdrawal recipients must be stamped too. Shield3 output
owners prove membership in the separate depth-32 stamp registry. Registration
is the narrowly checked exception for unstamped beneficiaries. Protocol rewards
and pre-execution system calls remain separate. Removing local wallet checks
does not bypass enforcing validators: a block containing an illegal transaction
is rejected by nodes enforcing the consensus rule.

## Sponsored stamp registration

A new wallet need not receive public TKM to register. A previously stamped
sponsor pays gas using its own public balance and nonce. A one-hour offer binds
the sponsor key/nonce/fees/gas, beneficiary PQ key, encrypted stamp and expiry.
The beneficiary supplies both an owner STARK and ML-DSA authorization; the
sponsor then signs the complete transaction. Only sponsor balance/nonce change.
Expired, altered, already registered or reused-owner authorizations are invalid.

Wallet steps: open **Register with a sponsor**, create the stamp request, let the
sponsor create a fee offer, authorize it in the new wallet, return the JSON
packet, then let the sponsor review the verified beneficiary and maximum fee
before submitting. Wait for canonical stamp confirmation before sharing a
payment code or sending. There is no automatic unlimited sponsorship service.

## Stronger sender privacy with a shared relay

Direct Shield3 sends expose the payer's PQ signer account. A shared relay makes
the **operator** the outer signer instead. The private owner and stamp
membership stay inside the STARK; the payer's PQ public key and address do not
appear in the relay packet. The operator must itself have a confirmed stamp.

1. The operator creates a PQ-signed one-hour fee offer containing chain, its
   public key, pending nonce, 7 million gas budget, fee caps and expiry.
2. The payer verifies that signed offer, confirms the full reserved fee, and
   creates a private spend with its hidden owner secret and one to four notes.
3. The wallet saves the exact unsigned packet before exporting it. The operator
   receives public transaction/proof bytes, never spending or viewing seeds.
4. The operator independently checks its identity/nonce, expiry, known roots,
   unspent inputs, native proof and intrinsic gas, then explicitly approves and
   signs the exact transaction. Its signature produces the actual transaction
   hash; the unsigned draft hash is not a confirmation hash.
5. The payer checks the prepared payment. Matching nullifier status plus exact
   transaction comparison identifies the actual pending or confirmed hash.

Consensus permits relay mode only for private spends with **no public
withdrawal**, expiry at most one hour ahead, and gas reserve exactly
`gasLimit * gasFeeCap`. The private notes fund that entire reserve. It is
credited to the operator before gas purchase; the operator receives unused gas
refunds. This is the quoted payment to the operator, not a promise to charge only
actual gas. A publicly empty stamped operator can therefore submit a valid
privately funded packet. Payer public balance and nonce do not change.

Notes in exported drafts remain locally reserved until broadcast/spend or
consensus expiry. This survives restarts in the wallet state directory. Local
cancellation cannot revoke an authorization already given to the operator;
there is no early release button. Stable request IDs retain exact packet bytes.
Operator submissions likewise save exact signed bytes before broadcast, so an
RPC timeout cannot create another spend. Competing operator transactions stale
the quoted nonce and require a new offer. Txpool pruning removes expired relay
transactions when the canonical head advances.

In **Send** or **Receive**, open **Shared relay**, import an offer, enter the
recipient and amount, and choose **Prepare payment through relay**. Download and
send the payment JSON to the operator. The operator imports it, chooses
**Review as operator**, then **Confirm operator submission**. The payer uses
**Check prepared payment** after submission. The downloaded packet contains an
opaque request ID: import your own packet and select its original payer wallet
to check it even after losing browser storage. Use a genuinely shared operator:
a dedicated personal relay can be correlated with its user.

This hides the payer account on-chain; it does not hide the operator account,
fees, timestamps, input count, public deposit/withdrawal values or network
traffic. The operator may correlate IP addresses and packet timing. Nullifiers
now require the recipient's independent secret key: original senders and
payment-only auditors cannot derive them from a note opening. Automatic
submission is supported for a user-selected relay; automatic relay discovery
and network anonymity are not implemented.

## Recipient-secret nullifiers and scoped viewing

For spending secret `sk`, derive `owner = Tip5(3001 || sk)` and
`nk = Tip5(3005 || sk)`. A note commitment retains its original owner binding.
Its nullifier is now `Tip5(3003 || chain/asset || nk || randomness || value)`.
The fixed native relation computes `nk` from the same private `sk` used to open
`owner`. It never accepts a separate prover-chosen key. Consequently one note
has one nullifier, including across direct, sponsored, relayed and batch sends.
Neither the payment address, outgoing note opening nor selected payment
capsule contains the recipient's `nk`.

Wallet key exports are version 2 and specify a permission explicitly:

| Permission | Information exposed | Spending authority |
| --- | --- | --- |
| Incoming | Incoming receipts and total received; no later spend identifiers | None |
| Full | Incoming/outgoing history and the wallet's later spends via `nk` | None |
| Payment | One confirmed output recipient and amount | None |
| Stamp | One self-declared name/country record | None |

Receive-only scans return `spendStatusKnown: false`, `balanceWei: ""`, total
`receivedWei`, and no spendable notes. A cumulative received total is not a
balance. Full scans exclude spent/pending/reserved notes and expose a spendable
balance. Incoming keys reject embedded outgoing/nullifier permissions; full
keys require both. The Receive tab exports only the selected permission and
can scan an imported incoming/full key or open a stamp disclosure without
unlocking a spending wallet. Stamp-only exports carry the authenticated record
and its 32-byte record key, never the master stamp KEM seed; this key cannot
recognize or decrypt payment output stamps. Previously shared master stamp
seeds cannot be revoked and may still recognize those records. Stamp labels
are self-declared; signature verification is not an identity/country certification.
Re-export old full viewing backups: their encryption seeds still derive the
same keys, but an old viewing backup did not contain `nk`. Keep the encrypted
spending backup to regenerate it. Full viewing keys remain sensitive.

## Automatic shared relay

Build the native relay executable with `make shield3-relay` and run:

```sh
./build/bin/shield3-relay \
  --rpc http://127.0.0.1:8545 \
  --keystore /path/to/encrypted-stamped-pq-keystore.json \
  --password-file /path/to/operator-password \
  --state-dir /path/to/private-relay-state
```

The operator stamp must already be confirmed. The executable unlocks only its
operator key, never a payer key. Default listening address is `127.0.0.1:8790`;
serve it through a TLS reverse proxy for remote users. Restrict access to the
password and state directory. Operator endpoints accept wallet JSON POSTs,
reject browser Origin headers, and do not enable CORS. Apply request limits at
the proxy for a public deployment: one operator deliberately leases only one
nonce while a payer proves, and an abandoned quote holds it until expiry.
Use separate operator accounts to serve concurrent quotes.

Choose **Shared relay** in the Send tab, enter its HTTPS URL, review the signed
operator/fee/expiry estimate, then confirm. The wallet prepares and durably saves
one authorized draft before contacting the operator. It downloads a recovery
packet, submits automatically and checks confirmation against its own node.
**Check / retry saved relay payment** works without retaining a wallet password
and always submits the same saved draft. Quotes also use stable opaque request
IDs; a retry returns the same signed offer. Changing payments requires resolving
the saved operation first. A locally abandoned authorization cannot be revoked
before consensus expiry.

The service exposes `/offer` (`requestId`) and `/submit` (`requestId`, unsigned
`transaction`). Quotes are publicly requestable and signed by the operator. Payments require
the fixed spending proof and operator signature; opaque IDs are retry
identifiers, not spending keys. It verifies stamp, nonce, expiry, roots, all input statuses,
output reuse, the full native proof and intrinsic gas before signing. A process
lock protects the operator state directory. Before broadcast it syncs the exact
signed transaction to a nonce-specific record; a lost RPC reply or restart can
only rebroadcast those bytes. A durable global request index also prevents one
request ID from authorizing a second nonce after the daemon advances. Another
packet at a reserved nonce or request ID is rejected. Wallets
check the returned PQ signature and every authorized field; a server's claimed
confirmation is ignored. Confirmation/pending/conflict/expiry comes from the
wallet's own node, using all nullifiers and the exact transaction match.

## Multiple-recipient payments and payouts

A transaction pays one to three recipients in output slots 0–2; slot 3 always
belongs to the payer and carries change. The Send tab adds/removes recipient
rows and reviews each authenticated stamped address. Empty payment slots use
fresh zero-valued notes for the payer; all four outputs still have independent
incoming/outgoing/stamp encryption and native stamp-membership proofs.

`BuildBatch` and `BuildRelayedBatch` accept `[]Payment`, and local `send` /
`prepare-relay` operations accept `payments: [{recipient, amountWei}, ...]`.
Amounts use exact integer wei. The **sum of payments**, rather than each row,
cannot exceed 5,000,000 TKM; native consensus also enforces the combined limit.
Fees/change are excluded. A wrong-chain or unstamped recipient, invalid amount,
excess payment count or insufficient four-input total rejects the whole batch.
Payment disclosures select one real output without decrypting the others.
Payout integrations can chunk larger lists into batches of at most three,
tracking a stable request ID and canonical hash for each batch independently.
No external pool/exchange deployment is changed by this repository update.

## Proof work and size

The helper ABI remains bounded at 256 digests, with zero inactive paths. The
prover supplies only active note paths plus four stamp paths to the VM; deposits
supply no input paths. Shared note/nullifier/stamp hashing, fixed depth-32 Merkle
loops and fixed range-check loops reduce the program's hashing table. All
ownership, range, conservation, limit, membership and uniqueness checks remain;
STARK security parameters and maximum proof/trace bounds are unchanged.
The deterministic single-input regression now measures 32,768 padded rows
(previously 65,536), with a Cascade table of 31,520 rows. These are fixture
measurements, not universal latency or phone-memory guarantees. Resource tests
measure table heights instead of assuming that fewer VM instructions
automatically produce a smaller proof.


## Selective payment disclosure

A payment disclosure contains version, chain, confirmed signed transaction hash,
output slot 0–2 and one **32-byte record key**. The key is derived from that
record's fresh KEM transcript and exported only after authenticating it. It
cannot open another record, including another record encrypted to the same
wallet key. No incoming/outgoing/stamp seed or spending secret is exported.
Change slot 3 and zero-value decoys cannot be disclosed through this operation.

Verification fetches the exact on-chain raw transaction and successful receipt,
checks chain/hash and canonical block identity, authenticates the selected
outgoing ciphertext, recomputes its full note commitment, and checks that the
recipient address's registered owner matches the opening. A final canonical
block check rejects a reorganization during inspection. It returns only the
selected recipient, amount, slot, transaction hash and confirmation block.
An unconfirmed payment remains unverifiable until successfully included.

This proves that the selected output paid the registered recipient. It does
not reveal or attest the true hidden payer identity. It reveals the selected
note's owner/randomness/value, but excludes the recipient's secret nullifier
key, so those details do not identify its later spend. Protect disclosure files:
they still expose that payment's recipient and amount. Other outputs and wallet
history remain encrypted.

Optional disclosure capsules encrypt this small bundle with the auditor's
ML-KEM public key, purpose 4 and a fresh random 64-byte context. Public capsule
metadata does not include the selected transaction hash. Only its private
64-byte auditor seed opens it. Auditors download their separate key file and
share only `publicKey`, never `auditKey`.

Wallet steps: open **Disclose or verify one payment**, enter the confirmed
transaction hash and payment slot, optionally paste the auditor public key,
then download the disclosure. To verify, paste its JSON and, for an encrypted
capsule, the private auditor key. Amounts are shown only in this local wallet
flow; these operations are absent from public node RPC. Neither record keys nor
auditor seeds are written to browser local storage by this flow.

## RPC and local wallet surface

Public `tkmprivacy` RPC exposes encrypted canonical output batches, activation
status (including `maxInputs: 4`), note paths, bounded same-snapshot path batches
(`shieldedV3Paths`, 1–4 distinct commitments), known-root checks
(`shieldedV3RootsKnown`) and pending/canonical nullifier status. All active inputs
of a pending transaction are checked. Scans reject an altered starting head and
exclude pending, spent or reserved notes.

Private `/shield3/` POST operations require a GUI token, loopback peer/host and
same-origin request. Added operations are `relay-offer`, `review-relay-offer`,
`prepare-relay`, `review-relay`, `submit-relay`, `relay-status`,
`fetch-relay-offer`, `submit-relay-draft`, `view-stamp`, `disclosure-key`,
`export-disclosure` and `verify-disclosure`. Proof packets have a bounded 20 MiB
hex/JSON body; normal operations are bounded to 512 KiB. Responses disable
caching. Spending/viewing seeds are cleared after use and never logged or saved
in submission records. Durable records store public transaction bytes, an
opaque request digest and local draft-account metadata, not note witnesses.
The same shared JS wallet flow runs in desktop and Android; private node/prover
work remains on the authenticated embedded loopback service.

## Legacy balances and builds

Post-fork V2 private transfers/deposits are disabled. The restricted V2 bridge
withdraws one whole legacy note to its own PQ signer, without private change or
sponsorship. Its inherited Groth16 verifier is retained for migration only.
Wait for that public withdrawal's confirmation, then shield public funds into
V3. Migration reveals the withdrawn amount.

Build the native library with `./scripts/shield3-build.sh`, then use the existing
`make gtkm`, `make gtkm-gui`, `make gtkm-gui-windows` and `./android/build.sh`
targets. Release builds require Rust 1.89, CGO and the appropriate Windows/NDK
cross linker. Rebuild the engine with `npm run build --prefix
internal/gui/wallet-engine`. Do not copy an old static library into a new node.
The existing `release.yml` and `shield3-crypto.yml` build/test the embedded
verifier; no remote proof trust or extra runtime service is needed. Proof
generation consumes several GiB of memory and can take minutes under load.
Successful Android packaging is not a physical-device performance test; small
phones need memory and latency validation before production use. Verification
and proof generation have different resource costs.

## Verification map

- Rust relation tests reject mutated active openings/paths, duplicate inputs,
  invalid counts, overflow, wrong owner/change, altered public statements and
  proofs. Genuine one- and four-input STARKs exercise fixed trace/proof bounds.
- Go native interoperability checks the exact updated public/private/path wire
  format against real generated proofs.
- The native wallet test executes deposits, stamps, sponsorship and private
  sends through consensus and the EVM, combines four notes through a different
  operator, checks hidden payer key/public nonce, atomic secondary-input failure,
  all canonical nullifier hashes, three-output batch sends, receive-only isolation,
  scoped disclosure, encrypted auditor capsules
  and rejection of orphaned receipts.
- Focused regressions cover selection/input limits, canonical nullifiers,
  relay expiry, loopback access, unchanged older request digests and durable
  signed/unsigned submissions. Shared engine/browser checks cover mobile flows.

See [the native protocol reference](../zk/shielded3/README.md) for proof encoding
and reproducible cryptographic test commands. Test vectors contain deterministic
test secrets only. Existing unrelated repository test/lint failures must be
reported separately from these feature checks.

See [the mined-node test report](SHIELD3_LIVE_TEST.md) for actual RandomX
mining, RPC transaction inclusion, balances, findings and reproduction commands.
