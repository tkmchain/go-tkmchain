# TVM example: atomic swap between TKM and XMR

This document shows an example TVM smart contract design for swapping native TKM/ANTD value against Monero (XMR). The TKM side is an HTLC-style contract: it locks TKM until either the XMR side reveals the swap secret or the timeout expires.

Important: Monero does not have Ethereum-style smart contracts. A real TKM/XMR atomic swap needs two parts:

1. A TKM-side TVM contract that enforces hashlock and timelock rules for the TKM funds.
2. An XMR-side swap protocol using Monero-compatible locking/refund/adaptor-signature logic off-chain.

The TVM contract cannot directly inspect Monero chain state. It only enforces the TKM-side lock. The swap protocol must be designed so revealing the same secret on one side gives the counterparty the ability to complete the other side.

## Swap flow

Actors:

- Alice has TKM and wants XMR.
- Bob has XMR and wants TKM.

High-level process:

1. Alice generates a random secret `s` and computes `h = keccak256(s)`.
2. Alice deploys or calls the TVM swap contract, locking TKM with:
   - Bob as the TKM claimer;
   - Alice as the TKM refund address;
   - `hashlock = h`;
   - `refundHeight` or `refundTime`.
3. Bob verifies the TKM contract is funded and contains the expected hashlock and timeout.
4. Bob starts the Monero-side atomic swap flow using the same secret commitment.
5. When Alice claims or completes the XMR side, the protocol reveals `s` to Bob.
6. Bob calls `claim(s)` on the TVM contract.
7. The TVM contract checks `keccak256(s) == hashlock`, transfers the locked TKM to Bob, and stores the revealed secret.
8. If Bob never completes the XMR side or never claims TKM, Alice calls `refund()` after the timeout and gets the locked TKM back.

## Contract state

A single swap record can be stored under a `swapId` derived from Alice, Bob, hashlock, amount, and timeout.

```cpp
struct Swap {
    address tkmDepositor;      // Alice
    address tkmClaimer;        // Bob
    uint256 amount;            // locked TKM/ANTD amount in wei units
    bytes32 hashlock;          // keccak256(secret)
    uint64 refundHeight;       // block height when Alice can refund
    bool claimed;
    bool refunded;
    bytes32 revealedSecret;    // optional, stored for audit/indexers
};
```

Required host functions for a production TVM runtime:

```cpp
namespace tvm_host {
    address caller();
    uint64 block_height();
    uint256 call_value();
    bytes32 keccak256(bytes data);

    bytes32 storage_load(bytes32 key);
    void storage_store(bytes32 key, bytes32 value);

    void transfer(address to, uint256 amount);
    void emit_log(bytes32 topic0, bytes data);
    void revert(string reason);
}
```

The current minimal TVM runtime in this repository only exposes a small conformance instruction set. The example below is the intended contract logic once the TVM host exposes hashing, caller context, value transfer, block height, logs, and ABI dispatch.

## Example TVM C++ contract

