//! Local-only compatibility surface for the Ziren SDK.
//!
//! The keeper disables Ziren's `network` feature and never constructs a tonic
//! transport.  The SDK still imports `tonic::async_trait` in its CUDA prover
//! module, so expose only that macro instead of compiling tonic's HTTP/TLS
//! stack (h2, rustls, and ring).

pub use async_trait::async_trait;
