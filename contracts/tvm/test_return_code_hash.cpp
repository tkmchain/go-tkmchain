// Deterministic TVM C++ fixture for the cpp-evm-v1 target.
//
// The current TVM runtime accepts a bounded instruction module, not arbitrary
// native machine code. This source is the auditable C++ contract fixture used by
// tests and deployment tooling to emit the module bytes for a contract that
// returns its TVM code hash when executed.

#include <array>
#include <cstdint>
#include <span>

namespace tkm::tvm {

enum class Opcode : std::uint8_t {
    ReturnInput = 0x00,
    ReturnCodeHash = 0x01,
    StorageLoad = 0x02,
    StorageStore = 0x03,
};

constexpr std::array<std::uint8_t, 1> test_return_code_hash_contract{
    static_cast<std::uint8_t>(Opcode::ReturnCodeHash),
};

constexpr std::span<const std::uint8_t> module() {
    return test_return_code_hash_contract;
}

} // namespace tkm::tvm
