//go:build cgo && randomx
// +build cgo,randomx

// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
package randomx

/*
#cgo CFLAGS: -I${SRCDIR}/../../build/_workspace/randomx/src
#cgo LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build -lrandomx -lstdc++ -lm
#include <stdlib.h>
#include <string.h>
#include "randomx.h"
*/
import "C"
import (
        "fmt"
        "math/big"
        "sync"
        "time"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethereum/go-ethereum/consensus"
        "github.com/ethereum/go-ethereum/core/types"
        "github.com/ethereum/go-ethereum/core/vm"
        "github.com/ethereum/go-ethereum/crypto/keccak"
        "github.com/ethereum/go-ethereum/params"
        "github.com/ethereum/go-ethereum/rlp"
        "github.com/ethereum/go-ethereum/rpc" 
)

// RandomX proof-of-work protocol constants.
var (
        maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
)

// Various error messages.
var (
        errInvalidMixHash = fmt.Errorf("invalid mix hash")
        errNoCache        = fmt.Errorf("randomx cache not initialized")
)

const (
        RandomXEpochLength         = 2048
        RandomXCacheSize           = 256 * 1024 * 1024
        RandomXDatasetSize         = 2 * 1024 * 1024 * 1024
        MaxConcurrentVerifications = 32
        shutdownTimeout            = 10 * time.Second
)

// RandomX flags
const (
        RANDOMX_FLAG_DEFAULT     C.randomx_flags = 0
        RANDOMX_FLAG_FULL_MEM    C.randomx_flags = 1
        RANDOMX_FLAG_JIT         C.randomx_flags = 2
        RANDOMX_FLAG_HARD_AES    C.randomx_flags = 4
        RANDOMX_FLAG_LARGE_PAGES C.randomx_flags = 8
        RANDOMX_FLAG_SECURE      C.randomx_flags = 16
)

// MinimumDifficulty is the absolute lowest allowed difficulty
//var MinimumDifficulty = big.NewInt(1310)

// RandomX is a consensus engine based on proof-of-work implementing the RandomX algorithm.
type RandomX struct {
        config           *params.RandomXConfig
        lock             sync.RWMutex
        fakeFail         *uint64
        fullFake         bool
        rotatingKings    []common.Address
        rotationInterval uint64
        cache            *C.randomx_cache
        dataset          *C.randomx_dataset
        cacheEpoch       uint64
        cacheMu          sync.RWMutex
        stopCh           chan struct{}
        wg               sync.WaitGroup
}

// RandomXCache is a wrapper for C RandomX cache
type RandomXCache struct {
        cache     *C.randomx_cache
        dataset   *C.randomx_dataset
        vmFull    *C.randomx_vm
        vmLight   *C.randomx_vm
        seedHash  []byte
        epoch     uint64
        createdAt time.Time
        lastUsed  time.Time
        mu        sync.RWMutex
}

// RandomXManager manages RandomX caches
type RandomXManager struct {
        mainCache         *RandomXCache
        secondaryCache    *RandomXCache
        mainSeedHash      []byte
        secondarySeedHash []byte
        mu                sync.RWMutex
        semaphore         chan struct{}
        maxThreads        int
}

// New creates a new RandomX consensus engine.
func New(config *params.RandomXConfig, threads int, mainKing common.Address, kingAddresses []common.Address) (*RandomX, error) {
        if config == nil {
                config = params.DefaultRandomXConfig()
        }
        if config.EpochLength == 0 {
                config.EpochLength = 2048
        }
        if config.CacheSizeMB == 0 {
                config.CacheSizeMB = 256
        }
        if config.DatasetSizeGB == 0 {
                config.DatasetSizeGB = 2
        }
        
        kings := make([]common.Address, len(kingAddresses))
        copy(kings, kingAddresses)
        if mainKing != (common.Address{}) {
                kings = append([]common.Address{mainKing}, kings...)
        }
        
        return &RandomX{
                config:           config,
                rotatingKings:    kings,
                rotationInterval: 100,
                stopCh:           make(chan struct{}),
        }, nil
}

// NewFaker creates a RandomX engine that accepts all seals (for testing).
func NewFaker() *RandomX {
        engine, _ := New(params.DefaultRandomXConfig(), 1, common.Address{}, nil)
        engine.fullFake = true
        return engine
}

// NewFullFaker creates a RandomX engine that accepts all seals and runs full fake mode.
func NewFullFaker() *RandomX {
        engine := NewFaker()
        engine.fullFake = true
        return engine
}

// NewRandomXManager creates a new RandomX manager
func NewRandomXManager() *RandomXManager {
        return &RandomXManager{
                semaphore:  make(chan struct{}, MaxConcurrentVerifications),
                maxThreads: 4,
        }
}

// Author implements consensus.Engine
func (rx *RandomX) Author(header *types.Header) (common.Address, error) {
        return header.Coinbase, nil
}

