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
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// MinerAPI provides an API to control the miner.
type MinerAPI struct {
	e *Ethereum
}

// NewMinerAPI creates a new MinerAPI instance.
func NewMinerAPI(e *Ethereum) *MinerAPI {
	return &MinerAPI{e}
}

// GetWork returns the current mining work package for external miners.
func (api *MinerAPI) GetWork() ([4]string, error) {
	return api.e.Miner().GetWork()
}

// SubmitWork submits a proof-of-work solution from an external miner.
func (api *MinerAPI) SubmitWork(nonce types.BlockNonce, hash common.Hash, digest common.Hash) bool {
	return api.e.Miner().SubmitWork(nonce, hash, digest)
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
