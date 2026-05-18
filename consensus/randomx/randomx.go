// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package randomx implements the RandomX proof-of-work consensus engine.
package randomx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/keccak"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
	"git.gammaspectra.live/P2Pool/go-randomx/v3"
)

// RandomX proof-of-work protocol constants.
var (
	FrontierBlockReward           = uint256.NewInt(5e+18) // Block reward in wei for successfully mining a block
	ByzantiumBlockReward          = uint256.NewInt(3e+18) // Block reward in wei for successfully mining a block upward from Byzantium
	ConstantinopleBlockReward     = uint256.NewInt(2e+18) // Block reward in wei for successfully mining a block upward from Constantinople
	maxUncles                     = 2                     // Maximum number of uncles allowed in a single block
	allowedFutureBlockTimeSeconds = int64(15)             // Max seconds from current time allowed for blocks, before they're considered future blocks
	maxUint256                    = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
)

// Various error messages to mark blocks invalid.
var (
	errOlderBlockTime  = errors.New("timestamp older than parent")
	errTooManyUncles   = errors.New("too many uncles")
	errDuplicateUncle  = errors.New("duplicate uncle")
	errUncleIsAncestor = errors.New("uncle is ancestor")
	errDanglingUncle   = errors.New("uncle's parent is not ancestor")
	errInvalidMixHash  = errors.New("invalid mix hash")
	errNoCache         = errors.New("randomx cache not initialized")
	errInvalidSeed     = errors.New("invalid randomx seed")
)

// RandomX is a consensus engine based on proof-of-work implementing the RandomX algorithm.
type RandomX struct {
	config      *params.RandomXConfig
	threads     int
	cache       *randomx.Cache
	dataset     *randomx.Dataset
	cacheEpoch  uint64
	cacheMu     sync.RWMutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
	
	// Testing/fake mode flags (only for unit tests)
	fakeMode    bool
	fakeFail    *uint64
	fakeDelay   *time.Duration
}

// New creates a new RandomX consensus engine.
func New(config *params.RandomXConfig, threads int) (*RandomX, error) {
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if config == nil {
		config = DefaultConfig()
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
	if config.MinMemory == 0 {
		config.MinMemory = 4 * 1024 * 1024 * 1024 // 4GB
	}

	return &RandomX{
		config:  config,
		threads: threads,
		stopCh:  make(chan struct{}),
	}, nil
}

// DefaultConfig returns the default RandomX configuration.
func DefaultConfig() *params.RandomXConfig {
	return &params.RandomXConfig{
		EpochLength:    2048,
		CacheSizeMB:    256,
		DatasetSizeGB:  2,
		MinMemory:      4 * 1024 * 1024 * 1024,
	}
}

// NewFaker creates a RandomX engine that skips proof-of-work verification (testing only).
func NewFaker() *RandomX {
	engine, _ := New(nil, 1)
	engine.fakeMode = true
	return engine
}

// NewFullFaker creates a RandomX engine that accepts all headers without verification (testing only).
func NewFullFaker() *RandomX {
	engine := NewFaker()
	engine.fakeMode = true
	return engine
}

// NewFakeFailer creates a RandomX engine that fails a specific block number (testing only).
func NewFakeFailer(fail uint64) *RandomX {
	engine := NewFaker()
	engine.fakeFail = &fail
	return engine
}

// NewFakeDelayer creates a RandomX engine with verification delay (testing only).
func NewFakeDelayer(delay time.Duration) *RandomX {
	engine := NewFaker()
	engine.fakeDelay = &delay
	return engine
}

// Author implements consensus.Engine, returning the header's coinbase.
func (randomx *RandomX) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (randomx *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	if randomx.fakeMode {
		if randomx.fakeDelay != nil {
			time.Sleep(*randomx.fakeDelay)
		}
		if randomx.fakeFail != nil && *randomx.fakeFail == header.Number.Uint64() {
			return errors.New("invalid tester pow")
		}
		return nil
	}

	// Short circuit if the header is known
	number := header.Number.Uint64()
	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}
	
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	
	return randomx.verifyHeader(chain, header, parent, false, seal, time.Now().Unix())
}

