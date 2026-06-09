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
//"encoding/binary"
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
RandomXCacheSize           = 256 * 1024 * 1024
RandomXDatasetSize         = 2 * 1024 * 1024 * 1024
MaxConcurrentVerifications = 32
shutdownTimeout            = 10 * time.Second
)

// RandomX flags matching Monero's implementation
const (
RANDOMX_FLAG_DEFAULT     = 0
RANDOMX_FLAG_FULL_MEM    = 1
RANDOMX_FLAG_JIT         = 2
RANDOMX_FLAG_HARD_AES    = 4
RANDOMX_FLAG_LARGE_PAGES = 8
RANDOMX_FLAG_SECURE      = 16
)

// Cache is a wrapper for RandomX cache
type Cache struct {
ptr *C.randomx_cache
}

// Dataset is a wrapper for RandomX dataset
type Dataset struct {
ptr *C.randomx_dataset
}

// VM is a wrapper for RandomX virtual machine
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

// Init initializes the cache with seed
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

// Close releases the cache
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

// InitDataset initializes the dataset from cache
func (d *Dataset) InitDataset(cache *Cache, start, count uint64) {
if d == nil || d.ptr == nil || cache == nil || cache.ptr == nil {
return
}
C.randomx_init_dataset(d.ptr, cache.ptr, C.ulong(start), C.ulong(count))
}

// Close releases the dataset
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

// CalculateHash computes RandomX hash
func (vm *VM) CalculateHash(input, output []byte) {
if vm == nil || vm.ptr == nil {
return
}
var inputPtr unsafe.Pointer
if len(input) > 0 {
inputPtr = unsafe.Pointer(&input[0])
}
C.randomx_calculate_hash(vm.ptr, inputPtr, C.size_t(len(input)), unsafe.Pointer(&output[0]))
}

// Close destroys the VM
func (vm *VM) Close() {
if vm != nil && vm.ptr != nil {
C.randomx_destroy_vm(vm.ptr)
vm.ptr = nil
}
}

// RandomX is a consensus engine based on proof-of-work implementing the RandomX algorithm.
type RandomX struct {
config           *params.RandomXConfig
lock             sync.RWMutex
fakeFail         *uint64
fullFake         bool
rotatingKings    []common.Address
rotationInterval uint64
cache            *Cache
dataset          *Dataset
cacheEpoch       uint64
cacheMu          sync.RWMutex
stopCh           chan struct{}
wg               sync.WaitGroup
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

// Author implements consensus.Engine
func (rx *RandomX) Author(header *types.Header) (common.Address, error) {
return header.Coinbase, nil
}

// VerifyHeader implements consensus.Engine
func (rx *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
return rx.verifyHeader(chain, header, nil)
}

// verifyHeader checks whether a header conforms to the RandomX consensus rules.
func (rx *RandomX) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
if rx.fullFake {
log.Debug("Full fake mode, accepting header", "number", header.Number)
return nil
}
if header.Number == nil {
return consensus.ErrInvalidNumber
}
if rx.fakeFail != nil && header.Number.Uint64() == *rx.fakeFail {
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

// VerifyHeaders implements consensus.Engine
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
header.Difficulty = new(big.Int).SetUint64(GenesisDifficulty)
}
}
return nil
}

