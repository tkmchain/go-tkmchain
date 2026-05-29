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

// Package eth implements the Ethereum protocol.
package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/filtermaps"
	"github.com/ethereum/go-ethereum/core/history"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state/pruner"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/blobpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/txpool/locals"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/internal/shutdowncheck"
	"github.com/ethereum/go-ethereum/internal/version"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/dnsdisc"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	gethversion "github.com/ethereum/go-ethereum/version"
)

const (
	// discmixTimeout is the fairness knob for the discovery mixer
	discmixTimeout = 100 * time.Millisecond

	// discoveryPrefetchBuffer is the number of peers to pre-fetch from discovery
	discoveryPrefetchBuffer = 32

	// maxParallelENRRequests is the maximum number of parallel ENR requests
	maxParallelENRRequests = 16
)

// Config contains the configuration options of the ETH protocol.
// Deprecated: use ethconfig.Config instead.
type Config = ethconfig.Config

// Ethereum implements the Ethereum full node service with RandomX PoW and Rotating King consensus.
type Ethereum struct {
	// Core protocol objects
	config         *ethconfig.Config
	txPool         *txpool.TxPool
	blobTxPool     *blobpool.BlobPool
	localTxTracker *locals.TxTracker
	blockchain     *core.BlockChain

	handler *handler
	discmix *enode.FairMix
	dropper *dropper

	// DB interfaces
	chainDb ethdb.Database // Block chain database

	engine         consensus.Engine
	accountManager *accounts.Manager

	filterMaps      *filtermaps.FilterMaps
	closeFilterMaps chan chan struct{}

	APIBackend *EthAPIBackend

	miner    *miner.Miner
	gasPrice *big.Int

	networkID     uint64
	netRPCService *ethapi.NetAPI

	p2pServer *p2p.Server

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and etherbase)

	shutdownTracker *shutdowncheck.ShutdownTracker // Tracks if and when the node has shutdown ungracefully

	// Rotating King configuration
	mainKingAddress common.Address
	kingAddresses   []common.Address
	rkLocks         map[common.Address]time.Time
}

