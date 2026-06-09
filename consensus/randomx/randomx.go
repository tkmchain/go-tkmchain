//go:build cgo && randomx
// +build cgo,randomx

// Copyright 2026 The go-ethereum Authors

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
    "fmt"
    "math/big"
    "sync"
    "time"
    "unsafe"

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
)

// Constants
var (
    maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
    GenesisDifficulty = big.NewInt(3) // Adjust if needed for your chain
)

var (
    errInvalidMixHash = fmt.Errorf("invalid mix hash")
    errNoCache        = fmt.Errorf("randomx cache not initialized")
)

const (
    RandomXEpochLength = 2048
)

// RandomX flags - Use JIT + HARD_AES for best performance
const (
    RANDOMX_FLAG_DEFAULT  = 0
    RANDOMX_FLAG_JIT      = 2
    RANDOMX_FLAG_HARD_AES = 4
    RANDOMX_FLAG_FULL_MEM = 1
)

// Cache, Dataset, VM wrappers
type Cache struct {
    ptr *C.randomx_cache
}

type Dataset struct {
    ptr *C.randomx_dataset
}

type VM struct {
    ptr *C.randomx_vm
}

// NewCache creates a RandomX cache
func NewCache(flags int) *Cache {
    cache := C.randomx_alloc_cache(C.randomx_flags(flags))
    if cache == nil {
        return nil
    }
    return &Cache{ptr: cache}
}

func (c *Cache) Init(seed []byte) {
    if c == nil || c.ptr == nil {
        return
    }
    var seedPtr unsafe.Pointer
    if len(seed) > 0 {
        seedPtr = unsafe.Pointer(&seed[0])
    }
    C.randomx_init_cache(c.ptr, seedPtr, C.size_t(len(seed)))
}

func (c *Cache) Close() {
    if c != nil && c.ptr != nil {
        C.randomx_release_cache(c.ptr)
        c.ptr = nil
    }
}

// NewDataset creates a RandomX dataset
func NewDataset(flags int) *Dataset {
    dataset := C.randomx_alloc_dataset(C.randomx_flags(flags))
    if dataset == nil {
        return nil
    }
    return &Dataset{ptr: dataset}
}

func (d *Dataset) InitDataset(cache *Cache, start, count uint64) {
    if d == nil || d.ptr == nil || cache == nil || cache.ptr == nil {
        return
    }
    C.randomx_init_dataset(d.ptr, cache.ptr, C.ulong(start), C.ulong(count))
}

func (d *Dataset) Close() {
    if d != nil && d.ptr != nil {
        C.randomx_release_dataset(d.ptr)
        d.ptr = nil
    }
}

// NewVM creates a RandomX virtual machine
func NewVM(flags int, cache *Cache, dataset *Dataset) *VM {
    var cCache *C.randomx_cache
    var cDataset *C.randomx_dataset
    if cache != nil {
        cCache = cache.ptr
    }
    if dataset != nil {
        cDataset = dataset.ptr
    }

    vm := C.randomx_create_vm(C.randomx_flags(flags), cCache, cDataset)
    if vm == nil {
        return nil
    }
    return &VM{ptr: vm}
}

func (vm *VM) CalculateHash(input, output []byte) {
    if vm == nil || vm.ptr == nil {
        log.Error("CalculateHash called with nil VM")
        return
    }
    var inputPtr unsafe.Pointer
    if len(input) > 0 {
        inputPtr = unsafe.Pointer(&input[0])
    }
    C.randomx_calculate_hash(vm.ptr, inputPtr, C.size_t(len(input)), unsafe.Pointer(&output[0]))
}

func (vm *VM) Close() {
    if vm != nil && vm.ptr != nil {
        C.randomx_destroy_vm(vm.ptr)
        vm.ptr = nil
    }
}

// RandomX consensus engine
type RandomX struct {
    config           *params.RandomXConfig
    lock             sync.RWMutex
    fullFake         bool
    rotatingKings    []common.Address
    rotationInterval uint64

    cache      *Cache
    dataset    *Dataset
    cacheEpoch uint64
    cacheMu    sync.RWMutex

    stopCh chan struct{}
}

