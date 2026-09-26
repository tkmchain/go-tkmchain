# Antartical consensus primitives

This package is the shared deterministic implementation surface for the
Antartical fork. It contains:

- EIP-4337/RIP-7560 UserOperation hashing and secp256k1 or ML-DSA-87
  authorization;
- optimistic execution wave construction from transaction access sets;
- multidimensional gas accounting;
- stateless state-witness and zk execution claim commitments;
- signed oracle observations and cross-chain replay keys;
- EOF v1 container validation;
- versioned modular precompile registration;
- differential execution engine comparison; and
- quorum-checked single-slot finality certificates.

All hashes include a protocol domain tag and chain identifiers where applicable.
The host EVM, zkEVM guest, and private transaction envelopes use these same
commitments when the Antartical rules are active.
