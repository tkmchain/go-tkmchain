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
	Enabled                    bool
	EpochLength                uint64
	CacheSize                  uint64
	DatasetSize                uint64
	MinMemory                  uint64
	PersistDataset             bool
	PostQuantumMainKingAddress common.Address
	QuantumResistantTime       *uint64
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

		// Diagnostic: attempt to compute and log the raw result and target to
		// help debugging external miner submissions that fail validation.
		// Best-effort only — don't fail the submission flow if diagnostics fail.
		epoch := rx.epochForBlock(nil, header.Number.Uint64())
		if uerr := rx.updateCacheForEpoch(epoch); uerr != nil {
			log.Warn("Failed to update RandomX cache for diagnostics", "err", uerr)
		} else {
			if vm, verr := rx.getVM(); verr == nil {
				defer vm.Close()
				result, sealHash := rx.randomXHash(header, vm)
				target := new(big.Int).Div(maxUint256, header.Difficulty)
				log.Warn("Invalid RandomX proof details",
					"nonce", nonce,
					"result", result.Text(16),
					"target", target.Text(16),
					"sealHash", sealHash.Hex(),
					"mixDigest", header.MixDigest.Hex(),
				)
			} else {
				log.Warn("Failed to create RandomX VM for diagnostics", "err", verr)
			}
		}

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
	if header == nil || header.Number == nil {
		return fmt.Errorf("invalid proof: missing block number")
	}

	num := header.Number.Uint64()
	if num == 0 {
		return nil
	}
	verifyChain := chain
	if verifyChain == nil {
		verifyChain = rx.chain
	}
	config := chainConfig(verifyChain)
	if config != nil && !config.IsRandomXTx(header.Number) {
		return nil
	}
	moneroProof := config != nil && config.IsRandomXMonero(header.Number)
	if header.Difficulty == nil || header.Difficulty.Sign() <= 0 {
		return fmt.Errorf("invalid proof: non-positive difficulty")
	}
	if !moneroProof && requiresStrictSealFields(num) {
		if header.MixDigest == (common.Hash{}) {
			return fmt.Errorf("invalid proof: empty mix digest")
		}
		if header.Nonce == (types.BlockNonce{}) {
			return fmt.Errorf("invalid proof: empty nonce")
		}
	}

	epoch := rx.epochForBlock(verifyChain, num)
	if err := rx.updateCacheForEpoch(epoch); err != nil {
		return err
	}

	vm, err := rx.getVM()
	if err != nil {
		return err
	}
	defer vm.Close()

	target := new(big.Int).Div(maxUint256, header.Difficulty)
	if moneroProof {
		return rx.verifyMoneroProof(header, vm)
	}

	// Try strict RandomX proof first
	if rx.validProof(header, vm, target) {
		return nil
	}

	kyoto := false
	if config != nil {
		kyoto = config.IsKyoto(header.Number, header.Time)
	}

	// Historical compatibility paths end at the mandatory pre-fork checkpoint.
	if rx.validProofWithNonceVariants(header, vm, target) {
		if kyoto {
			log.Warn("Accepted RandomX nonce byte-order variant", "number", header.Number.Uint64(), "hash", header.Hash())
		}
		return nil
	}

	// Historical external-miner blocks may carry a valid submitted result in
	// MixDigest even when current verifier builds cannot reproduce the exact
	// byte-order/VM combination. Keep this compatibility path below the strict
	// verifier attempts and bound post-Kyoto acceptance to the known historical
	// segment that was mined before the strict proof format was restored.
	if rx.allowStoredMixDigestProof(header, kyoto) && rx.validStoredMixDigest(header, target) {
		log.Warn("Accepted historical stored RandomX mix digest", "number", header.Number.Uint64(), "hash", header.Hash(), "kyoto", kyoto)
		return nil
	}

	// Early RandomX external-miner builds used a full-memory VM flag while later
	// verifier builds used the dataset without that flag. This still recomputes
	// the RandomX proof, so keep it available after Kyoto while rejecting the
	// stored-MixDigest shortcut above.
	if legacyVM, legacyErr := rx.getLegacyFullMemVM(); legacyErr == nil {
		defer legacyVM.Close()
		if rx.validProofWithNonceVariants(header, legacyVM, target) {
			log.Warn("Accepted legacy RandomX proof variant", "number", header.Number.Uint64(), "hash", header.Hash())
			return nil
		}
	}

	// Diagnostic: compute and log the raw RandomX result and target to aid debugging
	// of verification failures (nonce/difficulty/seed mismatches). Best-effort only.
	if vm != nil {
		if result, sealHash := rx.randomXHash(header, vm); result != nil && header.Difficulty != nil {
			target := new(big.Int).Div(maxUint256, header.Difficulty)
			log.Warn("Invalid RandomX proof details",
				"number", header.Number.Uint64(),
				"result", result.Text(16),
				"target", target.Text(16),
				"sealHash", sealHash.Hex(),
				"mixDigest", header.MixDigest.Hex(),
			)
		}
	} else {
		log.Warn("Invalid RandomX proof: VM unavailable for diagnostics", "number", header.Number.Uint64())
	}

	return fmt.Errorf("invalid proof: result > target")
}

