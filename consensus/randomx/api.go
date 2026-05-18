// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package randomx

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// API is the RPC API for the RandomX consensus engine.
type API struct {
	randomx *RandomX
}

// Hash computes the RandomX hash of the given input.
func (api *API) Hash(input hexutil.Bytes) (common.Hash, error) {
	if len(input) == 0 {
		return common.Hash{}, fmt.Errorf("input cannot be empty")
	}
	digest := api.randomx.hashBytes(input)
	return common.BytesToHash(digest), nil
}

// GetSeedHash returns the seed hash for the given block number.
func (api *API) GetSeedHash(blockNumber uint64) common.Hash {
	return api.randomx.seedHash(blockNumber)
}

// GetCurrentEpoch returns the current epoch for the given block number.
func (api *API) GetCurrentEpoch(blockNumber uint64) uint64 {
	return api.randomx.epoch(blockNumber)
}

// GetCacheInfo returns information about the current RandomX configuration.
func (api *API) GetCacheInfo() (map[string]interface{}, error) {
	return map[string]interface{}{
		"epoch_length":    api.randomx.config.EpochLength,
		"cache_size":      api.randomx.config.CacheSizeMB,
		"dataset_size":    api.randomx.config.DatasetSizeGB,
		"hash_iterations": api.randomx.config.HashIterations,
	}, nil
}

// VerifyPow verifies if a given nonce and mix digest are valid for a header.
func (api *API) VerifyPow(headerHash common.Hash, nonce uint64, mixDigest common.Hash, difficulty *hexutil.Big) (bool, error) {
	// This is a helper for external verification
	// Implementation would recreate the header and verify
	return false, fmt.Errorf("not implemented yet")
}
