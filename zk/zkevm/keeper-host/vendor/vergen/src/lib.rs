//! Minimal compatibility surface for Ziren's local-only build.
//!
//! The keeper does not ship or use network metadata. Ziren's build script
//! calls vergen only to set optional build-time labels, so avoid pulling its
//! git2 implementation and its unsafe transitive dependency.

#[derive(Default)]
pub struct EmitBuilder;

impl EmitBuilder {
    pub fn builder() -> Self {
        Self
    }

    pub fn build_timestamp(self) -> Self {
        self
    }

    pub fn git_sha(self, _enabled: bool) -> Self {
        self
    }

    pub fn emit(self) -> Result<(), std::io::Error> {
        Ok(())
    }
}