// New creates a new Ethereum object with RandomX consensus and Rotating King support
func New(stack *node.Node, config *ethconfig.Config) (*Ethereum, error) {
	// Ensure configuration values are compatible and sane
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	if !config.HistoryMode.IsValid() {
		return nil, fmt.Errorf("invalid history mode %d", config.HistoryMode)
	}
	if config.Miner.GasPrice == nil || config.Miner.GasPrice.Sign() <= 0 {
		log.Warn("Sanitizing invalid miner gas price", "provided", config.Miner.GasPrice, "updated", ethconfig.Defaults.Miner.GasPrice)
		config.Miner.GasPrice = new(big.Int).Set(ethconfig.Defaults.Miner.GasPrice)
	}
	if config.NoPruning && config.TrieDirtyCache > 0 && config.StateScheme == rawdb.HashScheme {
		if config.SnapshotCache > 0 {
			config.TrieCleanCache += config.TrieDirtyCache * 3 / 5
			config.SnapshotCache += config.TrieDirtyCache * 2 / 5
		} else {
			config.TrieCleanCache += config.TrieDirtyCache
		}
		config.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	// Open database
	dbOptions := node.DatabaseOptions{
		Cache:             config.DatabaseCache,
		Handles:           config.DatabaseHandles,
		AncientsDirectory: config.DatabaseFreezer,
		EraDirectory:      config.DatabaseEra,
		MetricsNamespace:  "eth/db/chaindata/",
		ReadOnly:          false,
	}
	chainDb, err := stack.OpenDatabaseWithOptions("chaindata", dbOptions)
	if err != nil {
		return nil, err
	}

	scheme, err := rawdb.ParseStateScheme(config.StateScheme, chainDb)
	if err != nil {
		return nil, err
	}

	// Try to recover offline state pruning only in hash-based
	if scheme == rawdb.HashScheme {
		if err := pruner.RecoverPruning(stack.ResolvePath(""), chainDb); err != nil {
			log.Error("Failed to recover state", "error", err)
		}
	}

	// Load chain configuration
	chainConfig, genesisHash, err := core.LoadChainConfig(chainDb, config.Genesis)
	if err != nil {
		return nil, err
	}

	if chainConfig.RandomX != nil {
		if config.SyncMode == ethconfig.SnapSync {
			log.Info("RandomX chain detected: disabling snap-sync, using full sync")
			config.SyncMode = ethconfig.FullSync
		}
		// Also disable snapshot cache for RandomX (not needed)
		if config.SnapshotCache > 0 {
			log.Debug("RandomX chain: disabling snapshot cache")
			config.SnapshotCache = 0
		}
	}
	// Prefer chain-configured king addresses so they are consensus-bound.
	mainKingAddress := chainConfig.MainKingAddress
	if mainKingAddress == (common.Address{}) {
		mainKingAddress = config.MainKingAddress
	}
	if mainKingAddress == (common.Address{}) {
		mainKingAddress = common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2")
	}
	kingAddresses := chainConfig.RotatingKingAddresses
	if len(kingAddresses) == 0 {
		kingAddresses = config.KingAddresses
	}
	if len(kingAddresses) == 0 {
		kingAddresses = []common.Address{
			common.HexToAddress("0x0000000000000000000000000000000000000002"),
			common.HexToAddress("0x0000000000000000000000000000000000000003"),
			common.HexToAddress("0x0000000000000000000000000000000000000004"),
		}
	}

	// Create RandomX consensus engine with Rotating King support
	engine, err := randomx.New(chainConfig.RandomX, config.RandomXMinerThreads, mainKingAddress, kingAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to create RandomX engine: %w", err)
	}

	// Set networkID to chainID by default
	networkID := config.NetworkId
	if networkID == 0 {
		networkID = chainConfig.ChainID.Uint64()
	}

	// Assemble the Ethereum object
	eth := &Ethereum{
		config:          config,
		chainDb:         chainDb,
		accountManager:  stack.AccountManager(),
		engine:          engine,
		networkID:       networkID,
		gasPrice:        config.Miner.GasPrice,
		p2pServer:       stack.Server(),
		discmix:         enode.NewFairMix(discmixTimeout),
		shutdownTracker: shutdowncheck.NewShutdownTracker(chainDb),
		mainKingAddress: mainKingAddress,
		kingAddresses:   kingAddresses,
		rkLocks:         make(map[common.Address]time.Time),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Info("Initialising Ethereum protocol with RandomX",
		"network", networkID,
		"dbversion", dbVer,
		"mainKing", mainKingAddress.Hex(),
		"rotatingKings", len(kingAddresses))

	// Check database version
	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Geth %s only supports v%d", *bcVersion, version.WithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			if bcVersion != nil {
				log.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			}
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}

	// Create history policy
	histPolicy, err := history.NewPolicy(config.HistoryMode, genesisHash)
	if err != nil {
		return nil, err
	}

	// Configure blockchain options
	options := &core.BlockChainConfig{
		TrieCleanLimit:          config.TrieCleanCache,
		NoPrefetch:              config.NoPrefetch,
		TrieDirtyLimit:          config.TrieDirtyCache,
		ArchiveMode:             config.NoPruning,
		TrieTimeLimit:           config.TrieTimeout,
		SnapshotLimit:           config.SnapshotCache,
		Preimages:               config.Preimages,
		StateHistory:            config.StateHistory,
		TrienodeHistory:         config.TrienodeHistory,
		NodeFullValueCheckpoint: config.NodeFullValueCheckpoint,
		BinTrieGroupDepth:       config.BinTrieGroupDepth,
		StateScheme:             scheme,
		HistoryPolicy:           histPolicy,
		TxLookupLimit:           int64(min(config.TransactionHistory, math.MaxInt64)),
		VmConfig: vm.Config{
			EnablePreimageRecording: config.EnablePreimageRecording,
		},
		TrieJournalDirectory:    stack.ResolvePath("triedb"),
		StateSizeTracking:       config.EnableStateSizeTracking,
		SlowBlockThreshold:      config.SlowBlockThreshold,
		StatelessSelfValidation: config.StatelessSelfValidation,
		EnableWitnessStats:      config.EnableWitnessStats,
	}

	// Configure VM tracing
	if config.VMTrace != "" {
		traceConfig := json.RawMessage("{}")
		if config.VMTraceJsonConfig != "" {
			traceConfig = json.RawMessage(config.VMTraceJsonConfig)
		}
		t, err := tracers.LiveDirectory.New(config.VMTrace, traceConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create tracer %s: %v", config.VMTrace, err)
		}
		options.VmConfig.Tracer = t
	}

	// Override chain config with provided settings
	var overrides core.ChainOverrides
	if config.OverrideOsaka != nil {
		overrides.OverrideOsaka = config.OverrideOsaka
	}
	if config.OverrideBPO1 != nil {
		overrides.OverrideBPO1 = config.OverrideBPO1
	}
	if config.OverrideBPO2 != nil {
		overrides.OverrideBPO2 = config.OverrideBPO2
	}
	if config.OverrideUBT != nil {
		overrides.OverrideUBT = config.OverrideUBT
	}
	options.Overrides = &overrides

	// Create blockchain
	eth.blockchain, err = core.NewBlockChain(chainDb, config.Genesis, eth.engine, options)
	if err != nil {
		return nil, err
	}

	// Initialize filter maps for log indexing
	fmConfig := filtermaps.Config{
		History:        config.LogHistory,
		Disabled:       config.LogNoHistory,
		ExportFileName: config.LogExportCheckpoints,
		HashScheme:     scheme == rawdb.HashScheme,
	}
	chainView := eth.newChainView(eth.blockchain.CurrentBlock())
	historyCutoff, _ := eth.blockchain.HistoryPruningCutoff()
	var finalBlock uint64
	if fb := eth.blockchain.CurrentFinalBlock(); fb != nil {
		finalBlock = fb.Number.Uint64()
	}
	filterMaps, err := filtermaps.NewFilterMaps(chainDb, chainView, historyCutoff, finalBlock, filtermaps.DefaultParams, fmConfig)
	if err != nil {
		return nil, err
	}
	eth.filterMaps = filterMaps
	eth.closeFilterMaps = make(chan chan struct{})

	// Initialize transaction pools
	if config.TxPool.Journal != "" {
		config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
	}
	legacyPool := legacypool.New(config.TxPool, eth.blockchain)

	if config.BlobPool.Datadir != "" {
		config.BlobPool.Datadir = stack.ResolvePath(config.BlobPool.Datadir)
	}
	eth.blobTxPool = blobpool.New(config.BlobPool, eth.blockchain, legacyPool.HasPendingAuth)

	eth.txPool, err = txpool.New(config.TxPool.PriceLimit, eth.blockchain, []txpool.SubPool{legacyPool, eth.blobTxPool})
	if err != nil {
		return nil, err
	}

	// Initialize local transaction tracker
	if !config.TxPool.NoLocals {
		rejournal := config.TxPool.Rejournal
		if rejournal < time.Second {
			log.Warn("Sanitizing invalid txpool journal time", "provided", rejournal, "updated", time.Second)
			rejournal = time.Second
		}
		eth.localTxTracker = locals.New(config.TxPool.Journal, rejournal, eth.blockchain.Config(), eth.txPool)
		stack.RegisterLifecycle(eth.localTxTracker)
	}

	// Create network handler
	cacheLimit := options.TrieCleanLimit + options.TrieDirtyLimit + options.SnapshotLimit
	if eth.handler, err = newHandler(&handlerConfig{
		NodeID:             eth.p2pServer.Self().ID(),
		Database:           chainDb,
		Chain:              eth.blockchain,
		TxPool:             eth.txPool,
		Network:            networkID,
		Sync:               config.SyncMode,
		BloomCache:         uint64(cacheLimit),
		RequiredBlocks:     config.RequiredBlocks,
		RotatingKingUpdate: eth.noteRotatingKing,
	}); err != nil {
		return nil, err
	}

	// Initialize connection dropper
	eth.dropper = newDropper(eth.p2pServer.MaxDialedConns(), eth.p2pServer.MaxInboundConns())

	// Initialize miner
	eth.miner = miner.New(eth, chainConfig, new(event.TypeMux), eth.engine, config.Miner.Recommit, config.Miner.GasFloor, config.Miner.GasCeil, nil)
	eth.miner.SetExtra(makeExtraData(config.Miner.ExtraData))

	// Setup API backend
	eth.APIBackend = &EthAPIBackend{
		extRPCEnabled:       stack.Config().ExtRPCEnabled(),
		allowUnprotectedTxs: stack.Config().AllowUnprotectedTxs,
		eth:                 eth,
		gpo:                 nil,
	}
	if eth.APIBackend.allowUnprotectedTxs {
		log.Info("Unprotected transactions allowed")
	}
	eth.APIBackend.gpo = gasprice.NewOracle(eth.APIBackend, config.GPO, config.Miner.GasPrice)

	// Start RPC service
	eth.netRPCService = ethapi.NewNetAPI(eth.p2pServer, networkID)

	// Register backend on the node
	stack.RegisterAPIs(eth.APIs())
	stack.RegisterProtocols(eth.Protocols())
	stack.RegisterLifecycle(eth)

	// Mark successful startup
	eth.shutdownTracker.MarkStartup()

	log.Info("Ethereum backend initialized successfully with RandomX and Rotating King")
	return eth, nil
}