func (rx *RandomX) verifyMoneroProof(header *types.Header, vm *VM) error {
	_, hash := rx.randomXHash(header, vm)
	if header.MixDigest != hash {
		return fmt.Errorf("invalid proof: mix digest mismatch: have %s, want %s", header.MixDigest, hash)
	}
	if !meetsMoneroDifficulty(hash, header.Difficulty) {
		return fmt.Errorf("invalid proof: little-endian RandomX result %s does not meet difficulty %s", hash, header.Difficulty)
	}
	return nil
}

// meetsMoneroDifficulty interprets the raw RandomX output as a little-endian
// 256-bit integer and applies Monero's hash*difficulty < 2^256 rule.
func meetsMoneroDifficulty(hash common.Hash, difficulty *big.Int) bool {
	if difficulty == nil || difficulty.Sign() <= 0 {
		return false
	}
	bytes := hash.Bytes()
	for left, right := 0, len(bytes)-1; left < right; left, right = left+1, right-1 {
		bytes[left], bytes[right] = bytes[right], bytes[left]
	}
	result := new(big.Int).SetBytes(bytes)
	return new(big.Int).Mul(result, difficulty).Cmp(maxUint256) < 0
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

const strictRandomXSealFieldsFromBlock = uint64(7888)

func requiresStrictSealFields(number uint64) bool {
	return number >= strictRandomXSealFieldsFromBlock
}

const (
	kyotoStoredMixDigestCompatUntil              = uint64(8192)
	privacyQuantumStoredMixDigestCompatFromBlock = uint64(20142)
	privacyQuantumStoredMixDigestCompatUntil     = uint64(20145)
	postPrivacyGapStoredMixDigestCompatFromBlock = uint64(20179)
	postPrivacyGapStoredMixDigestCompatUntil     = uint64(20373)
)

var postPrivacyGapStoredMixDigestCompatCheckpoints = map[uint64]common.Hash{
	20179: common.HexToHash("0xba8a7984e885eab29ff3042259233cdfef808aa5869ccb5caf0e24ad2e826f19"),
	20180: common.HexToHash("0x0cde0c10d5247b33ede3675b21740a4d6993e9baf13d900fc9d5a80595ded466"),
	20181: common.HexToHash("0xd792880f509cf59bb0e774133809833215b30942eb65d15652006ffcafd28338"),
	20182: common.HexToHash("0xa7a668c745267ab51146b841977375f0c76a8fd7fda5ffd149180327847e1467"),
	20183: common.HexToHash("0xf0d4a554e28b97fa8826df4b91ebbb573996e573078000d420a3378e677f08c5"),
	20184: common.HexToHash("0x5b8a800d1590c469765b19148e5223ba84ceaead9240c726d749a2594b9d62b3"),
	20185: common.HexToHash("0x50f9013cecc3af2fc040c5c0d0d885c3b948a4d8976a0358c7381ce61391991d"),
	20186: common.HexToHash("0x7c3b6829873eb16a4a67881e00a0e0aa28ae264d36b3be439de3d68eb2f8a633"),
	20187: common.HexToHash("0xa261d42d9c405bb53c1a35f1d1a23ddee6ca529a58379d53e0a76fec88afc1ff"),
	20188: common.HexToHash("0x08923e04f5aa109f4bf130ccb1d90c90b7f99f100ac0285530bf64c225197815"),
	20189: common.HexToHash("0x024487482aded2a57d47e2b7d39457ad4f7f5ff2d654b35739b5936b40de7028"),
	20190: common.HexToHash("0x8a61ae9e553e29cde6f6ff9a80b8215e958717b3dc349944c82c7a24d329eee9"),
	20191: common.HexToHash("0x0976fd36ca98f5efbb220dbcda79be3d72f6a043163c64f9b16ba822c1ba2479"),
	20192: common.HexToHash("0x37eaf319c6b791622b2006d6e19253aff759a72778b2f46bcf0fcdb05e79fcb8"),
	20193: common.HexToHash("0x7cd439491b7bb3effb3ea35a671cc3d7e8cb0afed72c6e09a8145a413f1cd18b"),
	20194: common.HexToHash("0x612c56757ebc99e75c7632b3ae5c0c37c09db09df23eb1aab55009f3a6d64e0d"),
	20195: common.HexToHash("0x2e4f550c5cbdd7b028e94051796d8830981f6a96f99b264e61ce52bd86125716"),
	20196: common.HexToHash("0xa4e7119618e5df9966bf1d3c9dbe042f076a83856f7f2ba74030f5e1db5a8ba2"),
	20197: common.HexToHash("0x974ace55515e9a23bca74151962c37734d2483f7695c69121de0e250f57f21d9"),
	20198: common.HexToHash("0xaee98eef71d771cae0be793c5aa55b82bb7389a87c303245e559d464d0da30de"),
	20199: common.HexToHash("0x9e5034b91ae9190cffb5fcd37ad52ebdda75168173c39d4e99343a54e4ad16f0"),
	20200: common.HexToHash("0x804d586af4bb4a89fa5fe7079964458c74fc5bced6ce63759d600a6afc5ab8a9"),
	20201: common.HexToHash("0x7a30a757d0590b7b4c56d77f238c2321283fcbdee8b8d4334d37e60b9ba939a2"),
	20202: common.HexToHash("0xa74977f01eab34e64431a6321c380d0656430a3262d9e1a640912a4516ac4d0e"),
	20203: common.HexToHash("0xdf54e60862c2c5a2dcba016e12f211a6409715d4ba7fc712c3941a1486d1d924"),
	20204: common.HexToHash("0xbefe93d86cb9ef86cdab621c0230421ff08c400d5a3287f8c79c9c8346976631"),
	20205: common.HexToHash("0x1b40ef16292c20ce5f727011cd13521adb079c40ed1f13059774c753dab4726c"),
	20206: common.HexToHash("0x725d7d5af973db7ca57da08aa4f5efef8c90ff853b588fb8ecd8c86a96297a0d"),
	20207: common.HexToHash("0xf49ed9bc9c273371c0c95db83ddef46d1f4116c76dffde6d1361189881d10637"),
	20208: common.HexToHash("0xb9bc66a1e1d91a95fa0537b9cce94d896a23969b65b3dc13ea69e7f604be60e1"),
	20209: common.HexToHash("0xba5977df68bcf2522e61170e6c538a65097e2e7247303442c99912dff203861c"),
	20210: common.HexToHash("0xcfb7a8edddc1c276d882aac5090c3418cd8b5577b4a80a3629ebbdd4b285855b"),
	20211: common.HexToHash("0x5658a264caae095fbf524d5cafb15dc181084f4823bead5eb0ee8a713b5dcc92"),
	20212: common.HexToHash("0x73e126fc6eae97799ca6cafa0a87b6cdce6e992fa966e23139c211e5da7b8a55"),
	20213: common.HexToHash("0x1851a23a5cd3d8df159d6f86ef9a75de3adc0e5fbd050cbe2a6225cfa6590da6"),
	20214: common.HexToHash("0x44f2dee9003008d78f6360d5ccd51ff49d768470e2c44dba19b949fc41aa2a8b"),
	20215: common.HexToHash("0xe2adb549ccb8baf59e93e11804c3015273271beefd1537c7558fd9712b7761a5"),
	20216: common.HexToHash("0x0fda89d2132dce6e2e806bb867c9777a2eae3f9cfbf51bca5ef0f78f9582ac7e"),
	20217: common.HexToHash("0x4cdcb417ac3b6d60c1d70455be389642036f462488c12830ea5423fb0578a3e6"),
	20218: common.HexToHash("0x731f756599fa738b71f7d2c6f88ce29c60936f43289a323c656d661b9708d9ca"),
	20219: common.HexToHash("0x2a685565781f68cd2e58a1c4f279fda069135095ac84b4af1e5609fed6d8ebab"),
	20220: common.HexToHash("0xc7afeaf23abdd95e4bc9bdf7ff3e12c5d7ce77eac8e03e083d4adc6886a4df47"),
	20221: common.HexToHash("0x0934ae091a3896fbb77f59593ead421a007fe04040ecf96d5dd27de5c479c2f2"),
	20222: common.HexToHash("0x4468b53588b2de6d7eb84755f6f8be3b2379e0af3a709784a8a26912ccb2fe09"),
	20223: common.HexToHash("0x9b1c7cc401be97f387a75180797e42dc671cffc86cf9511739673f47a999033a"),
	20224: common.HexToHash("0x7c23ec1a127842e4c385509412a1addc3d7a3e63f728871c54e1ccb86fdace59"),
	20225: common.HexToHash("0x23ff90146789000e37a52ba46d4590736639ee8fdaba2ac3b4aafc857d32b564"),
	20226: common.HexToHash("0xe87fa0c95429ecc40c784dd2958d925222aa0d0debb86f01cc3a67027f72a3bc"),
	20227: common.HexToHash("0xbbb0e77b0c8a35491d6a57a7d73c6c0080d8c6af5b5824278a046ba3f6c1c0d6"),
	20228: common.HexToHash("0x69a2d4417086303df6d1c3cbf7ae1154a380db22b7747fd3e83ae81a1ba316bb"),
	20229: common.HexToHash("0x289ad6a53f84258fe3a626f8c7a45df0b62af012e470eb173f2ec56192b85241"),
	20230: common.HexToHash("0x2fc5ab283ce18306d4cfea53a9fb003b42fd685effcdbe352b0e2182098a1baf"),
	20231: common.HexToHash("0xc8d39c66210c06eb059ea167b3f9d6eca2b1c961a8179caff8e69173eea6c448"),
	20232: common.HexToHash("0x19397d4dd32167cc9fb43346ced2f915f0f642e057d71f24a27c702fb8225e3e"),
	20233: common.HexToHash("0xf471fd7e58873db042901bb01f99ab5b72583a2759cddfdf0f54206bb05847be"),
	20234: common.HexToHash("0x164ef029d759b548d75f99d965c03b7e8477341ef962c4b262ae33d7c2b85fdd"),
	20235: common.HexToHash("0xb6fc3d2ae0aaba737e62e294597c438f8c2e12baeaeb7cdc37dd3b7e4adf97ac"),
	20236: common.HexToHash("0x0114c01d985da734a2a8d3ad4ee137f94107230c5ab1d527b2b75d8d048a00e9"),
	20237: common.HexToHash("0xa6866c6723ece700a365ddc7793246b4cb4df93053578da386b147256ef0f441"),
	20238: common.HexToHash("0x6c04dcb023b4e8b1b0444322d1a173bc5e61952fd62c91329b9f84975720941c"),
	20239: common.HexToHash("0x7a9908be27fc17404429cd3d2bee8673716fcd7a5b7be9900ff8d4b7fc19b82a"),
	20240: common.HexToHash("0xb93c5703d00b30b1d66aab69874a7a4b8cc2f50bdd00753c8ae5988447be955c"),
	20241: common.HexToHash("0x38472ebe28b0044f636e9c97fce3ab8b995398116223d1e3c90e08bb18619765"),
	20242: common.HexToHash("0xf783f3d00e6cdd5303eceef355343757dd7aeb474256590cab0fb2c9b9997733"),
	20243: common.HexToHash("0x8a068fab0b7349b7ff12299ae1c8a5414743c33860f834af951901cc81a183c6"),
	20244: common.HexToHash("0xb7e535fa74fbddc2d7704d3606a78c2536bd50e21dfaa2a76a7af4babb48a51c"),
	20245: common.HexToHash("0xfc3c724df50ac9abe9105b1f36cf683eaa99bd232e7791fe893a639f6ecbe3ea"),
	20246: common.HexToHash("0x154321c94b107d58265a32bd2aabd738ceebad9737bda9654e85fb3e7e5bd9d4"),
	20247: common.HexToHash("0x799f03b68c8bc9447d94dc92641b63fb0cce7ea19217269da822aa7f4fb1fa25"),
	20248: common.HexToHash("0x5259316ad595f8a85aacc567f640963a0e04bc4c1d5c0750044a87b7c05a4b3b"),
	20249: common.HexToHash("0x3b411050254617d7d1f3a748a69b04a129685e851c16a5ce171e6b6ef4d69714"),
	20250: common.HexToHash("0x4b0a8c197555bd3a2f31836bfb320829542bff2a39bd6abf49fee1e286eb5125"),
	20251: common.HexToHash("0x5fe26b9bfb1c1dd96af63f88d6a233d1150167a5cc0bab30e63334fe32468f2a"),
	20252: common.HexToHash("0xf0a657946af44d1c723d75faf4d70a1c2ed3a02b337bd2b8916fce3c0b713922"),
	20253: common.HexToHash("0xc0b636c5925bf6d4dbdd301e2446463300252863908a550e3d01799f2add2749"),
	20254: common.HexToHash("0xa0522928b331b8102a3289dc7bf20aaa507c15f234ded5f868ad1cd7569d0e46"),
	20255: common.HexToHash("0x24a1fa59e7fd895a505edf1f07ec6862bf9368b5f074f658410f23de8751d6ce"),
	20256: common.HexToHash("0xe2fee3b5bc301881cec560a7347d6d123236dd319a5c39ed141b5de749f00766"),
	20257: common.HexToHash("0x519890e0b47f4e9a6f786fe415e7175d4075c04be56fa0ab6d6cb6ae1062c255"),
	20258: common.HexToHash("0x6211040cf3a0b7fef242482c6d027d63c7776e06ed704d4609793004f414757f"),
	20259: common.HexToHash("0x23e4050af7571711d2c508c738b0699f46c61f86c465d3bcd88d5b95fd74409a"),
	20260: common.HexToHash("0xbdc8141ea8c384ad85d9d89e98cb5bee72a2293df73afcb248257bdc95253481"),
	20261: common.HexToHash("0xf8b43cc91baf5bd363466d827ee24e86a18a10f8c73811f8baf472faa9b4d1ef"),
	20262: common.HexToHash("0xa9bf672918ef526426b95567f1074d9581838f17c158992b837823bbd13d2655"),
	20263: common.HexToHash("0xf1e85f7f0450a6c12b53e93f4b56b2f20c243e7e5ca4f5eaefc17373ca49f139"),
	20264: common.HexToHash("0x785ec1f8a9d903a44dfb38147aac83a17c4191fc5e8858ec98e56896c6f1a11e"),
	20265: common.HexToHash("0xd89e4f9a772cb91e8a9bf4add699017e071eaaa9915174d15b38192ffe6dba40"),
	20266: common.HexToHash("0x0f2320687171d71804086b1e6bea44375bfb640a86a1ccf038ace383033ed989"),
	20369: common.HexToHash("0xbe28029213fd28ad8393a3f2ee76982441d2e4de09b0b3d3b5e77cdfff4533e2"),
	20370: common.HexToHash("0xc53450d9108bbedf82a74dfa063277f0072b8320bb35fe7dedd34624d94d4434"),
	20371: common.HexToHash("0x582809dbdc5f1ea71c5565497e925e7af2fead9f19f4c81b795e11c82852f539"),
	20372: common.HexToHash("0x44c6076c7abbec0cf22742856df7ab9703b1649de99ce72b1701256b156d91bd"),
	20373: common.HexToHash("0x6386f2bea3d034883ce29af5777408caeb3ecc8e055f25af79c8098d8de3fcea"),
}

func (rx *RandomX) allowStoredMixDigestProof(header *types.Header, kyoto bool) bool {
	if !kyoto {
		return true
	}
	if header == nil || header.Number == nil {
		return false
	}
	number := header.Number.Uint64()
	if number <= kyotoStoredMixDigestCompatUntil {
		return true
	}
	inPrivacyQuantumCompat := number >= privacyQuantumStoredMixDigestCompatFromBlock && number <= privacyQuantumStoredMixDigestCompatUntil
	inPostPrivacyGapCompat := number >= postPrivacyGapStoredMixDigestCompatFromBlock && number <= postPrivacyGapStoredMixDigestCompatUntil
	if !inPrivacyQuantumCompat && !inPostPrivacyGapCompat {
		return false
	}
	if inPostPrivacyGapCompat {
		checkpoint, ok := postPrivacyGapStoredMixDigestCompatCheckpoints[number]
		return ok && checkpoint == header.Hash()
	}
	checkpoint, ok := params.GetCheckpoint(number)
	return ok && checkpoint == header.Hash()
}

func (rx *RandomX) validStoredMixDigest(header *types.Header, target *big.Int) bool {
	if header.MixDigest == (common.Hash{}) {
		return false
	}
	return new(big.Int).SetBytes(header.MixDigest.Bytes()).Cmp(target) <= 0
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

				valid := result.Cmp(target) <= 0
				if config := chainConfig(chain); config != nil && config.IsRandomXMonero(localHeader.Number) {
					valid = meetsMoneroDifficulty(hash, localHeader.Difficulty)
				}
				if valid {
					localHeader.MixDigest = hash
					sealedBlock := block.WithSeal(localHeader)

					log.Info("✅ BLOCK MINED!",
						"block", localHeader.Number.Uint64(),
						"difficulty", localHeader.Difficulty,
						"nonce", nonce,
						"thread", threadID,
						"hash", hash.Hex(),
						"result", result.Text(16))
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
			header.Difficulty = new(big.Int).Set(MinDifficulty)
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
	edaMinDiff := EDAMinDifficulty

	if config := chainConfig(chain); config != nil {
		if config.IsPhone(new(big.Int).Add(parent.Number, big.NewInt(1)), time) {
			newDiff := calcPhoneDifficulty(currentDiff, diff, targetTime, edaMinDiff)
			log.Info("Phone hardfork difficulty adjustment",
				"old", currentDiff,
				"new", newDiff,
				"block_time", diff,
				"target_time", targetTime)
			return newDiff
		}
		if isEgyptConfig(config) && diff >= EgyptEDAThreshold {
			reductions := diff / EgyptEDAThreshold
			newDiff := applyEDAReductions(currentDiff, reductions, edaMinDiff)
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
			newDiff := applyMainnetEDAReductions(currentDiff, skippedIntervals, edaMinDiff)
			log.Info("Emergency difficulty adjustment applied",
				"old", currentDiff,
				"new", newDiff,
				"block_time", diff,
				"threshold", EDAThreshold,
				"reductions", skippedIntervals,
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

func calcPhoneDifficulty(currentDiff *big.Int, blockTime uint64, targetTime uint64, minDiff *big.Int) *big.Int {
	if currentDiff == nil || currentDiff.Sign() <= 0 {
		return new(big.Int).Set(minDiff)
	}
	if blockTime == 0 {
		return new(big.Int).Set(currentDiff)
	}
	if targetTime == 0 {
		targetTime = uint64(TargetBlockTime)
	}

	newDiff := new(big.Int).Set(currentDiff)
	if blockTime > targetTime {
		delta := blockTime - targetTime
		adjustment := new(big.Int).Mul(currentDiff, new(big.Int).SetUint64(delta))
		adjustment.Div(adjustment, new(big.Int).SetUint64(targetTime*8))
		maxDrop := new(big.Int).Div(currentDiff, big.NewInt(4))
		if adjustment.Cmp(maxDrop) > 0 {
			adjustment.Set(maxDrop)
		}
		if adjustment.Sign() == 0 {
			adjustment.SetInt64(1)
		}
		newDiff.Sub(newDiff, adjustment)

		if blockTime >= EDAThreshold {
			reductions := blockTime / EDAThreshold
			if reductions > 1 {
				newDiff = applyMainnetEDAReductions(newDiff, reductions-1, minDiff)
			}
		}
	} else if blockTime < targetTime {
		delta := targetTime - blockTime
		adjustment := new(big.Int).Mul(currentDiff, new(big.Int).SetUint64(delta))
		adjustment.Div(adjustment, new(big.Int).SetUint64(targetTime*4))
		maxRise := new(big.Int).Div(currentDiff, big.NewInt(4))
		if adjustment.Cmp(maxRise) > 0 {
			adjustment.Set(maxRise)
		}
		if adjustment.Sign() == 0 {
			adjustment.SetInt64(1)
		}
		newDiff.Add(newDiff, adjustment)
	}

	if newDiff.Cmp(minDiff) < 0 {
		return new(big.Int).Set(minDiff)
	}
	if newDiff.Cmp(MaxDifficulty) > 0 {
		return new(big.Int).Set(MaxDifficulty)
	}
	return newDiff
}

func applyMainnetEDAReductions(currentDiff *big.Int, reductions uint64, minDiff *big.Int) *big.Int {
	newDiff := new(big.Int).Set(currentDiff)
	for i := uint64(0); i < reductions; i++ {
		newDiff.Mul(newDiff, big.NewInt(75))
		newDiff.Div(newDiff, big.NewInt(100))
		if newDiff.Cmp(minDiff) <= 0 {
			return new(big.Int).Set(minDiff)
		}
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
	rx.finalizeRewards(chain, header, state, body)
}

// FinalizeAndAssemble finalizes RandomX state and assembles a block without dropping user transactions.
func (rx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	if body == nil {
		body = &types.Body{}
	}
	rx.finalizeRewards(chain, header, state, body)
	if header.Coinbase != (common.Address{}) && !rx.skipImplicitRewards(chain, header, body) {
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

func (rx *RandomX) finalizeRewards(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {
	blockNumber := header.Number.Uint64()
	rx.writeRotatingKingToState(state, blockNumber)
	if header.Coinbase == (common.Address{}) {
		log.Debug("RandomX finalize skipped rewards without coinbase", "block", blockNumber)
		return
	}
	if rx.distributeBodyRewardTransactions(state, body) {
		return
	}
	if rx.skipImplicitRewards(chain, header, body) {
		rx.distributeKyotoEmptyBlockRewards(state, header, CalculateBlockReward(blockNumber))
		return
	}
	blockReward := CalculateBlockReward(blockNumber)
	log.Info("RandomX finalize rewards", "block", blockNumber, "coinbase", header.Coinbase.Hex(), "reward", FormatANTD(blockReward))
	if blockReward.Sign() > 0 {
		rx.distributeRewardsToState(state, header, blockReward)
	}
}

func (rx *RandomX) skipImplicitRewards(chain consensus.ChainHeaderReader, header *types.Header, body *types.Body) bool {
	if chain == nil || chain.Config() == nil || !chain.Config().IsKyoto(header.Number, header.Time) {
		return false
	}
	if body == nil || len(body.Transactions) == 0 {
		return true
	}
	for _, tx := range body.Transactions {
		if types.IsBlockRewardTx(tx) {
			return false
		}
	}
	return true
}

func (rx *RandomX) distributeKyotoEmptyBlockRewards(state vm.StateDB, header *types.Header, blockReward *big.Int) {
	if blockReward == nil || blockReward.Sign() == 0 {
		return
	}
	mainKingReward := new(big.Int).Mul(blockReward, big.NewInt(50))
	mainKingReward.Div(mainKingReward, big.NewInt(100))
	minerReward := new(big.Int).Sub(new(big.Int).Set(blockReward), mainKingReward)
	mainKing := rx.mainKingAt(header)
	if mainKing != (common.Address{}) && mainKingReward.Sign() > 0 {
		state.AddBalance(mainKing, uint256.MustFromBig(mainKingReward), tracing.BalanceIncreaseRewardMineBlock)
	}
	if header.Coinbase != (common.Address{}) && minerReward.Sign() > 0 {
		state.AddBalance(header.Coinbase, uint256.MustFromBig(minerReward), tracing.BalanceIncreaseRewardMineBlock)
	}
}

func (rx *RandomX) FinalizeKyotoEmptyBlockForRoot(chain consensus.ChainHeaderReader, header *types.Header, statedb *state.StateDB, body *types.Body, targetRoot common.Hash, eip158 bool) bool {
	if chain == nil || chain.Config() == nil || !chain.Config().IsKyoto(header.Number, header.Time) {
		return false
	}
	if header.Coinbase == (common.Address{}) || body == nil {
		return false
	}
	for _, tx := range body.Transactions {
		if types.IsBlockRewardTx(tx) {
			return false
		}
	}
	blockNumber := header.Number.Uint64()
	blockReward := CalculateBlockReward(blockNumber)
	type candidate struct {
		name  string
		apply func(vm.StateDB)
	}
	candidates := []candidate{
		{name: "rotating-slot-kyoto-50-50", apply: func(s vm.StateDB) {
			rx.writeRotatingKingToState(s, blockNumber)
			rx.distributeKyotoEmptyBlockRewards(s, header, blockReward)
		}},
		{name: "kyoto-50-50", apply: func(s vm.StateDB) {
			rx.distributeKyotoEmptyBlockRewards(s, header, blockReward)
		}},
		{name: "rotating-slot-only", apply: func(s vm.StateDB) {
			rx.writeRotatingKingToState(s, blockNumber)
		}},
		{name: "no-finalize", apply: func(s vm.StateDB) {}},
		{name: "normal-implicit", apply: func(s vm.StateDB) {
			rx.writeRotatingKingToState(s, blockNumber)
			rx.distributeRewardsToState(s, header, blockReward)
		}},
	}
	for _, king := range historicalRotatingKingAddresses() {
		king := king
		candidates = append(candidates, candidate{
			name: "historical-rotating-slot-kyoto-50-50-" + king.Hex(),
			apply: func(s vm.StateDB) {
				writeRotatingKingToStateValue(s, king)
				rx.distributeKyotoEmptyBlockRewards(s, header, blockReward)
			},
		})
		candidates = append(candidates, candidate{
			name: "historical-rotating-slot-only-" + king.Hex(),
			apply: func(s vm.StateDB) {
				writeRotatingKingToStateValue(s, king)
			},
		})
		candidates = append(candidates, candidate{
			name: "historical-rotating-slot-normal-implicit-" + king.Hex(),
			apply: func(s vm.StateDB) {
				writeRotatingKingToStateValue(s, king)
				rx.distributeRewardsToStateWithRotatingKing(s, header, blockReward, king)
			},
		})
	}
	for _, candidate := range candidates {
		copyState := statedb.Copy()
		candidate.apply(copyState)
		root := copyState.IntermediateRoot(eip158)
		if root != targetRoot {
			continue
		}
		candidate.apply(statedb)
		log.Warn("Accepted Kyoto historical finalization", "block", blockNumber, "mode", candidate.name, "root", root)
		return true
	}
	log.Warn("No Kyoto historical finalization matched", "block", blockNumber, "target", targetRoot)
	return false
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

func (rx *RandomX) mainKingAt(header *types.Header) common.Address {
	if rx == nil || rx.config == nil || header == nil {
		return common.Address{}
	}
	if rx.config.PostQuantumMainKingAddress != (common.Address{}) && rx.config.QuantumResistantTime != nil && header.Time >= *rx.config.QuantumResistantTime {
		return rx.config.PostQuantumMainKingAddress
	}
	return rx.mainKing
}

func rewardKind(tx *types.Transaction) int {
	data := tx.Data()
	if len(data) == 0 {
		return -1
	}
	return int(data[len(data)-1])
}

func (rx *RandomX) writeRotatingKingToState(state vm.StateDB, blockNumber uint64) {
	writeRotatingKingToStateValue(state, rx.getRotatingKing(blockNumber))
}

func writeRotatingKingToStateValue(state vm.StateDB, rotatingKing common.Address) {
	state.SetState(params.SystemAddress, rotatingKingStateSlot, common.BytesToHash(rotatingKing.Bytes()))
}

func historicalRotatingKingAddresses() []common.Address {
	return []common.Address{
		common.HexToAddress("0x08959f2d8aaeb6a1a27a2dc3f6d0b07fd1fba21a"),
		common.HexToAddress("0xa934a0a34a11eaeadc9a850d1017db2cb2216672"),
		common.HexToAddress("0xa76a7a2ec8c25400739ef5556cca8ae0c6247123"),
		common.HexToAddress("0x577ebcf6d33f394e153156b1570fef0ab0ab4b3a"),
		common.HexToAddress("0x7a1e6b083943a95d45e8f3a07b90f93e63c0dbae"),
		common.HexToAddress("0x4901ae660c6346c633d09a800056368d3cd6199c"),
		common.HexToAddress("0xcea49fe4228945e23aa5fa3ddfdc643ceabae1e7"),
		common.HexToAddress("0x5a23aeec3ae9d21c93f5c560b4e080a6e3a68479"),
		common.HexToAddress("0xa5fa147d3705e2fc6aa80314c22f2673fef7a0ba"),
		common.HexToAddress("0x618d891c7faafc4769c398100d9ff0eca1e48ffb"),
		common.HexToAddress("0xf4829bb30f5f0b8d009a1f1f2fffb891878b2f73"),
		common.HexToAddress("0xb53681fc516570aa8c73266762c5494db7e5a260"),
		common.HexToAddress("0x4f8194062c21cfe921d294e79c6b913efc8f70ce"),
	}
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
	mainKing := rx.mainKingAt(header)
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
	mainKing := rx.mainKingAt(header)
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
	mainKing := rx.mainKingAt(header)
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
func (rx *RandomX) distributeRewardsToStateWithRotatingKing(state vm.StateDB, header *types.Header, totalReward *big.Int, rotatingKing common.Address) {
	mainKing, mainKingReward, _, rotatingKingReward, coinbase, minerReward := rx.rewardShares(header, totalReward)
	if rotatingKing == (common.Address{}) {
		if mainKing != (common.Address{}) {
			mainKingReward.Add(mainKingReward, rotatingKingReward)
		} else {
			minerReward.Add(minerReward, rotatingKingReward)
		}
		rotatingKingReward = new(big.Int)
	}
	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		state.AddBalance(mainKing, uint256.MustFromBig(mainKingReward), tracing.BalanceIncreaseRewardMineBlock)
	}
	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		state.AddBalance(rotatingKing, uint256.MustFromBig(rotatingKingReward), tracing.BalanceIncreaseRewardMineBlock)
	}
	if minerReward.Sign() > 0 && coinbase != (common.Address{}) {
		state.AddBalance(coinbase, uint256.MustFromBig(minerReward), tracing.BalanceIncreaseRewardMineBlock)
	}
}

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
