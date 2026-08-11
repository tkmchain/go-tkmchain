// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package miner

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

// RandomXSeedHash returns the RandomX seed hash for a given block number
func RandomXSeedHash(config *params.ChainConfig, blockNumber uint64) common.Hash {
	if config == nil || config.RandomX == nil {
		// Return a default seed hash if RandomX is not configured.
		return crypto.Keccak256Hash([]byte("randomx_default_seed"))
	}

	if config.RandomXTxBlock != nil {
		activation := config.RandomXTxBlock.Uint64()
		if blockNumber >= activation {
			blockNumber -= activation
		} else {
			blockNumber = 0
		}
	}

	epochLength := config.RandomX.EpochLength
	if epochLength == 0 {
		epochLength = 2048
	}

	epoch := blockNumber / epochLength
	if epoch == 0 {
		return crypto.Keccak256Hash([]byte("randomx_epoch_0_genesis"))
	}

	// Calculate seed hash for the epoch.
	seed := make([]byte, 32)
	for i := uint64(0); i < epoch; i++ {
		if i == 0 {
			seed = crypto.Keccak256([]byte("randomx_epoch_0_genesis"))
		} else {
			seed = crypto.Keccak256(seed)
		}
	}
	return common.BytesToHash(seed)
}
