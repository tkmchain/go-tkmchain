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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"git.gammaspectra.live/P2Pool/go-randomx/v3"
)

// Constants
var (
	allowedFutureBlockTimeSeconds = int64(15)
	maxUncles                     = 2
	maxUint256                    = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Errors
var (
	errOlderBlockTime  = errors.New("timestamp older than parent")
	errTooManyUncles   = errors.New("too many uncles")
	errDuplicateUncle  = errors.New("duplicate uncle")
	errUncleIsAncestor = errors.New("uncle is ancestor")
	errDanglingUncle   = errors.New("uncle's parent is not ancestor")
	errInvalidMixHash  = errors.New("invalid mix hash")
	errInvalidNonce    = errors.New("invalid nonce")
	errNoCache         = errors.New("randomx cache not initialized")
)

// RandomX is a consensus engine based on proof-of-work implementing the RandomX algorithm.
type RandomX struct {
	config     *params.RandomXConfig
	threads    int
	cache      *randomx.Cache
	dataset    *randomx.Dataset
	cacheEpoch uint64
	mu         sync.RWMutex
	cacheMu    sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// New creates a new RandomX consensus engine.
func New(config *params.RandomXConfig, threads int) (*RandomX, error) {
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if config == nil {
		config = DefaultConfig()
	}

	// Validate config
	if config.EpochLength == 0 {
		config.EpochLength = 2048
	}
	if config.CacheSizeMB == 0 {
		config.CacheSizeMB = 256
	}
	if config.DatasetSizeGB == 0 {
		config.DatasetSizeGB = 2
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
		EpochLength:   2048,
		CacheSizeMB:   256,
		DatasetSizeGB: 2,
		MinMemory:     4 * 1024 * 1024 * 1024, // 4GB
	}
}

// Author implements consensus.Engine, returning the header's coinbase.
func (r *RandomX) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (r *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	// Short circuit if the header is known
	number := header.Number.Uint64()
	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	return r.verifyHeader(chain, header, parent, false, seal, time.Now().Unix())
}

// VerifyHeaders verifies a batch of headers concurrently.
func (r *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	if len(headers) == 0 {
		abort, results := make(chan struct{}), make(chan error, 0)
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
				err = r.verifyHeader(chain, header, parent, false, seals[i], unixNow)
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

// VerifyUncles verifies the block's uncles.
func (r *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > maxUncles {
		return errTooManyUncles
	}
	if len(block.Uncles()) == 0 {
		return nil
	}

	// Track seen uncles and ancestors
	uncles := make(map[common.Hash]bool)
	ancestors := make(map[common.Hash]*types.Header)

	number, parent := block.NumberU64()-1, block.ParentHash()
	for i := 0; i < 7; i++ {
		ancestor := chain.GetHeader(parent, number)
		if ancestor == nil {
			break
		}
		ancestors[parent] = ancestor
		parent, number = ancestor.ParentHash, number-1
	}

	for _, uncle := range block.Uncles() {
		hash := uncle.Hash()
		if uncles[hash] {
			return errDuplicateUncle
		}
		uncles[hash] = true

		if ancestors[hash] != nil {
			return errUncleIsAncestor
		}
		if ancestors[uncle.ParentHash] == nil {
			return errDanglingUncle
		}

		if err := r.verifyHeader(chain, uncle, ancestors[uncle.ParentHash], true, false, time.Now().Unix()); err != nil {
			return err
		}
	}

	return nil
}

// verifyHeader performs the actual header verification.
func (r *RandomX) verifyHeader(chain consensus.ChainHeaderReader, header, parent *types.Header, uncle bool, seal bool, unixNow int64) error {
	// Check extra data size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}

	// Check timestamp
	if !uncle {
		if header.Time > uint64(unixNow+allowedFutureBlockTimeSeconds) {
			return consensus.ErrFutureBlock
		}
	}
	if header.Time <= parent.Time {
		return errOlderBlockTime
	}

	// Verify difficulty
	expected := r.CalcDifficulty(chain, header.Time, parent)
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
			return fmt.Errorf("invalid baseFee before fork: have %d, expected nil", header.BaseFee)
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
		if err := r.VerifySeal(chain, header); err != nil {
			return err
		}
	}

	// Verify DAO hard-fork extra data
	return misc.VerifyDAOHeaderExtraData(chain.Config(), header)
}

// Prepare initializes the difficulty field of a header.
func (r *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = r.CalcDifficulty(chain, header.Time, parent)
	return nil
}

// Finalize implements consensus.Engine, accumulating block rewards.
func (r *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body) {
	// Accumulate rewards
	accumulateRewards(chain.Config(), state, header, body.Uncles)

	// Finalize the state root
	header.Root = state.IntermediateRoot(true)
}

// FinalizeAndAssemble creates the final block.
func (r *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	// Finalize state
	r.Finalize(chain, header, state, body)

	// Assemble the final block
	return types.NewBlock(header, body, receipts, chain.Config().IsEip1559(header.Number)), nil
}

// Seal generates a new sealing request for the given input block.
func (r *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	// Ensure we have the RandomX cache and dataset for the current epoch
	epoch := r.epoch(block.NumberU64())
	if err := r.updateCacheForEpoch(epoch); err != nil {
		return fmt.Errorf("failed to update RandomX cache: %w", err)
	}

	// Start mining threads
	for i := 0; i < r.threads; i++ {
		r.wg.Add(1)
		go r.mine(block, chain, results, stop, i)
	}

	// Cleanup when done
	go func() {
		r.wg.Wait()
		close(results)
	}()

	return nil
}

// mine attempts to find a valid nonce for the given block.
func (r *RandomX) mine(block *types.Block, chain consensus.ChainHeaderReader, results chan<- *types.Block, stop <-chan struct{}, thread int) {
	defer r.wg.Done()

	header := block.Header()
	target := new(big.Int).Div(maxUint256, header.Difficulty)

	// Create a copy of the header for mining
	mineHeader := types.CopyHeader(header)

	// Get RandomX VM for this thread
	vm, err := r.getVM()
	if err != nil {
		log.Error("Failed to get RandomX VM", "error", err)
		return
	}
	defer vm.Close()

	// Calculate seed hash for this epoch
	seed := r.seedHash(block.NumberU64())

	// Mining loop
	nonce := uint64(0)
	attempts := uint64(0)

	for {
		select {
		case <-stop:
			return
		default:
			mineHeader.Nonce = types.EncodeNonce(nonce)

			// Compute RandomX hash
			mixDigest, result := r.hashimoto(mineHeader, seed, vm)

			if result.Cmp(target) <= 0 {
				// Found valid nonce!
				mineHeader.MixDigest = mixDigest
				sealedBlock := block.WithSeal(mineHeader)
				select {
				case results <- sealedBlock:
				case <-stop:
				}
				return
			}

			nonce++
			attempts++

			// Periodically log progress (every 1M attempts)
			if attempts%1_000_000 == 0 {
				log.Debug("Mining in progress", "thread", thread, "attempts", attempts, "hashrate", attempts/uint64(time.Since(start).Seconds()))
			}
		}
	}
}

// VerifySeal verifies the RandomX proof-of-work.
func (r *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
	// Ensure we have the RandomX cache for verification
	epoch := r.epoch(header.Number.Uint64())
	if err := r.updateCacheForEpoch(epoch); err != nil {
		return fmt.Errorf("failed to update RandomX cache: %w", err)
	}

	// Get VM for verification
	vm, err := r.getVM()
	if err != nil {
		return err
	}
	defer vm.Close()

	// Calculate seed hash
	seed := r.seedHash(header.Number.Uint64())

	// Verify RandomX PoW
	mixDigest, result := r.hashimoto(header, seed, vm)

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

// hashimoto is the core RandomX hash function.
func (r *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *randomx.VM) (common.Hash, *big.Int) {
	// Prepare input for RandomX
	// Format: seed (32 bytes) + nonce (8 bytes)
	input := make([]byte, 40)
	copy(input[:32], seed.Bytes())
	copy(input[32:], header.Nonce.Bytes())

	// Execute RandomX
	output := make([]byte, 32)
	vm.CalculateHash(input, output)

	// First 32 bytes are the mix digest
	mixDigest := common.BytesToHash(output[:32])

	// Convert the entire output to big.Int for difficulty comparison
	result := new(big.Int).SetBytes(output[:])

	return mixDigest, result
}

// CalcDifficulty is the difficulty adjustment algorithm.
func (r *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return CalcDifficulty(chain.Config(), time, parent)
}

// APIs returns the RPC APIs.
func (r *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{
		{
			Namespace: "randomx",
			Version:   "1.0",
			Service:   &API{randomx: r},
			Public:    true,
		},
	}
}

// Close terminates the RandomX engine.
func (r *RandomX) Close() error {
	close(r.stopCh)
	r.wg.Wait()

	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.cache != nil {
		r.cache.Close()
	}
	if r.dataset != nil {
		r.dataset.Close()
	}
	return nil
}

// epoch returns the epoch for a given block number.
func (r *RandomX) epoch(blockNum uint64) uint64 {
	return blockNum / r.config.EpochLength
}

// seedHash computes the seed hash for a given block number.
func (r *RandomX) seedHash(blockNum uint64) common.Hash {
	epoch := r.epoch(blockNum)
	
	// Calculate seed based on epoch using Keccak256
	seed := make([]byte, 32)
	for i := uint64(0); i < epoch; i++ {
		// Hash the seed repeatedly for each epoch
		hasher := keccak.NewLegacyKeccak256()
		hasher.Write(seed)
		seed = hasher.Sum(nil)
	}
	
	return common.BytesToHash(seed)
}

// updateCacheForEpoch ensures the RandomX cache and dataset are ready for the given epoch.
func (r *RandomX) updateCacheForEpoch(epoch uint64) error {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.cacheEpoch == epoch && r.cache != nil {
		return nil // Already have the correct cache
	}

	// Calculate seed for this epoch
	seed := r.seedHash(epoch * r.config.EpochLength)
	seedBytes := seed.Bytes()

	// Close old cache and dataset
	if r.cache != nil {
		r.cache.Close()
	}
	if r.dataset != nil {
		r.dataset.Close()
	}

	// Create new cache
	var err error
	r.cache, err = randomx.NewCache(seedBytes)
	if err != nil {
		return fmt.Errorf("failed to create RandomX cache: %w", err)
	}

	// Create dataset from cache for full mining mode
	// Note: Creating the dataset takes time (minutes) and requires memory
	r.dataset = randomx.NewDataset(r.cache)
	r.cacheEpoch = epoch

	log.Info("Initialized RandomX for new epoch", "epoch", epoch, "seed", seed.Hex())

	return nil
}

// getVM creates a new RandomX VM.
func (r *RandomX) getVM() (*randomx.VM, error) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	if r.cache == nil {
		return nil, errNoCache
	}

	// Create VM in full mode using dataset for mining
	// For light verification, we could use cache only
	if r.dataset != nil {
		return randomx.NewVM(nil, r.dataset), nil
	}
	return randomx.NewVM(r.cache, nil), nil
}
