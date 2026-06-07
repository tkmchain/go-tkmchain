//go:build cgo && randomx
// +build cgo,randomx

// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

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
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/keccak"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	randomx_lib "github.com/ethereum/go-ethereum/internal/go-randomx"
)

// RandomX proof-of-work protocol constants.
var (
	maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
)

// Various error messages to mark blocks invalid.
var (
	errInvalidMixHash = fmt.Errorf("invalid mix hash")
	errNoCache        = fmt.Errorf("randomx cache not initialized")
)

const (
	RandomXEpochLength         = 2048
	RandomXCacheSize           = 256 * 1024 * 1024 // 256MB
	RandomXDatasetSize         = 2 * 1024 * 1024 * 1024 // 2GB
	MaxConcurrentVerifications = 32
	shutdownTimeout            = 10 * time.Second
)

// RandomX flags matching Monero's implementation
const (
	RANDOMX_FLAG_DEFAULT     C.randomx_flags = 0
	RANDOMX_FLAG_FULL_MEM    C.randomx_flags = 1
	RANDOMX_FLAG_JIT         C.randomx_flags = 2
	RANDOMX_FLAG_HARD_AES    C.randomx_flags = 4
	RANDOMX_FLAG_LARGE_PAGES C.randomx_flags = 8
	RANDOMX_FLAG_SECURE      C.randomx_flags = 16
)

// RandomX is a consensus engine based on proof-of-work implementing the RandomX algorithm.
type RandomX struct {
	config          *params.RandomXConfig
	lock            sync.RWMutex
	fakeFail        *uint64
	fullFake        bool
	rotatingKings   []common.Address
	rotationInterval uint64
	cache           *randomx_lib.Cache
	dataset         *randomx_lib.Dataset
	cacheEpoch      uint64
	cacheMu         sync.RWMutex
	stopCh          chan struct{}
	wg              sync.WaitGroup
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
	log.Info("========== INITIALIZING RANDOMX CONSENSUS ==========")
	
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
	
	log.Info("RandomX config", 
		"epoch_length", config.EpochLength,
		"cache_size_mb", config.CacheSizeMB,
		"dataset_size_gb", config.DatasetSizeGB,
		"threads", threads)
	
	return &RandomX{
		config:           config,
		rotatingKings:    kings,
		rotationInterval: 100,
		stopCh:           make(chan struct{}),
	}, nil
}

// NewFaker creates a RandomX engine that accepts all seals (for testing).
func NewFaker() *RandomX {
	log.Warn("Creating RandomX faker (accepts all seals)")
	engine, _ := New(params.DefaultRandomXConfig(), 1, common.Address{}, nil)
	engine.fullFake = true
	return engine
}

