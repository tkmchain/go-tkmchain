// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package miner

import (
        "math/big"
        "sync"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethereum/go-ethereum/consensus"
        "github.com/ethereum/go-ethereum/core"
        "github.com/ethereum/go-ethereum/core/state"
        "github.com/ethereum/go-ethereum/core/types"
        "github.com/ethereum/go-ethereum/log"
)

// MinerConfig contains the miner configuration
type MinerConfig struct {
        Enabled   bool
        Etherbase common.Address
        ExtraData []byte
        GasPrice  *big.Int
        GasLimit  uint64
        Recommit  uint64
        GasFloor  uint64
        GasCeil   uint64
        Threads   int
}

// DefaultMinerConfig is the default miner configuration
var DefaultMinerConfig = MinerConfig{
        Enabled:  false,
        GasPrice: big.NewInt(1e9),
        GasLimit: 8000000,
        Threads:  1,
}

// Miner is the RandomX miner
type Miner struct {
        engine   consensus.Engine
        coinbase common.Address
        mining   bool
        mu       sync.RWMutex
        stopCh   chan struct{}
        chain    *core.BlockChain
        config   *MinerConfig
}

// New creates a new RandomX miner
func New(chain *core.BlockChain, engine consensus.Engine, config *MinerConfig) *Miner {
        if config == nil {
                config = &DefaultMinerConfig
        }
        return &Miner{
                engine:   engine,
                chain:    chain,
                coinbase: config.Etherbase,
                stopCh:   make(chan struct{}),
                config:   config,
        }
}

// Start starts the miner
func (m *Miner) Start(coinbase common.Address) {
        m.mu.Lock()
        defer m.mu.Unlock()

        if m.mining {
                return
        }

        if coinbase != (common.Address{}) {
                m.coinbase = coinbase
        }

        m.mining = true
        log.Info("RandomX miner started", "coinbase", m.coinbase.Hex())
}

// StartExternal starts external miner mode (getwork)
func (m *Miner) StartExternal(coinbase common.Address) {
        m.Start(coinbase)
        log.Info("External miner mode enabled - getwork available")
}

// Stop stops the miner
func (m *Miner) Stop() {
        m.mu.Lock()
        defer m.mu.Unlock()

        if !m.mining {
                return
        }

        m.mining = false
        close(m.stopCh)
        log.Info("RandomX miner stopped")
}

// Mining returns whether the miner is running
func (m *Miner) Mining() bool {
        m.mu.RLock()
        defer m.mu.RUnlock()
        return m.mining
}

// HashRate returns the current hashrate
func (m *Miner) HashRate() uint64 {
        if rx, ok := m.engine.(interface{ Hashrate() float64 }); ok {
                return uint64(rx.Hashrate())
        }
        return 0
}

// SetEtherbase sets the coinbase address
func (m *Miner) SetEtherbase(addr common.Address) {
        m.mu.Lock()
        defer m.mu.Unlock()
        m.coinbase = addr
        log.Info("Miner etherbase updated", "address", addr.Hex())
}

// SetExtra sets extra data for mined blocks
func (m *Miner) SetExtra(extra []byte) error {
        m.mu.Lock()
        defer m.mu.Unlock()
        if m.config != nil {
                m.config.ExtraData = extra
        }
        return nil
}

// SetEngine sets the consensus engine
func (m *Miner) SetEngine(engine consensus.Engine) {
        m.mu.Lock()
        defer m.mu.Unlock()
        m.engine = engine
}

// GetWork returns work for external miners (getwork RPC)
func (m *Miner) GetWork() ([4]string, error) {
        if rx, ok := m.engine.(interface{ GetWork() ([]string, error) }); ok {
                work, err := rx.GetWork()
                if err != nil {
                        return [4]string{}, err
                }
                var result [4]string
                copy(result[:], work)
                return result, nil
        }
        return [4]string{}, nil
}

// SubmitWork submits work from external miners
func (m *Miner) SubmitWork(nonce types.BlockNonce, hash common.Hash, digest common.Hash) bool {
        if rx, ok := m.engine.(interface{ SubmitWork(string, string, string) (bool, error) }); ok {
                nonceHex := common.Bytes2Hex(nonce[:])
                hashHex := hash.Hex()
                digestHex := digest.Hex()
                valid, err := rx.SubmitWork(nonceHex, hashHex, digestHex)
                if err != nil {
                        log.Error("SubmitWork failed", "error", err)
                        return false
                }
                return valid
        }
        return false
}

// Pending returns the pending block and state
func (m *Miner) Pending() (*types.Block, *state.StateDB) {
        if m.chain == nil {
                return nil, nil
        }

        currentHeader := m.chain.CurrentHeader()
        if currentHeader == nil {
                return nil, nil
        }

        // Create a pending block template
        header := &types.Header{
                ParentHash:  currentHeader.Hash(),
                Number:      new(big.Int).Add(currentHeader.Number, big.NewInt(1)),
                GasLimit:    m.config.GasLimit,
                Time:        currentHeader.Time + 1,
                Coinbase:    m.coinbase,
                Difficulty:  big.NewInt(1),
        }

        if m.engine != nil {
                header.Difficulty = m.engine.CalcDifficulty(nil, currentHeader.Time+1, currentHeader)
        }

        // Create empty block
        body := &types.Body{
                Transactions: []*types.Transaction{},
                Uncles:       []*types.Header{},
        }
        
        // Use nil hasher for block creation
        block := types.NewBlock(header, body, []*types.Receipt{}, nil)

        // Return block without state
        return block, nil
}