// New creates a new RandomX consensus engine with proper initialization
func New(config *params.RandomXConfig, threads int, mainKing common.Address, kingAddresses []common.Address) (*RandomX, error) {
    log.Info("========== INITIALIZING RANDOMX CONSENSUS ==========")

    if config == nil {
        config = params.DefaultRandomXConfig()
    }

    // Apply defaults
    if config.EpochLength == 0 {
        config.EpochLength = RandomXEpochLength
    }

    kings := make([]common.Address, len(kingAddresses))
    copy(kings, kingAddresses)
    if mainKing != (common.Address{}) {
        kings = append([]common.Address{mainKing}, kings...)
    }

    rx := &RandomX{
        config:           config,
        rotatingKings:    kings,
        rotationInterval: 100,
        stopCh:           make(chan struct{}),
    }

    // Initialize RandomX for epoch 0 immediately
    if err := rx.updateCacheForEpoch(0); err != nil {
        log.Error("Failed to initialize RandomX cache at startup", "err", err)
        return nil, err
    }

    log.Info("RandomX engine initialized successfully",
        "epoch_length", config.EpochLength,
        "threads", threads)

    return rx, nil
}

// NewFaker creates a RandomX engine that accepts all seals (for testing)
func NewFaker() *RandomX {
    log.Warn("Creating RandomX faker (accepts all seals)")
    engine, _ := New(params.DefaultRandomXConfig(), 1, common.Address{}, nil)
    engine.fullFake = true
    return engine
}

// ==================== Core Methods ====================

func (rx *RandomX) getVM() (*VM, error) {
    rx.cacheMu.RLock()
    defer rx.cacheMu.RUnlock()

    if rx.cache == nil {
        return nil, errNoCache
    }

    flags := RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES

    if rx.dataset != nil {
        vm := NewVM(flags, nil, rx.dataset)
        if vm == nil {
            return nil, fmt.Errorf("failed to create VM with full dataset")
        }
        return vm, nil
    }

    // Light mode
    vm := NewVM(flags, rx.cache, nil)
    if vm == nil {
        return nil, fmt.Errorf("failed to create VM with cache")
    }
    return vm, nil
}

func (rx *RandomX) updateCacheForEpoch(epoch uint64) error {
    rx.cacheMu.Lock()
    defer rx.cacheMu.Unlock()

    if rx.cacheEpoch == epoch && rx.cache != nil {
        return nil
    }

    seed := rx.seedHash(epoch * rx.config.EpochLength)
    seedBytes := seed.Bytes()

    log.Info("�� Initializing RandomX for new epoch",
        "epoch", epoch,
        "seed", seed.Hex()[:16]+"...")

    // Cleanup old resources
    if rx.cache != nil {
        rx.cache.Close()
        rx.cache = nil
    }
    if rx.dataset != nil {
        rx.dataset.Close()
        rx.dataset = nil
    }

    // Create cache
    rx.cache = NewCache(RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES)
    if rx.cache == nil {
        return fmt.Errorf("failed to allocate RandomX cache (out of memory?)")
    }
    rx.cache.Init(seedBytes)

    // Try to allocate full dataset (much faster)
    rx.dataset = NewDataset(RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES)
    if rx.dataset != nil {
        log.Info("�� Allocating full RandomX dataset (~2GB+). This may take 30-90 seconds...")
        start := time.Now()
        rx.dataset.InitDataset(rx.cache, 0, 0) // full dataset
        log.Info("✅ Full dataset initialized", "duration", time.Since(start))
    } else {
        log.Warn("⚠️ Could not allocate full dataset. Falling back to light mode (slower)")
    }

    rx.cacheEpoch = epoch
    log.Info("✅ RandomX ready", "epoch", epoch, "mode", map[bool]string{true: "full", false: "light"}[rx.dataset != nil])
    return nil
}

