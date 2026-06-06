// Copyright 2023 The go-ethereum Authors
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

package eth

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
)

// MinerAPI provides an API to control the miner and handle external miners (XMRig)
type MinerAPI struct {
	e *Ethereum
}

// NewMinerAPI creates a new MinerAPI instance.
func NewMinerAPI(e *Ethereum) *MinerAPI {
	return &MinerAPI{e: e}
}

// GetSeedHash returns the RandomX seed hash for the next block.
// This is used by XMRig miners to initialize their RandomX cache
func (api *MinerAPI) GetSeedHash() (common.Hash, error) {
	if api.e == nil || api.e.blockchain == nil {
		return common.Hash{}, errors.New("blockchain unavailable")
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return common.Hash{}, errors.New("latest block unavailable")
	}

	// Calculate seed hash for the next block (current height + 1)
	seedHash := miner.RandomXSeedHash(api.e.blockchain.Config(), head.Number.Uint64()+1)

	log.Debug("GetSeedHash called", "height", head.Number.Uint64()+1, "seed", seedHash.Hex()[:16])

	return seedHash, nil
}

// GetWork returns the current mining work package for external miners (XMRig).
// Returns: [headerHash, seedHash, target, blockHeight]
func (api *MinerAPI) GetWork() ([4]string, error) {
	if api.e.Miner() == nil {
		return [4]string{}, errors.New("miner not available")
	}

	work, err := api.e.Miner().GetWork()
	if err != nil {
		return [4]string{}, err
	}
	log.Debug("GetWork response for XMRig",
		"headerHash", work[0][:16],
		"seedHash", work[1][:16],
		"target", work[2][:16],
		"height", work[3])
	return work, nil
}

// SubmitWork submits a proof-of-work solution from an external miner (XMRig).
// XMRig calls this with: nonce, headerHash, mixDigest
// But we need to adapt to our RandomX consensus
func (api *MinerAPI) SubmitWork(nonce types.BlockNonce, hash common.Hash, digest common.Hash) bool {
	log.Info("SubmitWork called by external miner",
		"nonce", nonce,
		"headerHash", hash.Hex()[:16],
		"mixDigest", digest.Hex()[:16])

	if api.e.Miner() == nil {
		log.Error("Miner not available for submission")
		return false
	}

	work, err := api.e.Miner().GetWork()
	if err != nil {
		log.Error("No current work available for submission", "err", err)
		return false
	}
	if hash != common.HexToHash(work[0]) {
		log.Warn("Header hash mismatch",
			"expected", work[0][:16],
			"got", hash.Hex()[:16])
		return false
	}

	return api.e.Miner().SubmitWork(nonce, hash, digest)
}

// SubmitWorkRaw is an alternative version that accepts hex strings directly
// Useful for RPC calls from XMRig
func (api *MinerAPI) SubmitWorkRaw(nonceHex, headerHashHex, mixDigestHex string) bool {
	// Parse nonce
	var nonce types.BlockNonce
	nonceBytes := common.FromHex(nonceHex)
	if len(nonceBytes) >= 8 {
		copy(nonce[:], nonceBytes[:8])
	}

	// Parse header hash
	headerHash := common.HexToHash(headerHashHex)

	// Parse mix digest
	mixDigest := common.HexToHash(mixDigestHex)

	return api.SubmitWork(nonce, headerHash, mixDigest)
}

// SetExtra sets the extra data string that is included when this miner mines a block.
func (api *MinerAPI) SetExtra(extra string) (bool, error) {
	if err := api.e.Miner().SetExtra([]byte(extra)); err != nil {
		return false, err
	}
	return true, nil
}

// SetGasPrice sets the minimum accepted gas price for the miner.
func (api *MinerAPI) SetGasPrice(gasPrice hexutil.Big) bool {
	api.e.lock.Lock()
	api.e.gasPrice = (*big.Int)(&gasPrice)
	api.e.lock.Unlock()

	api.e.txPool.SetGasTip((*big.Int)(&gasPrice))
	return true
}

// SetGasLimit sets the gaslimit to target towards during mining.
func (api *MinerAPI) SetGasLimit(gasLimit hexutil.Uint64) bool {
	api.e.config.Miner.GasCeil = uint64(gasLimit)
	return true
}

// SetEtherbase sets the etherbase to receive mining rewards.
func (api *MinerAPI) SetEtherbase(etherbase common.Address) (bool, error) {
	if err := api.e.SetMinerEtherbase(etherbase); err != nil {
		return false, err
	}
	return true, nil
}