// makeExtraData creates the extra data for the miner
func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// Create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(gethversion.Major<<16 | gethversion.Minor<<8 | gethversion.Patch),
			"geth",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// APIs returns the collection of RPC services
func (s *Ethereum) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.APIBackend)

	// Append local APIs
	return append(apis, []rpc.API{
		{
			Namespace: "miner",
			Service:   NewMinerAPI(s),
		},
		{
			Namespace: "eth",
			Service:   downloader.NewDownloaderAPI(s.handler.downloader, s.blockchain),
		},
		{
			Namespace: "admin",
			Service:   NewAdminAPI(s),
		},
		{
			Namespace: "debug",
			Service:   NewDebugAPI(s),
		},
		{
			Namespace: "net",
			Service:   s.netRPCService,
		},
		{
			Namespace: "king",
			Service:   NewKingAPI(s),
		},
		{
			Namespace: "rk",
			Service:   NewKingAPI(s),
		},
	}...)
}

// ResetWithGenesisBlock resets the blockchain with the given genesis block
func (s *Ethereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

// Miner returns the miner instance
func (s *Ethereum) Miner() *miner.Miner { return s.miner }

// AccountManager returns the account manager
func (s *Ethereum) AccountManager() *accounts.Manager { return s.accountManager }

// BlockChain returns the blockchain instance
func (s *Ethereum) BlockChain() *core.BlockChain { return s.blockchain }

// TxPool returns the transaction pool
func (s *Ethereum) TxPool() *txpool.TxPool { return s.txPool }

// BlobTxPool returns the blob transaction pool
func (s *Ethereum) BlobTxPool() *blobpool.BlobPool { return s.blobTxPool }

// Engine returns the consensus engine
func (s *Ethereum) Engine() consensus.Engine { return s.engine }

// ChainDb returns the chain database
func (s *Ethereum) ChainDb() ethdb.Database { return s.chainDb }

// IsListening returns whether the node is listening
func (s *Ethereum) IsListening() bool { return true }

// Downloader returns the downloader
func (s *Ethereum) Downloader() *downloader.Downloader { return s.handler.downloader }

// Synced returns whether the node is synced
func (s *Ethereum) Synced() bool { return s.handler.synced.Load() }

// SetSynced sets the synced status
func (s *Ethereum) SetSynced() { s.handler.enableSyncedFeatures() }

// ArchiveMode returns whether archive mode is enabled
func (s *Ethereum) ArchiveMode() bool { return s.config.NoPruning }

// GetMainKingAddress returns the main king address
func (s *Ethereum) GetMainKingAddress() common.Address {
	return s.mainKingAddress
}

// GetKingAddresses returns all rotating king addresses
func (s *Ethereum) GetKingAddresses() []common.Address {
	return s.kingAddresses
}

// StartMining starts the RandomX miner with the configured settings
func (s *Ethereum) StartMining() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Check if mining is enabled in config
	if !s.config.Miner.Enabled {
		log.Info("Mining is not enabled in config, use --mine flag to enable")
		return nil
	}

	// Set etherbase if provided and not already set
	if s.config.Miner.Etherbase != (common.Address{}) {
		s.miner.SetEtherbase(s.config.Miner.Etherbase)
		log.Info("Setting miner etherbase", "address", s.config.Miner.Etherbase.Hex())
	}

	// Set extra data if provided
	if len(s.config.Miner.ExtraData) > 0 {
		s.miner.SetExtra(s.config.Miner.ExtraData)
	}

	// Set gas price and limit
	if s.config.Miner.GasPrice != nil && s.config.Miner.GasPrice.Sign() > 0 {
		s.txPool.SetGasTip(s.config.Miner.GasPrice)
	}
	if s.config.Miner.GasLimit > 0 {
		s.config.Miner.GasCeil = s.config.Miner.GasLimit
	}

	// Start the miner
	s.miner.Start(s.config.Miner.Etherbase)

	log.Info("RandomX miner started successfully",
		"threads", runtime.NumCPU(),
		"etherbase", s.config.Miner.Etherbase.Hex(),
		"gasprice", s.config.Miner.GasPrice,
		"gaslimit", s.config.Miner.GasLimit,
	)

	return nil
}

