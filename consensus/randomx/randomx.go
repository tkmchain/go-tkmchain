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

// Package randomx implements the RandomX proof-of-work consensus engine.
package randomx

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/types"
)

// RandomX is a proof-of-work consensus engine for RandomX-configured chains.
type RandomX struct {
	*ethash.Ethash
}

// NewFaker creates a RandomX consensus engine with fake PoW verification. The
// engine accepts all blocks' seals as valid while still enforcing the Ethereum
// proof-of-work consensus rules shared with ethash.
func NewFaker() *RandomX {
	return &RandomX{Ethash: ethash.NewFaker()}
}

// Seal generates a new sealing request for the given input block and pushes the
// result into the given channel.
func (randomx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	panic("randomx (pow) sealing not supported yet")
}
