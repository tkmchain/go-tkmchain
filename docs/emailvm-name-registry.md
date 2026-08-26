# EmailVM Permanent Name Registry

## Purpose and scope

The EmailVM name registry assigns canonical TKM mail names to PQ wallet
addresses through proof-bound shielded transactions. It covers two independent
name classes:

| Kind | Canonical name | Uniqueness rule |
| --- | --- | --- |
| Domain | `john` | Globally unique; only one canonical owner can register `@john`. |
| Mailbox | `alice@john` | The complete address is unique; only one canonical owner can register it. |

Mailbox usernames are scoped to their domain. For example, `alice@john` and
`alice@tkm` are different registry names and may have different owners. The
same complete address cannot be registered twice.

The registry is an EmailVM application state reconstructed from canonical
blockchain transactions. It does not store private keys, passphrases, viewing
keys, note openings, proofs, or plaintext mail.

## Canonical names

Clients and nodes lowercase and validate every name before hashing or creating
an action.

- Domains are 1-63 characters. They contain lowercase ASCII letters, digits,
  and interior hyphens.
- Usernames are 1-64 characters. They additionally permit `.`, `_`, and `-`.
- A name must start and end with a letter or digit.
- `tkm` is the reserved network domain and cannot be registered as a custom
  operator domain.
- A mailbox hash covers the complete `username@domain` string, not only the
  username.

Mixed-case input such as `Alice@John` is canonicalized to `alice@john` before
availability checks and hashing. It does not create a separate name.

## Registry hash specification

Registry hashes are Keccak-256 digests of a domain-separated byte sequence.
All string components are their canonical UTF-8/ASCII bytes, and `0x00` is one
zero byte.

```text
domain hash  = keccak256(
  "TKM_EMAILVM_REGISTRY_V1" || 0x00 || "domain" || 0x00 || canonical_domain
)

mailbox hash = keccak256(
  "TKM_EMAILVM_REGISTRY_V1" || 0x00 || "mailbox" || 0x00 ||
  canonical_username || "@" || canonical_domain
)
```

The kind separator prevents a domain and mailbox with similar text from
sharing a registry identity.

### Test vectors

| Input | Registry hash |
| --- | --- |
| Domain `john` | `0x1856997af25dc25a26ab6b7fd3bdc7aba219ee3c7e3091223d6bc20458ed6e04` |
| Mailbox `alice@john` | `0xc39314210816a49799cc7f12397ec4747fe9bdfad23c6b0801a7532b94d2b3c1` |
| Domain `tkm` | `0x4f553eae9c34a6ab91e5cf06af427f523457c698877869d0ceecdc7ca7ede4da` |
| Mailbox `info@tkm` | `0x4914aed3f5bd048acc62ec254480ce1e9b608ffb8a9453dc1e0ac239fac1d36c` |

Go, browser JavaScript, console, CLI, and reference-contract implementations
must produce these exact values.

## Blockchain commitment

New registrations use EmailVM action version 3. The proof-bound
`applicationData` begins with the existing `TKMEMAILVM1` marker and contains a
JSON action. Both the readable canonical name components and the calculated
registry hash are inside the transaction metadata covered by the shielded
intent and local PQ signature.

An operator action contains fields equivalent to:

```json
{
  "v": 3,
  "kind": "operator",
  "domain": "john",
  "units": 1000,
  "payout": "0xOperatorPayoutAddress",
  "registryHash": "0x1856997af25dc25a26ab6b7fd3bdc7aba219ee3c7e3091223d6bc20458ed6e04"
}
```

A mailbox purchase contains fields equivalent to:

```json
{
  "v": 3,
  "kind": "buy",
  "domain": "john",
  "username": "alice",
  "registryHash": "0xc39314210816a49799cc7f12397ec4747fe9bdfad23c6b0801a7532b94d2b3c1"
}
```

The serialized JSON is compact; field ordering is an encoding detail. Clients
must verify decoded values, not reproduce JSON text by hand.

## Canonical ownership and duplicate handling

Registration follows these rules:

1. The RPC rejects a plan when its canonical name or registry hash is already
   registered in the node's canonical index.
2. The browser independently calculates and verifies the returned registry
   hash before it constructs proofs or signs transactions.
3. During canonical block replay, the node validates the action's readable
   name, recalculates its hash, and rejects a mismatched version-3 action.
4. The first fully paid valid registration in canonical transaction order
   creates the permanent registry record.
