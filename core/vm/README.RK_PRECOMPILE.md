RotatingKing VM Precompile
==========================

Overview
--------
This repository includes a minimal RotatingKing (RK) precompiled contract for the EVM VM (core/vm). The precompile exposes compact, deterministic RotatingKing information to smart contracts via a VM precompile address. It is intentionally small and self-contained so it can be wired later to your chain's rotating-king implementation.

The precompile is currently implemented as a stub (placeholder data). Replace the stubbed getters with the deterministic accessor into your rotatingking subsystem to return real on-chain data.

Files added
-----------
- core/vm/precompile_rk.go
  - Implements the RK precompile as a `PrecompiledContract`.
  - Provides a small JSON payload with `currentKing`, `nextKing`, and `totalKings`.
  - Uses a fixed gas estimate; tune as required.
- core/vm/precompile_rk_test.go
  - Simple unit test that calls the precompile directly.

Precompile address
------------------
By default the precompile is registered at:
- Address: 0x00000000000000000000000000000000000000f1
  (the address may be changed — see Registration below)

The precompile is available to contracts by calling this address (see Usage).

Return format
-------------
The precompile returns a JSON-encoded payload (application-level bytes). Example:

{
  "currentKing": "0x0000000000000000000000000000000000000000",
  "nextKing":    "0x0000000000000000000000000000000000000001",
  "totalKings":  42
}

- currentKing: hex address string for the current rotating king
- nextKing: hex address string for the upcoming king
- totalKings: total number of kings in the rotation (integer)

You may change the encoding (binary, ABI-encoded tuple, etc.) if you prefer ABI-friendly outputs; JSON is intentionally simple for the initial implementation.

Gas model
---------
- The included implementation charges a small fixed gas amount (e.g. 300 gas). This is a placeholder. For production use:
  - Compute gas based on returned payload size or complexity.
  - Ensure the precompile is inexpensive and cannot be abused for DoS.
  - Gate heavy computations behind chain rules or disallow them.

Registration (where to put the map entry)
-----------------------------------------
Register the precompile in the VM precompile map (core/vm/contracts.go). Add the entry to the appropriate PrecompiledContracts map (for example PrecompiledContractsPrague and/or PrecompiledContractsOsaka) so it is enabled at the desired fork:

Example insertion inside the chosen map literal (adjust to code style):

```go
// RotatingKing precompile (custom address 0x...f1)
common.HexToAddress("0x00000000000000000000000000000000000000f1"): &rkPrecompileContract{},
