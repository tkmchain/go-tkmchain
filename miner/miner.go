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
        "errors"
        "fmt"
        "time"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethereum/go-ethereum/common/hexutil"
        "github.com/ethereum/go-ethereum/consensus"
        "github.com/ethereum/go-ethereum/crypto"
        "github.com/ethereum/go-ethereum/core"
        "github.com/ethereum/go-ethereum/core/state"
        "github.com/ethereum/go-ethereum/core/txpool"
        "github.com/ethereum/go-ethereum/core/types"
        "github.com/ethereum/go-ethereum/event"
        "github.com/ethereum/go-ethereum/log"
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
        miner.worker.setExternalOnly(false)
        miner.worker.start()
}

// StartExternal begins work generation for external miners without local sealing.
func (miner *Miner) StartExternal(coinbase common.Address) {
        miner.SetEtherbase(coinbase)
        miner.worker.setExternalOnly(true)
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

// ========== METHODS FOR XMRig ==========

// GetWork returns the current mining work for external miners (XMRig).
// Returns: [headerHash, seedHash, target, blockHeight]
func (miner *Miner) GetWork() ([4]string, error) {
        // Get the current pending block
        block, state := miner.worker.pending()
        if block == nil || state == nil {
                return [4]string{}, errors.New("no pending work available")
        }

        header := block.Header()

        // Calculate the seed hash for the next block's epoch
        seedHash := RandomXSeedHash(miner.eth.BlockChain().Config(), header.Number.Uint64()+1)

        // Target is the difficulty threshold
        target := header.Difficulty

        // Block height for the next block
        height := header.Number.Uint64() + 1

        result := [4]string{
                header.Hash().Hex(),           // Header hash (for block verification)
                seedHash.Hex(),                 // Seed hash (for RandomX calculation)
                hexutil.EncodeBig(target),      // Target difficulty
                hexutil.EncodeUint64(height),   // Block height
        }

        log.Debug("GetWork for XMRig",
                "height", height,
                "headerHash", result[0][:16],
                "seedHash", result[1][:16],
                "target", result[2][:16])

        return result, nil
}

// SubmitWork submits a proof-of-work solution from an external miner (XMRig).
// Parameters: nonce, headerHash, mixDigest
func (miner *Miner) SubmitWork(nonce types.BlockNonce, hash common.Hash, digest common.Hash) bool {
        log.Info("SubmitWork from XMRig",
                "nonce", nonce,
                "headerHash", hash.Hex()[:16],
                "mixDigest", digest.Hex()[:16])

        // Get the current pending block to build a full header
        block, state := miner.worker.pending()
        if block == nil || state == nil {
                log.Error("No pending work available for submission")
                return false
        }

        header := block.Header()

        // Create a new header with the submitted nonce and mix digest
        newHeader := types.CopyHeader(header)
        newHeader.MixDigest = digest
        newHeader.Nonce = nonce

        // Create a new block with the sealed header using NewBlock (not NewBlockWithHeader)
        // NewBlock expects (header, body, receipts, trie)
        sealedBlock := types.NewBlock(newHeader, block.Body(), nil, nil)

        // Verify the header using the consensus engine's VerifyHeader method
        // (VerifySeal is part of VerifyHeader for RandomX)
        if err := miner.engine.VerifyHeader(miner.eth.BlockChain(), newHeader); err != nil {
                log.Warn("Invalid proof-of-work submitted", "err", err)
                return false
        }

        log.Info("Valid proof-of-work submitted, submitting block to result channel",
                "nonce", nonce,
                "blockNumber", sealedBlock.NumberU64(),
                "mixDigest", digest.Hex()[:16])

        // Send the sealed block to the worker's result channel
        select {
        case miner.worker.resultCh <- sealedBlock:
                log.Info("Block submitted to result channel successfully")
                return true
        case <-time.After(5 * time.Second):
                log.Warn("Timeout submitting block to result channel")
                return false
        }
}

// RandomXSeedHash calculates the RandomX seed hash for a given block height.
// For epoch 0, seed hash is all zeros. For later epochs, it's Keccak256(previous seed).
func RandomXSeedHash(config *params.ChainConfig, blockNumber uint64) common.Hash {
        epochLength := uint64(2048)
        epoch := blockNumber / epochLength

        // For epoch 0, seed hash is all zeros
        if epoch == 0 {
                return common.Hash{}
        }

        // Calculate seed hash by hashing the previous seed repeatedly
        seed := make([]byte, 32)
        for i := uint64(0); i < epoch; i++ {
                seed = crypto.Keccak256(seed)
        }
        return common.BytesToHash(seed)
}
