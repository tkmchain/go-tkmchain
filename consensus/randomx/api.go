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
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// API is the RPC API for the RandomX consensus engine.
type API struct {
	randomx *RandomX
}

// Hash computes the RandomX hash of the given input using the current cache.
func (api *API) Hash(input hexutil.Bytes) (common.Hash, error) {
	if len(input) == 0 {
		return common.Hash{}, fmt.Errorf("input cannot be empty")
	}
	
	api.randomx.cacheMu.RLock()
	defer api.randomx.cacheMu.RUnlock()
	
	if api.randomx.cache == nil {
		return common.Hash{}, fmt.Errorf("RandomX cache not initialized")
	}
	
	vm, err := api.randomx.getVM()
	if err != nil {
		return common.Hash{}, err
	}
	defer vm.Close()
	
	output := make([]byte, 32)
	vm.CalculateHash(input, output)
	
	return common.BytesToHash(output), nil
}

// GetSeedHash returns the seed hash for the given block number.
func (api *API) GetSeedHash(blockNumber uint64) common.Hash {
	return api.randomx.seedHash(blockNumber)
}

// GetCurrentEpoch returns the current epoch for the given block number.
func (api *API) GetCurrentEpoch(blockNumber uint64) uint64 {
	return api.randomx.epoch(blockNumber)
}

// GetCacheInfo returns information about the current RandomX cache.
func (api *API) GetCacheInfo() (map[string]interface{}, error) {
	api.randomx.cacheMu.RLock()
	defer api.randomx.cacheMu.RUnlock()
	
	if api.randomx.cache == nil {
		return nil, fmt.Errorf("cache not initialized")
	}
	
	return map[string]interface{}{
		"epoch":      api.randomx.cacheEpoch,
		"cache_size": api.randomx.config.CacheSizeMB,
		"dataset_size": api.randomx.config.DatasetSizeGB,
	}, nil
}

// VerifyPow verifies if a given nonce and mix digest are valid for a header.
func (api *API) VerifyPow(headerHash common.Hash, nonce uint64, mixDigest common.Hash, difficulty *hexutil.Big) (bool, error) {
	// This is a helper for external verification
	// Implementation would recreate the header and verify
	return false, fmt.Errorf("not implemented yet")
}

