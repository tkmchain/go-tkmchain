// Copyright 2014 The go-ethereum Authors
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

// Package miner implements RandomX block creation and mining.
package miner

import (
	"context"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// Backend wraps all methods required for mining.
type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *txpool.TxPool
}

// Config is the configuration parameters of RandomX mining.
type Config struct {
	Enabled             bool           `toml:"-"`          // Whether mining is enabled
	Threads             int            `toml:"-"`          // Number of CPU threads for RandomX mining (0 = auto)
	Etherbase           common.Address `toml:"-"`          // Address for block mining rewards (deprecated)
	PendingFeeRecipient common.Address `toml:"-"`          // Address for pending block rewards
	ExtraData           hexutil.Bytes  `toml:",omitempty"` // Block extra data set by the miner
	GasCeil             uint64         `toml:",omitempty"` // Target gas ceiling for mined blocks
	GasPrice            *big.Int       `toml:",omitempty"` // Minimum gas price for mining a transaction
	GasLimit            uint64         `toml:",omitempty"` // Target gas limit for mined blocks
	Recommit            time.Duration  `toml:",omitempty"` // Time interval to re-create mining work
	MaxBlobsPerBlock    int            `toml:",omitempty"` // Maximum number of blobs per block

	// RandomX specific settings
	RandomXCacheSize   uint64 `toml:",omitempty"` // RandomX cache size in MB
	RandomXDatasetSize uint64 `toml:",omitempty"` // RandomX dataset size in GB
	RandomXEpochLength uint64 `toml:",omitempty"` // RandomX epoch length in blocks
	RandomXMinMemory   uint64 `toml:",omitempty"` // Minimum memory required for mining in GB
	RotationInterval   uint64 `toml:",omitempty"` // Block interval for rotating king selection
}

// DefaultConfig contains default settings for RandomX miner.
var DefaultConfig = Config{
	Enabled:            false,
	Threads:            0, // Auto-detect
	GasCeil:            60_000_000,
	GasPrice:           big.NewInt(params.GWei / 1000), // 1 Gwei default
	GasLimit:           8_000_000,                      // 8 million gas
	Recommit:           2 * time.Second,
	RandomXCacheSize:   256,
	RandomXDatasetSize: 2,
	RandomXEpochLength: 2048,
	RandomXMinMemory:   4,
	RotationInterval:   100,
}

// Miner is the main object which handles RandomX block creation and mining.
type Miner struct {
	confMu      sync.RWMutex
	config      *Config
	chainConfig *params.ChainConfig
	engine      consensus.Engine
	txpool      *txpool.TxPool
	prio        []common.Address // A list of senders to prioritize
	chain       *core.BlockChain
	pending     *pending
	pendingMu   sync.Mutex

	// Mining state
	running    atomic.Bool
	workers    []*Worker
	stopCh     chan struct{}
	wg         sync.WaitGroup
	hashRate   uint64
	hashRateMu sync.RWMutex

	// Metrics
	blocksMined   uint64
	lastMinedTime time.Time

	// King addresses for reward distribution
	mainKingAddr     common.Address
	rotatingKingAddr common.Address
	rotatingKings    []common.Address
	kingMu           sync.RWMutex
}

// New creates a new RandomX miner with provided config.
func New(eth Backend, config Config, engine consensus.Engine, mainKing common.Address, rotatingKings []common.Address) *Miner {
	// Auto-detect threads if not set
	if config.Threads <= 0 {
		config.Threads = runtime.NumCPU()
	}

	// Set default gas price if not set
	if config.GasPrice == nil || config.GasPrice.Sign() <= 0 {
		config.GasPrice = new(big.Int).Set(DefaultConfig.GasPrice)
	}

	// Set default gas limit if not set
	if config.GasLimit == 0 {
		config.GasLimit = DefaultConfig.GasLimit
	}

	// Validate RandomX config
	if config.RandomXCacheSize == 0 {
		config.RandomXCacheSize = DefaultConfig.RandomXCacheSize
	}
	if config.RandomXDatasetSize == 0 {
		config.RandomXDatasetSize = DefaultConfig.RandomXDatasetSize
	}
	if config.RandomXEpochLength == 0 {
		config.RandomXEpochLength = DefaultConfig.RandomXEpochLength
	}
	if config.RandomXMinMemory == 0 {
		config.RandomXMinMemory = DefaultConfig.RandomXMinMemory
	}

	miner := &Miner{
		config:           &config,
		chainConfig:      eth.BlockChain().Config(),
		engine:           engine,
		txpool:           eth.TxPool(),
		chain:            eth.BlockChain(),
		pending:          &pending{},
		stopCh:           make(chan struct{}),
		mainKingAddr:     mainKing,
		rotatingKingAddr: getCurrentRotatingKing(rotatingKings, 0), // Initial rotating king
		rotatingKings:    rotatingKings,
	}

	log.Info("RandomX miner initialized",
		"threads", config.Threads,
		"etherbase", config.PendingFeeRecipient.Hex(),
		"gasprice", config.GasPrice,
		"gaslimit", config.GasLimit,
		"enabled", config.Enabled,
	)

	return miner
}

