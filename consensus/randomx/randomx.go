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
    "sync/atomic"
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
    maxUint256        = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
    GenesisDifficulty = big.NewInt(3)
)

var (
    errInvalidMixHash = fmt.Errorf("invalid mix hash")
    errNoCache        = fmt.Errorf("randomx cache not initialized")
    errEngineClosed   = fmt.Errorf("randomx engine is closed")
)

const (
    RandomXEpochLength = 2048
)

const (
    RANDOMX_FLAG_JIT      = 2
    RANDOMX_FLAG_HARD_AES = 4
)

// CGO Wrappers
type Cache struct{ ptr *C.randomx_cache }
type Dataset struct{ ptr *C.randomx_dataset }
type VM struct{ ptr *C.randomx_vm }

func NewCache(flags int) *Cache {
    c := C.randomx_alloc_cache(C.randomx_flags(flags))
    if c == nil {
        return nil
    }
    return &Cache{ptr: c}
}

func (c *Cache) Init(seed []byte) {
    if c == nil || c.ptr == nil {
        return
    }
    var p unsafe.Pointer
    if len(seed) > 0 {
        p = unsafe.Pointer(&seed[0])
    }
    C.randomx_init_cache(c.ptr, p, C.size_t(len(seed)))
}

func (c *Cache) Close() {
    if c != nil && c.ptr != nil {
        C.randomx_release_cache(c.ptr)
        c.ptr = nil
    }
}

func NewDataset(flags int) *Dataset {
    d := C.randomx_alloc_dataset(C.randomx_flags(flags))
    if d == nil {
        return nil
    }
    return &Dataset{ptr: d}
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
        return
    }
    var inPtr unsafe.Pointer
    if len(input) > 0 {
        inPtr = unsafe.Pointer(&input[0])
    }
    C.randomx_calculate_hash(vm.ptr, inPtr, C.size_t(len(input)), unsafe.Pointer(&output[0]))
}

func (vm *VM) Close() {
    if vm != nil && vm.ptr != nil {
        C.randomx_destroy_vm(vm.ptr)
        vm.ptr = nil
    }
}

// RandomX Engine
type RandomX struct {
    config           *params.RandomXConfig
    fullFake         bool
    rotatingKings    []common.Address
    rotationInterval uint64

    cache      *Cache
    dataset    *Dataset
    cacheEpoch uint64
    cacheMu    sync.RWMutex
    lock       sync.RWMutex

    stopCh chan struct{}
    closed int32
    
    // Mining stats
    hashrate uint64
    hrMu     sync.RWMutex
}

// New creates a new RandomX consensus engine
func New(config *params.RandomXConfig, threads int, mainKing common.Address, kingAddresses []common.Address) (*RandomX, error) {
    log.Info("========== INITIALIZING RANDOMX CONSENSUS ==========")

    if config == nil {
        config = params.DefaultRandomXConfig()
    }
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

    // Initialize cache/dataset immediately
    if err := rx.updateCacheForEpoch(0); err != nil {
        return nil, fmt.Errorf("failed to initialize RandomX: %w", err)
    }

    log.Info("✅ RandomX engine initialized successfully")
    return rx, nil
}

func NewFaker() *RandomX {
    log.Warn("Creating RandomX faker (accepts all seals)")
    engine, _ := New(params.DefaultRandomXConfig(), 1, common.Address{}, nil)
    engine.fullFake = true
    return engine
}

func (rx *RandomX) isClosed() bool {
    return atomic.LoadInt32(&rx.closed) == 1
}

func (rx *RandomX) Close() error {
    atomic.StoreInt32(&rx.closed, 1)
    close(rx.stopCh)
    time.Sleep(400 * time.Millisecond)

    rx.cacheMu.Lock()
    if rx.cache != nil {
        rx.cache.Close()
        rx.cache = nil
    }
    if rx.dataset != nil {
        rx.dataset.Close()
        rx.dataset = nil
    }
    rx.cacheMu.Unlock()

    log.Info("RandomX resources released")
    return nil
}

// =================================== Core Functions ===================================

func (rx *RandomX) getVM() (*VM, error) {
    if rx.isClosed() {
        return nil, errEngineClosed
    }

    rx.cacheMu.RLock()
    defer rx.cacheMu.RUnlock()

    if rx.cache == nil {
        return nil, errNoCache
    }

    flags := RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES

    if rx.dataset != nil {
        if vm := NewVM(flags, nil, rx.dataset); vm != nil {
            return vm, nil
        }
    }
    if vm := NewVM(flags, rx.cache, nil); vm != nil {
        return vm, nil
    }
    return nil, fmt.Errorf("failed to create RandomX VM")
}

