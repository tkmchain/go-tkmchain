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
    "github.com/ethereum/go-ethereum/rlp"
    "github.com/ethereum/go-ethereum/rpc"
)

var (
    maxUint256        = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
    GenesisDifficulty = big.NewInt(3)
    MinDifficulty     = big.NewInt(3)
)

var (
    errNoCache      = fmt.Errorf("randomx cache not initialized")
    errEngineClosed = fmt.Errorf("randomx engine is closed")
    errInvalidWork  = fmt.Errorf("invalid work")
)

const (
    RandomXEpochLength = 2048
    TargetBlockTime    = 120 // seconds
    // Difficulty adjustment window for smoothing
    DifficultyWindow = 10
)

const (
    RANDOMX_FLAG_JIT      = 2
    RANDOMX_FLAG_HARD_AES = 4
)

type Config struct {
    Enabled        bool
    EpochLength    uint64
    CacheSize      uint64
    DatasetSize    uint64
    MinMemory      uint64
    PersistDataset bool
}

type Work struct {
    HeaderHash  string `json:"header_hash"`
    SeedHash    string `json:"seed_hash"`
    Target      string `json:"target"`
    Difficulty  string `json:"difficulty"`
    BlockNumber uint64 `json:"block_number"`
    Height      uint64 `json:"height"`
}

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

    hashrate      uint64
    hrMu          sync.RWMutex
    sharesValid   uint64
    sharesInvalid uint64
    currentWork   *Work
    workMu        sync.RWMutex

    chain consensus.ChainHeaderReader

    // Difficulty smoothing
    blockTimes []uint64
    diffMu     sync.RWMutex
}

func DefaultConfig() *Config {
    return &Config{
        Enabled:     true,
        EpochLength: RandomXEpochLength,
        CacheSize:   256,
        DatasetSize: 2,
        MinMemory:   4,
    }
}

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
        blockTimes:       make([]uint64, 0, DifficultyWindow),
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

// randomXHash computes RandomX hash (Monero-style)
func (rx *RandomX) randomXHash(header *types.Header, vm *VM) (*big.Int, common.Hash) {
    input := make([]byte, 40)
    sealHash := rx.SealHash(header)
    copy(input[:32], sealHash.Bytes())
    copy(input[32:], header.Nonce[:])

    output := make([]byte, 32)
    if vm != nil {
        vm.CalculateHash(input, output)
    }

    hash := common.BytesToHash(output)
    result := new(big.Int).SetBytes(output)

    return result, hash
}

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

// generateWork gets work for the NEXT block
func (rx *RandomX) generateWork() (*Work, error) {
    var blockNum uint64 = 1
    var difficulty *big.Int = GenesisDifficulty
    var parentHash common.Hash

    if rx.chain != nil {
        currentHeader := rx.chain.CurrentHeader()
        if currentHeader != nil {
            blockNum = currentHeader.Number.Uint64() + 1
            parentHash = currentHeader.Hash()

            // Calculate difficulty based on parent block time
            difficulty = rx.CalcDifficulty(rx.chain, uint64(time.Now().Unix()), currentHeader)

            log.Info("Generating work",
                "height", blockNum,
                "parent_difficulty", currentHeader.Difficulty,
                "new_difficulty", difficulty)
        }
    }

    header := &types.Header{
        Number:     big.NewInt(int64(blockNum)),
        Difficulty: difficulty,
        Time:       uint64(time.Now().Unix()),
        ParentHash: parentHash,
    }

    sealHash := rx.SealHash(header)
    seedHash := rx.seedHash(rx.epoch(blockNum))
    target := new(big.Int).Div(maxUint256, difficulty)

    return &Work{
        HeaderHash:  hex.EncodeToString(sealHash.Bytes()),
        SeedHash:    hex.EncodeToString(seedHash.Bytes()),
        Target:      fmt.Sprintf("%064x", target),
        Difficulty:  difficulty.String(),
        BlockNumber: blockNum,
        Height:      blockNum,
    }, nil
}