// VerifyHeaders verifies a batch of headers concurrently.
func (randomx *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	if randomx.fakeMode || len(headers) == 0 {
		abort, results := make(chan struct{}), make(chan error, len(headers))
		for i := 0; i < len(headers); i++ {
			results <- nil
		}
		return abort, results
	}

	abort := make(chan struct{})
	results := make(chan error, len(headers))
	unixNow := time.Now().Unix()

	go func() {
		for i, header := range headers {
			var parent *types.Header
			if i == 0 {
				parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
			} else if headers[i-1].Hash() == headers[i].ParentHash {
				parent = headers[i-1]
			}

			var err error
			if parent == nil {
				err = consensus.ErrUnknownAncestor
			} else {
				err = randomx.verifyHeader(chain, header, parent, false, seals[i], unixNow)
			}

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()

	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus rules.
func (randomx *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if randomx.fakeMode {
		return nil
	}
	
	if len(block.Uncles()) > maxUncles {
		return errTooManyUncles
	}
	if len(block.Uncles()) == 0 {
		return nil
	}
	
	uncles, ancestors := mapset.NewSet[common.Hash](), make(map[common.Hash]*types.Header)
	number, parent := block.NumberU64()-1, block.ParentHash()
	
	for i := 0; i < 7; i++ {
		ancestor := chain.GetHeader(parent, number)
		if ancestor == nil {
			break
		}
		ancestors[parent] = ancestor
		if ancestor.UncleHash != types.EmptyUncleHash {
			ancestorBlock := chain.GetBlock(parent, number)
			if ancestorBlock == nil {
				break
			}
			for _, uncle := range ancestorBlock.Uncles() {
				uncles.Add(uncle.Hash())
			}
		}
		parent, number = ancestor.ParentHash, number-1
	}
	
	ancestors[block.Hash()] = block.Header()
	uncles.Add(block.Hash())
	
	for _, uncle := range block.Uncles() {
		hash := uncle.Hash()
		if uncles.Contains(hash) {
			return errDuplicateUncle
		}
		uncles.Add(hash)
		if ancestors[hash] != nil {
			return errUncleIsAncestor
		}
		if ancestors[uncle.ParentHash] == nil || uncle.ParentHash == block.ParentHash() {
			return errDanglingUncle
		}
		if err := randomx.verifyHeader(chain, uncle, ancestors[uncle.ParentHash], true, false, time.Now().Unix()); err != nil {
			return err
		}
	}
	
	return nil
}

// verifyHeader performs the actual header verification.
func (randomx *RandomX) verifyHeader(chain consensus.ChainHeaderReader, header, parent *types.Header, uncle bool, seal bool, unixNow int64) error {
	// Check extra data size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}
	
	// Check timestamp
	if !uncle && header.Time > uint64(unixNow+allowedFutureBlockTimeSeconds) {
		return consensus.ErrFutureBlock
	}
	if header.Time <= parent.Time {
		return errOlderBlockTime
	}
	
	// Verify difficulty
	expected := randomx.CalcDifficulty(chain, header.Time, parent)
	if expected.Cmp(header.Difficulty) != 0 {
		return fmt.Errorf("invalid difficulty: have %v, want %v", header.Difficulty, expected)
	}
	
	// Verify gas limit
	if header.GasLimit > params.MaxGasLimit {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, params.MaxGasLimit)
	}
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}
	
	// Verify EIP-1559 attributes
	if !chain.Config().IsLondon(header.Number) {
		if header.BaseFee != nil {
			return fmt.Errorf("invalid baseFee before fork: have %d, expected 'nil'", header.BaseFee)
		}
		if err := misc.VerifyGaslimit(parent.GasLimit, header.GasLimit); err != nil {
			return err
		}
	} else if err := eip1559.VerifyEIP1559Header(chain.Config(), parent, header); err != nil {
		return err
	}
	
	// Verify block number
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return consensus.ErrInvalidNumber
	}
	
	// Verify seal if requested
	if seal {
		if err := randomx.VerifySeal(chain, header); err != nil {
			return err
		}
	}
	
	// Verify DAO hard-fork extra data
	return misc.VerifyDAOHeaderExtraData(chain.Config(), header)
}

// Prepare initializes the difficulty field of a header.
func (randomx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = randomx.CalcDifficulty(chain, header.Time, parent)
	return nil
}

// Finalize implements consensus.Engine, accumulating block and uncle rewards.
func (randomx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body) {
	// Accumulate rewards
	accumulateRewards(chain.Config(), state, header, body.Uncles)
	
	// Finalize the state root
	header.Root = state.IntermediateRoot(true)
}

// FinalizeAndAssemble implements consensus.Engine, creating the final block.
func (randomx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	// Finalize state
	randomx.Finalize(chain, header, state, body)
	
	// Set other header fields
	header.Bloom = types.CreateBloom(receipts)
	
	// Assemble the final block
	return types.NewBlock(header, body, receipts, chain.Config().IsEip1559(header.Number)), nil
}

