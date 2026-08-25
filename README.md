
# Go Tkmchain

Golang execution layer implementation of the Tkmchain protocol with **RandomX PoW** and **Rotating Kings (RK)** governance.

[![API Reference](
https://pkg.go.dev/badge/github.com/tkmchain/go-tkmchain
)](https://pkg.go.dev/github.com/ethereum/go-ethereum?tab=doc)
[![Go Report Card](https://goreportcard.com/badge/github.com/ethereum/go-ethereum)](https://goreportcard.com/report/github.com/ethereum/go-ethereum)
[![Travis](https://app.travis-ci.com/ethereum/go-ethereum.svg?branch=master)](https://app.travis-ci.com/github/ethereum/go-ethereum)
[![Discord](https://img.shields.io/badge/discord-join%20chat-blue.svg)](https://discord.gg/vbJF7PrXF8)
[![Twitter](https://img.shields.io/twitter/follow/go_tkmchain)](https://x.com/go_tkmchain)

Automated builds are available for stable releases and the unstable master branch. Binary archives are published at https://tkmchain.site/download.html.

---

## Release Notes

- [Kyoto Release Notes](docs/kyoto-release.md)
- [Shielded Privacy Release Notes](docs/shielded-privacy-release.md)
- [Shielded V2 Recipient Binding](docs/shielded-v2-recipient-binding-20260820.md)
- [Post-Quantum Wallet Integration](docs/pq-wallet-integration.md)

---

## Shielded Privacy Activation

Mainnet shielded privacy activates at `2026-08-10 06:00:00 UTC`
(`privacyCommitmentTime = 1786341600`). Egypt test network has privacy
commitments active from genesis.

After activation, block processing rejects normal transparent user
transactions. Accepted user transactions must use the `TKMSHIELD1` envelope,
target the shielded pool address, carry zero transparent `tx.Value`, include
real BN254 Groth16 shielded spend proofs, and use exactly four padded output
commitments.

The mainnet `shieldedGroth16VerifyingKey` artifact is embedded in chain config
using the `TKMG16VK1` encoding. The ceremony tooling used to produce and encode
that artifact lives in `cmd/shielded-ceremony`; the local key generator for
development lives in `cmd/shielded-setup`.

Key ceremony artifact hashes recorded for this build:

```text
verifying.hex: a307f78a326e1a6fc70ada418f906d94e52c43aa5ebc0c962daa12ff6eae567e
verifying.key: c5cfb0c58b1a9a6823e8b4973dc122590b6568253d4152a7ac928cce8f157d79
```

The proving key is not embedded in the node binary. It is a public circuit
artifact and may be mirrored by anyone; private note witnesses, local signing
keys, and any ceremony toxic-waste material must never be published.

The recovery Groth16 verifier activates on mainnet at
`2026-08-20 12:00:00 UTC` (`1787227200`). Recovery artifacts:

```text
proving.key:  7c3dc3b9f33e522e84665189fa02c08299d209daaa80f96d2dfa6ad43dc2be40
verifying.key: 24a3dcf939acc41bc236c628e556ad80fb0a8e381f8f93a095fcc44196fcea9b
verifying.hex: 214b4671b3110d14936117b92cb3a4266895afd7d0725fe1099377c02bbc0fef
```

Recipient-bound Shielded V2 activates at `2026-08-21 09:00:00 UTC`
(`1787302800`). V2 binds every note to the ML-DSA-87 account recovered from
the transaction signature, enables payments to another person's
`tkmshield2.` payment code, and uses a separately domain-separated circuit and
key pair. V1 envelopes are accepted before the timestamp and V2 envelopes are
required at and after it.

V2 also supports proof-backed shielded withdrawals. `POST
/build-withdrawal` spends one note, creates an encrypted V2 change note when
needed, and releases the proven public value from the shielded pool to the
transparent `0x` recipient encoded in the signed intent. The endpoint returns
an unsigned PQ transaction; the ML-DSA-87 key and passphrase remain in the
client. This reuses the V2 circuit/proving key, but every validator must run a
binary containing the withdrawal consensus rules before a withdrawal is sent.

At `2026-08-22 13:00:00 UTC` (`1787403600`), V2 private spends can reserve
their maximum transaction gas cost inside the proof. Consensus releases that
reserve to the recovered PQ sender before normal gas purchase, so transfers and
withdrawals work with a zero transparent balance. Nodes reject non-zero
`gasSponsorValue` fields before that timestamp. The client subtracts the reserve
from shielded change and must verify it before signing. Values remain uint64 per
proof; clients send larger amounts as several single-note proofs using the same
keys.

```text
proving.key:  248d2a299233c0d57e5a03d30cba62d4dde8f716594e67585842065b5eebd626
verifying.key: de7585bcaea8bbf14fbd7e7a42aa2724e6e1ee925f62fa507a4d38403ed9d62b
verifying.hex: f244511eee64c0af44c97dd2fef4e2158fe52690e4c1c0cd03d2113907be6924
```

---

## Quantum-Resistant Transaction Activation

Mainnet quantum-resistant transaction rules activate at
`2026-08-10 06:00:00 UTC` (`quantumResistantTime = 1786341600`), the same
timestamp as shielded privacy activation. Egypt test network activates the rule
from genesis.

This release requires Go `1.25.0` or newer and uses
`github.com/emmansun/gmsm` for ML-DSA support. The new consensus transaction
type is `PQTkmTxType` (`0x06`). PQ transactions carry:

- `pqAlgorithm`: currently `ML-DSA-87`;
- `pqPublicKey`: canonical FIPS 204 public-key bytes;
- `pqSignature`: ML-DSA signature over the transaction signing hash.

After `quantumResistantTime`, block processing and the txpool reject transparent
legacy/ECDSA user transaction types. Synthetic protocol block-reward
transactions remain protocol-generated and are not wallet/user signatures.

Main King rewards can be scheduled across the hardfork without replacing the
legacy address. `mainKingAddress` remains the pre-fork address, while
`postQuantumMainKingAddress` becomes active at `quantumResistantTime` when set.

PQ account addresses remain EVM-compatible 20-byte `common.Address` values for
contract and state compatibility, but they are domain-separated from legacy
secp256k1 addresses using the hash domain `tkmchain:pq-address:v1:`.

Wallet support is versioned instead of overloading legacy ECDSA key files.
Version 4 PQ keystore files store the encrypted ML-DSA-87 seed, public key,
algorithm, address, and the existing scrypt/AES-CTR/MAC envelope. Existing
version 3 ECDSA keystores remain readable for pre-fork migration.

RPC/account helpers added through the `tkm` API:

- `newPQAccountWithPassphrase`
- `importPQSeedWithPassphrase`
- `exportPQAccount`
- `accountAlgorithm`
- `accountAlgorithms`
- `pqMigrationData`
- `sendMigrationToPQWithPassphrase`
- `preparePQMigrationWithPassphrase`
- `preparePQMigrationWithPassphrases`
- `autoMigrateToPQWithPassphrase`
- `autoMigrateToPQWithPassphrases`

Before the hardfork, users migrate by creating a PQ account and sending a
legacy-signed value transfer to the PQ address. The migration transfer should
carry the `TKMPQMIG1` payload generated from the PQ public key through
`pqMigrationData`, `preparePQMigrationWithPassphrase`, or
`autoMigrateToPQWithPassphrase`; the payload binds the destination address to
its ML-DSA-87 public key for wallet and explorer verification. The auto-migrate
helper creates the PQ keystore, attaches the migration payload, signs the
legacy transfer, and submits it before activation. After the hardfork,
legacy-signed migration is closed and normal user transactions must be signed
as `PQTkmTxType`.

Deterministic PQ test vector:

```text
seed:    000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
address: 0x803e6EE61B7Ecba64eDF13ce0c4a8a65C495e5A5
```

Local tooling:

```bash
ethkey generate --pq --pqseed <32-byte-hex-seed> --passwordfile pass.txt key.json
ethkey inspect --passwordfile pass.txt key.json
```

---

## Rotating Kings (RK) Governance

Tkmchain features a unique **Rotating Kings** governance system with a 10/40/50 reward split:

| Role | Share | Description |
|------|-------|-------------|
|    **Main King** | 10% | Permanent king, network leadership |
|    **Rotating King** | 40% | Rotates every 100 blocks, decentralized governance |
| ⛏️ **Miner** | 50% | Secures the network via RandomX mining |

### Reward Distribution

```
Block Reward = 200 TKM (halving every ~4 years)
├── Main King:     20 TKM (10%)
├── Rotating King: 80 TKM (40%)
└── Miner:        100 TKM (50%)
```

### King Registration

To become a Rotating King:
1. Hold at least **50,001 TKM**: 50,000 TKM is locked as the Rotating King stake and 1 TKM is reserved as the registration fee.
2. Register your address with the `rk_add` RPC method.
3. Remain funded while the address is active. Registered kings are removed when the stake lock expires or the address no longer satisfies the funding requirement.
4. Kings rotate every 100 blocks by default. Each king serves for one rotation period before the next registered address receives the Rotating King slot.

### Rotating King RPC API

The Rotating King service is exposed through the `king`, `mainking`, `rk`, and `rotatingking` RPC namespaces. The short `rk` namespace is intended for operational scripts, while `mainking` is used for checkpoint submission. Enable the namespaces on HTTP or WebSocket explicitly when serving remote RPC:

```shell
gtkm --http --http.addr 127.0.0.1 --http.api eth,net,web3,rk,mainking,randomx,miner
```

Common calls:

| Method | Parameters | Description |
|--------|------------|-------------|
| `rk_add` | `address` | Registers a funded address as a Rotating King candidate and returns its status. |
| `rk_list` | none | Lists registered and locked Rotating King addresses with status metadata. |
| `rk_status` | `address` | Returns status for one address, including lock, rotation, and reward fields. |
| `rk_getKingStats` | optional ignored value | Returns current king, next king, total registered kings, rotation height, and per-king statuses. |
| `rotatingking_getInfo` | none | Returns the current schedule, main king, current king, next king, and rotation interval. |
| `rotatingking_getCurrentKing` | none | Returns the address assigned to the current block's Rotating King slot. |
| `rotatingking_getRotationHistory` | optional `limit` | Returns recent rotation boundaries derived from chain height. |
| `mainking_addCheckpoint` | `number`, `hash` | Adds and broadcasts a checkpoint after the Main King node verifies the local block hash. |

Example JSON-RPC requests:

```shell
# Register a funded Rotating King address
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"rk_add","params":["0xYourKingAddress"]}'

# Inspect the current schedule and all registered kings
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"rk_getKingStats","params":[null]}'

# Add a checkpoint from the Main King node
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"mainking_addCheckpoint","params":[12345,"0xBlockHash"]}'
```

---

## Supply And Reward Accounting

Tkmchain includes a non-consensus supply accounting index through the `tkmsupply` RPC namespace and `gtkm supply` CLI command. It scans canonical blocks, persists cumulative totals in the node database, and can return totals at historical block heights.

This does not require a hardfork. It does not change balances, rewards, headers, validation, or consensus rules. It is an accounting index built from canonical chain data.

Tracked totals:

- genesis supply from the stored genesis allocation;
- total issued block rewards;
- total supply at a block height;
- cumulative Main King rewards;
- cumulative Rotating King rewards;
- cumulative miner rewards.

Enable the RPC namespace on HTTP if needed:

```bash
./build/bin/gtkm --http --http.api eth,net,web3,tkm,tkmsupply
```

CLI examples:

```bash
# Current head
gtkm supply latest /home/mike/.tkmchain/gtkm.ipc

# Historical height
gtkm supply at --block 10000 /home/mike/.tkmchain/gtkm.ipc

# Build or extend the persisted index to a height
gtkm supply sync --block 10000 /home/mike/.tkmchain/gtkm.ipc
```

RPC methods:

```text
tkmsupply_latest()
tkmsupply_atBlock(blockNumber)
tkmsupply_sync(blockNumber)
```

For blocks that contain synthetic reward transactions, the accounting uses the actual reward marker values. For older implicit-reward blocks, it derives the reward split from canonical block headers and the RandomX reward schedule.

## Governance Disclosure Ledger

Tkmchain includes a non-consensus governance disclosure ledger through the `tkmgov` RPC namespace and `gtkm governance` CLI command. It is used to publish Main King signed, append-only hashes of public governance documents such as Rotating King selections, checkpoint explanations, roadmap statements, hardfork notices, and development-fund commitments.

This does not require a hardfork. It stores public disclosure metadata in the node database and mirrors it under the datadir governance folder. Full documents should live in `docs/governance/`, while `tkmgov` stores the content hash, previous disclosure hash, Main King signature, and optional anchor transaction hash. See [docs/GOVERNANCE_DISCLOSURES.md](docs/GOVERNANCE_DISCLOSURES.md).

## TVM Institutional Suite

Tkmchain includes an application-layer institutional suite for banks, schools, government agencies, enterprises, cooperatives, NGOs, and other public or private institutions that need auditable records without changing consensus. It is deployed as a normal contract and exposed through a non-consensus `tkminstitution` RPC namespace.

The suite stores hashes, status fields, ownership/admin addresses, timestamps, metadata URIs, and transaction provenance. It should not store private documents directly on-chain. Private files remain with the institution, user, or external storage provider, while Tkmchain stores the verifiable proof.

Mainnet deployment:

```text
Contract: TkmInstitutionalSuite
Address:  0x43aeb055883863cfe40804e386bec801b4ca63ec
Tx hash:  0xcad679cf00644ec75008d79c2f104bde5584fe5e4f66a2987dd137d9730de12a
Block:    0x3122
Owner:    0x4441d6fEd0836B77a503e0B2788bfEd6FD8c23A8
```

Supported modules:

| Module | Purpose |
|--------|---------|
| Institution Registry | Register verified organizations, rotate admins, and suspend or revoke institutions. |
| Credential Registry | Issue and revoke school, training, staff, professional, and compliance credentials. |
| Document Registry | Publish hashes for licenses, permits, land records, tax clearances, approvals, and certificates. |
| Invoice Settlement | Issue invoices, track payment state, and connect institutional payments to TKM transactions. |
| Escrow Vault | Support buyer, seller, and arbitrator workflows for trade, procurement, and service delivery. |
| Procurement Registry | Publish tender, bid, award, and audit-proof records. |
| Grant Registry | Track scholarships, grants, beneficiary records, and disbursement proofs. |
| Audit Disclosure Registry | Publish public disclosures with previous-hash linking for audit trails and corrections. |

### Institutional RPC and Web3

The `tkminstitution` RPC namespace is a helper layer. It discovers the deployed contract and builds ABI calldata for wallets, explorers, and institution portals. It does not send transactions, unlock accounts, change balances, change validation, or require a hardfork. Signing remains in the wallet, offline signer, password RPC flow, or private daemon.

Enable it explicitly when serving RPC from a node that does not use the defaults:

```shell
./build/bin/gtkm \
  --http --http.addr 127.0.0.1 --http.port 8545 \
  --http.api eth,net,web3,tkm,tvm,tkminstitution
```

Common RPC methods:

| Method | Purpose |
|--------|---------|
| `tkminstitution_status` | Returns the deployed contract address, deployment tx, block, owner, modules, selectors, and status enums. |
| `tkminstitution_contractAddress` | Returns the canonical institutional suite contract address. |
| `tkminstitution_textHash` | Returns `keccak256` of UTF-8 text for names, metadata labels, document IDs, and record labels. |
| `tkminstitution_institutionID` | Returns the same institution ID generated by the contract from admin address and registration hash. |
| `tkminstitution_recordID` | Returns deterministic document, credential, invoice, procurement, grant, or disclosure IDs. |
| `tkminstitution_registerInstitutionData` | Builds calldata for registering an institution. |
| `tkminstitution_setInstitutionStatusData` | Builds calldata to activate, suspend, or revoke an institution. |
| `tkminstitution_rotateInstitutionAdminData` | Builds calldata to rotate the admin address for an institution. |
| `tkminstitution_issueDocumentData` | Builds calldata for document proof issuance. |
| `tkminstitution_issueCredentialData` | Builds calldata for credential issuance. |
| `tkminstitution_issueInvoiceData` | Builds calldata for invoice issuance. |
| `tkminstitution_publishProcurementData` | Builds calldata for procurement records. |
| `tkminstitution_publishGrantData` | Builds calldata for grant and scholarship records. |
| `tkminstitution_publishDisclosureData` | Builds calldata for audit disclosures. |

Example status call:

```shell
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tkminstitution_status","params":[]}'
```

Example hash helper:

```shell
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tkminstitution_textHash","params":["Example University"]}'
```

Example calldata builder:

```shell
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tkminstitution_registerInstitutionData","params":[{"admin":"0x1111111111111111111111111111111111111111","nameHash":"0x0000000000000000000000000000000000000000000000000000000000000000","institutionTypeHash":"0x0000000000000000000000000000000000000000000000000000000000000000","registrationHash":"0x0000000000000000000000000000000000000000000000000000000000000000","metadataHash":"0x0000000000000000000000000000000000000000000000000000000000000000","metadataURI":"ipfs://institution-metadata"}]}'
```

The returned calldata should be sent to `0x43aeb055883863cfe40804e386bec801b4ca63ec` using `eth_sendTransaction`, an offline signed raw transaction, or the node password RPC flow.

In `gtkm attach`, the web3 extension exposes:

```javascript
tkminstitution.status()
tkminstitution.contractAddress()
tkminstitution.textHash("Example University")
tkminstitution.institutionID(admin, registrationHash)
tkminstitution.recordID("DOCUMENT", issuerId, contentHash)
tkminstitution.registerInstitutionData({...})
tkminstitution.issueCredentialData({...})
tkminstitution.issueDocumentData({...})
tkminstitution.issueInvoiceData({...})
tkminstitution.publishProcurementData({...})
tkminstitution.publishGrantData({...})
tkminstitution.publishDisclosureData({...})
```

### Institution Security Model

Institutional records should be treated as public proofs, not private storage. A professional deployment should use:

- hashed document contents instead of raw private files;
- metadata URIs that can be pinned or mirrored;
- institution admin keys separated from website hosting keys;
- owner-only institution onboarding and status changes;
- revocation paths for bad credentials, stale documents, or compromised institution admins;
- previous-hash linking for disclosures and corrections;
- explorer pages that show issuer, transaction hash, timestamp, status, and revocation state.

Related docs:

- [docs/tvm/institutional-suite.md](./docs/tvm/institutional-suite.md)
- [contracts/tvm/institutional_suite_manifest.cpp](./contracts/tvm/institutional_suite_manifest.cpp)

---

## TKM Phone Numbers, Messages, and Calls

TKM Phone is a network-native phone-number system built into `gtkm`. It adds MainKing-signed phone-number buckets, operator sales, registered SIM/device keys, encrypted number-to-number messages, and WebRTC voice-call signaling through the `tkmphone` RPC namespace.

Public apps:

| App | URL | Purpose |
|-----|-----|---------|
| Phone market | https://phone.tkmchain.site | Buy buckets, open operator inventory, sell numbers, register SIM slots, send messages, and start calls. |
| Explorer | https://block.tkmchain.site | Inspect phone buckets, registered numbers, transactions, blocks, and addresses. |
| Wallet | https://wallet.tkmchain.site | Create wallets and send TKM payments through the public RPC. |
| Swap | https://swap.tkmchain.site | Use the ANTD/TKM swap surface. |

### Phone Hardfork

Phone write operations are gated by the `PhoneTime` hardfork. On TKMChain mainnet (`chainId 8979`), `PhoneTime` is `1784709000` (`2026-07-22T08:30:00Z`) and is already active on current mainnet heads. Before activation, read-only helpers such as status, price, bucket listing, signing-hash, and WebRTC configuration calls are available, but state-changing phone calls are rejected.

Check activation:

```shell
./build/bin/gtkm tkmphone status

curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tkmphone_status","params":[]}'
```

### Bucket Economics

- MainKing creates phone numbers only as signed buckets.
- One bucket contains **5 phone numbers**.
- One number price is **5,000 TKM**, so one bucket costs **25,000 TKM**.
- MainKing can publish only **5 unsold buckets** at a time.
- MainKing can generate the next 5-bucket round only after all current buckets are bought.
- Operators buy a bucket from MainKing, open it, and receive 5 numbers.
- Operators can sell one number to a buyer for **10,000 TKM**.
- Bucket creation, operator purchase, bucket opening, and number sale all carry transaction/provenance hashes.
- Forged numbers are rejected unless they come from a MainKing-signed bucket and belong to the selling operator.

### MainKing Bucket Generation

MainKing bucket generation should be done from a private `gtkm` node. Do not put the MainKing password or key inside a public website. A hosted phone marketplace should display public bucket/order state and let MainKing approve operator purchases from the private node.

The `--seed` value is a fresh 32-byte random value for the bucket round. It is not the RandomX mining seed.

```shell
SEED=0x$(openssl rand -hex 32)
CREATION_TX=0xYourMinedMainKingCreationTransactionHash

./build/bin/gtkm tkmphone prices
./build/bin/gtkm tkmphone next-round
./build/bin/gtkm tkmphone bucket-hash --seed $SEED --creation-tx $CREATION_TX
./build/bin/gtkm tkmphone generate-buckets \
  --seed $SEED \
  --creation-tx $CREATION_TX \
  --mainking 0xc40F4A0b4df81F8f67A88B179a8b2271107a9ac2
./build/bin/gtkm tkmphone buckets
```

For offline signing, sign the digest returned by `bucket-hash` and pass it explicitly:

```shell
./build/bin/gtkm tkmphone generate-buckets \
  --seed $SEED \
  --creation-tx $CREATION_TX \
  --signature 0xMainKingSignature
```

### RPC and Web3

Enable the phone RPC namespace on nodes that serve phone apps:

```shell
./build/bin/gtkm \
  --http --http.addr 0.0.0.0 --http.port 8545 \
  --http.api eth,net,web3,tkm,tkmphone,mainking,miner \
  --http.vhosts '*' --http.corsdomain '*'
```

Do not expose password-capable RPC methods to untrusted networks.

Common phone RPC methods:

| Method | Purpose |
|--------|---------|
| `tkmphone_status` | Returns phone fork activation and head status. |
| `tkmphone_buckets` | Lists generated buckets and assignment/payment metadata. |
| `tkmphone_listOperators` | Lists approved operators and bucket assignments. |
| `tkmphone_openBucket` | Lets an approved operator open their assigned bucket. |
| `tkmphone_sellNumber` | Transfers a sold number after canonical buyer payment validation. |
| `tkmphone_numberOwnershipProof` | Returns stable MainKing -> operator -> user ownership proof hashes for a number. |
| `tkmphone_registeredNumbers` | Lists numbers with active registered device keys. |
| `tkmphone_registerDeviceKey` | Registers a SIM/device key for an owned number. |
| `tkmphone_sendEncryptedMessage` | Stores and propagates an encrypted message. |
| `tkmphone_startCall`, `tkmphone_acceptCall`, `tkmphone_endCall` | Store and propagate signed WebRTC call signaling. |
| `tkmphone_webRTCConfig` | Returns audio-only WebRTC signaling limits and STUN hints. |

The Web3 extension exposes the same workflow through `web3.tkmphone.*`, including bucket status, operator inventory, registered-number inspection, device-key signing hashes, encrypted messages, call signaling, contacts, blocked numbers, recovery, reports, propagation queue, and import propagation.

### SIM Registration and Privacy

A number can send messages or calls only after its owner registers an active device key. Website SIM slots are local client profiles for owned numbers; the chain validates the registered number, owner signature, and active device key before accepting sensitive actions.

The message and call payloads stored by `gtkm` are encrypted. Inbox, outbox, notification, and call views should be shown only to the owning user or selected SIM slot in client applications.

### State Persistence

`gtkm` persists authoritative phone state in the node database and writes a readable mirror under the instance datadir. Bucket and number records include stable ownership hashes (`issueHash`, `assignHash`, `transferHash`, and `ownerHash`) so marketplace, wallet, explorer, and chat clients can export SIM files for the correct current owner without exposing MainKing custody:

```text
~/.tkmchain/gtkm/phone/state.json
```

The mirror includes buckets, bucket `creationTx`, operator `paymentTx`, generated numbers, number `salePaymentTx`, messages, calls, device keys, notifications, contacts, recovery records, reports, propagation records, and counters.

For the full operational guide, see [docs/tkmphone.md](./docs/tkmphone.md). Release-level hardfork notes are in [docs/kyoto-release.md](./docs/kyoto-release.md).

---

## EmailVM and TKM Domains

`gtkm` provides canonical shielded domain and encrypted-email infrastructure
through the `tkmdomain` and `emailvm` RPC namespaces. The shared namespace is
`username@tkm`; custom operators can register names such as `@john` and buy a
fixed number of subscriber units.

The first canonical PQ-signed shielded claim creates `@tkm` and permanently
sets its signer as the super address. All operator registration and capacity
fees go to that address; mailbox sales under custom domains go to the
operator's configurable payout address.

- Custom domain registration: **30,000 TKM**.
- Subscriber capacity: **100 TKM per unit**.
- A 1,000-unit custom domain: **130,000 TKM total**.
- One mailbox under `@tkm` or an operator domain: **100 TKM**.

From `gtkm attach`:

```javascript
domain.claimSuper() // only while @tkm is unclaimed; first canonical claim wins
domain.operator(1000, "130000", "john")
domain.buy("alice", "john")
domain.buy("alice", "tkm")
domain.pending()
```

These calls return shielded action plans; private notes, proofs, keys, and PQ
signatures remain in the local wallet. Large fees are split into parallel
proof-backed installments and activate only after the exact canonical total is
mined. Encrypted EmailVM messages use the same proof-bound application metadata,
so no new hardfork or proving ceremony is required.

See [docs/emailvm.md](./docs/emailvm.md) for RPC, CLI, prover, privacy, indexing,
and operator details.

---

## ⛏️ RandomX (RX) Mining

Tkmchain uses **RandomX PoW** - an ASIC-resistant mining algorithm optimized for CPUs:

### Mining Features

- **Algorithm**: Monero's RandomX (CPU-friendly)
- **Block Time**: 120 seconds (2 minutes)
- **Difficulty Adjustment**: 2x cap per block (smooth, self-correcting)
- **Epoch Length**: 2,048 blocks (~2.8 days)

### Mining Commands

Build `gtkm` with RandomX support before mining:

```shell
make gtkm
```

Start from a fully synced node, set the reward address with `--miner.etherbase`, and select the number of CPU mining threads with `--miner.threads`:

```shell
# Start CPU mining
gtkm --mine --miner.threads=2 --miner.etherbase=0xYourAddress

# Configure mining threads
gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress

# Solo mining mode on a full node
gtkm --mine --miner.etherbase=0xYourAddress --syncmode=full

# Mining with boost (JIT + AES)
gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress --randomx.boost
```

For external RandomX miners, run a node that exposes mining work. `gtkm` provides the standard `miner_*` calls, a `randomx_*` namespace, and a local stratum bridge on `127.0.0.1:3333` when external mining is started by the miner service. Keep stratum bound to localhost unless you place it behind trusted network controls.

```shell
# Node with HTTP work APIs for a local external miner
gtkm --syncmode=full --http --http.addr 127.0.0.1 --http.api eth,net,web3,miner,randomx \
  --miner.etherbase=0xYourAddress
```

### RandomX Mining RPC API

The mining work tuple is `[sealHash, seedHash, target, blockHeight]`. External miners hash the seal hash with a nonce using the RandomX cache selected by `seedHash`, then submit the nonce and digest back to the node.

| Method | Parameters | Description |
|--------|------------|-------------|
| `miner_getWork` | none | Returns `[sealHash, seedHash, target, blockHeight]` for external miners. |
| `miner_submitWork` | `nonce`, `sealHash`, `digest` | Submits a proof-of-work solution. Returns `true` when accepted. |
| `miner_getSeedHash` | none | Returns the RandomX seed hash for the next block. |
| `randomx_getSeedHash` | optional `blockNumber` | Returns the seed hash for the next block or for the supplied block number. |
| `randomx_getSeedHashForBlock` | `blockNumber` | Returns the seed hash for a specific block number. |
| `randomx_getWork` | none | Returns the same external-mining work tuple as `miner_getWork`. |
| `randomx_submitWork` | `nonce`, `sealHash`, `digest` | Submits typed nonce/hash/digest values. |
| `randomx_submitWorkRaw` | `nonceHex`, `sealHashHex`, `digestHex` | Submits hex strings directly from mining adapters. |
| `randomx_getCurrentHeight` | none | Returns the current canonical block height. |
| `randomx_getHashrate` | none | Returns the node miner hashrate counter. |

Example work loop calls:

```shell
# Fetch work
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"randomx_getWork","params":[]}'

# Fetch the seed hash for block 2048
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"randomx_getSeedHashForBlock","params":[2048]}'

# Submit a solution returned by an external miner
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"randomx_submitWorkRaw","params":["0xNonce8Bytes","0xSealHash","0xDigest"]}'
```

### Difficulty Adjustment Formula

```go
ratio = (targetTime * 100) / actualTime
if ratio > 200 { ratio = 200 }  // Max 2x increase
if ratio < 50 { ratio = 50 }    // Min 0.5x decrease
newDiff = currentDiff * ratio / 100
```

---

## Building the Source

For prerequisites and detailed build instructions please read the [Installation Instructions](https://gtkm.tkmchain.site/docs/getting-started/installing-gtkm).

Building `gtkm` requires both a Go (version 1.23 or later) and a C compiler. You can install them using your favourite package manager. For a validator or wallet RPC server, build only the two runtime programs with stripped debug data and memory-safe compiler parallelism:

```shell
make production
```

This reuses an existing RandomX library and Go's build cache. Override
`GO_BUILD_P=4` only on a machine with more memory. To build the full developer
utility suite (`clef`, `devp2p`, `abigen`, `evm`, and `rlpdump`) as well:

```shell
make all
```

### Cross-Platform Builds

Install mingw for Windows cross-compilation:

```shell
sudo apt-get install gcc-mingw-w64-x86-64 gcc-mingw-w64-i686

# Build Windows 64-bit only
make cross-windows

# Build all Windows architectures
make cross-windows-all

# Build all platforms (Linux, Windows, macOS)
make cross-all-all
```

### Cross-Platform Output

```
build/dist/
├── windows/
│   ├── gtkm-windows-amd64.exe
│   └── gtkm-windows-386.exe
├── darwin/
│   ├── gtkm-darwin-amd64
│   └── gtkm-darwin-arm64
└── linux/
    ├── gtkm-linux-amd64
    ├── gtkm-linux-386
    └── gtkm-linux-arm64
```

---

## Executables

The go-tkmchain project comes with several wrappers/executables found in the `cmd` directory.

| Command | Description |
| :-----: | ----------- |
| **`gtkm`** | Our main Tkmchain CLI client. It is the entry point into the Tkmchain network (main-, test- or private net), capable of running as a full node (default), archive node (retaining all historical state) or a light node (retrieving data live). It can be used by other processes as a gateway into the Tkmchain network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `gtkm --help` and the [CLI page](https://gtkm.tkmchain.site/docs/fundamentals/command-line-options) for command line options. |
| `clef` | Stand-alone signing tool, which can be used as a backend signer for `gtkm`. |
| `devp2p` | Utilities to interact with nodes on the networking layer, without running a full blockchain. |
| `abigen` | Source code generator to convert Tkmchain contract definitions into easy-to-use, compile-time type-safe Go packages. It operates on plain [Tkmchain contract ABIs](https://docs.soliditylang.org/en/develop/abi-spec.html) with expanded functionality if the contract bytecode is also available. However, it also accepts Solidity source files, making development much more streamlined. Please see our [Native DApps](https://gtkm.tkmchain.site/docs/developers/dapp-developer/native-bindings) page for details. |
| `evm` | Developer utility version of the EVM (Tkmchain Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug run`). |
| `rlpdump` | Developer utility tool to convert binary RLP ([Recursive Length Prefix](https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp)) dumps (data encoding used by the Tkmchain protocol both network as well as consensus wise) to user-friendlier hierarchical representation (e.g. `rlpdump --hex CE0183FFFFFFC4C304050583616263`). |

---

## Running `gtkm`

Going through all the possible command line flags is out of scope here (please consult our [CLI Wiki page](https://gtkm.tkmchain.site/docs/fundamentals/command-line-options)), but we've enumerated a few common parameter combos to get you up to speed quickly on how you can run your own `gtkm` instance.

### Hardware Requirements

**Minimum:**
- CPU with 4+ cores (AES-NI support recommended for RandomX)
- 8GB RAM
- 1TB free storage space to sync the Mainnet
- 8 MBit/sec download Internet service

**Recommended:**
- Fast CPU with 8+ cores (AES-NI, AVX2 support)
- 16GB+ RAM
- High-performance SSD with at least 1TB of free space
- 25+ MBit/sec download Internet service

### Full Node on the Main Tkmchain Network

By far the most common scenario is people wanting to simply interact with the Tkmchain network: create accounts; transfer funds; deploy and interact with contracts. For this particular use case, the user doesn't care about years-old historical data, so we can sync quickly to the current state of the network. To do so:

```shell
$ gtkm console
```

This command will:
- Start `gtkm` in snap sync mode (default, can be changed with the `--syncmode` flag), causing it to download more data in exchange for avoiding processing the entire history of the Tkmchain network, which is very CPU intensive.
- Start the built-in interactive [JavaScript console](https://gtkm.tkmchain.site/docs/interacting-with-gtkm/javascript-console), (via the trailing `console` subcommand) through which you can interact using [`web3` methods](https://github.com/ChainSafe/web3.js/blob/0.20.7/DOCUMENTATION.md) (note: the `web3` version bundled within `gtkm` is very old, and not up to date with official docs), as well as `gtkm`'s own [management APIs](https://gtkm.tkmchain.site/docs/interacting-with-gtkm/rpc). This tool is optional and if you leave it out you can always attach it to an already running `gtkm` instance with `gtkm attach`.

### Mining with RandomX

```shell
# Start mining with 2 threads
$ gtkm --mine --miner.threads=2 --miner.etherbase=0xYourAddress

# Start mining with boost mode (JIT + AES)
$ gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress --randomx.boost

# Configure RandomX cache size
$ gtkm --mine --randomx.cache-size=256 --randomx.dataset-size=2
```

### A Full Node on the Holesky Test Network

Transitioning towards developers, if you'd like to play around with creating Tkmchain contracts, you almost certainly would like to do that without any real money involved until you get the hang of the entire system. In other words, instead of attaching to the main network, you want to join the **test** network with your node, which is fully equivalent to the main network, but with play-Ether only.

```shell
$ gtkm --holesky console
```

The `console` subcommand has the same meaning as above and is equally useful on the testnet too.

Specifying the `--holesky` flag, however, will reconfigure your `gtkm` instance a bit:
- Instead of connecting to the main Tkmchain network, the client will connect to the Holesky test network, which uses different P2P bootnodes, different network IDs and genesis states.
- Instead of using the default data directory (`~/.tkmchain` on Linux for example), `gtkm` will nest itself one level deeper into a `holesky` subfolder (`~/.tkmchain/holesky` on Linux). Note, on OSX and Linux this also means that attaching to a running testnet node requires the use of a custom endpoint since `gtkm attach` will try to attach to a production node endpoint by default, e.g., `gtkm attach <datadir>/holesky/gtkm.ipc`. Windows users are not affected by this.

*Note: Although some internal protective measures prevent transactions from crossing over between the main network and test network, you should always use separate accounts for play and real money. Unless you manually move accounts, `gtkm` will by default correctly separate the two networks and will not make any accounts available between them.*

### Configuration

As an alternative to passing the numerous flags to the `gtkm` binary, you can also pass a configuration file via:

```shell
$ gtkm --config /path/to/your_config.toml
```

To get an idea of how the file should look like you can use the `dumpconfig` subcommand to export your existing configuration:

```shell
$ gtkm --your-favourite-flags dumpconfig
```

#### Docker Quick Start

One of the quickest ways to get Tkmchain up and running on your machine is by using Docker:

```shell
docker run -d --name tkmchain-node -v /Users/alice/tkmchain:/root \
           -p 8545:8545 -p 3000:3000 \
           tkmchain/client-go
```

This will start `gtkm` in snap-sync mode with a DB memory allowance of 1GB, as the above command does. It will also create a persistent volume in your home directory for saving your blockchain as well as map the default ports. There is also an `alpine` tag available for a slim version of the image.

Do not forget `--http.addr 0.0.0.0`, if you want to access RPC from other containers and/or hosts. By default, `gtkm` binds to the local interface and RPC endpoints are not accessible from the outside.

### Programmatically Interfacing `gtkm` Nodes

As a developer, sooner rather than later you'll want to start interacting with `gtkm` and the Tkmchain network via your own programs and not manually through the console. To aid this, `gtkm` has built-in support for a JSON-RPC based APIs ([standard APIs](https://ethereum.org/en/developers/docs/apis/json-rpc/) and [`gtkm` specific APIs](https://gtkm.tkmchain.site/docs/interacting-with-gtkm/rpc)). These can be exposed via HTTP, WebSockets and IPC (UNIX sockets on UNIX based platforms, and named pipes on Windows).

The IPC interface is enabled by default and exposes all the APIs supported by `gtkm`, whereas the HTTP and WS interfaces need to manually be enabled and only expose a subset of APIs due to security reasons. These can be turned on/off and configured as you'd expect.

**HTTP based JSON-RPC API options:**
- `--http` Enable the HTTP-RPC server
- `--http.addr` HTTP-RPC server listening interface (default: `localhost`)
- `--http.port` HTTP-RPC server listening port (default: `8545`)
- `--http.api` API's offered over the HTTP-RPC interface (default: `tkm,net,web3,miner,randomx`)
- `--http.corsdomain` Comma separated list of domains from which to accept cross-origin requests (browser enforced)
- `--ws` Enable the WS-RPC server
- `--ws.addr` WS-RPC server listening interface (default: `localhost`)
- `--ws.port` WS-RPC server listening port (default: `8546`)
- `--ws.api` API's offered over the WS-RPC interface (default: `tkm,net,web3,miner,randomx`)
- `--ws.origins` Origins from which to accept WebSocket requests
- `--ipcdisable` Disable the IPC-RPC server
- `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

#### Rotating Kings API Methods

```javascript
// Get current Rotating King
web3.eth.getRotatingKing().then(console.log)

// Get Main King
web3.tkm.getMainKing().then(console.log)

// Get reward distribution
web3.tkm.getRewardDistribution().then(console.log)

// Register as King
web3.tkm.registerAsKing().then(console.log)
```

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to connect via HTTP, WS or IPC to a `gtkm` node configured with the above flags and you'll need to speak [JSON-RPC](https://www.jsonrpc.org/specification) on all transports. You can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based transport before doing so! Hackers on the internet are actively trying to subvert Tkmchain nodes with exposed APIs! Further, all browser tabs can access locally running web servers, so malicious web pages could try to subvert locally available APIs!**

### Operating a Private Network

Maintaining your own private network is more involved as a lot of configurations taken for granted in the official networks need to be manually set up.

Unfortunately since [the Merge](https://ethereum.org/en/roadmap/merge/) it is no longer possible to easily set up a network of gtkm nodes without also setting up a corresponding beacon chain.

There are three different solutions depending on your use case:
- If you are looking for a simple way to test smart contracts from go in your CI, you can use the [Simulated Backend](https://gtkm.tkmchain.site/docs/developers/dapp-developer/native-bindings#blockchain-simulator).
- If you want a convenient single node environment for testing, you can use our [Dev Mode](https://gtkm.tkmchain.site/docs/developers/dapp-developer/dev-mode).
- If you are looking for a multiple node test network, you can set one up quite easily with [Kurtosis](https://gtkm.tkmchain.site/docs/fundamentals/kurtosis).

---

## Configuration Reference

### RandomX Mining Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `--mine` | Enable RandomX CPU mining | `false` |
| `--miner.threads` | Number of mining threads | `0` (auto) |
| `--miner.etherbase` | Address to receive mining rewards | First account |
| `--randomx.cache-size` | Cache size in MB | `256` |
| `--randomx.dataset-size` | Dataset size in GB | `2` |
| `--randomx.epoch-length` | Blocks per epoch | `2048` |
| `--randomx.min-memory` | Minimum memory in GB | `4` |
| `--randomx.boost` | Enable JIT + AES acceleration | `false` |

### Rotating Kings Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `--king.main` | Main king address (10% rewards) | `0x...` |
| `--king.rotating` | Rotating king addresses (40% rewards) | `0x...` |
| `--king.rotation-interval` | Blocks between rotations | `100` |

---

## Contribution

Thank you for considering helping out with the source code! We welcome contributions from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to go-tkmchain, please fork, fix, commit and send a pull request for the maintainers to review and merge into the main code base. If you wish to submit more complex changes though, please check up with the core devs first on [our Discord Server](https://discord.gg/invite/nthXNEv) to ensure those changes are in line with the general philosophy of the project and/or get some early feedback which can make both your efforts much lighter as well as our review and merge procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:
- Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/doc/cmd/gofmt/)).
- Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary) guidelines.
- Pull requests need to be based on and opened against the `master` branch.
- Commit messages should be prefixed with the package(s) they modify.
  - E.g. "tkm, rpc: make trace configs optional"

Please see the [Developers' Guide](https://gtkm.tkmchain.site/docs/developers/gtkm-developer/dev-guide) for more details on configuring your environment, managing project dependencies, and testing procedures.

### Contributing to gtkm.tkmchain.site

For contributions to the [go-tkmchain website](https://gtkm.tkmchain.site), please checkout and raise pull requests against the `website` branch. For more detailed instructions please see the `website` branch [README](https://github.com/tkmchain/go-tkmchain/tree/website#readme) or the [contributing](https://gtkm.tkmchain.site/docs/developers/gtkm-developer/contributing) page of the website.

---

## License

The go-tkmchain library (i.e. all code outside of the `cmd` directory) is licensed under the [GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html), also included in our repository in the `COPYING.LESSER` file.

The go-tkmchain binaries (i.e. all code inside of the `cmd` directory) are licensed under the [GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also included in our repository in the `COPYING` file.

---

## Quick Reference

```shell
# Build
make production              # Fast server build: gtkm + managed prover
make gtkm                    # Build only gtkm
make all                     # Build all tools
make cross-windows           # Build Windows 64-bit
make cross-linux             # Build Linux
make cross-all-all           # Build all platforms

# Run with mining
gtkm --mine --miner.threads=2 --miner.etherbase=0xYourAddress

# Run with Rotating Kings
gtkm --king.main=0x... --king.rotating=0x... --mine

# Check Rotating Kings
gtkm attach --exec "tkm.getRotatingKing()"

# RandomX mining with boost
gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress --randomx.boost

# Full node with RPC enabled
gtkm --http --http.api "tkm,net,web3,miner,randomx" --mine --miner.threads=2

# Export configuration
gtkm dumpconfig > config.toml

# Run with config file
gtkm --config config.toml
```
