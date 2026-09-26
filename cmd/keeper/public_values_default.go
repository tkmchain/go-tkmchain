//go:build !ziren

package main

import "github.com/ethereum/go-ethereum/common"

// commitKeeperPublicValues is deliberately a no-op outside the zkVM guest.
// Keeping the hook in the normal build prevents proof-only behavior from
// changing ordinary stateless execution.
func commitKeeperPublicValues(_ Payload, _ common.Hash, _ common.Hash) {}
