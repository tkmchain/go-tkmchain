// Copyright 2017 The go-ethereum Authors
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
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// RandomX is a consensus engine based on proof-of-work implementing the randomx
// algorithm.
type RandomX struct {
	fakeFail  *uint64        // Block number which fails PoW check even in fake mode
	fakeDelay *time.Duration // Time delay to sleep for before returning from verify
	fakeFull  bool           // Accepts everything as valid
}

// New creates a RandomX consensus engine.
func New(config *params.RandomXConfig, threads int) *RandomX {
	return NewFaker()
}

// NewFaker creates an randomx consensus engine with a fake PoW scheme that accepts
// all blocks' seal as valid, though they still have to conform to the Ethereum
// consensus rules.
func NewFaker() *RandomX {
	return new(RandomX)
}

// NewFakeFailer creates a randomx consensus engine with a fake PoW scheme that
// accepts all blocks as valid apart from the single one specified, though they
// still have to conform to the Ethereum consensus rules.
func NewFakeFailer(fail uint64) *RandomX {
	return &RandomX{
		fakeFail: &fail,
	}
}

// NewFakeDelayer creates a randomx consensus engine with a fake PoW scheme that
// accepts all blocks as valid, but delays verifications by some time, though
// they still have to conform to the Ethereum consensus rules.
func NewFakeDelayer(delay time.Duration) *RandomX {
	return &RandomX{
		fakeDelay: &delay,
	}
}

// NewFullFaker creates an randomx consensus engine with a full fake scheme that
// accepts all blocks as valid, without checking any consensus rules whatsoever.
func NewFullFaker() *RandomX {
	return &RandomX{
		fakeFull: true,
	}
}

// Close closes the exit channel to notify all backend threads exiting.
func (randomx *RandomX) Close() error {
	return nil
}

// Seal generates a new sealing request for the given input block and pushes
// the result into the given channel. For the randomx engine, this method will
// just panic as sealing is not supported anymore.
func (randomx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	panic("randomx (pow) sealing not supported any more")
}
