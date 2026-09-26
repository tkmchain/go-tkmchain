//! Compatibility builder used when the Ziren network feature is disabled.
//!
//! The upstream SDK's build script always calls tonic-build, although the
//! generated service is compiled only with its `network` feature.  Keeping a
//! no-op builder lets local proving builds avoid pulling protobuf/TLS tooling
//! that is never used by the keeper.

#[derive(Default)]
pub struct Builder;

pub fn configure() -> Builder {
    Builder
}

impl Builder {
    pub fn protoc_arg(self, _arg: impl AsRef<str>) -> Self {
        self
    }

    pub fn compile(
        self,
        _protos: &[impl AsRef<str>],
        _includes: &[impl AsRef<str>],
    ) -> Result<(), std::io::Error> {
        Ok(())
    }
}