func (rx *RandomX) SubmitWork(nonceHex string, headerHashHex string, mixDigestHex string) (bool, error) {
    if rx.isClosed() {
        return false, errEngineClosed
    }

    log.Info("SubmitWork received", "nonce", nonceHex[:16])

    nonceBytes, err := hex.DecodeString(nonceHex)
    if err != nil || len(nonceBytes) != 8 {
        atomic.AddUint64(&rx.sharesInvalid, 1)
        return false, errInvalidWork
    }
    nonce := binary.BigEndian.Uint64(nonceBytes)

    rx.workMu.RLock()
    currentWork := rx.currentWork
    rx.workMu.RUnlock()

    if currentWork == nil {
        atomic.AddUint64(&rx.sharesInvalid, 1)
        return false, fmt.Errorf("no current work")
    }

    header := &types.Header{
        Nonce:      types.EncodeNonce(nonce),
        Number:     big.NewInt(int64(currentWork.BlockNumber)),
        Difficulty: GenesisDifficulty,
        Time:       uint64(time.Now().Unix()),
    }

    if d, ok := new(big.Int).SetString(currentWork.Difficulty, 10); ok {
        header.Difficulty = d
    }

    if mixDigestHex != "" {
        mixDigestBytes, err := hex.DecodeString(mixDigestHex)
        if err == nil && len(mixDigestBytes) >= 32 {
            header.MixDigest = common.BytesToHash(mixDigestBytes[:32])
        }
    }

    if err := rx.VerifySeal(nil, header); err != nil {
        atomic.AddUint64(&rx.sharesInvalid, 1)
        return false, err
    }

    atomic.AddUint64(&rx.sharesValid, 1)
    log.Info("✅ Valid Monero-style RandomX proof!", "nonce", nonce)
    return true, nil
}

func (rx *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
    if rx.fullFake || rx.isClosed() {
        return nil
    }

    num := header.Number.Uint64()
    if num == 0 {
        return nil
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

    result, _ := rx.randomXHash(header, vm)

    target := new(big.Int).Div(maxUint256, header.Difficulty)
    if result.Cmp(target) > 0 {
        return fmt.Errorf("invalid proof: result > target")
    }

    return nil
}

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
    target := new(big.Int).Div(maxUint256, sealHeader.Difficulty)

    log.Info("⛏️ Monero-style RandomX mining",
        "block", sealHeader.Number.Uint64(),
        "difficulty", sealHeader.Difficulty)

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
        result, hash := rx.randomXHash(sealHeader, vm)
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
            sealHeader.MixDigest = hash
            sealedBlock := block.WithSeal(sealHeader)

            log.Info("�� BLOCK MINED!",
                "block", sealHeader.Number.Uint64(),
                "difficulty", sealHeader.Difficulty,
                "nonce", nonce)

            select {
            case results <- sealedBlock:
            default:
            }
            return nil
        }

        nonce++
        if nonce == 0 {
            nonce = 1
        }
    }
}

// Prepare sets the difficulty for the block BEFORE mining
func (rx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
    if header.Number == nil {
        header.Number = new(big.Int)
    }

    // If difficulty is not set, calculate it from parent
    if header.Difficulty == nil || header.Difficulty.Sign() == 0 {
        // For block 0 (genesis), use genesis difficulty
        if header.Number.Uint64() == 0 {
            header.Difficulty = GenesisDifficulty
            return nil
        }

        // Get parent header
        parentHash := header.ParentHash
        parentNum := header.Number.Uint64() - 1
        parentHeader := chain.GetHeader(parentHash, parentNum)

        if parentHeader != nil {
            // Calculate difficulty based on parent block time
            newDifficulty := rx.CalcDifficulty(chain, header.Time, parentHeader)
            header.Difficulty = newDifficulty

            log.Info("�� Difficulty set in Prepare",
                "block", header.Number.Uint64(),
                "parent_difficulty", parentHeader.Difficulty,
                "new_difficulty", newDifficulty,
                "block_time", header.Time-parentHeader.Time)
        } else {
            header.Difficulty = GenesisDifficulty
        }
    }

    return nil
}