func (rx *RandomX) updateCacheForEpoch(epoch uint64) error {
    if rx.isClosed() {
        return errEngineClosed
    }

    rx.cacheMu.Lock()
    defer rx.cacheMu.Unlock()

    if rx.cacheEpoch == epoch && rx.cache != nil {
        return nil
    }

    seed := rx.seedHash(epoch * rx.config.EpochLength)
    seedBytes := seed.Bytes()

    log.Info("�� Initializing RandomX", "epoch", epoch, "seed", seed.Hex()[:16]+"...")

    if rx.cache != nil {
        rx.cache.Close()
        rx.cache = nil
    }
    if rx.dataset != nil {
        rx.dataset.Close()
        rx.dataset = nil
    }

    rx.cache = NewCache(RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES)
    if rx.cache == nil {
        return fmt.Errorf("failed to allocate RandomX cache")
    }
    rx.cache.Init(seedBytes)

    if ds := NewDataset(RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES); ds != nil {
        log.Info("�� Initializing full RandomX dataset...")
        ds.InitDataset(rx.cache, 0, 0)
        rx.dataset = ds
        log.Info("✅ Full dataset ready")
    } else {
        log.Warn("⚠️ Falling back to light mode (cache only)")
    }

    rx.cacheEpoch = epoch
    return nil
}

func (rx *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *VM) (common.Hash, *big.Int) {
    input := make([]byte, 40)
    sealHash := rx.SealHash(header)
    copy(input[:32], sealHash.Bytes())
    copy(input[32:], header.Nonce[:])

    output := make([]byte, 32)
    if vm != nil {
        vm.CalculateHash(input, output)
    }

    mixDigest := common.BytesToHash(output)
    result := new(big.Int).SetBytes(output)

    return mixDigest, result
}

// =================================== Consensus Engine Methods ===================================

func (rx *RandomX) Author(header *types.Header) (common.Address, error) {
    return header.Coinbase, nil
}

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

// Full Seal implementation - FIXED for solo mining
func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
    if rx.fullFake || rx.isClosed() {
        select {
        case results <- block:
        default:
        }
        return nil
    }

    header := block.Header()

    // Already sealed case
    if header.MixDigest != (common.Hash{}) {
        if err := rx.VerifySeal(chain, header); err != nil {
            return err
        }
        select {
        case results <- block:
        default:
        }
        return nil
    }

    // Initialize cache/dataset for this epoch
    epoch := rx.epoch(header.Number.Uint64())
    if err := rx.updateCacheForEpoch(epoch); err != nil {
        return fmt.Errorf("failed to update cache: %w", err)
    }

    vm, err := rx.getVM()
    if err != nil {
        return fmt.Errorf("failed to get RandomX VM: %w", err)
    }
    defer vm.Close()

    sealHeader := types.CopyHeader(header)
    seed := rx.seedHash(sealHeader.Number.Uint64())
    target := new(big.Int).Div(maxUint256, sealHeader.Difficulty)
    
    log.Info("⛏️  Starting RandomX mining", 
        "block", sealHeader.Number.Uint64(),
        "difficulty", sealHeader.Difficulty,
        "target", target,
        "seed", seed.Hex()[:16])

    // Start from a random nonce to avoid collisions between miners
    startNonce := uint64(time.Now().UnixNano())
    nonce := startNonce
    attempts := uint64(0)
    startTime := time.Now()

    for {
        select {
        case <-stop:
            log.Debug("Mining stopped", "attempts", attempts)
            return nil
        case <-rx.stopCh:
            return nil
        default:
        }

        // Update nonce
        sealHeader.Nonce = types.EncodeNonce(nonce)
        
        // Calculate RandomX hash
        mixDigest, result := rx.hashimoto(sealHeader, seed, vm)
        attempts++
        
        // Update hashrate every 1000 attempts
        if attempts%1000 == 0 {
            elapsed := time.Since(startTime).Seconds()
            if elapsed > 0 {
                hr := float64(attempts) / elapsed
                rx.hrMu.Lock()
                rx.hashrate = uint64(hr)
                rx.hrMu.Unlock()
                
                log.Debug("Mining progress", 
                    "attempts", attempts,
                    "hashrate", fmt.Sprintf("%.2f H/s", hr),
                    "current_nonce", nonce)
            }
        }

        // Check if we found a valid nonce
        if result.Cmp(target) <= 0 {
            sealHeader.MixDigest = mixDigest
            sealedBlock := block.WithSeal(sealHeader)
            
            elapsed := time.Since(startTime)
            log.Info("�� BLOCK MINED SUCCESSFULLY ��", 
                "block", sealHeader.Number.Uint64(),
                "nonce", nonce,
                "attempts", attempts,
                "elapsed", elapsed,
                "hashrate", fmt.Sprintf("%.2f H/s", float64(attempts)/elapsed.Seconds()),
                "mix_digest", mixDigest.Hex(),
                "block_hash", sealedBlock.Hash().Hex())
            
            select {
            case results <- sealedBlock:
                log.Info("✅ Block submitted to consensus", "block", sealedBlock.NumberU64())
            case <-stop:
                log.Warn("Block found but mining stopped")
            }
            return nil
        }

        // Increment nonce, wrap around if needed
        nonce++
        if nonce == 0 {
            nonce = 1
        }
        
        // Log every million attempts for long-running searches
        if attempts > 0 && attempts%1000000 == 0 {
            log.Info("Still mining...", 
                "block", sealHeader.Number.Uint64(),
                "attempts", attempts,
                "hashrate", fmt.Sprintf("%.2f H/s", float64(attempts)/time.Since(startTime).Seconds()))
        }
    }
}

