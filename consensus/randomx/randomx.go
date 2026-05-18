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
	randomx_lib "git.gammaspectra.live/P2Pool/go-randomx/v3"
)

// RandomX proof-of-work protocol constants.
var (
	FrontierBlockReward           = uint256.NewInt(5e+18)
	ByzantiumBlockReward          = uint256.NewInt(3e+18)
	ConstantinopleBlockReward     = uint256.NewInt(2e+18)
	maxUncles                     = 2
	allowedFutureBlockTimeSeconds = int64(15)
	maxUint256                    = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)
)

// Errors
var (
	errOlderBlockTime  = errors.New("timestamp older than parent")
	errTooManyUncles   = errors.New("too many uncles")
	errDuplicateUncle  = errors.New("duplicate uncle")
	errUncleIsAncestor = errors.New("uncle is ancestor")
	errDanglingUncle   = errors.New("uncle's parent is not ancestor")
	errInvalidMixHash  = errors.New("invalid mix hash")
	errNoCache         = errors.New("randomx cache not initialized")
)

// RandomX consensus engine
type RandomX struct {
	config      *params.RandomXConfig
	threads     int
	cache       *randomx_lib.Cache
	dataset     *randomx_lib.Dataset
	cacheEpoch  uint64
	cacheMu     sync.RWMutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
	fakeMode    bool
	fakeFail    *uint64
	fakeDelay   *time.Duration
}

// New creates a new RandomX consensus engine
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
		config.MinMemory = 4 * 1024 * 1024 * 1024
	}

	return &RandomX{
		config:  config,
		threads: threads,
		stopCh:  make(chan struct{}),
	}, nil
}

// DefaultConfig returns default RandomX configuration
func DefaultConfig() *params.RandomXConfig {
	return &params.RandomXConfig{
		EpochLength:   2048,
		CacheSizeMB:   256,
		DatasetSizeGB: 2,
		MinMemory:     4 * 1024 * 1024 * 1024,
	}
}

// NewFaker creates a fake RandomX engine for testing
func NewFaker() *RandomX {
	engine, _ := New(nil, 1)
	engine.fakeMode = true
	return engine
}

// Author returns the coinbase
func (r *RandomX) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks if a header conforms to consensus rules
func (r *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	if r.fakeMode {
		if r.fakeDelay != nil {
			time.Sleep(*r.fakeDelay)
		}
		if r.fakeFail != nil && *r.fakeFail == header.Number.Uint64() {
			return errors.New("invalid tester pow")
		}
		return nil
	}

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

// VerifyHeaders verifies multiple headers concurrently
func (r *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	if r.fakeMode || len(headers) == 0 {
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

// VerifyUncles verifies block uncles
func (r *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if r.fakeMode {
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
		if err := r.verifyHeader(chain, uncle, ancestors[uncle.ParentHash], true, false, time.Now().Unix()); err != nil {
			return err
		}
	}

	return nil
}

// verifyHeader performs header verification
func (r *RandomX) verifyHeader(chain consensus.ChainHeaderReader, header, parent *types.Header, uncle bool, seal bool, unixNow int64) error {
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}

	if !uncle && header.Time > uint64(unixNow+allowedFutureBlockTimeSeconds) {
		return consensus.ErrFutureBlock
	}
	if header.Time <= parent.Time {
		return errOlderBlockTime
	}

	expected := r.CalcDifficulty(chain, header.Time, parent)
	if expected.Cmp(header.Difficulty) != 0 {
		return fmt.Errorf("invalid difficulty: have %v, want %v", header.Difficulty, expected)
	}

	if header.GasLimit > params.MaxGasLimit {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, params.MaxGasLimit)
	}
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}

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

	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return consensus.ErrInvalidNumber
	}

	if seal {
		if err := r.VerifySeal(chain, header); err != nil {
			return err
		}
	}

	return misc.VerifyDAOHeaderExtraData(chain.Config(), header)
}

// Prepare initializes the difficulty field
func (r *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = r.CalcDifficulty(chain, header.Time, parent)
	return nil
}

// Finalize accumulates block rewards
func (r *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body) {
	accumulateRewards(chain.Config(), state, header, body.Uncles)
	header.Root = state.IntermediateRoot(true)
}

// FinalizeAndAssemble creates the final block
func (r *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	r.Finalize(chain, header, state, body)
	header.Bloom = types.CreateBloom(receipts)
	return types.NewBlock(header, body, receipts, nil), nil
}

// Seal generates a new sealing request
func (r *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	if r.fakeMode {
		select {
		case results <- block.WithSeal(block.Header()):
		case <-stop:
		}
		return nil
	}

	epoch := r.epoch(block.NumberU64())
	if err := r.updateCacheForEpoch(epoch); err != nil {
		return fmt.Errorf("failed to update RandomX cache: %w", err)
	}

	for i := 0; i < r.threads; i++ {
		r.wg.Add(1)
		go r.mine(block, results, stop, i)
	}

	go func() {
		r.wg.Wait()
		close(results)
	}()

	return nil
}

