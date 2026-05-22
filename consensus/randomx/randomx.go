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
	"os"
	"runtime"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/consensus/rotatingking"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/keccak"
	randomx_lib "github.com/ethereum/go-ethereum/internal/go-randomx"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
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

// Various error messages to mark blocks invalid.
var (
	errOlderBlockTime  = errors.New("timestamp older than parent")
	errTooManyUncles   = errors.New("too many uncles")
	errDuplicateUncle  = errors.New("duplicate uncle")
	errUncleIsAncestor = errors.New("uncle is ancestor")
	errDanglingUncle   = errors.New("uncle's parent is not ancestor")
	errInvalidMixHash  = errors.New("invalid mix hash")
	errNoCache         = errors.New("randomx cache not initialized")
)

// RandomX is a consensus engine based on proof-of-work implementing the RandomX algorithm.
type RandomX struct {
	config       *params.RandomXConfig
	threads      int
	cache        *randomx_lib.Cache
	dataset      *randomx_lib.Dataset
	datasetRAM   []byte // RAM cache for dataset
	cacheEpoch   uint64
	cacheMu      sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
	fakeMode     bool
	fakeFail     *uint64
	fakeDelay    *time.Duration
	rotatingKing *rotatingking.RotatingKingManager

	// RAM caching flags
	persistDataset bool // Whether to persist dataset to disk
	useRAMCache    bool // Whether to use RAM cache
}

// New creates a new RandomX consensus engine.
func New(config *params.RandomXConfig, threads int, mainKing common.Address, kingAddresses []common.Address) (*RandomX, error) {
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

	// Check for RAM cache environment variable
	useRAMCache := os.Getenv("RANDOMX_RAM_CACHE") == "true" || config.UseRAMCache

	// Initialize rotating king manager
	rotationInterval := uint64(100) // Rotate every 100 blocks
	kingManager := rotatingking.NewRotatingKingManager(mainKing, kingAddresses, rotationInterval)

	return &RandomX{
		config:         config,
		threads:        threads,
		stopCh:         make(chan struct{}),
		rotatingKing:   kingManager,
		persistDataset: true, // Default to persist
		useRAMCache:    useRAMCache,
	}, nil
}

// DefaultConfig returns the default RandomX configuration.
func DefaultConfig() *params.RandomXConfig {
	return &params.RandomXConfig{
		EpochLength:   2048,
		CacheSizeMB:   256,
		DatasetSizeGB: 2,
		MinMemory:     4 * 1024 * 1024 * 1024,
	}
}

