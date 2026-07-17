// Copyright 2026 The go-tkmchain Authors
// This file is part of the go-tkmchain library.
//
// The go-tkmchain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-tkmchain library. If not, see <http://www.gnu.org/licenses/>.

//go:build cgo && randomx
// +build cgo,randomx

package randomx

/*
#cgo CFLAGS: -I${SRCDIR}/../../build/_workspace/randomx/src
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-host -lrandomx -lstdc++ -lm
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-linux-arm64 -lrandomx -lstdc++ -lm
#cgo linux,arm LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-linux-arm -lrandomx -lstdc++ -lm
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-windows-amd64 -lrandomx -lstdc++ -lwinpthread
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-darwin-amd64 -lrandomx -lc++ -lm -framework CoreFoundation -framework Security
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-darwin-arm64 -lrandomx -lc++ -lm -framework CoreFoundation -framework Security

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
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/keccak"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"
)

var (
	maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)

	rotatingKingStateSlot = crypto.Keccak256Hash([]byte("randomx.rotatingking"))
)

var (
	errNoCache      = fmt.Errorf("randomx cache not initialized")
	errEngineClosed = fmt.Errorf("randomx engine is closed")
	errInvalidWork  = fmt.Errorf("invalid work")
)

const (
	RandomXEpochLength = 2048
	TargetBlockTime    = 120 // seconds

	// EDAThreshold is the no-block interval that triggers Emergency Difficulty Adjustment.
	EDAThreshold = 7 * 60 // seconds

	// EgyptEDAThreshold keeps the RandomX testnet usable with small pool/miner hashrates.
	EgyptEDAThreshold = 30 // seconds
)

const (
	RANDOMX_FLAG_HARD_AES = 2
	RANDOMX_FLAG_FULL_MEM = 4
	RANDOMX_FLAG_JIT      = 8
)

func randomXBaseFlags() int {
	return RANDOMX_FLAG_HARD_AES
}

func randomXFastFlags() int {
	flags := randomXBaseFlags()
	// RandomX JIT is not consistently available on darwin and arm64/aarch64
	// targets. On those platforms executable-memory restrictions or missing JIT
	// support can make randomx_create_vm return nil even though interpreter mode
	// works correctly.
	if runtime.GOOS == "darwin" || runtime.GOARCH == "arm64" {
		return flags
	}
	return flags | RANDOMX_FLAG_JIT
}

func randomXFlagCandidates(extra int) []int {
	candidates := []int{
		randomXFastFlags() | extra,
		randomXBaseFlags() | extra,
		extra,
	}
	unique := candidates[:0]
	seen := make(map[int]struct{}, len(candidates))
	for _, flags := range candidates {
		if _, ok := seen[flags]; ok {
			continue
		}
		seen[flags] = struct{}{}
		unique = append(unique, flags)
	}
	return unique
}

func newVMWithFallback(cache *Cache, dataset *Dataset, extra int) (*VM, int) {
	for _, flags := range randomXFlagCandidates(extra) {
		if vm := NewVM(flags, cache, dataset); vm != nil {
			return vm, flags
		}
		log.Warn("RandomX VM creation failed, trying fallback flags", "flags", flags)
	}
	return nil, 0
}

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

type miningState struct {
	mu          sync.Mutex
	nonce       uint64
	lastCheck   time.Time
	hashCount   uint64
	foundBlocks uint64
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

func DatasetItemCount() uint64 {
	return uint64(C.randomx_dataset_item_count())
}

func (d *Dataset) InitDataset(cache *Cache, start, count uint64) {
	if d == nil || d.ptr == nil || cache == nil || cache.ptr == nil {
		return
	}
	if count == 0 {
		count = DatasetItemCount()
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
	config                  *Config
	fullFake                bool
	mainKing                common.Address
	rotatingKings           []common.Address
	rotatingKingActivations map[common.Address]uint64
	rotationInterval        uint64
	miningThreads           int

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
}

// NewFaker creates a fake RandomX engine for testing purposes
func NewFaker() *RandomX {
	// Create a minimal config for testing
	config := DefaultConfig()

	// Use a fake cache for testing
	fakeRx := &RandomX{
		config:                  config,
		fullFake:                true,
		rotatingKings:           []common.Address{common.Address{}},
		rotatingKingActivations: make(map[common.Address]uint64),
		rotationInterval:        100,
		miningThreads:           1,
		stopCh:                  make(chan struct{}),
	}

	return fakeRx
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
	if threads <= 0 {
		threads = runtime.NumCPU()
	}

	kings := make([]common.Address, 0, len(kingAddresses))
	activations := make(map[common.Address]uint64, len(kingAddresses))
	for _, king := range kingAddresses {
		if king == (common.Address{}) {
			continue
		}
		kings = append(kings, king)
		activations[king] = 0
	}

	rx := &RandomX{
		config:                  config,
		mainKing:                mainKing,
		rotatingKings:           kings,
		rotatingKingActivations: activations,
		rotationInterval:        100,
		miningThreads:           threads,
		stopCh:                  make(chan struct{}),
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

	if rx.dataset != nil {
		if vm, flags := newVMWithFallback(nil, rx.dataset, RANDOMX_FLAG_FULL_MEM); vm != nil {
			log.Debug("Created RandomX dataset VM", "flags", flags)
			return vm, nil
		}
	}
	if vm, flags := newVMWithFallback(rx.cache, nil, 0); vm != nil {
		log.Debug("Created RandomX cache VM", "flags", flags)
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

	log.Info("Initializing RandomX", "epoch", epoch, "seed", seed.Hex()[:16]+"...")

	if rx.cache != nil {
		rx.cache.Close()
		rx.cache = nil
	}
	if rx.dataset != nil {
		rx.dataset.Close()
		rx.dataset = nil
	}

	rx.cache = NewCache(randomXFastFlags())
	if rx.cache == nil {
		return fmt.Errorf("failed to allocate RandomX cache")
	}
	rx.cache.Init(seedBytes)

	persistDataset := rx.config != nil && rx.config.PersistDataset
	if persistDataset {
		if ds := NewDataset(randomXFastFlags()); ds != nil {
			items := DatasetItemCount()
			log.Info("Initializing full RandomX dataset...", "items", items)
			ds.InitDataset(rx.cache, 0, items)
			rx.dataset = ds
			log.Info("✅ Full dataset ready")
		} else {
			log.Warn("⚠️ Falling back to light mode (cache only)")
		}
	} else {
		log.Info("RandomX light mode ready")
	}

	rx.cacheEpoch = epoch
	return nil
}

// randomXHash computes
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
	seedHash := rx.seedHash(rx.epochForBlock(rx.chain, blockNum))
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
	log.Info("Valid RandomX proof!", "nonce", nonce)
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
	if chain != nil {
		config := chain.Config()
		if config != nil && !config.IsRandomXTx(header.Number) {
			return nil
		}
	}

	epoch := rx.epochForBlock(chain, num)
	if err := rx.updateCacheForEpoch(epoch); err != nil {
		return err
	}

	vm, err := rx.getVM()
	if err != nil {
		return err
	}
	defer vm.Close()

	target := new(big.Int).Div(maxUint256, header.Difficulty)
	if rx.validProof(header, vm, target) {
		return nil
	}

	if chain != nil && chain.Config().IsKyoto(header.Number, header.Time) {
		return fmt.Errorf("invalid proof: result > target")
	}

	if rx.validProofWithNonceVariants(header, vm, target) {
		return nil
	}

	// Early RandomX external-miner builds used a full-memory VM flag while later
	// verifier builds used the dataset without that flag. Accept both modes only
	// before Kyoto so already-mined RandomX blocks remain syncable while new blocks
	// use one strict proof format.
	if legacyVM, legacyErr := rx.getLegacyFullMemVM(); legacyErr == nil {
		defer legacyVM.Close()
		if rx.validProofWithNonceVariants(header, legacyVM, target) {
			log.Warn("Accepted legacy RandomX proof variant", "number", header.Number.Uint64(), "hash", header.Hash())
			return nil
		}
	}

	return fmt.Errorf("invalid proof: result > target")
}

func (rx *RandomX) validProof(header *types.Header, vm *VM, target *big.Int) bool {
	result, _ := rx.randomXHash(header, vm)
	return result.Cmp(target) <= 0
}

func (rx *RandomX) validProofWithNonceVariants(header *types.Header, vm *VM, target *big.Int) bool {
	if rx.validProof(header, vm, target) {
		return true
	}

	legacyHeader := types.CopyHeader(header)
	for i := 0; i < len(header.Nonce); i++ {
		legacyHeader.Nonce[i] = header.Nonce[len(header.Nonce)-1-i]
	}
	result, _ := rx.randomXHash(legacyHeader, vm)
	return result.Cmp(target) <= 0
}

func (rx *RandomX) getLegacyFullMemVM() (*VM, error) {
	if rx.isClosed() {
		return nil, errEngineClosed
	}

	rx.cacheMu.RLock()
	defer rx.cacheMu.RUnlock()

	if rx.cache == nil {
		return nil, errNoCache
	}
	if rx.dataset == nil {
		return nil, fmt.Errorf("legacy full-memory dataset is unavailable")
	}

	if vm, flags := newVMWithFallback(nil, rx.dataset, RANDOMX_FLAG_FULL_MEM); vm != nil {
		log.Debug("Created legacy full-memory RandomX VM", "flags", flags)
		return vm, nil
	}
	return nil, fmt.Errorf("failed to create legacy full-memory RandomX VM")
}

func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	rx.chain = chain

	if chain == nil {
		return fmt.Errorf("chain is nil")
	}

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

	epoch := rx.epochForBlock(chain, header.Number.Uint64())
	if err := rx.updateCacheForEpoch(epoch); err != nil {
		return err
	}

	sealHeader := types.CopyHeader(header)
	target := new(big.Int).Div(maxUint256, sealHeader.Difficulty)
	threads := rx.getMiningThreads()

	log.Info("⛏️ RandomX mining started",
		"block", sealHeader.Number.Uint64(),
		"difficulty", sealHeader.Difficulty,
		"threads", threads,
		"target", target.String())

	found := make(chan *types.Block, 1)
	errCh := make(chan error, 1)
	done := make(chan struct{})
	var doneOnce sync.Once
	var wg sync.WaitGroup

	// Use atomic for shared mining state
	var nonceCounter uint64
	var hashCount uint64
	lastCheck := time.Now()

	// Add timeout for production stability
	miningTimeout := time.NewTimer(5 * time.Minute)
	defer miningTimeout.Stop()

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()

			vm, err := rx.getVM()
			if err != nil {
				select {
				case errCh <- fmt.Errorf("failed to get RandomX VM: %w", err):
				default:
				}
				return
			}
			defer vm.Close()

			localHeader := types.CopyHeader(sealHeader)

			for {
				select {
				case <-stop:
					return
				case <-rx.stopCh:
					return
				case <-done:
					return
				case <-miningTimeout.C:
					log.Warn("Mining timeout, restarting")
					miningTimeout.Reset(5 * time.Minute)
					continue
				default:
				}

				// Get next nonce atomically
				nonce := atomic.AddUint64(&nonceCounter, 1) - 1

				// Handle nonce overflow
				if nonce == ^uint64(0) {
					log.Warn("Nonce space exhausted, resetting")
					atomic.StoreUint64(&nonceCounter, 0)
					continue
				}

				localHeader.Nonce = types.EncodeNonce(nonce)
				result, hash := rx.randomXHash(localHeader, vm)

				// Update hash count
				atomic.AddUint64(&hashCount, 1)

				// Log progress periodically
				if atomic.LoadUint64(&hashCount)%10000 == 0 {
					elapsed := time.Since(lastCheck).Seconds()
					if elapsed > 0 {
						hr := float64(atomic.LoadUint64(&hashCount)) / elapsed
						rx.hrMu.Lock()
						rx.hashrate = uint64(hr)
						rx.hrMu.Unlock()
						log.Debug("Mining progress",
							"thread", threadID,
							"hashrate", fmt.Sprintf("%.2f", hr),
							"nonce", nonce)
					}
				}

				if result.Cmp(target) <= 0 {
					localHeader.MixDigest = hash
					sealedBlock := block.WithSeal(localHeader)

					log.Info("✅ BLOCK MINED!",
						"block", localHeader.Number.Uint64(),
						"difficulty", localHeader.Difficulty,
						"nonce", nonce,
						"thread", threadID,
						"hash", hash.Hex())

					doneOnce.Do(func() { close(done) })
					select {
					case found <- sealedBlock:
					default:
					}
					return
				}
			}
		}(i)
	}

	// Wait for mining result
	var sealErr error
	select {
	case sealedBlock := <-found:
		select {
		case results <- sealedBlock:
			log.Info("�� Block submitted to results channel", "block", sealedBlock.Number())
		default:
			log.Warn("Results channel full, block dropped")
		}
	case sealErr = <-errCh:
		log.Error("Mining error", "error", sealErr)
	case <-stop:
		log.Info("Mining stopped by signal")
	case <-rx.stopCh:
		log.Info("Mining stopped by engine closure")
	}

	doneOnce.Do(func() { close(done) })
	wg.Wait()

	return sealErr
}

func (rx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	if header.Number == nil {
		header.Number = new(big.Int)
	}
	if header.UncleHash == (common.Hash{}) {
		header.UncleHash = types.EmptyUncleHash
	}
	if header.TxHash == (common.Hash{}) {
		header.TxHash = types.EmptyTxsHash
	}
	if header.ReceiptHash == (common.Hash{}) {
		header.ReceiptHash = types.EmptyReceiptsHash
	}

	if header.Difficulty == nil || header.Difficulty.Sign() == 0 {
		if header.Number.Uint64() == 0 {
			header.Difficulty = GenesisDifficulty
			return nil
		}

		var parentHeader *types.Header
		if chain != nil {
			parentHash := header.ParentHash
			parentNum := header.Number.Uint64() - 1
			parentHeader = chain.GetHeader(parentHash, parentNum)
		}

		if parentHeader != nil {
			newDifficulty := rx.CalcDifficulty(chain, header.Time, parentHeader)
			header.Difficulty = newDifficulty

			log.Info("Difficulty set in Prepare",
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

// CalcDifficulty: very aggressive but with x2cap
func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	if parent == nil {
		return GenesisDifficulty
	}

	parentTime := parent.Time
	var diff uint64
	if time > parentTime {
		diff = time - parentTime
	} else {
		diff = parentTime - time
	}

	targetTime := uint64(TargetBlockTime)
	currentDiff := new(big.Int).Set(parent.Difficulty)
	minDiff := MinDifficulty

	if config := chainConfig(chain); config != nil {
		if isEgyptConfig(config) && diff >= EgyptEDAThreshold {
			reductions := diff / EgyptEDAThreshold
			newDiff := applyEDAReductions(currentDiff, reductions, minDiff)
			log.Info("Egypt emergency difficulty adjustment applied",
				"old", currentDiff,
				"new", newDiff,
				"block_time", diff,
				"threshold", EgyptEDAThreshold,
				"reductions", reductions)
			return newDiff
		}
		if config.IsEDA(new(big.Int).Add(parent.Number, big.NewInt(1)), time) && diff >= EDAThreshold {
			skippedIntervals := diff / EDAThreshold
			newDiff := applyMainnetEDAReduction(currentDiff, minDiff)
			log.Info("Emergency difficulty adjustment applied",
				"old", currentDiff,
				"new", newDiff,
				"block_time", diff,
				"threshold", EDAThreshold,
				"reductions", 1,
				"skipped_intervals", skippedIntervals)
			return newDiff
		}
	}

	if diff > targetTime*10 {
		log.Info("Long gap since parent block, keeping current difficulty",
			"difficulty", currentDiff,
			"block_time", diff,
			"target_time", targetTime)
		return currentDiff
	}

	if diff > 0 {
		// Calculate ratio: (targetTime * 100) / diff
		ratio := new(big.Int).SetUint64(targetTime)
		ratio.Mul(ratio, big.NewInt(100))
		ratio.Div(ratio, new(big.Int).SetUint64(diff))

		// Cap the ratio at 200 (2x)
		if ratio.Cmp(big.NewInt(200)) > 0 {
			ratio = big.NewInt(200) // 2x cap
		}

		// Minimum ratio: 0.5x (50)
		if ratio.Cmp(big.NewInt(50)) < 0 {
			ratio = big.NewInt(50) // 0.5x minimum
		}

		// Apply the ratio: newDiff = currentDiff * ratio / 100
		newDiff := new(big.Int).Mul(currentDiff, ratio)
		newDiff.Div(newDiff, big.NewInt(100))

		if newDiff.Cmp(minDiff) < 0 {
			return minDiff
		}

		if newDiff.Cmp(MaxDifficulty) > 0 {
			newDiff.Set(MaxDifficulty)
		}

		log.Info("Difficulty adjustment (2x cap)",
			"old", currentDiff,
			"new", newDiff,
			"ratio", fmt.Sprintf("%.2f", float64(ratio.Int64())/100),
			"block_time_ms", diff*1000)

		return newDiff
	}

	log.Info("⚠️ Block time too small, using current difficulty", "difficulty", currentDiff)
	return currentDiff
}

func chainConfig(chain consensus.ChainHeaderReader) *params.ChainConfig {
	if chain == nil {
		return nil
	}
	return chain.Config()
}

func isEgyptConfig(config *params.ChainConfig) bool {
	return config != nil && config.ChainID != nil && config.ChainID.Cmp(params.EgyptChainConfig.ChainID) == 0
}

func applyMainnetEDAReduction(currentDiff *big.Int, minDiff *big.Int) *big.Int {
	newDiff := new(big.Int).Mul(currentDiff, big.NewInt(75))
	newDiff.Div(newDiff, big.NewInt(100))
	if newDiff.Cmp(minDiff) < 0 {
		return new(big.Int).Set(minDiff)
	}
	return newDiff
}

func applyEDAReductions(currentDiff *big.Int, reductions uint64, minDiff *big.Int) *big.Int {
	newDiff := new(big.Int).Set(currentDiff)
	for i := uint64(0); i < reductions; i++ {
		newDiff.Div(newDiff, big.NewInt(4))
		if newDiff.Cmp(minDiff) <= 0 {
			return new(big.Int).Set(minDiff)
		}
	}
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

func (rx *RandomX) epochForBlock(chain consensus.ChainHeaderReader, blockNum uint64) uint64 {
	if chain != nil {
		if config := chain.Config(); config != nil && config.RandomXTxBlock != nil {
			activation := config.RandomXTxBlock.Uint64()
			if blockNum >= activation {
				blockNum -= activation
			} else {
				blockNum = 0
			}
		}
	}
	return rx.epoch(blockNum)
}

func (rx *RandomX) epoch(blockNum uint64) uint64 {
	epochLength := uint64(RandomXEpochLength)
	if rx.config != nil && rx.config.EpochLength > 0 {
		epochLength = rx.config.EpochLength
	}
	return blockNum / epochLength
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

func (rx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {
	rx.finalizeRewards(header, state, body)
}

// FinalizeAndAssemble finalizes RandomX state and assembles a block without dropping user transactions.
func (rx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	if body == nil {
		body = &types.Body{}
	}
	rx.finalizeRewards(header, state, body)
	if header.Coinbase != (common.Address{}) {
		rewards := rx.RewardTransactions(header, receipts)
		before := len(body.Transactions)
		body.Transactions = appendRewardTransactions(body.Transactions, rewards)
		if added := body.Transactions[before:]; len(added) > 0 {
			receipts = append(receipts, rewardReceipts(added, header, header.GasUsed)...)
		}
	}
	if len(receipts) > 0 {
		header.Bloom = types.MergeBloom(receipts)
	}
	eip158 := false
	if chain != nil && chain.Config() != nil {
		eip158 = chain.Config().IsEIP158(header.Number)
	}
	header.Root = state.IntermediateRoot(eip158)
	return types.NewBlock(header, body, receipts, trie.NewStackTrie(nil)), nil
}

func (rx *RandomX) finalizeRewards(header *types.Header, state vm.StateDB, body *types.Body) {
	blockNumber := header.Number.Uint64()
	rx.writeRotatingKingToState(state, blockNumber)
	if header.Coinbase == (common.Address{}) {
		log.Debug("RandomX finalize skipped rewards without coinbase", "block", blockNumber)
		return
	}
	if rx.distributeBodyRewardTransactions(state, body) {
		return
	}
	blockReward := CalculateBlockReward(blockNumber)
	log.Info("RandomX finalize rewards", "block", blockNumber, "coinbase", header.Coinbase.Hex(), "reward", FormatANTD(blockReward))
	if blockReward.Sign() > 0 {
		rx.distributeRewardsToState(state, header, blockReward)
	}
}

func (rx *RandomX) distributeBodyRewardTransactions(state vm.StateDB, body *types.Body) bool {
	if body == nil || len(body.Transactions) == 0 {
		return false
	}
	start := -1
	for i, tx := range body.Transactions {
		if types.IsBlockRewardTx(tx) {
			start = i
			break
		}
	}
	if start < 0 {
		return false
	}
	for _, tx := range body.Transactions[start:] {
		if !types.IsBlockRewardTx(tx) {
			return false
		}
	}
	for _, tx := range body.Transactions[start:] {
		to := tx.To()
		if to == nil || tx.Value().Sign() == 0 {
			continue
		}
		recipient := *to
		if rewardKind(tx) == types.BlockRewardRotatingKing && recipient == (common.Address{}) && rx.mainKing != (common.Address{}) {
			recipient = rx.mainKing
		}
		if recipient == (common.Address{}) {
			continue
		}
		state.AddBalance(recipient, uint256.MustFromBig(tx.Value()), tracing.BalanceIncreaseRewardMineBlock)
	}
	return true
}

func rewardKind(tx *types.Transaction) int {
	data := tx.Data()
	if len(data) == 0 {
		return -1
	}
	return int(data[len(data)-1])
}

func (rx *RandomX) writeRotatingKingToState(state vm.StateDB, blockNumber uint64) {
	rotatingKing := rx.getRotatingKing(blockNumber)
	state.SetState(params.SystemAddress, rotatingKingStateSlot, common.BytesToHash(rotatingKing.Bytes()))
}

// RewardTransactions returns the deterministic synthetic transactions for block rewards.
// StateProcessor uses this during block import to validate synced reward markers.
func (rx *RandomX) RewardTransactions(header *types.Header, receipts []*types.Receipt) []*types.Transaction {
	blockNumber := header.Number.Uint64()
	blockReward := CalculateBlockReward(blockNumber)
	totalReward := CalculateTotalReward(blockReward, nil)
	mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward := rx.rewardMarkerShares(header, totalReward)

	rewards := make([]*types.Transaction, 0, 3)
	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMainKing, mainKing, mainKingReward))
	}
	if rotatingKingReward.Sign() > 0 {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward))
	}
	if minerReward.Sign() > 0 && miner != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, minerReward))
	}
	return rewards
}

func (rx *RandomX) CompatibleRewardTransactions(header *types.Header, receipts []*types.Receipt) [][]*types.Transaction {
	canonical := rx.RewardTransactions(header, receipts)
	candidates := [][]*types.Transaction{canonical}
	for _, candidate := range [][]*types.Transaction{
		rx.legacyRewardTransactions(header, receipts),
		rx.fallbackRewardTransactions(header, receipts),
	} {
		if len(candidate) > 0 && !containsRewardTransactionSet(candidates, candidate) {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func containsRewardTransactionSet(candidates [][]*types.Transaction, target []*types.Transaction) bool {
	for _, candidate := range candidates {
		if sameRewardTransactions(candidate, target) {
			return true
		}
	}
	return false
}

func (rx *RandomX) fallbackRewardTransactions(header *types.Header, receipts []*types.Receipt) []*types.Transaction {
	blockNumber := header.Number.Uint64()
	blockReward := CalculateBlockReward(blockNumber)
	totalReward := CalculateTotalReward(blockReward, nil)
	mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward := rx.rewardShares(header, totalReward)
	rewards := make([]*types.Transaction, 0, 3)
	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMainKing, mainKing, mainKingReward))
	}
	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward))
	} else if rotatingKing == (common.Address{}) && mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward))
	}
	if minerReward.Sign() > 0 && miner != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, minerReward))
	}
	return rewards
}

func (rx *RandomX) legacyRewardTransactions(header *types.Header, receipts []*types.Receipt) []*types.Transaction {
	blockNumber := header.Number.Uint64()
	blockReward := CalculateBlockReward(blockNumber)
	totalReward := CalculateTotalReward(blockReward, nil)
	mainKing := rx.mainKing
	rotatingKing := rx.getRotatingKing(blockNumber)
	miner := header.Coinbase

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)
	if totalReward != nil && totalReward.Sign() > 0 {
		totalRewardBig := new(big.Int).Set(totalReward)
		mainKingReward.Mul(totalRewardBig, big.NewInt(10))
		mainKingReward.Div(mainKingReward, big.NewInt(100))
		rotatingKingReward.Mul(totalRewardBig, big.NewInt(40))
		rotatingKingReward.Div(rotatingKingReward, big.NewInt(100))
		minerReward.Mul(totalRewardBig, big.NewInt(50))
		minerReward.Div(minerReward, big.NewInt(100))
		actualTotal := new(big.Int).Add(mainKingReward, rotatingKingReward)
		actualTotal.Add(actualTotal, minerReward)
		if actualTotal.Cmp(totalRewardBig) != 0 {
			minerReward.Add(minerReward, new(big.Int).Sub(totalRewardBig, actualTotal))
		}
	}
	return []*types.Transaction{
		types.NewBlockRewardTx(blockNumber, types.BlockRewardMainKing, mainKing, mainKingReward),
		types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward),
		types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, minerReward),
	}
}

func (rx *RandomX) rewardMarkerShares(header *types.Header, totalReward *big.Int) (common.Address, *big.Int, common.Address, *big.Int, common.Address, *big.Int) {
	blockNumber := header.Number.Uint64()
	mainKing := rx.mainKing
	rotatingKing := rx.getRotatingKing(blockNumber)
	miner := header.Coinbase

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)
	if totalReward == nil || totalReward.Sign() == 0 {
		return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
	}
	totalRewardBig := new(big.Int).Set(totalReward)
	mainKingReward.Mul(totalRewardBig, big.NewInt(10))
	mainKingReward.Div(mainKingReward, big.NewInt(100))
	rotatingKingReward.Mul(totalRewardBig, big.NewInt(40))
	rotatingKingReward.Div(rotatingKingReward, big.NewInt(100))
	minerReward.Mul(totalRewardBig, big.NewInt(50))
	minerReward.Div(minerReward, big.NewInt(100))

	actualTotal := new(big.Int).Add(mainKingReward, rotatingKingReward)
	actualTotal.Add(actualTotal, minerReward)
	if actualTotal.Cmp(totalRewardBig) != 0 {
		minerReward.Add(minerReward, new(big.Int).Sub(totalRewardBig, actualTotal))
	}
	if mainKing == (common.Address{}) {
		minerReward.Add(minerReward, mainKingReward)
		minerReward.Add(minerReward, rotatingKingReward)
		mainKingReward = new(big.Int)
		rotatingKingReward = new(big.Int)
	}
	return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
}

func sameRewardTransactions(a, b []*types.Transaction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Hash() != b[i].Hash() {
			return false
		}
	}
	return true
}

func appendRewardTransactions(txs []*types.Transaction, rewards []*types.Transaction) []*types.Transaction {
	if len(rewards) == 0 {
		return txs
	}
	if len(txs) >= len(rewards) {
		matches := true
		start := len(txs) - len(rewards)
		for i := range rewards {
			if txs[start+i].Hash() != rewards[i].Hash() {
				matches = false
				break
			}
		}
		if matches {
			return txs
		}
	}
	return append(txs, rewards...)
}

func rewardReceipts(txs []*types.Transaction, header *types.Header, cumulativeGas uint64) []*types.Receipt {
	receipts := make([]*types.Receipt, 0, len(txs))
	for _, tx := range txs {
		receipts = append(receipts, &types.Receipt{
			Type:              tx.Type(),
			Status:            types.ReceiptStatusSuccessful,
			CumulativeGasUsed: cumulativeGas,
			TxHash:            tx.Hash(),
			GasUsed:           0,
			EffectiveGasPrice: new(big.Int),
		})
	}
	return receipts
}

func (rx *RandomX) rewardShares(header *types.Header, totalReward *big.Int) (common.Address, *big.Int, common.Address, *big.Int, common.Address, *big.Int) {
	blockNumber := header.Number.Uint64()
	mainKing := rx.mainKing
	rotatingKing := rx.getRotatingKing(blockNumber)
	miner := header.Coinbase

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)
	if totalReward == nil || totalReward.Sign() == 0 {
		return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
	}
	totalRewardBig := new(big.Int).Set(totalReward)
	mainKingReward.Mul(totalRewardBig, big.NewInt(10))
	mainKingReward.Div(mainKingReward, big.NewInt(100))
	rotatingKingReward.Mul(totalRewardBig, big.NewInt(40))
	rotatingKingReward.Div(rotatingKingReward, big.NewInt(100))
	minerReward.Mul(totalRewardBig, big.NewInt(50))
	minerReward.Div(minerReward, big.NewInt(100))

	actualTotal := new(big.Int).Add(mainKingReward, rotatingKingReward)
	actualTotal.Add(actualTotal, minerReward)
	if actualTotal.Cmp(totalRewardBig) != 0 {
		minerReward.Add(minerReward, new(big.Int).Sub(totalRewardBig, actualTotal))
	}
	if mainKing == (common.Address{}) {
		minerReward.Add(minerReward, mainKingReward)
		mainKingReward = new(big.Int)
	}
	if rotatingKing == (common.Address{}) {
		if mainKing != (common.Address{}) {
			mainKingReward.Add(mainKingReward, rotatingKingReward)
		} else {
			minerReward.Add(minerReward, rotatingKingReward)
		}
		rotatingKingReward = new(big.Int)
	}
	return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
}

// distributeRewardsToState distributes rewards using vm.StateDB interface
func (rx *RandomX) distributeRewardsToState(state vm.StateDB, header *types.Header, totalReward *big.Int) {
	blockNumber := header.Number.Uint64()
	mainKing, mainKingReward, rotatingKing, rotatingKingReward, coinbase, minerReward := rx.rewardShares(header, totalReward)

	log.Info("========================================")
	log.Info("REWARD DISTRIBUTION")
	log.Info("========================================")
	log.Info("Block", "number", blockNumber, "totalReward", FormatANTD(totalReward))

	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		state.AddBalance(mainKing, uint256.MustFromBig(mainKingReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Info("Main King reward", "address", mainKing.Hex(), "amount", FormatANTD(mainKingReward))
	}
	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		state.AddBalance(rotatingKing, uint256.MustFromBig(rotatingKingReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Info("Rotating King reward", "address", rotatingKing.Hex(), "amount", FormatANTD(rotatingKingReward))
	}
	if minerReward.Sign() > 0 && coinbase != (common.Address{}) {
		state.AddBalance(coinbase, uint256.MustFromBig(minerReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Info("Miner reward", "address", coinbase.Hex(), "amount", FormatANTD(minerReward))
	}

	log.Info("========================================")
	log.Info("REWARD DISTRIBUTION COMPLETE", "block", blockNumber, "totalReward", FormatANTD(totalReward))
	log.Info("========================================")
}

// SetRotationInterval updates how many blocks each rotating king receives rewards for.
func (rx *RandomX) SetRotationInterval(interval uint64) {
	if interval == 0 {
		return
	}
	rx.lock.Lock()
	defer rx.lock.Unlock()
	rx.rotationInterval = interval
}

// SetThreads updates how many CPU worker goroutines RandomX sealing uses.
func (rx *RandomX) SetThreads(threads int) {
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	rx.lock.Lock()
	defer rx.lock.Unlock()
	rx.miningThreads = threads
}

func (rx *RandomX) getMiningThreads() int {
	rx.lock.RLock()
	defer rx.lock.RUnlock()
	if rx.miningThreads <= 0 {
		return runtime.NumCPU()
	}
	return rx.miningThreads
}

// AddRotatingKing registers an address in the rotating king list if it is not present.
func (rx *RandomX) AddRotatingKing(address common.Address) {
	rx.AddRotatingKingAt(address, 0)
}

// AddRotatingKingAt registers an address in the rotating king list if it is not present.
func (rx *RandomX) AddRotatingKingAt(address common.Address, activationHeight uint64) {
	if address == (common.Address{}) {
		return
	}
	rx.lock.Lock()
	defer rx.lock.Unlock()
	if rx.rotatingKingActivations == nil {
		rx.rotatingKingActivations = make(map[common.Address]uint64)
	}
	for _, existing := range rx.rotatingKings {
		if existing == address {
			if current, ok := rx.rotatingKingActivations[address]; !ok || activationHeight < current {
				rx.rotatingKingActivations[address] = activationHeight
			}
			return
		}
	}
	rx.rotatingKings = append(rx.rotatingKings, address)
	rx.rotatingKingActivations[address] = activationHeight
}

// getRotatingKing returns the rotating king for a given block
func (rx *RandomX) getRotatingKing(blockNumber uint64) common.Address {
	rx.lock.RLock()
	defer rx.lock.RUnlock()
	if len(rx.rotatingKings) == 0 || rx.rotationInterval == 0 {
		return common.Address{}
	}

	var current common.Address
	for height := uint64(0); height <= blockNumber; height += rx.rotationInterval {
		active := rx.activeRotatingKingsAtLocked(height)
		if len(active) == 0 {
			continue
		}
		index := indexOfRotatingKing(active, current)
		if current == (common.Address{}) || index < 0 {
			current = active[0]
		} else if height != 0 {
			current = active[(index+1)%len(active)]
		}
	}
	return current
}

func (rx *RandomX) activeRotatingKingsAtLocked(blockNumber uint64) []common.Address {
	active := make([]common.Address, 0, len(rx.rotatingKings))
	for _, address := range rx.rotatingKings {
		if activation := rx.rotatingKingActivations[address]; activation > blockNumber {
			continue
		}
		active = append(active, address)
	}
	return active
}

func indexOfRotatingKing(addresses []common.Address, address common.Address) int {
	for index, candidate := range addresses {
		if candidate == address {
			return index
		}
	}
	return -1
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
	return api.randomx.seedHash(api.randomx.epochForBlock(api.randomx.chain, bn)), nil
}

func (api *RandomXAPI) GetCurrentEpoch(blockNumber uint64) uint64 {
	return api.randomx.epochForBlock(api.randomx.chain, blockNumber)
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