func getCurrentRotatingKing(rotatingKings []common.Address, blockHeight uint64) common.Address {
	if len(rotatingKings) == 0 {
		return common.Address{}
	}
	interval := uint64(100)
	index := (blockHeight / interval) % uint64(len(rotatingKings))
	return rotatingKings[index]
}

// SetMainKing sets the main king address
func (miner *Miner) SetMainKing(address common.Address) {
	miner.kingMu.Lock()
	defer miner.kingMu.Unlock()
	miner.mainKingAddr = address
	log.Info("Main king address updated", "address", address.Hex())
}

// SetRotatingKings sets the rotating king addresses
func (miner *Miner) SetRotatingKings(addresses []common.Address) {
	miner.kingMu.Lock()
	defer miner.kingMu.Unlock()
	miner.rotatingKings = addresses
	log.Info("Rotating kings updated", "count", len(addresses))
}

// GetCurrentRotatingKing returns the current rotating king based on block height
func (miner *Miner) GetCurrentRotatingKing(blockHeight uint64) common.Address {
	miner.kingMu.RLock()
	defer miner.kingMu.RUnlock()

	if len(miner.rotatingKings) == 0 {
		return common.Address{}
	}

	interval := uint64(100) // Default rotation interval
	if miner.config.RotationInterval > 0 {
		interval = miner.config.RotationInterval
	}

	index := (blockHeight / interval) % uint64(len(miner.rotatingKings))
	return miner.rotatingKings[index]
}

// GetMainKing returns the main king address
func (miner *Miner) GetMainKing() common.Address {
	miner.kingMu.RLock()
	defer miner.kingMu.RUnlock()
	return miner.mainKingAddr
}

// Start begins the RandomX mining process with the configured number of threads.
func (miner *Miner) Start() error {
	miner.confMu.Lock()
	defer miner.confMu.Unlock()

	if !miner.config.Enabled {
		return fmt.Errorf("mining is not enabled in config")
	}

	if miner.running.Load() {
		return fmt.Errorf("miner already running")
	}

	// Check if etherbase is set
	if miner.config.PendingFeeRecipient == (common.Address{}) {
		return fmt.Errorf("etherbase address not set")
	}

	// Verify RandomX engine compatibility
	randomxEngine, ok := miner.engine.(*randomx.RandomX)
	if !ok {
		return fmt.Errorf("engine is not RandomX, cannot mine")
	}

	// Initialize RandomX cache and dataset for current epoch
	blockNum := miner.chain.CurrentHeader().Number.Uint64()
	if err := randomxEngine.InitializeForBlock(blockNum); err != nil {
		return fmt.Errorf("failed to initialize RandomX: %w", err)
	}

	// Start mining workers
	miner.running.Store(true)
	miner.stopCh = make(chan struct{})

	miner.workers = make([]*Worker, 0, miner.config.Threads)
	for i := 0; i < miner.config.Threads; i++ {
		worker := NewWorker(miner, i)
		miner.workers = append(miner.workers, worker)
		worker.Start()
	}

	// Start hashrate monitor
	miner.wg.Add(1)
	go miner.hashRateMonitor()

	log.Info("RandomX miner started",
		"threads", miner.config.Threads,
		"etherbase", miner.config.PendingFeeRecipient.Hex(),
	)

	return nil
}