5. Later actions with the same canonical name/hash cannot replace its owner,
   transaction hash, or registration block.

Two clients can request plans before either transaction is canonical. In that
race, canonical transaction order decides the owner. Clients should refresh
availability immediately before broadcasting and should wait for canonical
registration before treating a name as owned. The carrier shielded transaction
may still be valid even when its duplicate EmailVM action loses the race, so
applications should not deliberately submit competing purchases.

“Permanent” means permanent within the canonical chain. A chain reorganization
can change provisional registrations. The EmailVM index detects a mismatched
indexed block hash, discards its cache, and replays the new canonical chain.

## Registry record

`tkmdomain_registration` resolves a registry hash to the canonical record:

```json
{
  "registryHash": "0x4914aed3f5bd048acc62ec254480ce1e9b608ffb8a9453dc1e0ac239fac1d36c",
  "kind": "mailbox",
  "name": "info@tkm",
  "domain": "tkm",
  "username": "info",
  "owner": "0xOwnerPQAddress",
  "txHash": "0xCanonicalRegistrationTransaction",
  "block": "0xCanonicalBlockNumber"
}
```

Domain and mailbox listing records also expose `registryHash`, their canonical
registration/purchase transaction, and block number.

## JSON-RPC

Enable `tkmdomain` in the HTTP API list. The registry methods are read-only:

| Method | Parameters | Result |
| --- | --- | --- |
| `tkmdomain_domainHash` | `[domain]` | Deterministic canonical domain hash. |
| `tkmdomain_mailboxHash` | `[username, domain]` | Deterministic complete mailbox hash. |
| `tkmdomain_registration` | `[registryHash]` | Canonical owner/name/transaction record. |
| `tkmdomain_domain` | `[domain]` | Domain record including its registry hash. |
| `tkmdomain_mailbox` | `[username@domain]` | Mailbox record including its registry hash. |
| `tkmdomain_status` | `[]` | Index head and total `registrations`. |

Example:

```bash
curl -sS -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","id":1,"method":"tkmdomain_mailboxHash","params":["alice","john"]}' \
  https://wallet.tkmchain.site/rpc

curl -sS -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","id":2,"method":"tkmdomain_registration","params":["0xc39314210816a49799cc7f12397ec4747fe9bdfad23c6b0801a7532b94d2b3c1"]}' \
  https://wallet.tkmchain.site/rpc
```

An unavailable hash returns `name registration not found`. Attempting to buy
an existing mailbox returns `mailbox is already registered`.

## Console and CLI

From `gtkm attach`:

```javascript
domain.hash("john")
domain.mailboxHash("alice", "john")
domain.registration(domain.mailboxHash("alice", "john"))
domain.get("john")
domain.mailbox("alice@john")
```

From the shell:

```bash
./build/bin/gtkm domain hash john
./build/bin/gtkm domain mailbox-hash alice john
./build/bin/gtkm domain registration 0xc39314210816a49799cc7f12397ec4747fe9bdfad23c6b0801a7532b94d2b3c1
./build/bin/gtkm domain mailbox alice@john
```

Add `--rpc https://host/rpc` to inspect a remote node; local IPC is the
default.

## Compatibility and node upgrades

- Version-1 actions retain the original 30,000 + 100-per-unit fee rules.
- Version-2 actions retain the 2,500 + 1-per-unit fee rules introduced with
  the current pricing.
- Version-3 uses the version-2 fee rules and adds the explicit registry hash.
- When an upgraded node loads or rebuilds its EmailVM index, completed
  version-1 and version-2 domain/mailbox records are deterministically inserted
  into the same hash registry. Existing owners do not need to register again.

The registry uses the existing proof-bound shielded application-data field, so
it does not require a consensus hardfork or a new proving ceremony. All public
EmailVM RPC nodes should nevertheless run the upgraded software; older nodes
validate carrier transactions but do not understand version-3 EmailVM actions
or expose the registry RPC methods.

## Privacy and security notes

- Domain names, mailbox addresses, registry hashes, owners, registration
  transaction hashes, block numbers, and encrypted-mail routing metadata are
  public.
- Mail plaintext and encryption private keys remain browser/client-local.
- A registry hash is an identifier and integrity commitment, not encryption;
  names come from a small public namespace and can be guessed.
- Never send PQ seeds, passwords, shielded notes, or viewing private keys to a
  public RPC server.
- Recalculate and compare `registryHash`, payment recipient, fee, and all
  readable action fields before locally signing a plan.

