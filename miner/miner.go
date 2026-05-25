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
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
)

// Backend wraps all methods required for mining.
type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *txpool.TxPool
}

// Miner creates blocks and searches for proof-of-work values (RandomX).
type Miner struct {
	mux      *event.TypeMux
	worker   *worker // RandomX worker
	recommit time.Duration
	coinbase common.Address
	eth      Backend
	engine   consensus.Engine
	exitCh   chan struct{}
}

// New creates a new RandomX miner with the given configuration.
func New(eth Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine, recommit time.Duration, gasFloor, gasCeil uint64, isLocalBlock func(block *types.Block) bool) *Miner {
	miner := &Miner{
		eth:      eth,
		mux:      mux,
		recommit: recommit,
		engine:   engine,
		exitCh:   make(chan struct{}),
		worker:   newWorker(config, engine, eth, mux, recommit, gasFloor, gasCeil, isLocalBlock),
	}
	return miner
}

// Start begins the RandomX mining process.
func (miner *Miner) Start(coinbase common.Address) {
	miner.SetEtherbase(coinbase)
	miner.worker.start()
}

// Stop terminates the RandomX mining process.
func (miner *Miner) Stop() {
	miner.worker.stop()
}

// Close shuts down the miner and releases resources.
func (miner *Miner) Close() {
	miner.worker.close()
	close(miner.exitCh)
}

// Mining returns true if the miner is currently running.
func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}

// HashRate returns the current hashrate in hashes per second.
func (miner *Miner) HashRate() uint64 {
	return 0
}

// SetExtra sets the extra data field of the block header.
func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length: %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
}

// SetRecommitInterval sets the interval for re‑creating sealing work.
func (miner *Miner) SetRecommitInterval(interval time.Duration) {
	miner.recommit = interval
	miner.worker.setRecommitInterval(interval)
}

// Pending returns the currently pending block and its associated state.
func (miner *Miner) Pending() (*types.Block, *state.StateDB) {
	return miner.worker.pending()
}

// PendingBlock returns the currently pending block.
func (miner *Miner) PendingBlock() *types.Block {
	return miner.worker.pendingBlock()
}

// SetEtherbase sets the address that will receive mining rewards.
func (miner *Miner) SetEtherbase(addr common.Address) {
	miner.coinbase = addr
	miner.worker.setEtherbase(addr)
}