// NewFaker creates a RandomX engine that skips proof-of-work verification (testing only).
func NewFaker() *RandomX {
	engine, _ := New(nil, 1, common.Address{}, nil)
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
func (r *RandomX) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (r *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
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

	return r.verifyHeader(chain, header, parent, false, true, time.Now().Unix())
}

// VerifyHeaders verifies a batch of headers concurrently.
func (r *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
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
				err = r.verifyHeader(chain, header, parent, false, true, unixNow)
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

// verifyHeader performs the actual header verification.
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
			return fmt.Errorf("invalid baseFee before fork: have %d, expected 'nil'", header.BaseFee)
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

// Prepare initializes the difficulty field of a header.
func (r *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = r.CalcDifficulty(chain, header.Time, parent)
	return nil
}

// Finalize implements consensus.Engine, accumulating block and uncle rewards.
func (r *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, stateDB vm.StateDB, body *types.Body) {
	// Get kings from chain config
	mainKing := r.GetMainKing()
	rotatingKing := r.GetRotatingKing(header.Number.Uint64())

	// Calculate rewards
	totalFees := big.NewInt(0)
	blockReward := CalculateBlockReward(header.Number.Uint64())
	totalReward := CalculateTotalReward(blockReward, totalFees)

	// Convert vm.StateDB to *state.StateDB for reward distribution
	// Since our reward functions expect *state.StateDB, we need to handle this
	statedb, ok := stateDB.(*state.StateDB)
	if !ok {
		log.Error("Failed to convert StateDB to expected type")
		return
	}

	// Distribute rewards
	DistributeRewards(statedb, mainKing, rotatingKing, header.Coinbase, totalReward, header.Number.Uint64())

	// Handle uncle rewards
	for _, uncle := range body.Uncles {
		uncleReward := new(big.Int).Div(blockReward, big.NewInt(32))
		statedb.AddBalance(uncle.Coinbase, uint256.MustFromBig(uncleReward), tracing.BalanceIncreaseRewardMineUncle)
	}

	// Finalize state root
	header.Root = statedb.IntermediateRoot(chain.Config().IsEIP158(header.Number))
}

// FinalizeAndAssemble implements consensus.Engine, creating the final block.
func (r *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, stateDB vm.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	r.Finalize(chain, header, stateDB.(*state.StateDB), body)
	totalFees := GetTotalTransactionFees(header, receipts)
	if totalFees.Sign() > 0 {
		mainKing := r.GetMainKing()
		rotatingKing := r.GetRotatingKing(header.Number.Uint64())
		DistributeRewards(stateDB.(*state.StateDB), mainKing, rotatingKing, header.Coinbase, totalFees, header.Number.Uint64())
		header.Root = stateDB.(*state.StateDB).IntermediateRoot(chain.Config().IsEIP158(header.Number))
	}
	header.Bloom = types.MergeBloom(receipts)
	return types.NewBlock(header, body, receipts, nil), nil
}

// GetMainKing returns the configured main king address.
func (r *RandomX) GetMainKing() common.Address {
	if r.rotatingKing == nil {
		return common.Address{}
	}
	return r.rotatingKing.GetMainKing()
}

// GetRotatingKing returns the active rotating king address.
func (r *RandomX) GetRotatingKing(_ uint64) common.Address {
	if r.rotatingKing == nil {
		return common.Address{}
	}
	return r.rotatingKing.GetCurrentKing()
}

// Seal generates a new sealing request for the given input block.
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

// mine attempts to find a valid nonce for the given block.
func (r *RandomX) mine(block *types.Block, results chan<- *types.Block, stop <-chan struct{}, thread int) {
    defer r.wg.Done()

    header := block.Header()
    target := new(big.Int).Div(maxUint256, header.Difficulty)
    mineHeader := types.CopyHeader(header)
    
    log.Info("�� Mining thread started", "thread", thread, "target", target, "difficulty", header.Difficulty)

    vm, err := r.getVM()
    if err != nil {
        log.Error("Failed to get RandomX VM", "error", err)
        return
    }
    defer vm.Close()
    
    log.Info("✅ RandomX VM acquired", "thread", thread)

    seed := r.seedHash(block.NumberU64())
    startNonce := uint64(thread)
    step := uint64(r.threads)
    started := time.Now()
    attempts := uint64(0)

    log.Info("�� Starting mining loop", "thread", thread, "startNonce", startNonce, "step", step)

    for nonce := startNonce; ; nonce += step {
        select {
        case <-stop:
            log.Info("Mining stopped", "thread", thread)
            return
        default:
            mineHeader.Nonce = types.EncodeNonce(nonce)
            
            // Log every 100,000 attempts
            if attempts%100000 == 0 {
                log.Debug("Mining progress", "thread", thread, "attempts", attempts, "nonce", nonce)
            }
            
            mixDigest, result := r.hashimoto(mineHeader, seed, vm)

            if result.Cmp(target) <= 0 {
                log.Info("�� NONCE FOUND!", "thread", thread, "nonce", nonce, "attempts", attempts, "result", result, "target", target)
                mineHeader.MixDigest = mixDigest
                sealedBlock := block.WithSeal(mineHeader)
                select {
                case results <- sealedBlock:
                    log.Info("✅ Block submitted to results channel", "number", block.NumberU64())
                case <-stop:
                    log.Info("Stop signal received after finding nonce")
                }
                return
            }

            attempts++
            
            if attempts%1000000 == 0 {
                hashrate := float64(attempts) / time.Since(started).Seconds()
                log.Info("Mining stats", "thread", thread, "attempts", attempts, "hashrate", hashrate, "nonce", nonce)
            }
        }
    }
}

// VerifySeal verifies the RandomX proof-of-work.
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

// hashimoto is the core RandomX hash function
func (r *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *randomx_lib.VM) (common.Hash, *big.Int) {
    input := make([]byte, 40)
    copy(input[:32], seed.Bytes())

    nonceBytes := make([]byte, 8)
    binary.LittleEndian.PutUint64(nonceBytes, header.Nonce.Uint64())
    copy(input[32:], nonceBytes)

    output := make([]byte, 32)
    vm.CalculateHash(input, output)
    
    // Debug first hash only
    if header.Nonce.Uint64() < 10 {
        log.Debug("Hashimoto called", "nonce", header.Nonce.Uint64(), "output_prefix", output[:4])
    }

    mixDigest := common.BytesToHash(output)
    result := new(big.Int).SetBytes(output)

    return mixDigest, result
}

// CalcDifficulty is the difficulty adjustment algorithm.
func (r *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return CalcDifficulty(chain.Config(), time, parent)
}

// APIs returns the RPC APIs provided by the RandomX engine.
func (r *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{{
		Namespace: "randomx",
		Version:   "1.0",
		Service:   &API{randomx: r},
		Public:    true,
	}}
}

// Close with RAM cleanup
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

	// Free RAM cache
	if r.datasetRAM != nil {
		r.datasetRAM = nil
		log.Info("Freed RandomX RAM cache")

		// Force GC to release memory
		runtime.GC()
	}

	return nil
}

// GetDatasetLocation returns current dataset storage mode
func (r *RandomX) GetDatasetLocation() string {
	if r.useRAMCache {
		return "RAM"
	}
	return "Disk"
}

// InitializeForBlock preloads cache and dataset for the epoch of blockNum.
func (r *RandomX) InitializeForBlock(blockNum uint64) error {
	return r.updateCacheForEpoch(r.epoch(blockNum))
}

// CurrentEpoch returns the epoch currently cached by the engine.
func (r *RandomX) CurrentEpoch() uint64 {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	return r.cacheEpoch
}

// epoch returns the epoch for a given block number.
func (r *RandomX) epoch(blockNum uint64) uint64 {
	return blockNum / r.config.EpochLength
}

// seedHash computes the seed hash for a given block number.
func (r *RandomX) seedHash(blockNum uint64) common.Hash {
	epoch := r.epoch(blockNum)
	seed := make([]byte, 32)
	for i := uint64(0); i < epoch; i++ {
		seed = crypto.Keccak256(seed)
	}
	return common.BytesToHash(seed)
}

// updateCacheForEpoch with RAM caching support
// updateCacheForEpoch - Use light mode only (no dataset)
func (r *RandomX) updateCacheForEpoch(epoch uint64) error {
    r.cacheMu.Lock()
    defer r.cacheMu.Unlock()

    if r.cacheEpoch == epoch && r.cache != nil {
        return nil
    }

    seed := r.seedHash(epoch * r.config.EpochLength)
    seedBytes := seed.Bytes()

    log.Info("Initializing RandomX for new epoch", "epoch", epoch, "seed", seed.Hex())

    // Close old resources
    if r.cache != nil {
        r.cache.Close()
    }
    if r.dataset != nil {
        r.dataset.Close()
    }

    startTime := time.Now()

    // Create cache only - no dataset for now
    var err error
    r.cache, err = randomx_lib.NewCache(0)
    if err != nil {
        return fmt.Errorf("failed to create RandomX cache: %w", err)
    }
    r.cache.Init(seedBytes)
    log.Info("RandomX cache created", "epoch", epoch, "duration", time.Since(startTime))

    // IMPORTANT: Don't create dataset - use light mode only
    r.dataset = nil
    log.Info("RandomX running in LIGHT MODE (cache only)")

    r.cacheEpoch = epoch
    return nil
}

// createDatasetInRAM creates dataset in RAM without disk I/O
func (r *RandomX) createDatasetInRAM(epoch uint64, cache *randomx_lib.Cache) (*randomx_lib.Dataset, error) {
	dataset, err := randomx_lib.NewDataset(0)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataset in RAM: %w", err)
	}
	dataset.InitDataset(cache, 0, randomx_lib.DatasetItemCount)
	log.Info("RandomX dataset allocated in RAM", "size_gb", r.config.DatasetSizeGB, "epoch", epoch)
	return dataset, nil
}

// getVM - Light mode only
func (r *RandomX) getVM() (*randomx_lib.VM, error) {
    r.cacheMu.RLock()
    defer r.cacheMu.RUnlock()

    if r.cache == nil {
        return nil, fmt.Errorf("RandomX cache not initialized")
    }

    // Use light mode (cache only) - no dataset
    log.Debug("Creating RandomX VM in light mode")
    vm, err := randomx_lib.NewVM(0, r.cache, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create RandomX VM: %w", err)
    }
    
    return vm, nil
}

// SealHash returns the hash of a block prior to it being sealed.
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
func accumulateRewards(config *params.ChainConfig, stateDB vm.StateDB, header *types.Header, uncles []*types.Header) {
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
		stateDB.AddBalance(uncle.Coinbase, r, tracing.BalanceIncreaseRewardMineUncle)

		r.Rsh(blockReward, 5)
		reward.Add(reward, r)
	}

	stateDB.AddBalance(header.Coinbase, reward, tracing.BalanceIncreaseRewardMineBlock)
}
