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

package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
)

// RandomXAPI provides RandomX-specific RPC methods for XMRig miners.
type RandomXAPI struct {
	e *Ethereum
}

// NewRandomXAPI creates a new RandomX API instance.
func NewRandomXAPI(e *Ethereum) *RandomXAPI {
	return &RandomXAPI{e: e}
}

// GetSeedHash returns the RandomX seed hash for the next block, or a specific block when provided.
// XMRig uses this to initialize its RandomX cache for the current epoch.
func (api *RandomXAPI) GetSeedHash(block *hexutil.Uint64) (common.Hash, error) {
	if api.e == nil || api.e.blockchain == nil {
		return common.Hash{}, errors.New("blockchain unavailable")
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return common.Hash{}, errors.New("latest block unavailable")
	}
	blockNumber := head.Number.Uint64() + 1
	if block != nil {
		blockNumber = uint64(*block)
	}

	seedHash := miner.RandomXSeedHash(api.e.blockchain.Config(), blockNumber)
	log.Debug("GetSeedHash called", "block", blockNumber, "seed", seedHash.Hex()[:16])

	return seedHash, nil
}

// GetSeedHashForBlock returns the RandomX seed hash for a specific block number.
func (api *RandomXAPI) GetSeedHashForBlock(block hexutil.Uint64) common.Hash {
	seedHash := miner.RandomXSeedHash(api.e.blockchain.Config(), uint64(block))
	log.Debug("GetSeedHashForBlock called", "block", uint64(block), "seed", seedHash.Hex()[:16])
	return seedHash
}

// GetWork returns the current mining work package for XMRig miners.
// Returns: [headerHash, seedHash, target, blockHeight]
// This matches what XMRig expects for RandomX mining.
func (api *RandomXAPI) GetWork() ([4]string, error) {
	if api.e.Miner() == nil {
		return [4]string{}, errors.New("miner not available")
	}

	work, err := api.e.Miner().GetWork()
	if err != nil {
		return [4]string{}, err
	}
	log.Debug("GetWork for XMRig",
		"headerHash", work[0][:16],
		"seedHash", work[1][:16],
		"target", work[2][:16],
		"height", work[3])
	return work, nil
}

// SubmitWork submits a RandomX solution from XMRig to the daemon.
// XMRig calls this with: nonce, headerHash, mixDigest
// The headerHash must match the current work's header hash.
func (api *RandomXAPI) SubmitWork(nonce types.BlockNonce, hash common.Hash, digest common.Hash) bool {
	log.Info("SubmitWork from XMRig",
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

	ok := api.e.Miner().SubmitWork(nonce, hash, digest)
	if ok {
		log.Info("Valid RandomX solution submitted by XMRig", "nonce", nonce, "height", work[3])
	} else {
		log.Warn("Invalid RandomX solution rejected", "nonce", nonce, "headerHash", hash.Hex()[:16])
	}
	return ok
}

// SubmitWorkRaw accepts hex strings directly from RPC calls.
// This is the method that XMRig will call via JSON-RPC.
func (api *RandomXAPI) SubmitWorkRaw(nonceHex, headerHashHex, mixDigestHex string) bool {
	log.Info("SubmitWorkRaw from XMRig",
		"nonce", nonceHex[:16],
		"headerHash", headerHashHex[:16],
		"mixDigest", mixDigestHex[:16])

	// Parse nonce
	var nonce types.BlockNonce
	nonceBytes := common.FromHex(nonceHex)
	if len(nonceBytes) >= 8 {
		copy(nonce[:], nonceBytes[:8])
	} else {
		log.Error("Invalid nonce length", "nonce", nonceHex)
		return false
	}

	// Parse header hash
	headerHash := common.HexToHash(headerHashHex)

	// Parse mix digest
	mixDigest := common.HexToHash(mixDigestHex)

	return api.SubmitWork(nonce, headerHash, mixDigest)
}

// GetCurrentHeight returns the current block height for XMRig reference.
func (api *RandomXAPI) GetCurrentHeight() (hexutil.Uint64, error) {
	if api.e == nil || api.e.blockchain == nil {
		return 0, errors.New("blockchain unavailable")
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return 0, errors.New("latest block unavailable")
	}
	return hexutil.Uint64(head.Number.Uint64()), nil
}

// GetHashrate returns the current hashrate for monitoring.
func (api *RandomXAPI) GetHashrate() hexutil.Uint64 {
	if api.e.Miner() == nil {
		return 0
	}
	return hexutil.Uint64(api.e.Miner().HashRate())
}