// VerifyHeader implements consensus.Engine
func (rx *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
        if rx.fullFake {
                return nil
        }
        if rx.fakeFail != nil && header.Number.Uint64() == *rx.fakeFail {
                return consensus.ErrInvalidNumber
        }
        if header.Number.Sign() == 0 {
                return nil
        }
        parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
        if parent == nil {
                return consensus.ErrUnknownAncestor
        }
        if parent.Number.Uint64()+1 != header.Number.Uint64() {
                return consensus.ErrInvalidNumber
        }
        return rx.VerifySeal(chain, header)
}

// VerifyHeaders implements consensus.Engine
func (rx *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
        abort := make(chan struct{})
        results := make(chan error, len(headers))
        go func() {
                for _, header := range headers {
                        select {
                        case <-abort:
                                return
                        case results <- rx.VerifyHeader(chain, header):
                        }
                }
        }()
        return abort, results
}

// VerifyUncles implements consensus.Engine
func (rx *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
        if len(block.Uncles()) > 0 {
                return consensus.ErrUnknownAncestor
        }
        return nil
}

// Prepare implements consensus.Engine
func (rx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
        if header.Number == nil {
                header.Number = new(big.Int)
        }
        if header.Difficulty == nil {
                if parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1); parent != nil {
                        header.Difficulty = rx.CalcDifficulty(chain, header.Time, parent)
                } else {
                        header.Difficulty = new(big.Int).Set(MinimumDifficulty)
                }
        }
        return nil
}

// Finalize implements consensus.Engine
func (rx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {
}

// FinalizeAndAssemble implements consensus.Engine
func (rx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
        rx.Finalize(chain, header, state, body)
        return types.NewBlock(header, body, receipts, nil), nil
}

// Seal implements consensus.Engine
func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
        select {
        case results <- block:
        case <-stop:
        }
        return nil
}

// SealHash returns the hash of a block prior to it being sealed
func (rx *RandomX) SealHash(header *types.Header) common.Hash {
        hasher := keccak.NewLegacyKeccak256()

        enc := []interface{}{
                header.ParentHash,
                header.UncleHash,
                header.Coinbase,
                header.Root,
                header.TxHash,
                header.ReceiptHash,
                header.Bloom,
                header.Difficulty,
                header.Number,
                header.GasLimit,
                header.GasUsed,
                header.Time,
                header.Extra,
        }

        if header.BaseFee != nil {
                enc = append(enc, header.BaseFee)
        }
        if header.WithdrawalsHash != nil {
                enc = append(enc, header.WithdrawalsHash)
        }
        if header.ExcessBlobGas != nil {
                enc = append(enc, header.ExcessBlobGas)
        }
        if header.BlobGasUsed != nil {
                enc = append(enc, header.BlobGasUsed)
        }

        rlp.Encode(hasher, enc)

        var hash common.Hash
        hasher.Sum(hash[:0])
        return hash
}

// VerifySeal verifies the RandomX proof-of-work
func (rx *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
        if rx.fullFake {
                return nil
        }
        // For now, accept all seals (will be implemented with actual RandomX verification)
        return nil
}

// CalcDifficulty implements consensus.Engine
func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
        // Use the imported CalcDifficulty from difficulty.go
        return CalculateNextDifficulty(parent, func(number uint64) *types.Header {
                return chain.GetHeaderByNumber(number)
        })
}

// Close implements consensus.Engine
func (rx *RandomX) Close() error {
        close(rx.stopCh)
        rx.wg.Wait()
        return nil
}

// APIs returns the RPC APIs provided by the RandomX engine
func (rx *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
        return []rpc.API{
                {
                        Namespace: "randomx",
                        Version:   "1.0",
                        Service:   &RandomXAPI{randomx: rx},
                        Public:    true,
                },
        }
}

// RandomXAPI is the RPC API for RandomX
type RandomXAPI struct {
        randomx *RandomX
}

// GetSeedHash returns the RandomX seed hash for the next block
func (api *RandomXAPI) GetSeedHash(block *uint64) (common.Hash, error) {
        return common.Hash{}, nil
}

// GetCurrentEpoch returns current epoch
func (api *RandomXAPI) GetCurrentEpoch(blockNumber uint64) uint64 {
        return blockNumber / RandomXEpochLength
}

// GetCacheInfo returns cache information
func (api *RandomXAPI) GetCacheInfo() (map[string]interface{}, error) {
        return map[string]interface{}{
                "cache_size":   RandomXCacheSize / 1024 / 1024,
                "dataset_size": RandomXDatasetSize / 1024 / 1024 / 1024,
        }, nil
}

// Helper functions for RandomXManager
func (m *RandomXManager) GetCache(epoch uint64, seedHash []byte) (*RandomXCache, error) {
        return nil, nil
}

func (c *RandomXCache) ComputeHash(seedHash, nonce []byte) ([]byte, error) {
        return nil, nil
}