// CalcDifficulty calculates difficulty with moderate adjustment (Monero-style)
func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
    if parent == nil {
        return GenesisDifficulty
    }

    // Calculate time since last block
    parentTime := parent.Time
    var blockTime uint64
    if time > parentTime {
        blockTime = time - parentTime
    } else {
        blockTime = parentTime - time
    }

    // Store block time for smoothing
    rx.diffMu.Lock()
    rx.blockTimes = append(rx.blockTimes, blockTime)
    if len(rx.blockTimes) > DifficultyWindow {
        rx.blockTimes = rx.blockTimes[1:]
    }
    rx.diffMu.Unlock()

    // Calculate average block time from recent blocks
    rx.diffMu.RLock()
    avgBlockTime := uint64(0)
    if len(rx.blockTimes) > 0 {
        sum := uint64(0)
        for _, t := range rx.blockTimes {
            sum += t
        }
        avgBlockTime = sum / uint64(len(rx.blockTimes))
    } else {
        avgBlockTime = blockTime
    }
    rx.diffMu.RUnlock()

    // Use average for difficulty calculation
    diff := avgBlockTime
    if diff == 0 {
        diff = 1
    }

    targetTime := uint64(TargetBlockTime)
    currentDiff := new(big.Int).Set(parent.Difficulty)

    // Calculate ratio: targetTime / actualTime (capped for stability)
    ratio := float64(targetTime) / float64(diff)
    
    // Moderate adjustment: cap ratio between 0.5 and 2.0
    // This prevents extreme difficulty swings
    if ratio > 2.0 {
        ratio = 2.0
    } else if ratio < 0.5 {
        ratio = 0.5
    }

    // Apply ratio with moderate adjustment (Monero uses 1/2 difficulty window)
    // Using 8% adjustment per block (similar to Monero's 0.08 factor)
    adjustment := ratio
    if adjustment > 1.0 {
        // Increase difficulty gradually
        adjustment = 1.0 + (adjustment-1.0)*0.08
    } else {
        // Decrease difficulty gradually
        adjustment = 1.0 - (1.0-adjustment)*0.08
    }

    // Apply adjustment
    newDiff := new(big.Int).Set(currentDiff)
    if adjustment > 1.0 {
        // Increase difficulty
        multiplier := new(big.Int).SetInt64(int64(adjustment * 1000000))
        newDiff.Mul(newDiff, multiplier)
        newDiff.Div(newDiff, big.NewInt(1000000))
    } else {
        // Decrease difficulty
        divisor := new(big.Int).SetInt64(int64(1000000 / adjustment))
        newDiff.Mul(newDiff, big.NewInt(1000000))
        newDiff.Div(newDiff, divisor)
    }

    // Ensure difficulty doesn't drop below minimum
    if newDiff.Cmp(MinDifficulty) < 0 {
        return MinDifficulty
    }

    // Ensure difficulty doesn't increase too rapidly (max 4x per block)
    maxIncrease := new(big.Int).Mul(currentDiff, big.NewInt(4))
    if newDiff.Cmp(maxIncrease) > 0 {
        newDiff.Set(maxIncrease)
    }

    log.Info("�� Difficulty adjustment (moderate)",
        "block_time_s", diff,
        "avg_block_time_s", avgBlockTime,
        "ratio", fmt.Sprintf("%.2f", ratio),
        "adjustment", fmt.Sprintf("%.2f", adjustment),
        "old", currentDiff,
        "new", newDiff)

    return newDiff
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

func (rx *RandomX) epoch(blockNum uint64) uint64 {
    return blockNum / rx.config.EpochLength
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
    rlp.Encode(hasher, enc)
    var hash common.Hash
    hasher.Sum(hash[:0])
    return hash
}

func (rx *RandomX) Author(header *types.Header) (common.Address, error) {
    return header.Coinbase, nil
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

func (rx *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
    return []rpc.API{
        {Namespace: "randomx", Version: "1.0", Service: &RandomXAPI{randomx: rx}, Public: true},
        {Namespace: "miner", Version: "1.0", Service: &MinerAPI{randomx: rx}, Public: true},
    }
}

type RandomXAPI struct{ randomx *RandomX }

func (api *RandomXAPI) GetSeedHash(block *uint64) (common.Hash, error) {
    bn := uint64(0)
    if block != nil {
        bn = *block
    }
    return api.randomx.seedHash(api.randomx.epoch(bn)), nil
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

type MinerAPI struct{ randomx *RandomX }

func (api *MinerAPI) GetWork() ([]string, error) {
    return api.randomx.GetWork()
}

func (api *MinerAPI) SubmitWork(nonce, headerHash, mixDigest string) (bool, error) {
    return api.randomx.SubmitWork(nonce, headerHash, mixDigest)
}

func (api *MinerAPI) GetHashrate() float64 {
    return api.randomx.Hashrate()
}

func CalculateNextDifficulty(parent *types.Header, getHeaderByNumber func(uint64) *types.Header) *big.Int {
    if parent == nil {
        return GenesisDifficulty
    }
    return GenesisDifficulty
}