// Stop terminates the RandomX mining process.
func (miner *Miner) Stop() error {
	miner.confMu.Lock()
	defer miner.confMu.Unlock()

	if !miner.running.Load() {
		return nil
	}

	// Signal stop to all workers
	close(miner.stopCh)

	// Wait for all workers to stop
	for _, worker := range miner.workers {
		worker.Stop()
	}

	miner.wg.Wait()
	miner.running.Store(false)

	log.Info("RandomX miner stopped", "blocks_mined", miner.blocksMined)
	return nil
}

// Running returns whether the miner is currently running.
func (miner *Miner) Running() bool {
	return miner.running.Load()
}

// Mining returns whether the miner is currently mining (alias for Running).
func (miner *Miner) Mining() bool {
	return miner.running.Load()
}

// HashRate returns the current total mining hashrate in hashes per second.
func (miner *Miner) HashRate() uint64 {
	miner.hashRateMu.RLock()
	defer miner.hashRateMu.RUnlock()
	return miner.hashRate
}

// GetHashRate returns the current hashrate (RPC friendly).
func (miner *Miner) GetHashRate() uint64 {
	return miner.HashRate()
}

// hashRateMonitor periodically updates the total hashrate.
func (miner *Miner) hashRateMonitor() {
	defer miner.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-miner.stopCh:
			return
		case <-ticker.C:
			var totalRate uint64
			for _, worker := range miner.workers {
				totalRate += worker.HashRate()
			}
			miner.hashRateMu.Lock()
			miner.hashRate = totalRate
			miner.hashRateMu.Unlock()

			if totalRate > 0 {
				log.Debug("RandomX hashrate updated", "total", totalRate, "workers", len(miner.workers))
			}
		}
	}
}

// SetExtra sets the content used to initialize the block extra field.
func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.confMu.Lock()
	miner.config.ExtraData = extra
	miner.confMu.Unlock()
	return nil
}

// SetEtherbase sets the address that will receive mining rewards.
func (miner *Miner) SetEtherbase(address common.Address) error {
	if address == (common.Address{}) {
		return fmt.Errorf("invalid etherbase address")
	}
	miner.confMu.Lock()
	miner.config.PendingFeeRecipient = address
	miner.config.Etherbase = address
	miner.confMu.Unlock()
	log.Info("Miner etherbase updated", "address", address.Hex())
	return nil
}

// SetGasCeil sets the gaslimit to strive for when mining blocks.
func (miner *Miner) SetGasCeil(ceil uint64) {
	miner.confMu.Lock()
	miner.config.GasCeil = ceil
	miner.confMu.Unlock()
	log.Info("Miner gas ceiling updated", "ceil", ceil)
}

// SetGasLimit sets the target gas limit for mined blocks.
func (miner *Miner) SetGasLimit(limit uint64) {
	miner.confMu.Lock()
	miner.config.GasLimit = limit
	miner.confMu.Unlock()
	log.Info("Miner gas limit updated", "limit", limit)
}

// SetGasPrice sets the minimum gas price for transaction inclusion.
func (miner *Miner) SetGasPrice(price *big.Int) {
	miner.confMu.Lock()
	miner.config.GasPrice = price
	miner.confMu.Unlock()
	log.Info("Miner gas price updated", "price", price)
}

// SetThreads dynamically changes the number of mining threads.
func (miner *Miner) SetThreads(threads int) error {
	miner.confMu.Lock()
	defer miner.confMu.Unlock()

	if threads <= 0 {
		threads = runtime.NumCPU()
	}

	if !miner.running.Load() {
		miner.config.Threads = threads
		return nil
	}

	// Restart miners with new thread count
	if err := miner.Stop(); err != nil {
		return err
	}

	miner.config.Threads = threads
	if err := miner.Start(); err != nil {
		return err
	}

	log.Info("Miner threads updated", "threads", threads)
	return nil
}

// SetPrioAddresses sets a list of addresses to prioritize for transaction inclusion.
func (miner *Miner) SetPrioAddresses(prio []common.Address) {
	miner.confMu.Lock()
	miner.prio = prio
	miner.confMu.Unlock()
}

// Pending returns the currently pending block and associated receipts, logs and statedb.
func (miner *Miner) Pending() (*types.Block, types.Receipts, *state.StateDB) {
	pending := miner.getPending()
	if pending == nil {
		return nil, nil, nil
	}
	return pending.block, pending.receipts, pending.stateDB.Copy()
}