```cpp
// SPDX-License-Identifier: MIT
// Example only: TVM host APIs are illustrative and must map to consensus host calls.

#include <stdint.h>
#include <string.h>

using bytes32 = unsigned char[32];
using address = unsigned char[20];

struct uint256 {
    unsigned char bytes[32];
};

struct Swap {
    address depositor;
    address claimer;
    uint256 amount;
    bytes32 hashlock;
    uint64_t refundHeight;
    bool claimed;
    bool refunded;
    bytes32 revealedSecret;
};

namespace tvm_host {
    address caller();
    uint64_t block_height();
    uint256 call_value();
    bytes32 keccak256(const unsigned char* data, uint32_t len);

    bytes32 storage_load(bytes32 key);
    void storage_store(bytes32 key, bytes32 value);

    void transfer(address to, uint256 amount);
    void emit_log(bytes32 topic0, const unsigned char* data, uint32_t len);
    [[noreturn]] void revert(const char* reason);
}

static bool eq32(const bytes32 a, const bytes32 b) {
    unsigned char diff = 0;
    for (int i = 0; i < 32; i++) {
        diff |= a[i] ^ b[i];
    }
    return diff == 0;
}

static bool eq20(const address a, const address b) {
    unsigned char diff = 0;
    for (int i = 0; i < 20; i++) {
        diff |= a[i] ^ b[i];
    }
    return diff == 0;
}

static bytes32 swap_key(bytes32 swapId, uint8_t field) {
    unsigned char buf[33];
    memcpy(buf, swapId, 32);
    buf[32] = field;
    return tvm_host::keccak256(buf, 33);
}

class TkmXmrAtomicSwap {
public:
    // Create and fund a swap. Alice calls this with TKM value attached.
    void createSwap(bytes32 swapId, address claimer, bytes32 hashlock, uint64_t refundHeight) {
        address depositor = tvm_host::caller();
        uint256 amount = tvm_host::call_value();

        if (is_zero(amount)) {
            tvm_host::revert("amount is zero");
        }
        if (refundHeight <= tvm_host::block_height()) {
            tvm_host::revert("refund height is not future");
        }
        if (exists(swapId)) {
            tvm_host::revert("swap already exists");
        }

        store_address(swap_key(swapId, 0), depositor);
        store_address(swap_key(swapId, 1), claimer);
        store_uint256(swap_key(swapId, 2), amount);
        tvm_host::storage_store(swap_key(swapId, 3), hashlock);
        store_uint64(swap_key(swapId, 4), refundHeight);
        store_bool(swap_key(swapId, 5), false); // claimed
        store_bool(swap_key(swapId, 6), false); // refunded

        emit_swap_created(swapId, depositor, claimer, hashlock, refundHeight, amount);
    }

    // Bob claims locked TKM by revealing the secret.
    void claim(bytes32 swapId, const unsigned char* secret, uint32_t secretLen) {
        if (!exists(swapId)) {
            tvm_host::revert("swap does not exist");
        }
        if (load_bool(swap_key(swapId, 5))) {
            tvm_host::revert("already claimed");
        }
        if (load_bool(swap_key(swapId, 6))) {
            tvm_host::revert("already refunded");
        }

        address caller = tvm_host::caller();
        address claimer = load_address(swap_key(swapId, 1));
        if (!eq20(caller, claimer)) {
            tvm_host::revert("caller is not claimer");
        }

        bytes32 expectedHash = tvm_host::storage_load(swap_key(swapId, 3));
        bytes32 actualHash = tvm_host::keccak256(secret, secretLen);
        if (!eq32(actualHash, expectedHash)) {
            tvm_host::revert("invalid secret");
        }

        store_bool(swap_key(swapId, 5), true);
        tvm_host::storage_store(swap_key(swapId, 7), actualHash);

        uint256 amount = load_uint256(swap_key(swapId, 2));
        tvm_host::transfer(claimer, amount);

        emit_swap_claimed(swapId, actualHash);
    }

    // Alice refunds after timeout if Bob never claims.
    void refund(bytes32 swapId) {
        if (!exists(swapId)) {
            tvm_host::revert("swap does not exist");
        }
        if (load_bool(swap_key(swapId, 5))) {
            tvm_host::revert("already claimed");
        }
        if (load_bool(swap_key(swapId, 6))) {
            tvm_host::revert("already refunded");
        }

        uint64_t refundHeight = load_uint64(swap_key(swapId, 4));
        if (tvm_host::block_height() < refundHeight) {
            tvm_host::revert("refund not available yet");
        }

        address caller = tvm_host::caller();
        address depositor = load_address(swap_key(swapId, 0));
        if (!eq20(caller, depositor)) {
            tvm_host::revert("caller is not depositor");
        }

        store_bool(swap_key(swapId, 6), true);

        uint256 amount = load_uint256(swap_key(swapId, 2));
        tvm_host::transfer(depositor, amount);

        emit_swap_refunded(swapId);
    }

private:
    bool exists(bytes32 swapId) {
        // A real implementation should store an explicit exists flag.
        return !is_zero(load_uint256(swap_key(swapId, 2)));
    }

    bool is_zero(uint256 value) {
        unsigned char x = 0;
        for (int i = 0; i < 32; i++) x |= value.bytes[i];
        return x == 0;
    }

    // Placeholder serializers. Production TVM ABI/storage libraries should provide these.
    void store_address(bytes32 key, address value);
    address load_address(bytes32 key);
    void store_uint256(bytes32 key, uint256 value);
    uint256 load_uint256(bytes32 key);
    void store_uint64(bytes32 key, uint64_t value);
    uint64_t load_uint64(bytes32 key);
    void store_bool(bytes32 key, bool value);
    bool load_bool(bytes32 key);

    void emit_swap_created(bytes32 swapId, address depositor, address claimer, bytes32 hashlock, uint64_t refundHeight, uint256 amount);
    void emit_swap_claimed(bytes32 swapId, bytes32 secretHash);
    void emit_swap_refunded(bytes32 swapId);
};
```

## ABI-style interface

For wallets and indexers, expose ABI-compatible selectors equivalent to:

```solidity
interface ITkmXmrAtomicSwap {
    event SwapCreated(bytes32 indexed swapId, address indexed depositor, address indexed claimer, bytes32 hashlock, uint64 refundHeight, uint256 amount);
    event SwapClaimed(bytes32 indexed swapId, bytes32 secretHash);
    event SwapRefunded(bytes32 indexed swapId);

    function createSwap(bytes32 swapId, address claimer, bytes32 hashlock, uint64 refundHeight) external payable;
    function claim(bytes32 swapId, bytes calldata secret) external;
    function refund(bytes32 swapId) external;
}
```

## Example deployment metadata

When building the TVM deployment envelope, include metadata describing the ABI and XMR swap parameters:

```json
{
  "name": "TkmXmrAtomicSwap",
  "version": "1.0.0",
  "abi": [
    "createSwap(bytes32,address,bytes32,uint64)",
    "claim(bytes32,bytes)",
    "refund(bytes32)"
  ],
  "xmr": {
    "network": "mainnet",
    "protocol": "adaptor-signature-atomic-swap",
    "hash": "keccak256(secret)"
  }
}
```

Then wrap the compiled module:

```sh
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"tvm_buildDeployment",
    "params":[{
      "code":"0x...compiled_tvm_module...",
      "metadata":"0x...metadata_json_hex...",
      "memoryPages":4,
      "stackSlots":128,
      "callDepth":8
    }],
    "id":1
  }' | jq '.'
```

Use the returned `deploymentCode` as the data of a contract creation transaction.

## Safety notes

- Use a strong random 32-byte secret. Never reuse it across swaps.
- The TKM refund timeout must be longer than the XMR-side timeout window so Alice can safely recover if Bob disappears.
- The XMR-side protocol must guarantee that revealing the secret on one side enables the counterparty to complete the other side.
- Do not rely on the TVM contract to observe XMR directly. It cannot.
- Store the revealed secret or its hash in contract storage/logs so swap clients and auditors can verify completion.
- Keep the TVM module deterministic and avoid unsupported host behavior.