// mine finds a valid nonce
func (r *RandomX) mine(block *types.Block, results chan<- *types.Block, stop <-chan struct{}, thread int) {
	defer r.wg.Done()

	header := block.Header()
	target := new(big.Int).Div(maxUint256, header.Difficulty)
	mineHeader := types.CopyHeader(header)

	vm, err := r.getVM()
	if err != nil {
		log.Error("Failed to get RandomX VM", "error", err)
		return
	}
	defer vm.Close()

	seed := r.seedHash(block.NumberU64())
	startNonce := uint64(thread)
	step := uint64(r.threads)
	started := time.Now()
	attempts := uint64(0)

	for nonce := startNonce; ; nonce += step {
		select {
		case <-stop:
			return
		default:
			mineHeader.Nonce = types.EncodeNonce(nonce)
			mixDigest, result := r.hashimoto(mineHeader, seed, vm)

			if result.Cmp(target) <= 0 {
				mineHeader.MixDigest = mixDigest
				sealedBlock := block.WithSeal(mineHeader)
				select {
				case results <- sealedBlock:
				case <-stop:
				}
				log.Info("Mined new block", "number", block.NumberU64(), "nonce", nonce)
				return
			}

			attempts++
			if attempts%1_000_000 == 0 {
				hashrate := float64(attempts) / time.Since(started).Seconds()
				log.Debug("RandomX mining", "thread", thread, "hashrate", hashrate)
			}
		}
	}
}

// VerifySeal verifies the RandomX proof-of-work
func (r *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
	if r.fakeMode {
		if r.fakeDelay != nil {
			time.Sleep(*r.fakeDelay)
		}
		return nil
	}

	epoch := r.epoch(header.Number.Uint64())
	if err := r.updateCacheForEpoch(epoch); err != nil {
		return fmt.Errorf("failed to update RandomX cache: %w", err)
	}

	vm, err := r.getVM()
	if err != nil {
		return err
	}
	defer vm.Close()

	seed := r.seedHash(header.Number.Uint64())
	mixDigest, result := r.hashimoto(header, seed, vm)

	if !bytes.Equal(mixDigest.Bytes(), header.MixDigest.Bytes()) {
		return errInvalidMixHash
	}

	target := new(big.Int).Div(maxUint256, header.Difficulty)
	if result.Cmp(target) > 0 {
		return fmt.Errorf("invalid proof-of-work: result %s > target %s", result.String(), target.String())
	}

	return nil
}

// hashimoto computes RandomX hash
func (r *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *randomx_lib.VM) (common.Hash, *big.Int) {
	// Prepare input: seed (32 bytes) + nonce (8 bytes)
	input := make([]byte, 40)
	copy(input[:32], seed.Bytes())
	
	// Convert nonce to bytes properly
	nonceBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonceBytes, header.Nonce.Uint64())
	copy(input[32:], nonceBytes)

	// Execute RandomX
	output := make([]byte, 32)
	vm.CalculateHash(input, output)

	mixDigest := common.BytesToHash(output[:32])
	result := new(big.Int).SetBytes(output[:])

	return mixDigest, result
}

// CalcDifficulty returns the difficulty adjustment
func (r *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return CalcDifficulty(chain.Config(), time, parent)
}

// APIs returns RPC APIs
func (r *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{{
		Namespace: "randomx",
		Version:   "1.0",
		Service:   &API{randomx: r},
		Public:    true,
	}}
}

// Close terminates the engine
func (r *RandomX) Close() error {
	select {
	case <-r.stopCh:
	default:
		close(r.stopCh)
	}
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

// epoch returns the current epoch
func (r *RandomX) epoch(blockNum uint64) uint64 {
	return blockNum / r.config.EpochLength
}

// seedHash computes the seed hash
func (r *RandomX) seedHash(blockNum uint64) common.Hash {
	epoch := r.epoch(blockNum)
	seed := make([]byte, 32)
	for i := uint64(0); i < epoch; i++ {
		seed = crypto.Keccak256(seed)
	}
	return common.BytesToHash(seed)
}

// updateCacheForEpoch updates RandomX cache and dataset
func (r *RandomX) updateCacheForEpoch(epoch uint64) error {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.cacheEpoch == epoch && r.cache != nil {
		return nil
	}

	seed := r.seedHash(epoch * r.config.EpochLength)
	seedBytes := seed.Bytes()

	log.Info("Initializing RandomX for new epoch", "epoch", epoch, "seed", seed.Hex())

	if r.cache != nil {
		r.cache.Close()
	}
	if r.dataset != nil {
		r.dataset.Close()
	}

	var err error
	startTime := time.Now()
	r.cache, err = randomx_lib.NewCache(seedBytes)
	if err != nil {
		return fmt.Errorf("failed to create RandomX cache: %w", err)
	}
	log.Info("RandomX cache created", "duration", time.Since(startTime))

	startTime = time.Now()
	r.dataset = randomx_lib.NewDataset(r.cache)
	log.Info("RandomX dataset created", "duration", time.Since(startTime))

	r.cacheEpoch = epoch
	return nil
}

// getVM creates a new RandomX VM
func (r *RandomX) getVM() (*randomx_lib.VM, error) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	if r.cache == nil {
		return nil, errNoCache
	}

	if r.dataset != nil {
		return randomx_lib.NewVM(nil, r.dataset), nil
	}
	return randomx_lib.NewVM(r.cache, nil), nil
}

// SealHash returns the hash before sealing
func (r *RandomX) SealHash(header *types.Header) (hash common.Hash) {
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
	rlp.Encode(hasher, enc)
	hasher.Sum(hash[:0])
	return hash
}

// accumulateRewards credits block rewards
func accumulateRewards(config *params.ChainConfig, stateDB *state.StateDB, header *types.Header, uncles []*types.Header) {
	blockReward := FrontierBlockReward
	if config.IsByzantium(header.Number) {
		blockReward = ByzantiumBlockReward
	}
	if config.IsConstantinople(header.Number) {
		blockReward = ConstantinopleBlockReward
	}

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