// Seal generates a new sealing request for the given input block.
func (randomx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	if randomx.fakeMode {
		// In fake mode, return the block immediately
		select {
		case results <- block.WithSeal(block.Header()):
		case <-stop:
		}
		return nil
	}
	
	// Ensure we have the RandomX cache and dataset for the current epoch
	epoch := randomx.epoch(block.NumberU64())
	if err := randomx.updateCacheForEpoch(epoch); err != nil {
		return fmt.Errorf("failed to update RandomX cache: %w", err)
	}
	
	// Start mining threads
	for i := 0; i < randomx.threads; i++ {
		randomx.wg.Add(1)
		go randomx.mine(block, results, stop, i)
	}
	
	// Cleanup when done
	go func() {
		randomx.wg.Wait()
		close(results)
	}()
	
	return nil
}

// mine attempts to find a valid nonce for the given block.
func (randomx *RandomX) mine(block *types.Block, results chan<- *types.Block, stop <-chan struct{}, thread int) {
	defer randomx.wg.Done()
	
	header := block.Header()
	target := new(big.Int).Div(maxUint256, header.Difficulty)
	mineHeader := types.CopyHeader(header)
	
	// Get RandomX VM for this thread
	vm, err := randomx.getVM()
	if err != nil {
		log.Error("Failed to get RandomX VM", "error", err)
		return
	}
	defer vm.Close()
	
	// Calculate seed hash for this epoch
	seed := randomx.seedHash(block.NumberU64())
	
	// Mining loop with nonce distribution across threads
	startNonce := uint64(thread)
	step := uint64(randomx.threads)
	started := time.Now()
	attempts := uint64(0)
	
	for nonce := startNonce; ; nonce += step {
		select {
		case <-stop:
			return
		default:
			mineHeader.Nonce = types.EncodeNonce(nonce)
			
			// Compute RandomX hash
			mixDigest, result := randomx.hashimoto(mineHeader, seed, vm)
			
			if result.Cmp(target) <= 0 {
				// Found valid nonce!
				mineHeader.MixDigest = mixDigest
				sealedBlock := block.WithSeal(mineHeader)
				select {
				case results <- sealedBlock:
				case <-stop:
				}
				log.Info("Mined new block", "number", block.NumberU64(), "hash", sealedBlock.Hash(), "nonce", nonce, "attempts", attempts)
				return
			}
			
			attempts++
			
			// Periodically log progress
			if attempts%1_000_000 == 0 {
				hashrate := float64(attempts) / time.Since(started).Seconds()
				log.Debug("RandomX mining in progress", "thread", thread, "attempts", attempts, "hashrate", hashrate)
			}
		}
	}
}

// VerifySeal verifies the RandomX proof-of-work.
func (randomx *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
	if randomx.fakeMode {
		if randomx.fakeDelay != nil {
			time.Sleep(*randomx.fakeDelay)
		}
		return nil
	}
	
	// Ensure we have the RandomX cache for verification
	epoch := randomx.epoch(header.Number.Uint64())
	if err := randomx.updateCacheForEpoch(epoch); err != nil {
		return fmt.Errorf("failed to update RandomX cache: %w", err)
	}
	
	// Get VM for verification
	vm, err := randomx.getVM()
	if err != nil {
		return err
	}
	defer vm.Close()
	
	// Calculate seed hash
	seed := randomx.seedHash(header.Number.Uint64())
	
	// Verify RandomX PoW
	mixDigest, result := randomx.hashimoto(header, seed, vm)
	
	// Check mix digest
	if !bytes.Equal(mixDigest.Bytes(), header.MixDigest.Bytes()) {
		return errInvalidMixHash
	}
	
	// Check difficulty target
	target := new(big.Int).Div(maxUint256, header.Difficulty)
	if result.Cmp(target) > 0 {
		return fmt.Errorf("invalid proof-of-work: result %s > target %s", result.String(), target.String())
	}
	
	return nil
}

// hashimoto is the core RandomX hash function using the real RandomX algorithm.
func (randomx *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *randomx.VM) (common.Hash, *big.Int) {
	// Prepare input for RandomX
	// Format: seed (32 bytes) + nonce (8 bytes)
	input := make([]byte, 40)
	copy(input[:32], seed.Bytes())
	copy(input[32:], header.Nonce.Bytes())
	
	// Execute real RandomX hash
	output := make([]byte, 32)
	vm.CalculateHash(input, output)
	
	// Output is the mix digest
	mixDigest := common.BytesToHash(output[:32])
	
	// Convert to big.Int for difficulty comparison
	result := new(big.Int).SetBytes(output[:])
	
	return mixDigest, result
}

// CalcDifficulty is the difficulty adjustment algorithm.
func (randomx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return CalcDifficulty(chain.Config(), time, parent)
}

// APIs returns the RPC APIs provided by the RandomX engine.
func (randomx *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{{
		Namespace: "randomx",
		Version:   "1.0",
		Service:   &API{randomx: randomx},
		Public:    true,
	}}
}

