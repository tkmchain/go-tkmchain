# Browser-native Shield3 proving

The web wallet builds Shield3 transactions in a dedicated Web Worker. The
worker contains two release artifacts:

- `tkm-shield3-stark.wasm`: the pinned Rust Triton STARK relation and fixed
  bounded ABI;
- `tkm-shield3-go.wasm`: the canonical Go note scanner, transaction builder,
  stamp/deposit builder, and ML-DSA-87 signer.

The browser supplies public JSON-RPC responses to the worker. The spending
seed, decrypted note data, Merkle witness, and proof stay inside the browser;
only the final signed transaction is sent with `eth_sendRawTransaction`. No
loopback prover, SSH tunnel, bearer token, or public proof service is needed.

Build the assets from the repository root:

```bash
./scripts/shield3-browser-build.sh dist/shield3-browser
```

The build uses Rust 1.89, Triton VM 8.0.0, Go `js/wasm`, and browser Web
Crypto for entropy. The release workflow publishes the same three files in a
`tkm-shield3-browser-<tag>.tar.gz` artifact.

The worker accepts only the canonical Shield3 request shape. It can build a
stamped registration, a public-to-private deposit, or a one-to-three-recipient
private batch. It checks the chain ID, stamp, recipient code, note anchor,
nullifiers, and native proof before signing. Legacy Shielded V2 builders are
not selected after Antartical activation.
