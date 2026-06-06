//go:build cgo && randomx
// +build cgo,randomx

// Copyright 2026 The go-ethereum Authors
package randomx

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// API is the RPC API for RandomX.
type API struct {
	randomx *RandomXManager
}

// Hash computes the RandomX hash for a 32-byte seed hash and 8-byte nonce input.
func (api *API) Hash(input hexutil.Bytes) (common.Hash, error) {
	if api.randomx == nil {
		return common.Hash{}, fmt.Errorf("RandomX manager not initialized")
	}
	if len(input) != 40 {
		return common.Hash{}, fmt.Errorf("input must be 40 bytes: 32-byte seed hash and 8-byte nonce")
	}

	seedHash := input[:32]
	nonce := input[32:40]
	cache, err := api.randomx.GetCache(0, seedHash)
	if err != nil {
		return common.Hash{}, err
	}
	hash, err := cache.ComputeHash(seedHash, nonce)
	if err != nil {
		return common.Hash{}, err
	}
	return common.BytesToHash(hash), nil
}

// GetSeedHash returns seed hash for block number.
func (api *API) GetSeedHash(blockNumber uint64) common.Hash {
	return seedHash(blockNumber)
}

// GetCurrentEpoch returns current epoch.
func (api *API) GetCurrentEpoch(blockNumber uint64) uint64 {
	return epoch(blockNumber)
}

// GetCacheInfo returns cache information.
func (api *API) GetCacheInfo() (map[string]interface{}, error) {
	if api.randomx == nil {
		return nil, fmt.Errorf("RandomX manager not initialized")
	}
	api.randomx.mu.RLock()
	defer api.randomx.mu.RUnlock()

	if api.randomx.mainCache == nil && api.randomx.secondaryCache == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	return map[string]interface{}{
		"main_seed_hash":      hexutil.Encode(api.randomx.mainSeedHash),
		"secondary_seed_hash": hexutil.Encode(api.randomx.secondarySeedHash),
		"cache_size":          RandomXCacheSize,
		"dataset_size":        RandomXDatasetSize,
	}, nil
}

// GetDatasetInfo returns information about dataset storage.
func (api *API) GetDatasetInfo() map[string]interface{} {
	if api.randomx == nil {
		return map[string]interface{}{
			"in_ram":        DefaultRAMCacheEnabled,
			"cache_size_mb": RandomXCacheSize / 1024 / 1024,
			"size_gb":       DefaultRAMCacheSizeGB,
		}
	}
	api.randomx.mu.RLock()
	defer api.randomx.mu.RUnlock()

	mainDataset := api.randomx.mainCache != nil && api.randomx.mainCache.dataset != nil
	secondaryDataset := api.randomx.secondaryCache != nil && api.randomx.secondaryCache.dataset != nil
	return map[string]interface{}{
		"main_seed_hash":      hexutil.Encode(api.randomx.mainSeedHash),
		"secondary_seed_hash": hexutil.Encode(api.randomx.secondarySeedHash),
		"size_gb":             DefaultRAMCacheSizeGB,
		"in_ram":              DefaultRAMCacheEnabled,
		"cache_size_mb":       RandomXCacheSize / 1024 / 1024,
		"main_dataset":        mainDataset,
		"secondary_dataset":   secondaryDataset,
	}
}

func epoch(blockNumber uint64) uint64 {
	return blockNumber / RandomXEpochLength
}

func seedHash(blockNumber uint64) common.Hash {
	var enc [8]byte
	binary.LittleEndian.PutUint64(enc[:], epoch(blockNumber))
	return crypto.Keccak256Hash(enc[:])
}