// Close terminates any background threads maintained by the consensus engine.
func (randomx *RandomX) Close() error {
	select {
	case <-randomx.stopCh:
	default:
		close(randomx.stopCh)
	}
	randomx.wg.Wait()
	
	randomx.cacheMu.Lock()
	defer randomx.cacheMu.Unlock()
	
	if randomx.cache != nil {
		randomx.cache.Close()
	}
	if randomx.dataset != nil {
		randomx.dataset.Close()
	}
	
	return nil
}

// epoch returns the epoch for a given block number.
func (randomx *RandomX) epoch(blockNum uint64) uint64 {
	return blockNum / randomx.config.EpochLength
}

// seedHash computes the seed hash for a given block number.
func (randomx *RandomX) seedHash(blockNum uint64) common.Hash {
	epoch := randomx.epoch(blockNum)
	seed := make([]byte, 32)
	
	// Calculate seed based on epoch using Keccak256
	for i := uint64(0); i < epoch; i++ {
		seed = crypto.Keccak256(seed)
	}
	
	return common.BytesToHash(seed)
}

// updateCacheForEpoch ensures the RandomX cache and dataset are ready for the given epoch.
func (randomx *RandomX) updateCacheForEpoch(epoch uint64) error {
	randomx.cacheMu.Lock()
	defer randomx.cacheMu.Unlock()
	
	// Check if we already have the correct cache
	if randomx.cacheEpoch == epoch && randomx.cache != nil {
		return nil
	}
	
	// Calculate seed for this epoch
	seed := randomx.seedHash(epoch * randomx.config.EpochLength)
	seedBytes := seed.Bytes()
	
	log.Info("Initializing RandomX for new epoch", "epoch", epoch, "seed", seed.Hex())
	
	// Close old cache and dataset
	if randomx.cache != nil {
		randomx.cache.Close()
	}
	if randomx.dataset != nil {
		randomx.dataset.Close()
	}
	
	// Create new cache
	var err error
	startTime := time.Now()
	randomx.cache, err = randomx.NewCache(seedBytes)
	if err != nil {
		return fmt.Errorf("failed to create RandomX cache: %w", err)
	}
	
	log.Info("RandomX cache created", "epoch", epoch, "duration", time.Since(startTime))
	
	// Create dataset from cache for full mining mode
	// Note: This takes time and memory (2GB+)
	startTime = time.Now()
	randomx.dataset = randomx.NewDataset(randomx.cache)
	log.Info("RandomX dataset created", "epoch", epoch, "duration", time.Since(startTime))
	
	randomx.cacheEpoch = epoch
	
	return nil
}

// getVM creates a new RandomX VM for hash computation.
func (randomx *RandomX) getVM() (*randomx.VM, error) {
	randomx.cacheMu.RLock()
	defer randomx.cacheMu.RUnlock()
	
	if randomx.cache == nil {
		return nil, errNoCache
	}
	
	// Create VM in full mode using dataset for mining performance
	// Falls back to cache-only mode if dataset not available
	if randomx.dataset != nil {
		return randomx.NewVM(nil, randomx.dataset), nil
	}
	return randomx.NewVM(randomx.cache, nil), nil
}

// SealHash returns the hash of a block prior to it being sealed.
func (randomx *RandomX) SealHash(header *types.Header) (hash common.Hash) {
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
	if header.ParentBeaconRoot != nil {
		enc = append(enc, header.ParentBeaconRoot)
	}
	if header.SlotNumber != nil {
		enc = append(enc, header.SlotNumber)
	}
	rlp.Encode(hasher, enc)
	hasher.Sum(hash[:0])
	return hash
}

// accumulateRewards credits the coinbase of the given block with the mining reward.
func accumulateRewards(config *params.ChainConfig, stateDB *state.StateDB, header *types.Header, uncles []*types.Header) {
	// Select the correct block reward based on chain progression
	blockReward := FrontierBlockReward
	if config.IsByzantium(header.Number) {
		blockReward = ByzantiumBlockReward
	}
	if config.IsConstantinople(header.Number) {
		blockReward = ConstantinopleBlockReward
	}
	
	// Accumulate the rewards for the miner and any included uncles
	reward := new(uint256.Int).Set(blockReward)
	r := new(uint256.Int)
	hNum, _ := uint256.FromBig(header.Number)
	
	for _, uncle := range uncles {
		uNum, _ := uint256.FromBig(uncle.Number)
		r.AddUint64(uNum, 8)
		r.Sub(r, hNum)
		r.Mul(r, blockReward)
		r.Rsh(r, 3)
		stateDB.AddBalance(uncle.Coinbase, r)
		
		r.Rsh(blockReward, 5)
		reward.Add(reward, r)
	}
	
	stateDB.AddBalance(header.Coinbase, reward)
}
