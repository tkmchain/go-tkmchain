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

package miner

import (
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var errNoMiningWork = errors.New("no mining work available")

// GetWork returns the current mining work package for external miners.
func (miner *Miner) GetWork() ([4]string, error) {
	return miner.worker.getWork()
}

// SubmitWork submits a proof-of-work solution from an external miner.
func (miner *Miner) SubmitWork(nonce types.BlockNonce, hash common.Hash, digest common.Hash) bool {
	return miner.worker.submitWork(nonce, hash, digest)
}

func (w *worker) getWork() ([4]string, error) {
	var res [4]string

	w.pendingMu.RLock()
	var latest *task
	var sealHash common.Hash
	for hash, task := range w.pendingTasks {
		if latest == nil || task.block.NumberU64() > latest.block.NumberU64() || task.block.NumberU64() == latest.block.NumberU64() && task.createdAt.After(latest.createdAt) {
			latest = task
			sealHash = hash
		}
	}
	w.pendingMu.RUnlock()

	if latest == nil {
		return res, errNoMiningWork
	}
	header := latest.block.Header()
	if header.Difficulty == nil || header.Difficulty.Sign() <= 0 {
		return res, errors.New("mining work has invalid difficulty")
	}
	target := new(big.Int).Div(new(big.Int).Lsh(common.Big1, 256), header.Difficulty)

	res[0] = sealHash.Hex()
	res[1] = randomXSeedHash(w.config, header.Number.Uint64()).Hex()
	res[2] = common.BytesToHash(target.Bytes()).Hex()
	res[3] = common.BytesToHash(new(big.Int).SetUint64(header.Number.Uint64()).Bytes()).Hex()
	return res, nil
}

func (w *worker) submitWork(nonce types.BlockNonce, hash common.Hash, digest common.Hash) bool {
	w.pendingMu.RLock()
	task, exist := w.pendingTasks[hash]
	w.pendingMu.RUnlock()
	if !exist {
		log.Warn("Work submitted for stale or unknown mining work", "sealhash", hash)
		return false
	}

	header := types.CopyHeader(task.block.Header())
	header.Nonce = nonce
	header.MixDigest = digest

	verifier, ok := w.engine.(interface {
		VerifySeal(consensus.ChainHeaderReader, *types.Header) error
	})
	if ok {
		if err := verifier.VerifySeal(w.chain, header); err != nil {
			log.Warn("Invalid proof-of-work submitted", "sealhash", hash, "err", err)
			return false
		}
	}

	select {
	case w.resultCh <- task.block.WithSeal(header):
		return true
	case <-w.exitCh:
		return false
	case <-time.After(5 * time.Second):
		log.Warn("Timeout submitting external proof-of-work", "sealhash", hash)
		return false
	}
}

func randomXSeedHash(config *params.ChainConfig, block uint64) common.Hash {
	epochLength := uint64(2048)
	if config != nil && config.RandomX != nil && config.RandomX.EpochLength != 0 {
		epochLength = config.RandomX.EpochLength
	}
	epoch := block / epochLength
	seed := common.Hash{}
	for i := uint64(0); i < epoch; i++ {
		seed = crypto.Keccak256Hash(seed.Bytes())
	}
	return seed
}