// GetPending returns the pending block (alias for Pending).
func (miner *Miner) GetPending() (*types.Block, types.Receipts, *state.StateDB) {
	return miner.Pending()
}

// BuildPayload builds the payload according to the provided parameters.
func (miner *Miner) BuildPayload(ctx context.Context, args *BuildPayloadArgs, witness bool) (*Payload, error) {
	return miner.buildPayload(ctx, args, witness)
}

// getPending retrieves the pending block based on the current head block.
func (miner *Miner) getPending() *newPayloadResult {
	header := miner.chain.CurrentHeader()
	miner.pendingMu.Lock()
	defer miner.pendingMu.Unlock()

	if cached := miner.pending.resolve(header.Hash()); cached != nil {
		return cached
	}

	var (
		timestamp  = uint64(time.Now().Unix())
		withdrawal types.Withdrawals
	)

	if miner.chainConfig.IsShanghai(new(big.Int).Add(header.Number, big.NewInt(1)), timestamp) {
		withdrawal = []*types.Withdrawal{}
	}

	ret := miner.generateWork(context.Background(),
		&generateParams{
			timestamp:   timestamp,
			forceTime:   false,
			parentHash:  header.Hash(),
			coinbase:    miner.config.PendingFeeRecipient,
			random:      common.Hash{},
			withdrawals: withdrawal,
			beaconRoot:  nil,
			noTxs:       false,
		}, false)

	if ret.err != nil {
		return nil
	}
	miner.pending.update(header.Hash(), ret)
	return ret
}

// SubmitWork submits successfully mined block to the blockchain.
func (miner *Miner) SubmitWork(block *types.Block) error {
	// Insert the block into the blockchain
	if _, err := miner.chain.InsertChain([]*types.Block{block}); err != nil {
		log.Error("Failed to insert mined block", "error", err)
		return err
	}

	// Update metrics
	atomic.AddUint64(&miner.blocksMined, 1)
	miner.lastMinedTime = time.Now()

	log.Info("Block successfully mined",
		"number", block.NumberU64(),
		"hash", block.Hash(),
		"total_mined", miner.blocksMined,
	)

	return nil
}

// GetMiningInfo returns detailed mining information for RPC.
func (miner *Miner) GetMiningInfo() map[string]interface{} {
	miner.confMu.RLock()
	defer miner.confMu.RUnlock()

	info := map[string]interface{}{
		"enabled":      miner.config.Enabled,
		"mining":       miner.running.Load(),
		"threads":      miner.config.Threads,
		"etherbase":    miner.config.PendingFeeRecipient.Hex(),
		"hashrate":     miner.HashRate(),
		"gasprice":     miner.config.GasPrice.String(),
		"gaslimit":     miner.config.GasLimit,
		"blocks_mined": atomic.LoadUint64(&miner.blocksMined),
	}

	if !miner.lastMinedTime.IsZero() {
		info["last_mined"] = miner.lastMinedTime.Unix()
	}

	// Add RandomX specific info
	if randomxEngine, ok := miner.engine.(*randomx.RandomX); ok {
		info["randomx"] = map[string]interface{}{
			"epoch":           randomxEngine.CurrentEpoch(),
			"cache_size_mb":   miner.config.RandomXCacheSize,
			"dataset_size_gb": miner.config.RandomXDatasetSize,
			"epoch_length":    miner.config.RandomXEpochLength,
			"min_memory_gb":   miner.config.RandomXMinMemory,
		}
	}

	return info
}

// GetStats returns miner statistics.
func (miner *Miner) GetStats() (uint64, uint64, time.Time) {
	return miner.HashRate(), atomic.LoadUint64(&miner.blocksMined), miner.lastMinedTime
}

// Create new worker (to be called from worker.go)
func (miner *Miner) createWorker(index int) *Worker {
	return NewWorker(miner, index)
}

// Update pending block after new head (for worker to call)
func (miner *Miner) updatePending() {
	miner.pendingMu.Lock()
	defer miner.pendingMu.Unlock()

	// Clear cached pending block when head changes
	header := miner.chain.CurrentHeader()
	miner.pending.clear(header.Hash())
}
