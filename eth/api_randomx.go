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
	"github.com/ethereum/go-ethereum/miner"
)

// RandomXAPI provides RandomX-specific RPC methods.
type RandomXAPI struct {
	e *Ethereum
}

// NewRandomXAPI creates a new RandomX API instance.
func NewRandomXAPI(e *Ethereum) *RandomXAPI {
	return &RandomXAPI{e: e}
}

// GetSeedHash returns the RandomX seed hash for the next block.
func (api *RandomXAPI) GetSeedHash() (common.Hash, error) {
	if api.e == nil || api.e.blockchain == nil {
		return common.Hash{}, errors.New("blockchain unavailable")
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return common.Hash{}, errors.New("latest block unavailable")
	}
	return miner.RandomXSeedHash(api.e.blockchain.Config(), head.Number.Uint64()+1), nil
}

// GetSeedHashForBlock returns the RandomX seed hash for a specific block number.
func (api *RandomXAPI) GetSeedHashForBlock(block hexutil.Uint64) common.Hash {
	return miner.RandomXSeedHash(api.e.blockchain.Config(), uint64(block))
}
