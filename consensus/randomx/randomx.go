//go:build cgo && randomx
// +build cgo,randomx

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
//    "github.com/ethereum/go-ethereum/params"
    "github.com/ethereum/go-ethereum/rlp"
    "github.com/ethereum/go-ethereum/rpc"
)

// Constants - using values consistent with reward.go
var (
    maxUint256        = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
    GenesisDifficulty = big.NewInt(3)
)

var (
    errInvalidMixHash = fmt.Errorf("invalid mix hash")
    errNoCache        = fmt.Errorf("randomx cache not initialized")
    errEngineClosed   = fmt.Errorf("randomx engine is closed")
    errInvalidWork    = fmt.Errorf("invalid work")
)

const (
    RandomXEpochLength = 2048
)

const (
    RANDOMX_FLAG_JIT      = 2
    RANDOMX_FLAG_HARD_AES = 4
)

// Config holds RandomX configuration
type Config struct {
    Enabled        bool
    EpochLength    uint64
    CacheSize      uint64
    DatasetSize    uint64
    MinMemory      uint64
    PersistDataset bool
}

// Work represents mining work for external miners
type Work struct {
    HeaderHash  string `json:"header_hash"`
    SeedHash    string `json:"seed_hash"`
    Target      string `json:"target"`
    Difficulty  string `json:"difficulty"`
    BlockNumber uint64 `json:"block_number"`
    Height      uint64 `json:"height"`
}

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
    config           *Config
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
    hashrate      uint64
    hrMu          sync.RWMutex
    sharesValid   uint64
    sharesInvalid uint64
    currentWork   *Work
    workMu        sync.RWMutex
    
    // Chain context
    chain consensus.ChainHeaderReader
}

// DefaultConfig returns the default RandomX configuration
func DefaultConfig() *Config {
    return &Config{
        Enabled:     true,
        EpochLength: RandomXEpochLength,
        CacheSize:   256,
        DatasetSize: 2,
        MinMemory:   4,
    }
}

