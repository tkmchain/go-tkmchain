# Shielded Spend Circuit

This package contains the first auditable implementation of the Tkmchain shielded spend circuit.

Status: implementation and test-vector scaffold. This is not an audit report and does not by itself make the circuit mainnet-ready.

## Circuit Version

- Name: `TKM_SHIELDED_SPEND_V1`
- Curve: BN254
- Proof system: Groth16 over R1CS
- Circuit hash: MiMC over BN254 scalar field elements
- Merkle depth: 32
- Output slots: 4 fixed slots, padded by wallets when fewer outputs are created

## Public Inputs

The public input order matches the node verifier's `ShieldedProofContext` public input order:

1. `ChainID`
2. `BlockNumber`
3. `TxHashHi`
4. `TxHashLo`
5. `SpendIndex`
6. `Nullifier`
7. `Anchor`
8. `BalanceCommitment`
9. `PublicValue`
10. `OutputRoot`
11. `BindingSigHash`

## Private Witness

The private witness contains:

- note owner secret;
- input note randomness;
- input note value;
- asset ID;
- 32 Merkle path sibling nodes;
- 32 Merkle path direction bits;
- 4 output recipient secrets;
- 4 output values;
- 4 output randomness values;
- 4 output commitments.

## Proved Statements

For each valid proof, the circuit proves:

- the input note commitment is derived from owner secret, asset ID, note value, and note randomness;
- the input note is included in the commitment tree whose public root is `Anchor`;
- the public nullifier is derived from the owner secret and note randomness;
- every public output commitment is derived from a private recipient secret, asset ID, output value, and output randomness;
- the public output root commits to the fixed padded output commitment set;
- input value equals total output value plus public value;
- the public balance commitment binds input value, total output value, public value, asset ID, and output root;
- the binding hash ties the spend secret to the nullifier, output root, balance commitment, chain ID, and split transaction hash limbs.

## Audit Focus

Auditors should review:

- whether these constraints are sufficient for note ownership, no inflation, and no double-spend soundness;
- whether all public input field encodings match consensus code;
- whether all field-element hashes are canonical and all amounts are range constrained;
- whether MiMC domain constants are unique and fixed;
- whether the fixed output-slot padding rule is implemented identically in wallets and nodes;
- whether `BindingSigHash` is the right authorization primitive or should be replaced by a full in-circuit signature check.