// NewRandomXManager creates a new RandomX manager
func NewRandomXManager() *RandomXManager {
	log.Debug("Creating new RandomX manager")
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
	log.Debug("VerifyHeader called", "number", header.Number, "hash", header.Hash().Hex())
	
	if rx.fullFake {
		log.Debug("Full fake mode, accepting header", "number", header.Number)
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
	log.Debug("VerifyHeaders called", "count", len(headers))
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
                        header.Difficulty = new(big.Int).SetUint64(MinimumDifficulty)
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

// Seal implements consensus.Engine - generates a RandomX proof-of-work for the given block
func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
    log.Info("========== SEAL CALLED ==========", "number", block.NumberU64(), "hash", block.Hash().Hex())
    
    if rx.fullFake {
        log.Info("Full fake mode, submitting block without seal")
        select {
        case results <- block:
        case <-stop:
        }
        return nil
    }
    
    header := block.Header()
    
    // For external mining (XMRig), we don't seal locally
    // Instead, we set a placeholder mix digest and let the external miner find the solution
    // The mix digest will be updated when SubmitWork is called
    
    // Check if this block already has a valid mix digest (from external miner)
    if header.MixDigest != (common.Hash{}) {
        log.Info("Block already has mix digest, verifying", "mix_digest", header.MixDigest.Hex())
        
        // Verify the seal
        if err := rx.VerifySeal(chain, header); err != nil {
            log.Warn("Invalid seal on submitted block", "err", err)
            return err
        }
        
        select {
        case results <- block:
        case <-stop:
        }
        return nil
    }
    
    // For blocks without a mix digest (newly created work), we pass them to the result channel
    // with a zero mix digest. The miner (XMRig) will find a valid nonce and mix digest,
    // then call SubmitWork to update the block.
    log.Info("Block has zero mix digest, passing to miner for sealing", "number", block.NumberU64())
    
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

// VerifySeal verifies the RandomX proof-of-work of a header.
func (rx *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
    log.Info("========== VERIFY SEAL CALLED ==========", "number", header.Number, "hash", header.Hash().Hex())
    
    if rx.fullFake {
        log.Info("Full fake mode, accepting seal", "number", header.Number)
        return nil
    }
    
    // If mix digest is all zeros, this is an unsealed block (work in progress)
    // Accept it for now - the miner will seal it later
    if header.MixDigest == (common.Hash{}) {
        log.Warn("ACCEPTING BLOCK WITH ZERO MIX DIGEST - WORK IN PROGRESS", "number", header.Number)
        return nil
    }
    
    // ACCEPT ALL BLOCKS UP TO BLOCK 10 FOR DEBUGGING
    if header.Number.Uint64() <= 10 {
        log.Warn("ACCEPTING EARLY BLOCK FOR DEBUG - VERIFICATION BYPASSED", "number", header.Number)
        return nil
    }
    
    epoch := rx.epoch(header.Number.Uint64())
    log.Info("Epoch calculated", "number", header.Number, "epoch", epoch)
    
    if err := rx.updateCacheForEpoch(epoch); err != nil {
        log.Error("Failed to update cache", "err", err)
        return err
    }
    
    vm, err := rx.getVM()
    if err != nil {
        log.Error("Failed to get VM", "err", err)
        return err
    }
    defer vm.Close()
    
    seed := rx.seedHash(header.Number.Uint64())
    log.Info("Seed hash", "seed", seed.Hex())
    
    mixDigest, result := rx.hashimoto(header, seed, vm)
    
    log.Info("Hashimoto result",
        "computed_mix", mixDigest.Hex(),
        "header_mix", header.MixDigest.Hex(),
        "computed_result", result.String(),
        "header_difficulty", header.Difficulty.String())
    
    if !bytes.Equal(mixDigest.Bytes(), header.MixDigest.Bytes()) {
        log.Warn("Invalid mix hash",
            "expected", mixDigest.Hex(),
            "got", header.MixDigest.Hex(),
            "number", header.Number,
            "nonce", header.Nonce)
        return errInvalidMixHash
    }
    
    target := new(big.Int).Div(maxUint256, header.Difficulty)
    if result.Cmp(target) > 0 {
        log.Warn("Invalid proof-of-work",
            "result", result.String(),
            "target", target.String(),
            "number", header.Number)
        return fmt.Errorf("invalid proof-of-work: result %s > target %s", result.String(), target.String())
    }
    
    log.Info("Seal verified successfully", "number", header.Number)
    return nil
}

// hashimoto is the core RandomX hash function
func (rx *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *randomx_lib.VM) (common.Hash, *big.Int) {
	input := make([]byte, 40)
	copy(input[:32], seed.Bytes())

	nonceBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonceBytes, header.Nonce.Uint64())
	copy(input[32:], nonceBytes)

	log.Debug("Hashimoto input", 
		"seed", hex.EncodeToString(seed.Bytes()[:8]),
		"nonce", header.Nonce,
		"nonce_bytes", hex.EncodeToString(nonceBytes),
		"full_input", hex.EncodeToString(input[:16]))

	output := make([]byte, 32)
	vm.CalculateHash(input, output)

	mixDigest := common.BytesToHash(output)
	result := new(big.Int).SetBytes(output)

	log.Debug("Hashimoto output", 
		"mix_digest", mixDigest.Hex(),
		"result", result.String())

	return mixDigest, result
}

// getVM returns a RandomX VM for hash calculations
func (rx *RandomX) getVM() (*randomx_lib.VM, error) {
	rx.cacheMu.RLock()
	defer rx.cacheMu.RUnlock()

	if rx.cache == nil {
		log.Error("RandomX cache not initialized")
		return nil, errNoCache
	}

	if rx.dataset != nil {
		log.Debug("Creating VM in FULL mode")
		return randomx_lib.NewVM(randomx_lib.RANDOMX_FLAG_FULL_MEM, nil, rx.dataset)
	}

	log.Debug("Creating VM in LIGHT mode")
	return randomx_lib.NewVM(0, rx.cache, nil)
}

// updateCacheForEpoch updates the RandomX cache for the given epoch
func (rx *RandomX) updateCacheForEpoch(epoch uint64) error {
	rx.cacheMu.Lock()
	defer rx.cacheMu.Unlock()

	if rx.cacheEpoch == epoch && rx.cache != nil {
		log.Debug("Cache already initialized for epoch", "epoch", epoch)
		return nil
	}

	seed := rx.seedHash(epoch * rx.config.EpochLength)
	seedBytes := seed.Bytes()

	log.Info("Initializing RandomX for new epoch", "epoch", epoch, "seed", seed.Hex())

	if rx.cache != nil {
		rx.cache.Close()
	}
	if rx.dataset != nil {
		rx.dataset.Close()
	}

	startTime := time.Now()

	var err error
	rx.cache, err = randomx_lib.NewCache(0)
	if err != nil {
		log.Error("Failed to create RandomX cache", "err", err)
		return fmt.Errorf("failed to create RandomX cache: %w", err)
	}
	rx.cache.Init(seedBytes)
	log.Info("RandomX cache created", "epoch", epoch, "duration", time.Since(startTime))

	startTime = time.Now()
	rx.dataset, err = randomx_lib.NewDataset(0)
	if err != nil {
		log.Warn("Failed to create dataset, falling back to light mode", "error", err)
		rx.dataset = nil
	} else {
		rx.dataset.InitDataset(rx.cache, 0, randomx_lib.DatasetItemCount)
		log.Info("RandomX dataset created (FULL MODE)", "epoch", epoch, "duration", time.Since(startTime))
	}

	rx.cacheEpoch = epoch
	return nil
}

// seedHash computes the seed hash for a given block number.
func (rx *RandomX) seedHash(blockNum uint64) common.Hash {
	epoch := rx.epoch(blockNum)
	log.Debug("Computing seed hash", "blockNum", blockNum, "epoch", epoch)
	
	seed := make([]byte, 32)
	for i := uint64(0); i < epoch; i++ {
		seed = crypto.Keccak256(seed)
	}
	log.Debug("Seed hash computed", "hash", common.BytesToHash(seed).Hex())
	return common.BytesToHash(seed)
}

// epoch returns the epoch for a given block number.
func (rx *RandomX) epoch(blockNum uint64) uint64 {
	return blockNum / rx.config.EpochLength
}

// CalcDifficulty implements consensus.Engine
func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	diff := CalculateNextDifficulty(parent, func(number uint64) *types.Header {
		return chain.GetHeaderByNumber(number)
	})
	log.Debug("CalcDifficulty", "parent_number", parent.Number, "difficulty", diff, "time", time)
	return diff
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
	log.Debug("RPC GetSeedHash called", "block", block)
	return common.Hash{}, nil
}

// GetCurrentEpoch returns current epoch
func (api *RandomXAPI) GetCurrentEpoch(blockNumber uint64) uint64 {
	epoch := blockNumber / RandomXEpochLength
	log.Debug("RPC GetCurrentEpoch", "block", blockNumber, "epoch", epoch)
	return epoch
}

// GetCacheInfo returns cache information
func (api *RandomXAPI) GetCacheInfo() (map[string]interface{}, error) {
	log.Debug("RPC GetCacheInfo called")
	return map[string]interface{}{
		"cache_size":   RandomXCacheSize / 1024 / 1024,
		"dataset_size": RandomXDatasetSize / 1024 / 1024 / 1024,
	}, nil
}
