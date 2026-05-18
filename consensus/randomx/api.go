// Copyright 2026 The go-ethereum Authors
package randomx

import (
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// API is the RPC API for RandomX
type API struct {
	randomx *RandomX
}

// Hash computes RandomX hash of input
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

// GetSeedHash returns seed hash for block number
func (api *API) GetSeedHash(blockNumber uint64) common.Hash {
	return api.randomx.seedHash(blockNumber)
}

// GetCurrentEpoch returns current epoch
func (api *API) GetCurrentEpoch(blockNumber uint64) uint64 {
	return api.randomx.epoch(blockNumber)
}

// GetCacheInfo returns cache information
func (api *API) GetCacheInfo() (map[string]interface{}, error) {
	api.randomx.cacheMu.RLock()
	defer api.randomx.cacheMu.RUnlock()

	if api.randomx.cache == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	return map[string]interface{}{
		"epoch":       api.randomx.cacheEpoch,
		"cache_size":  api.randomx.config.CacheSizeMB,
		"dataset_size": api.randomx.config.DatasetSizeGB,
	}, nil
}