// StopMining stops the RandomX miner
func (s *Ethereum) StopMining() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.miner.Mining() {
		return nil
	}

	s.miner.Stop()
	log.Info("RandomX miner stopped")
	return nil
}

// IsMining returns whether the RandomX miner is currently running
func (s *Ethereum) IsMining() bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.miner.Mining()
}

// GetMiningInfo returns detailed mining information
func (s *Ethereum) GetMiningInfo() map[string]interface{} {
	s.lock.RLock()
	defer s.lock.RUnlock()

	info := map[string]interface{}{
		"enabled":      s.config.Miner.Enabled,
		"mining":       s.miner.Mining(),
		"threads":      runtime.NumCPU(),
		"etherbase":    s.config.Miner.Etherbase.Hex(),
		"hashrate":     s.miner.HashRate(),
		"gasprice":     s.config.Miner.GasPrice.String(),
		"gaslimit":     s.config.Miner.GasLimit,
		"block_number": s.blockchain.CurrentBlock().Number.Uint64(),
	}
	pending, queued := s.txPool.Stats()
	info["pending_txs"] = pending + queued

	// Add RandomX specific info from engine
	if r, ok := s.engine.(*randomx.RandomX); ok {
		info["randomx"] = map[string]interface{}{
			"epoch":       r.CurrentEpoch(),
			"mining_mode": "full",
		}
	}

	// Add king info
	info["main_king"] = s.mainKingAddress.Hex()
	info["rotating_king"] = s.getCurrentRotatingKing()

	return info
}