// Finalize implements consensus.Engine
func (rx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {
log.Info("Finalizing block", "number", header.Number, "txs", len(body.Transactions))
}

// FinalizeAndAssemble implements consensus.Engine
func (rx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
log.Info("FinalizeAndAssemble called", "number", header.Number, "txs", len(body.Transactions))

if len(receipts) > 0 {
header.Bloom = types.MergeBloom(receipts)
}

block := types.NewBlock(header, body, receipts, nil)
return block, nil
}

// Seal implements consensus.Engine
func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
if rx.fullFake {
select {
case results <- block:
case <-stop:
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
case <-stop:
}
return nil
}

epoch := rx.epoch(header.Number.Uint64())
if err := rx.updateCacheForEpoch(epoch); err != nil {
return err
}

vm, err := rx.getVM()
if err != nil {
return err
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
if result.Cmp(target) > 0 {
continue
}
sealHeader.MixDigest = mixDigest
sealedBlock := block.WithSeal(sealHeader)
select {
case results <- sealedBlock:
case <-stop:
}
return nil
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

// VerifySeal verifies the RandomX proof-of-work of a header.
func (rx *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
if rx.fullFake {
return nil
}

// Accept zero mix digest only for genesis block (block 0)
// Block 0 has no proof-of-work, it's the starting point
if header.Number.Uint64() == 0 {
if header.MixDigest == (common.Hash{}) {
log.Info("ACCEPTING GENESIS BLOCK WITH ZERO MIX DIGEST", "number", header.Number)
return nil
}
// Genesis block must have zero mix digest
if header.MixDigest != (common.Hash{}) {
log.Error("GENESIS BLOCK MUST HAVE ZERO MIX DIGEST", "number", header.Number)
return errInvalidMixHash
}
}

// For blocks 1-10 (initial chain bootstrap), accept any mix digest
// This allows the chain to start and miners to connect
if header.Number.Uint64() >= 1 && header.Number.Uint64() <= 10 {
log.Info("ACCEPTING EARLY BLOCK FOR BOOTSTRAP", "number", header.Number, "mix_digest", header.MixDigest.Hex()[:16])
return nil
}

// For blocks 11 and above, require valid RandomX proof
if header.Number.Uint64() > 10 {
// Reject ANY block with zero mix digest for production blocks
if header.MixDigest == (common.Hash{}) {
log.Error("REJECTING BLOCK WITH ZERO MIX DIGEST - INVALID PROOF OF WORK",
"number", header.Number,
"hash", header.Hash().Hex())
return errInvalidMixHash
}
}

// Full RandomX verification for blocks 11 and above
epoch := rx.epoch(header.Number.Uint64())
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
mixDigest, result := rx.hashimoto(header, seed, vm)

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

log.Debug("Seal verified successfully", "number", header.Number)
return nil
}

// hashimoto is the core RandomX hash function
func (rx *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *VM) (common.Hash, *big.Int) {
input := make([]byte, 40)
sealHash := rx.SealHash(header)
copy(input[:32], sealHash.Bytes())
copy(input[32:], header.Nonce[:])

output := make([]byte, 32)
vm.CalculateHash(input, output)

mixDigest := common.BytesToHash(output)
result := new(big.Int).SetBytes(output)

return mixDigest, result
}

// getVM returns a RandomX VM for hash calculations
func (rx *RandomX) getVM() (*VM, error) {
rx.cacheMu.RLock()
defer rx.cacheMu.RUnlock()

if rx.cache == nil {
return nil, errNoCache
}

if rx.dataset != nil {
return NewVM(RANDOMX_FLAG_DEFAULT, nil, rx.dataset), nil
}

return NewVM(RANDOMX_FLAG_DEFAULT, rx.cache, nil), nil
}

// updateCacheForEpoch updates the RandomX cache for the given epoch
func (rx *RandomX) updateCacheForEpoch(epoch uint64) error {
rx.cacheMu.Lock()
defer rx.cacheMu.Unlock()

if rx.cacheEpoch == epoch && rx.cache != nil {
return nil
}

seed := rx.seedHash(epoch * rx.config.EpochLength)
seedBytes := seed.Bytes()

log.Info("Initializing RandomX for new epoch", "epoch", epoch, "seed", seed.Hex())

if rx.cache != nil {
rx.cache.Close()
rx.cache = nil
}
if rx.dataset != nil {
rx.dataset.Close()
rx.dataset = nil
}

startTime := time.Now()

// Create cache
rx.cache = NewCache(RANDOMX_FLAG_DEFAULT)
if rx.cache == nil {
return fmt.Errorf("failed to create RandomX cache")
}
rx.cache.Init(seedBytes)
log.Info("RandomX cache created", "epoch", epoch, "duration", time.Since(startTime))

// Try to create dataset for full mode
startTime = time.Now()
rx.dataset = NewDataset(RANDOMX_FLAG_DEFAULT)
if rx.dataset == nil {
log.Warn("Failed to create dataset, falling back to light mode")
} else {
rx.dataset.InitDataset(rx.cache, 0, 0)
log.Info("RandomX dataset created (FULL MODE)", "epoch", epoch, "duration", time.Since(startTime))
}

rx.cacheEpoch = epoch
return nil
}

// seedHash computes the seed hash for a given block number.
func (rx *RandomX) seedHash(blockNum uint64) common.Hash {
epoch := rx.epoch(blockNum)
seed := make([]byte, 32)
for i := uint64(0); i < epoch; i++ {
seed = crypto.Keccak256(seed)
}
return common.BytesToHash(seed)
}

// epoch returns the epoch for a given block number.
func (rx *RandomX) epoch(blockNum uint64) uint64 {
return blockNum / rx.config.EpochLength
}

// CalcDifficulty implements consensus.Engine
func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
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

// ==================== King Management Methods ====================

// GetMainKing returns the main king address
func (rx *RandomX) GetMainKing() common.Address {
rx.lock.RLock()
defer rx.lock.RUnlock()
if len(rx.rotatingKings) > 0 {
return rx.rotatingKings[0]
}
return common.Address{}
}

// GetRotatingKing returns the rotating king at the given block height
func (rx *RandomX) GetRotatingKing(blockHeight uint64) common.Address {
rx.lock.RLock()
defer rx.lock.RUnlock()
if len(rx.rotatingKings) == 0 {
return common.Address{}
}
if rx.rotationInterval == 0 {
return rx.rotatingKings[0]
}
index := (blockHeight / rx.rotationInterval) % uint64(len(rx.rotatingKings))
return rx.rotatingKings[index]
}

// SetRotatingKings sets the rotating king addresses
func (rx *RandomX) SetRotatingKings(kings []common.Address) {
rx.lock.Lock()
defer rx.lock.Unlock()
rx.rotatingKings = kings
}

// SetRotationInterval sets the rotation interval
func (rx *RandomX) SetRotationInterval(interval uint64) {
rx.lock.Lock()
defer rx.lock.Unlock()
rx.rotationInterval = interval
}

// ==================== RPC API ====================

// RandomXAPI is the RPC API for RandomX
type RandomXAPI struct {
randomx *RandomX
}

// GetSeedHash returns the RandomX seed hash for the next block
func (api *RandomXAPI) GetSeedHash(block *uint64) (common.Hash, error) {
if api.randomx == nil {
return common.Hash{}, nil
}
blockNumber := uint64(0)
if block != nil {
blockNumber = *block
}
return api.randomx.seedHash(blockNumber), nil
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

// GetMainKing returns the main king address via RPC
func (api *RandomXAPI) GetMainKing() common.Address {
return api.randomx.GetMainKing()
}

// GetRotatingKing returns the rotating king at the given block height
func (api *RandomXAPI) GetRotatingKing(blockHeight uint64) common.Address {
return api.randomx.GetRotatingKing(blockHeight)
}
