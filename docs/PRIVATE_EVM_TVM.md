# Private EVM/TVM transition proving

Antartical rejects transparent EVM and TVM transactions. User transactions are
admitted only as Shield3 or Shield4 envelopes; the bounded TVM storage
transition implemented in `zk/shielded3/stark` is shielded payload logic rather
than a standalone transaction type.

## What the proof covers

The native Triton VM STARK proves, without revealing the storage key or either
storage value:

1. the ML-DSA transaction intent is bound to one chain and one TVM code hash;
2. the hidden old value is a member of the authenticated private storage tree;
3. the hidden new value produces the committed new root through the same
   32-level Merkle path;
4. read operations preserve the value; and
5. write operations derive the new value from the old value, code hash, hidden
   key, and encrypted call intent inside the relation.

The transaction carries only the roots, code hash, operation, encrypted call
payload, and proof. Consensus stores the new root and rejects a non-canonical
old root or a reused intent.

## Native ABI

`TKMPTVM1` uses native operation 11 to prove and operation 12 to verify inside
a Shield3/Shield4 envelope. The public statement contains 34 field words. The
witness contains 21 field words plus 32 Merkle sibling digests. Proof
verification is bounded by the same field-element and proof-size limits as
Shield3.

## Boundary

This is a real private storage-transition proof for the current deterministic
TVM model. It is not a general zkEVM: arbitrary EVM bytecode, public EVM
storage, logs, and unrestricted TVM host calls are not admitted after
Antartical. Supporting arbitrary EVM semantics requires a separate zkEVM
instruction circuit and a private state model for every observable EVM effect;
encrypted calldata alone is not a valid substitute.

Nodes built without the native verifier fail closed and reject private TVM
proofs.