func (rx *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
    if rx.fullFake || rx.isClosed() {
        return nil
    }

    num := header.Number.Uint64()

    if num == 0 {
        if header.MixDigest != (common.Hash{}) {
            return errInvalidMixHash
        }
        return nil
    }

    if num <= 20 {
        log.Info("ACCEPTING EARLY BLOCK FOR BOOTSTRAP", "number", num)
        return nil
    }

    if header.MixDigest == (common.Hash{}) {
        log.Error("REJECTING BLOCK WITH ZERO MIX DIGEST", "number", num)
        return errInvalidMixHash
    }

    if err := rx.updateCacheForEpoch(rx.epoch(num)); err != nil {
        return err
    }

    vm, err := rx.getVM()
    if err != nil {
        return err
    }
    defer vm.Close()

    mixDigest, result := rx.hashimoto(header, rx.seedHash(num), vm)

    if !bytes.Equal(mixDigest.Bytes(), header.MixDigest.Bytes()) {
        log.Error("Mix digest mismatch", 
            "expected", header.MixDigest.Hex(), 
            "got", mixDigest.Hex())
        return errInvalidMixHash
    }

    target := new(big.Int).Div(maxUint256, header.Difficulty)
    if result.Cmp(target) > 0 {
        log.Error("Invalid proof-of-work", 
            "result", result,
            "target", target)
        return fmt.Errorf("invalid proof-of-work")
    }

    log.Debug("Seal verified", "number", num, "nonce", header.Nonce.Uint64())
    return nil
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
    // Empty implementation
}

func (rx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
    if len(receipts) > 0 {
        header.Bloom = types.MergeBloom(receipts)
    }
    return types.NewBlock(header, body, receipts, nil), nil
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

func (rx *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
    if len(block.Uncles()) > 0 {
        return consensus.ErrUnknownAncestor
    }
    return nil
}

// King Management
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

// RPC API
type RandomXAPI struct {
    randomx *RandomX
}

func (api *RandomXAPI) GetSeedHash(block *uint64) (common.Hash, error) {
    bn := uint64(0)
    if block != nil {
        bn = *block
    }
    return api.randomx.seedHash(bn), nil
}

func (api *RandomXAPI) GetCurrentEpoch(blockNumber uint64) uint64 {
    return blockNumber / RandomXEpochLength
}

func (api *RandomXAPI) GetHashrate() float64 {
    api.randomx.hrMu.RLock()
    defer api.randomx.hrMu.RUnlock()
    return float64(api.randomx.hashrate)
}

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

// CalculateNextDifficulty - Difficulty adjustment algorithm
func CalculateNextDifficulty(parent *types.Header, getHeaderByNumber func(uint64) *types.Header) *big.Int {
    // Simple difficulty adjustment for testing
    if parent == nil {
        return GenesisDifficulty
    }
    
    // For low difficulty, just return parent difficulty
    // This ensures we can mine blocks quickly for testing
    if parent.Difficulty.Cmp(big.NewInt(100)) < 0 {
        return parent.Difficulty
    }
    
    // Otherwise, adjust based on time
    expectedTime := uint64(15) // 15 seconds per block
    parentTime := parent.Time
    
    if parentTime > expectedTime {
        // Decrease difficulty if blocks are taking too long
        return new(big.Int).Div(parent.Difficulty, big.NewInt(2))
    } else if parentTime < expectedTime/2 {
        // Increase difficulty if blocks are too fast
        return new(big.Int).Mul(parent.Difficulty, big.NewInt(2))
    }
    
    return parent.Difficulty
}
