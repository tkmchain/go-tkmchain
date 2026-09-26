//go:build ziren

package main

import (
	"github.com/ProjectZKM/Ziren/crates/go-runtime/zkvm_runtime"
	"github.com/ethereum/go-ethereum/common"
)

// KeeperPublicValues is the fixed public statement for the generalized EVM
// execution proof. The block, transactions, and stateless witness remain
// private zkVM inputs; only these commitments are exposed to a verifier.
// Version prevents a future statement layout from being accepted by a v1
// verifier.
type KeeperPublicValues struct {
	Version     uint32
	ChainID     uint64
	BlockNumber uint64
	BlockHash   [32]byte
	StateRoot   [32]byte
	ReceiptRoot [32]byte
}

func commitKeeperPublicValues(payload Payload, stateRoot, receiptRoot common.Hash) {
	if payload.Block == nil {
		zkvm_runtime.RuntimeExit(13)
		return
	}
	values := KeeperPublicValues{
		Version:     1,
		ChainID:     payload.ChainID,
		BlockNumber: payload.Block.NumberU64(),
		BlockHash:   [32]byte(payload.Block.Hash()),
		StateRoot:   [32]byte(stateRoot),
		ReceiptRoot: [32]byte(receiptRoot),
	}
	zkvm_runtime.Commit(values)
}
