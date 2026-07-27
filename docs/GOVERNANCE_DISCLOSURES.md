# TKM Governance Disclosure Ledger

TKMChain now includes a non-consensus governance disclosure ledger through the `tkmgov` RPC namespace and the `gtkm governance` CLI command.

This does not change block validation, account state, fork rules, EVM execution, RandomX, rewards, or checkpoint enforcement. It is an append-only public-record service stored in the node database and mirrored as readable JSON under the node datadir:

```text
~/.tkmchain/governance/state.json
```

Depending on the configured datadir layout, the path is resolved by `gtkm` from `--datadir`.

## Purpose

The chain can prove balances, registered Rotating Kings, reward distribution, checkpoint hashes, and signed checkpoint messages. It cannot, by itself, preserve the original public explanation behind a governance decision.

The disclosure ledger fills that gap. It records a Main King signed hash of the original document for:

- Rotating King selection and removal explanations;
- checkpoint decision explanations;
- hardfork notices;
- roadmap statements;
- development-fund and Main King fund commitments;
- emergency actions;
- postmortems;
- security announcements.

The full text should live in the public repository, normally under:

```text
docs/governance/YYYY-MM-DD-title.md
```

The ledger stores the document hash, metadata, previous disclosure hash, Main King signature, and optional anchor transaction hash.

## RPC Methods

Enable `tkmgov` on the desired transport, for example:

```bash
./build/bin/gtkm --http --http.addr 0.0.0.0 --http.api eth,net,web3,tkmgov
```

Methods:

```text
tkmgov_disclosureHash(kind, title, version, contentHash, uri, previousHash, timestamp)
tkmgov_publishDisclosure(kind, title, version, contentHash, uri, previousHash, timestamp, anchorTx, signature)
tkmgov_getDisclosure(id)
tkmgov_listDisclosures(kind, from, limit)
tkmgov_latestDisclosure(kind)
tkmgov_verifyDisclosure(id)
```

`publishDisclosure` requires a signature from the configured Main King address. Signatures are accepted as raw digest signatures and standard Ethereum signed-message signatures over the digest.

If `anchorTx` is non-zero and the node has canonical transaction indexing available, `gtkm` verifies that the transaction:

- is canonical;
- was sent by Main King;
- was sent to Main King;
- contains the `TKMGOV_DISCLOSURE_V1` prefix and the disclosure hash in calldata.

If `anchorTx` is zero, the Main King signature still makes the record official, but the record is not anchored by a transaction hash.

## CLI Workflow

Create a disclosure document:

```bash
mkdir -p docs/governance
nano docs/governance/2026-07-27-checkpoint-7165.md
```

Calculate the disclosure hash:

```bash
./build/bin/gtkm governance hash   --kind checkpoint   --title "Checkpoint 7165"   --version 1   --file docs/governance/2026-07-27-checkpoint-7165.md   --uri docs/governance/2026-07-27-checkpoint-7165.md
```

Sign with an unlocked Main King account:

```bash
./build/bin/gtkm governance sign   --hash 0xDisclosureHash   --mainking 0xc40F4A0b4df81F8f67A88B179a8b2271107a9ac2
```

Publish the record:

```bash
./build/bin/gtkm governance publish   --kind checkpoint   --title "Checkpoint 7165"   --version 1   --file docs/governance/2026-07-27-checkpoint-7165.md   --uri docs/governance/2026-07-27-checkpoint-7165.md   --signature 0xMainKingSignature
```

List records:

```bash
./build/bin/gtkm governance list
./build/bin/gtkm governance list --kind checkpoint
```

Verify a record:

```bash
./build/bin/gtkm governance verify --id 1
```

## Optional On-Chain Anchor Transaction

For stronger public evidence, Main King should send a normal transaction to itself with calldata containing:

```text
TKMGOV_DISCLOSURE_V1 || disclosureHash || contentHash || previousHash
```

After the transaction is mined, publish with:

```bash
./build/bin/gtkm governance publish   --kind checkpoint   --title "Checkpoint 7165"   --version 1   --file docs/governance/2026-07-27-checkpoint-7165.md   --uri docs/governance/2026-07-27-checkpoint-7165.md   --anchor-tx 0xCanonicalAnchorTransactionHash   --signature 0xMainKingSignature
```

## Policy

Use this policy for public governance:

1. No Rotating King announcement is official unless it has a governance disclosure record.
2. No checkpoint announcement is official unless it has both a signed checkpoint and a governance disclosure record.
3. No roadmap or fund commitment should be silently edited after publication. Publish a new version linked by `previousHash`.
4. Every hardfork notice should have a disclosure hash before activation.
5. Websites and explorers should display records, but the Main King signed disclosure hash is the source of truth.

## Public Answer

TkmChain has verifiable chain facts and signed checkpoint security. The governance disclosure ledger adds the missing permanent public record for original explanations and commitments. Every important public decision can now be published as a Main King signed, append-only disclosure hash, optionally anchored by a canonical TKMChain transaction.