// New creates a new RandomX consensus engine
func New(config *Config, threads int, mainKing common.Address, kingAddresses []common.Address) (*RandomX, error) {
    log.Info("========== INITIALIZING RANDOMX CONSENSUS ==========")

    if config == nil {
        config = DefaultConfig()
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

    if err := rx.updateCacheForEpoch(0); err != nil {
        return nil, fmt.Errorf("failed to initialize RandomX: %w", err)
    }

    log.Info("✅ RandomX engine initialized successfully", "threads", threads)
    return rx, nil
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

func (rx *RandomX) GetEpochLength() uint64 {
    return rx.config.EpochLength
}

func (rx *RandomX) Hashrate() float64 {
    rx.hrMu.RLock()
    defer rx.hrMu.RUnlock()
    return float64(rx.hashrate)
}

func (rx *RandomX) GetSharesFound() uint64 {
    return atomic.LoadUint64(&rx.sharesValid)
}

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

    seed := rx.seedHash(epoch)
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

// GetWork returns work for external miners
func (rx *RandomX) GetWork() ([]string, error) {
    if rx.isClosed() {
        return nil, errEngineClosed
    }

    work, err := rx.generateWork()
    if err != nil {
        return nil, err
    }

    rx.workMu.Lock()
    rx.currentWork = work
    rx.workMu.Unlock()

    return []string{work.HeaderHash, work.SeedHash, work.Target}, nil
}

func (rx *RandomX) generateWork() (*Work, error) {
    header := &types.Header{
        Number:     big.NewInt(1),
        Difficulty: GenesisDifficulty,
        Time:       uint64(time.Now().Unix()),
    }

    sealHash := rx.SealHash(header)
    seedHash := rx.seedHash(rx.epoch(1))
    target := new(big.Int).Div(maxUint256, GenesisDifficulty)

    if seedHash == (common.Hash{}) {
        seedHash = crypto.Keccak256Hash([]byte("randomx_genesis_seed"))
    }

    return &Work{
        HeaderHash:  hex.EncodeToString(sealHash.Bytes()),
        SeedHash:    hex.EncodeToString(seedHash.Bytes()),
        Target:      fmt.Sprintf("%064x", target),
        Difficulty:  GenesisDifficulty.String(),
        BlockNumber: 1,
        Height:      1,
    }, nil
}

// SubmitWork validates and submits work from external miners
func (rx *RandomX) SubmitWork(nonceHex string, headerHashHex string, mixDigestHex string) (bool, error) {
    if rx.isClosed() {
        return false, errEngineClosed
    }

    log.Info("SubmitWork received", "nonce", nonceHex[:16], "header_hash", headerHashHex[:16])

    nonceBytes, err := hex.DecodeString(nonceHex)
    if err != nil || len(nonceBytes) != 8 {
        atomic.AddUint64(&rx.sharesInvalid, 1)
        return false, errInvalidWork
    }
    nonce := binary.BigEndian.Uint64(nonceBytes)

    headerHashBytes, err := hex.DecodeString(headerHashHex)
    if err != nil || len(headerHashBytes) != 32 {
        atomic.AddUint64(&rx.sharesInvalid, 1)
        return false, errInvalidWork
    }

    mixDigestBytes, err := hex.DecodeString(mixDigestHex)
    if err != nil || len(mixDigestBytes) != 32 {
        atomic.AddUint64(&rx.sharesInvalid, 1)
        return false, errInvalidWork
    }

    header := &types.Header{
        MixDigest:  common.BytesToHash(mixDigestBytes),
        Nonce:      types.EncodeNonce(nonce),
        Number:     big.NewInt(1),
        Difficulty: GenesisDifficulty,
    }

    if err := rx.VerifySeal(nil, header); err != nil {
        atomic.AddUint64(&rx.sharesInvalid, 1)
        return false, err
    }

    atomic.AddUint64(&rx.sharesValid, 1)
    log.Info("✅ Valid work submitted!", "nonce", nonce)
    return true, nil
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

// Seal implements consensus.Engine - Main mining function
func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
    rx.chain = chain
    
    if rx.fullFake || rx.isClosed() {
        select {
        case results <- block:
        default:
        }
        return nil
    }

    header := block.Header()

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
    seedHash := rx.seedHash(epoch)
    target := new(big.Int).Div(maxUint256, sealHeader.Difficulty)

    log.Info("⛏️ Starting RandomX mining",
        "block", sealHeader.Number.Uint64(),
        "difficulty", sealHeader.Difficulty,
        "target", target)

    startNonce := uint64(time.Now().UnixNano())
    nonce := startNonce
    attempts := uint64(0)
    startTime := time.Now()

    for {
        select {
        case <-stop:
            return nil
        case <-rx.stopCh:
            return nil
        default:
        }

        sealHeader.Nonce = types.EncodeNonce(nonce)
        mixDigest, result := rx.hashimoto(sealHeader, seedHash, vm)
        attempts++

        if attempts%1000 == 0 {
            elapsed := time.Since(startTime).Seconds()
            if elapsed > 0 {
                hr := float64(attempts) / elapsed
                rx.hrMu.Lock()
                rx.hashrate = uint64(hr)
                rx.hrMu.Unlock()
            }
        }

        if result.Cmp(target) <= 0 {
            sealHeader.MixDigest = mixDigest
            sealedBlock := block.WithSeal(sealHeader)

            log.Info("�� BLOCK MINED!", "block", sealHeader.Number.Uint64(), "nonce", nonce, "attempts", attempts)
            select {
            case results <- sealedBlock:
            case <-stop:
            }
            return nil
        }

        nonce++
        if nonce == 0 {
            nonce = 1
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

    if header.MixDigest == (common.Hash{}) {
        return errInvalidMixHash
    }

    epoch := rx.epoch(num)
    if err := rx.updateCacheForEpoch(epoch); err != nil {
        return err
    }

    vm, err := rx.getVM()
    if err != nil {
        return err
    }
    defer vm.Close()

    seedHash := rx.seedHash(epoch)
    mixDigest, result := rx.hashimoto(header, seedHash, vm)

    if !bytes.Equal(mixDigest.Bytes(), header.MixDigest.Bytes()) {
        return errInvalidMixHash
    }

    target := new(big.Int).Div(maxUint256, header.Difficulty)
    if result.Cmp(target) > 0 {
        return fmt.Errorf("invalid proof-of-work")
    }

    return nil
}

func (rx *RandomX) epoch(blockNum uint64) uint64 {
    return blockNum / rx.config.EpochLength
}

func (rx *RandomX) seedHash(epoch uint64) common.Hash {
    if epoch == 0 {
        return crypto.Keccak256Hash([]byte("randomx_epoch_0_genesis"))
    }

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

func (rx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
    if header.Number == nil {
        header.Number = new(big.Int)
    }
    if header.Difficulty == nil {
        header.Difficulty = GenesisDifficulty
    }
    return nil
}

func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
        if parent == nil {
                return GenesisDifficulty
        }
        
        // Get parent timestamp
        parentTime := parent.Time
        
        // Calculate time difference
        var diff uint64
        if time > parentTime {
                diff = time - parentTime
        } else {
                diff = parentTime - time
        }
        
        // Target block time (120 seconds from your reward.go)
        targetTime := uint64(120)
        
        // Get current difficulty
        currentDiff := new(big.Int).Set(parent.Difficulty)
        
        // Minimum difficulty
        minDiff := big.NewInt(3)
        
        // Adjust difficulty based on block time
        if diff < targetTime/2 {
                // Block came too fast - increase difficulty
                newDiff := new(big.Int).Mul(currentDiff, big.NewInt(3))
                newDiff.Div(newDiff, big.NewInt(2))
                if newDiff.Cmp(minDiff) < 0 {
                        return minDiff
                }
                return newDiff
        } else if diff > targetTime*3/2 {
                // Block took too long - decrease difficulty
                newDiff := new(big.Int).Mul(currentDiff, big.NewInt(2))
                newDiff.Div(newDiff, big.NewInt(3))
                if newDiff.Cmp(minDiff) < 0 {
                        return minDiff
                }
                return newDiff
        }
        
        return currentDiff
}

func (rx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {}

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
    return rx.VerifySeal(chain, header)
}

func (rx *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
    abort := make(chan struct{})
    results := make(chan error, len(headers))
    go func() {
        for _, header := range headers {
            err := rx.VerifySeal(chain, header)
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

func (rx *RandomX) AddRotatingKing(address common.Address) {
    rx.lock.Lock()
    defer rx.lock.Unlock()
    for _, a := range rx.rotatingKings {
        if a == address {
            return
        }
    }
    rx.rotatingKings = append(rx.rotatingKings, address)
}

// =================================== RPC APIs ===================================

func (rx *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
    return []rpc.API{
        {
            Namespace: "randomx",
            Version:   "1.0",
            Service:   &RandomXAPI{randomx: rx},
            Public:    true,
        },
        {
            Namespace: "miner",
            Version:   "1.0",
            Service:   &MinerAPI{randomx: rx},
            Public:    true,
        },
    }
}

type RandomXAPI struct {
    randomx *RandomX
}

func (api *RandomXAPI) GetSeedHash(block *uint64) (common.Hash, error) {
    bn := uint64(0)
    if block != nil {
        bn = *block
    }
    epoch := api.randomx.epoch(bn)
    return api.randomx.seedHash(epoch), nil
}

func (api *RandomXAPI) GetCurrentEpoch(blockNumber uint64) uint64 {
    return api.randomx.epoch(blockNumber)
}

func (api *RandomXAPI) GetHashrate() float64 {
    return api.randomx.Hashrate()
}

func (api *RandomXAPI) GetStats() map[string]interface{} {
    return map[string]interface{}{
        "valid_shares":   atomic.LoadUint64(&api.randomx.sharesValid),
        "invalid_shares": atomic.LoadUint64(&api.randomx.sharesInvalid),
        "hashrate":       api.randomx.Hashrate(),
        "epoch":          api.randomx.cacheEpoch,
    }
}

type MinerAPI struct {
    randomx *RandomX
}

func (api *MinerAPI) GetWork() ([]string, error) {
    return api.randomx.GetWork()
}

func (api *MinerAPI) SubmitWork(nonce string, headerHash string, mixDigest string) (bool, error) {
    return api.randomx.SubmitWork(nonce, headerHash, mixDigest)
}

func (api *MinerAPI) GetHashrate() float64 {
    return api.randomx.Hashrate()
}

// CalculateNextDifficulty for difficulty.go
func CalculateNextDifficulty(parent *types.Header, getHeaderByNumber func(uint64) *types.Header) *big.Int {
    if parent == nil {
        return GenesisDifficulty
    }
    return GenesisDifficulty
}