// hashimoto - core hashing function
func (rx *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *VM) (common.Hash, *big.Int) {
    input := make([]byte, 40)
    sealHash := rx.SealHash(header)
    copy(input[:32], sealHash.Bytes())
    copy(input[32:], header.Nonce[:])

    output := make([]byte, 32)

    if vm == nil || vm.ptr == nil {
        log.Error("VM is nil in hashimoto!")
        return common.Hash{}, new(big.Int)
    }

    vm.CalculateHash(input, output)

    mixDigest := common.BytesToHash(output)
    result := new(big.Int).SetBytes(output)

    log.Debug("RandomX hash",
        "sealHash", sealHash.Hex()[:16],
        "nonce", header.Nonce.Uint64(),
        "mixDigest", mixDigest.Hex(),
        "isZero", mixDigest == (common.Hash{}))

    return mixDigest, result
}

// ==================== Consensus Engine Interface ====================

func (rx *RandomX) Author(header *types.Header) (common.Address, error) {
    return header.Coinbase, nil
}

func (rx *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
    return rx.verifyHeader(chain, header, nil)
}

func (rx *RandomX) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
    if rx.fullFake {
        return nil
    }
    if header.Number == nil {
        return consensus.ErrInvalidNumber
    }
    if header.Number.Sign() == 0 {
        return nil
    }

    // Parent check
    number := header.Number.Uint64()
    var parent *types.Header
    if len(parents) > 0 {
        parent = parents[len(parents)-1]
    } else {
        parent = chain.GetHeader(header.ParentHash, number-1)
    }
    if parent == nil || parent.Number.Uint64()+1 != number || parent.Hash() != header.ParentHash {
        return consensus.ErrUnknownAncestor
    }

    return rx.VerifySeal(chain, header)
}

func (rx *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
    if rx.fullFake {
        return nil
    }

    // Genesis
    if header.Number.Uint64() == 0 {
        if header.MixDigest != (common.Hash{}) {
            return errInvalidMixHash
        }
        return nil
    }

    // Bootstrap phase (blocks 1-20)
    if header.Number.Uint64() <= 20 {
        log.Info("ACCEPTING EARLY BLOCK FOR BOOTSTRAP", "number", header.Number, "mix", header.MixDigest.Hex()[:16])
        return nil
    }

    // Reject zero mix digest after bootstrap
    if header.MixDigest == (common.Hash{}) {
        log.Error("REJECTING BLOCK WITH ZERO MIX DIGEST", "number", header.Number)
        return errInvalidMixHash
    }

    // Full verification
    epoch := rx.epoch(header.Number.Uint64())
    if err := rx.updateCacheForEpoch(epoch); err != nil {
        return err
    }

    vm, err := rx.getVM()
    if err != nil {
        log.Error("Failed to get VM for verification", "err", err)
        return err
    }
    defer vm.Close()

    seed := rx.seedHash(header.Number.Uint64())
    mixDigest, result := rx.hashimoto(header, seed, vm)

    if !bytes.Equal(mixDigest.Bytes(), header.MixDigest.Bytes()) {
        return errInvalidMixHash
    }

    target := new(big.Int).Div(maxUint256, header.Difficulty)
    if result.Cmp(target) > 0 {
        return fmt.Errorf("invalid proof-of-work")
    }

    return nil
}

func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
    if rx.fullFake {
        select { case results <- block: default: }
        return nil
    }

    header := block.Header()

    if header.MixDigest != (common.Hash{}) {
        if err := rx.VerifySeal(chain, header); err != nil {
            return err
        }
        select { case results <- block: default: }
        return nil
    }

    // Initialize cache/dataset
    epoch := rx.epoch(header.Number.Uint64())
    if err := rx.updateCacheForEpoch(epoch); err != nil {
        return err
    }

    vm, err := rx.getVM()
    if err != nil {
        return fmt.Errorf("failed to get RandomX VM: %w", err)
    }
    defer vm.Close()

    sealHeader := types.CopyHeader(header)
    seed := rx.seedHash(sealHeader.Number.Uint64())
    target := new(big.Int).Div(maxUint256, sealHeader.Difficulty)

    for nonce := sealHeader.Nonce.Uint64(); ; nonce++ {
        select {
        case <-stop:
            return nil
        default:
        }

        sealHeader.Nonce = types.EncodeNonce(nonce)
        mixDigest, result := rx.hashimoto(sealHeader, seed, vm)

        if result.Cmp(target) <= 0 {
            sealHeader.MixDigest = mixDigest
            sealedBlock := block.WithSeal(sealHeader)
            select {
            case results <- sealedBlock:
            case <-stop:
            }
            return nil
        }
    }
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