func (s *Ethereum) noteRotatingKing(address common.Address, unlock time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.recordRotatingKingLocked(address, unlock)
}

func (s *Ethereum) recordRotatingKingLocked(address common.Address, unlock time.Time) {
	if current, ok := s.rkLocks[address]; !ok || unlock.After(current) {
		s.rkLocks[address] = unlock
	}
	for _, existing := range s.kingAddresses {
		if existing == address {
			return
		}
	}
	s.kingAddresses = append(s.kingAddresses, address)
}

// getCurrentRotatingKing returns the current rotating king based on block height
func (s *Ethereum) getCurrentRotatingKing() common.Address {
	if len(s.kingAddresses) == 0 {
		return common.Address{}
	}

	// Get rotation interval from chain config or use default
	interval := uint64(100)
	if s.blockchain.Config().RotatingKingRotationInterval > 0 {
		interval = s.blockchain.Config().RotatingKingRotationInterval
	}

	blockNum := s.blockchain.CurrentBlock().Number.Uint64()
	index := (blockNum / interval) % uint64(len(s.kingAddresses))
	return s.kingAddresses[index]
}

// SetMinerThreads is unsupported for this miner implementation.
func (s *Ethereum) SetMinerThreads(threads int) error {
	return fmt.Errorf("setting miner threads is not supported")
}

// SetMinerEtherbase dynamically sets the reward recipient address
func (s *Ethereum) SetMinerEtherbase(address common.Address) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if address == (common.Address{}) {
		return fmt.Errorf("invalid etherbase address")
	}

	s.config.Miner.Etherbase = address
	s.miner.SetEtherbase(address)
	log.Info("Updated miner etherbase", "address", address.Hex())
	return nil
}

// GetHashRate returns the current hashrate
func (api *MinerAPI) GetHashRate() hexutil.Uint64 {
	return hexutil.Uint64(api.e.miner.HashRate())
}

// GetMiningInfo returns detailed mining information
func (api *MinerAPI) GetMiningInfo() map[string]interface{} {
	return api.e.GetMiningInfo()
}

// Protocols returns all configured network protocols
func (s *Ethereum) Protocols() []p2p.Protocol {
	protos := eth.MakeProtocols((*ethHandler)(s.handler), s.networkID, s.discmix)
	if s.config.SnapshotCache > 0 {
		protos = append(protos, snap.MakeProtocols((*snapHandler)(s.handler))...)
	}
	return protos
}

// Start implements node.Lifecycle, starting all internal goroutines
func (s *Ethereum) Start() error {
	if err := s.setupDiscovery(); err != nil {
		return err
	}

	s.shutdownTracker.Start()
	s.handler.Start(s.p2pServer.MaxPeers)
	s.dropper.Start(s.p2pServer, func() bool { return !s.Synced() })
	s.filterMaps.Start()
	go s.updateFilterMapsHeads()

	log.Info("Ethereum backend started with RandomX consensus")
	return nil
}

