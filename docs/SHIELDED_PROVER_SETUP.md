Shielded prover setup and shielded ceremony

Overview

This doc shows commands to run the shielded ceremony (produce proving.key and verifying.hex) and how to deploy the verifying key for gtkm. For mainnet, use an MPC ceremony across multiple participants. The script included below is a single-participant example useful for dev/testing only.

Prereqs

- Go toolchain (go 1.20+)
- gnark & gnark-crypto are used via go modules in this repo; no external install required
- Build/test tooling: make (used by project CI)

Quick single-participant ceremony (dev only)

# create and enter working dir
mkdir -p /tmp/shielded-ceremony && cd /tmp/shielded-ceremony

# Phase 1 (init, single contribute, verify)
go run ../../cmd/shielded-ceremony init-phase1 -out phase1-0.bin
# (optionally: have other participants run contribute-phase1)
go run ../../cmd/shielded-ceremony contribute-phase1 -in phase1-0.bin -out phase1-1.bin
go run ../../cmd/shielded-ceremony verify-phase1 -beacon 0xDEADBEEF -out commons.bin phase1-1.bin

# Phase 2 (init, single contribute, finalize)
go run ../../cmd/shielded-ceremony init-phase2 -commons commons.bin -out phase2-0.bin
# (optionally: have other participants run contribute-phase2)
go run ../../cmd/shielded-ceremony contribute-phase2 -in phase2-0.bin -out phase2-1.bin

# Finalize -> produces proving.key (private), verifying.key, and verifying.hex (chain-format hex)
go run ../../cmd/shielded-ceremony finalize -commons commons.bin -beacon 0xDEADBEEF -pk proving.key -vk verifying.key -vk-hex verifying.hex phase2-1.bin

Notes

- Keep proving.key secret and on prover infrastructure only. Do NOT commit it to repo or share it.
- verifying.hex is the chain-format TKMG16VK1 verifying key (0x... hex). This is the public artifact needed by nodes to verify proofs.

Installing the verifying key into gtkm

There are two common options:

1) Embed into chain config / artifact: copy the hex into the chain artifact slot expected by your genesis/config tooling (MainnetShieldedGroth16VerifyingKey) before node startup.

2) Use node RPC (if available) to set the verifying key at runtime (example RPC: SetShieldedGroth16VerifyingKey or a dedicated admin endpoint). If using a runtime RPC, restart or signal the node to reload chain artifacts if required.

Running a prover (high level)

- Prover host: keep proving.key and proving infrastructure on a single, restricted host.
- The prover program loads proving.key, constructs a SpendCircuit witness (use the zk/shielded package helpers and test vectors), runs groth16.Prove, and encodes the TKMSHIELD1 envelope via core.EncodeShieldedTransaction.
- The prover should submit the shielded transaction via eth_sendRawTransaction (or have the node sign and send if you prefer) targeting params.ShieldedPoolAddress with value 0 and data == encoded envelope.

Pool integration

- Until a real prover is available, the pool will hold transparent payouts (see README). You can either:
  - Run a prover service that the pool can call to create and broadcast shielded payouts.
  - Or continue manual payouts via the dashboard/API.

More help

If you want, the next step can be: (A) generate a minimal prover skeleton program that demonstrates loading proving.key and producing one TKMSHIELD1 tx for a supplied witness, or (B) produce a more complete example that signs/submits the raw tx. Which do you want?