// Helper methods
func (rx *RandomX) epoch(blockNum uint64) uint64 {
    return blockNum / rx.config.EpochLength
}

func (rx *RandomX) seedHash(blockNum uint64) common.Hash {
    epoch := rx.epoch(blockNum)
    seed := make([]byte, 32)
    for i := uint64(0); i < epoch; i++ {
        seed = crypto.Keccak256(seed)
    }
    return common.BytesToHash(seed)
}

func (rx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
    if header.Number == nil {
        header.Number = new(big.Int)
    }
    if header.Difficulty == nil {
        if parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1); parent != nil {
            header.Difficulty = rx.CalcDifficulty(chain, header.Time, parent)
        } else {
            header.Difficulty = GenesisDifficulty
        }
    }
    return nil
}

func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
    return CalculateNextDifficulty(parent, func(number uint64) *types.Header {
        return chain.GetHeaderByNumber(number)
    })
}

func (rx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {
    // No-op or custom logic
}

func (rx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
    if len(receipts) > 0 {
        header.Bloom = types.MergeBloom(receipts)
    }
    return types.NewBlock(header, body, receipts, nil), nil
}

func (rx *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
    if len(block.Uncles()) > 0 {
        return consensus.ErrUnknownAncestor
    }
    return nil
}

func (rx *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
    abort := make(chan struct{})
    results := make(chan error, len(headers))
    go func() {
        for i, header := range headers {
            err := rx.verifyHeader(chain, header, headers[:i])
            select {
            case <-abort:
                return
            case results <- err:
            }
        }
    }()
    return abort, results
}

func (rx *RandomX) Close() error {
    close(rx.stopCh)
    // Cleanup resources
    rx.cacheMu.Lock()
    if rx.cache != nil {
        rx.cache.Close()
    }
    if rx.dataset != nil {
        rx.dataset.Close()
    }
    rx.cacheMu.Unlock()
    return nil
}

// RPC APIs
func (rx *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
    return []rpc.API{{
        Namespace: "randomx",
        Version:   "1.0",
        Service:   &RandomXAPI{randomx: rx},
        Public:    true,
    }}
}

// King management and RPC API (unchanged from your version)
type RandomXAPI struct{ randomx *RandomX }

func (api *RandomXAPI) GetSeedHash(block *uint64) (common.Hash, error) {
    if api.randomx == nil {
        return common.Hash{}, nil
    }
    bn := uint64(0)
    if block != nil {
        bn = *block
    }
    return api.randomx.seedHash(bn), nil
}

func (api *RandomXAPI) GetCurrentEpoch(blockNumber uint64) uint64 {
    return blockNumber / RandomXEpochLength
}

func (api *RandomXAPI) GetCacheInfo() (map[string]interface{}, error) {
    return map[string]interface{}{
        "cache_size_mb": 256,
        "dataset_size_gb": 2,
    }, nil
}

func (api *RandomXAPI) GetMainKing() common.Address {
    return api.randomx.GetMainKing()
}

func (api *RandomXAPI) GetRotatingKing(blockHeight uint64) common.Address {
    return api.randomx.GetRotatingKing(blockHeight)
}

// King methods
func (rx *RandomX) GetMainKing() common.Address {
    rx.lock.RLock()
    defer rx.lock.RUnlock()
    if len(rx.rotatingKings) > 0 {
        return rx.rotatingKings[0]
    }
    return common.Address{}
}

func (rx *RandomX) GetRotatingKing(blockHeight uint64) common.Address {
    rx.lock.RLock()
    defer rx.lock.RUnlock()
    if len(rx.rotatingKings) == 0 {
        return common.Address{}
    }
    index := (blockHeight / rx.rotationInterval) % uint64(len(rx.rotatingKings))
    return rx.rotatingKings[index]
}

func (rx *RandomX) SetRotatingKings(kings []common.Address) {
    rx.lock.Lock()
    defer rx.lock.Unlock()
    rx.rotatingKings = kings
}

func (rx *RandomX) SetRotationInterval(interval uint64) {
    rx.lock.Lock()
    defer rx.lock.Unlock()
    rx.rotationInterval = interval
}