// newChainView creates a new chain view for filter maps
func (s *Ethereum) newChainView(head *types.Header) *filtermaps.ChainView {
	if head == nil {
		return nil
	}
	return filtermaps.NewChainView(s.blockchain, head.Number.Uint64(), head.Hash())
}

// updateFilterMapsHeads updates filter maps heads
func (s *Ethereum) updateFilterMapsHeads() {
	headEventCh := make(chan core.ChainEvent, 10)
	blockProcCh := make(chan bool, 10)
	sub := s.blockchain.SubscribeChainEvent(headEventCh)
	sub2 := s.blockchain.SubscribeBlockProcessingEvent(blockProcCh)
	defer func() {
		sub.Unsubscribe()
		sub2.Unsubscribe()
		for {
			select {
			case <-headEventCh:
			case <-blockProcCh:
			default:
				return
			}
		}
	}()

	var head *types.Header
	setHead := func(newHead *types.Header) {
		if newHead == nil {
			return
		}
		if head == nil || newHead.Hash() != head.Hash() {
			head = newHead
			chainView := s.newChainView(head)
			if chainView == nil {
				return
			}
			historyCutoff, _ := s.blockchain.HistoryPruningCutoff()
			var finalBlock uint64
			if fb := s.blockchain.CurrentFinalBlock(); fb != nil {
				finalBlock = fb.Number.Uint64()
			}
			s.filterMaps.SetTarget(chainView, historyCutoff, finalBlock)
		}
	}
	setHead(s.blockchain.CurrentBlock())

	for {
		select {
		case ev := <-headEventCh:
			setHead(ev.Header)
		case blockProc := <-blockProcCh:
			s.filterMaps.SetBlockProcessing(blockProc)
		case <-time.After(time.Second * 10):
			setHead(s.blockchain.CurrentBlock())
		case ch := <-s.closeFilterMaps:
			close(ch)
			return
		}
	}
}

// setupDiscovery sets up peer discovery
func (s *Ethereum) setupDiscovery() error {
	eth.StartENRUpdater(s.blockchain, s.p2pServer.LocalNode())

	dnsclient := dnsdisc.NewClient(dnsdisc.Config{})

	// Add eth nodes from DNS
	if len(s.config.EthDiscoveryURLs) > 0 {
		iter, err := dnsclient.NewIterator(s.config.EthDiscoveryURLs...)
		if err != nil {
			return err
		}
		s.discmix.AddSource(iter)
	}

	// Add snap nodes from DNS
	if len(s.config.SnapDiscoveryURLs) > 0 {
		iter, err := dnsclient.NewIterator(s.config.SnapDiscoveryURLs...)
		if err != nil {
			return err
		}
		s.discmix.AddSource(iter)
	}

	// Add DHT nodes from discv4
	if s.p2pServer.DiscoveryV4() != nil {
		iter := s.p2pServer.DiscoveryV4().RandomNodes()
		resolverFunc := func(ctx context.Context, enr *enode.Node) *enode.Node {
			nn, _ := s.p2pServer.DiscoveryV4().RequestENR(enr)
			return nn
		}
		iter = enode.AsyncFilter(iter, resolverFunc, maxParallelENRRequests)
		iter = enode.Filter(iter, eth.NewNodeFilter(s.blockchain))
		iter = enode.NewBufferIter(iter, discoveryPrefetchBuffer)
		s.discmix.AddSource(iter)
	}

	// Add DHT nodes from discv5
	if s.p2pServer.DiscoveryV5() != nil {
		filter := eth.NewNodeFilter(s.blockchain)
		iter := enode.Filter(s.p2pServer.DiscoveryV5().RandomNodes(), filter)
		iter = enode.NewBufferIter(iter, discoveryPrefetchBuffer)
		s.discmix.AddSource(iter)
	}

	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines
func (s *Ethereum) Stop() error {
	if err := s.StopMining(); err != nil {
		log.Warn("Failed to stop RandomX miner during shutdown", "err", err)
	}

	s.discmix.Close()
	s.dropper.Stop()
	s.handler.Stop()

	ch := make(chan struct{})
	s.closeFilterMaps <- ch
	<-ch
	s.filterMaps.Stop()
	s.txPool.Close()
	s.blockchain.Stop()
	s.engine.Close()

	s.shutdownTracker.Stop()
	s.chainDb.Close()

	log.Info("Ethereum backend stopped")
	return nil
}

// Function for min
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
