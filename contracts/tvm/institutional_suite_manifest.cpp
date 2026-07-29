// SPDX-License-Identifier: MIT
// TKMChain TVM institutional-suite manifest fixture.
//
// The current cpp-evm-v1 TVM target intentionally accepts only bounded
// deterministic conformance opcodes. This fixture documents the application
// suite that is deployed as normal contract state today, while TVM stores the
// verifiable module metadata and code hash for explorer visibility.
//
// Runtime operation emitted by tooling: ReturnCodeHash (0x01)
// Manifest target: TKMInstitutionalSuite
// Deployed suite: 0x43aeb055883863cfe40804e386bec801b4ca63ec
//
// Future TVM runtime versions can expand this manifest into native registry
// handlers after deterministic execution, metering, storage layout, and ABI
// compatibility are fully specified and tested.

namespace tkm::tvm::institutional {

struct SuiteManifest {
    const char* name = "TkmInstitutionalSuite";
    const char* version = "1";
    const char* deployedSuite = "0x43aeb055883863cfe40804e386bec801b4ca63ec";
    const char* modules[8] = {
        "InstitutionRegistry",
        "CredentialRegistry",
        "DocumentRegistry",
        "InvoiceSettlement",
        "EscrowVault",
        "ProcurementRegistry",
        "GrantRegistry",
        "AuditDisclosureRegistry",
    };
};

// Tooling emits the deterministic TVM opcode byte 0x01 for this fixture.
// The runtime returns the committed module hash, allowing explorers and users
// to verify that the stored TVM envelope matches this audited manifest.
constexpr unsigned char ReturnCodeHash = 0x01;

} // namespace tkm::tvm::institutional
