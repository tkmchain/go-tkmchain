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

package randomx

import (
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

var(

 MinimumDifficulty      = big.NewInt(1310)
)
// RandomX is the RandomX proof-of-work consensus engine.
type RandomX struct {
	config           *params.RandomXConfig
	threads          int
	mainKing         common.Address
	rotatingKings    []common.Address
	rotationInterval uint64
	fakeFail         *uint64
	fullFake         bool
	lock             sync.RWMutex
}

// New creates a RandomX proof-of-work consensus engine.
func New(config *params.RandomXConfig, threads int, mainKing common.Address, rotatingKings []common.Address) (*RandomX, error) {
	if config == nil {
		config = params.DefaultRandomXConfig()
	}
	if threads <= 0 {
		threads = 1
	}
	engine := &RandomX{
		config:        config,
		threads:       threads,
		mainKing:      mainKing,
		rotatingKings: append([]common.Address(nil), rotatingKings...),
	}
	return engine, nil
}

// NewFaker creates a RandomX engine that accepts all seals.
func NewFaker() *RandomX {
	engine, _ := New(params.DefaultRandomXConfig(), 1, common.Address{}, nil)
	return engine
}

// NewFullFaker creates a RandomX engine that accepts all seals and runs full fake mode.
func NewFullFaker() *RandomX {
	engine := NewFaker()
	engine.fullFake = true
	return engine
}

// NewFakeFailer creates a RandomX engine that rejects one configured block number.
func NewFakeFailer(number uint64) *RandomX {
	engine := NewFaker()
	engine.fakeFail = &number
	return engine
}

// Author implements consensus.Engine, returning the block coinbase as author.
func (rx *RandomX) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader implements consensus.Engine.
func (rx *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
	if rx.fakeFail != nil && header.Number.Uint64() == *rx.fakeFail {
		return consensus.ErrInvalidNumber
	}
	if header.Number.Sign() == 0 {
		return nil
	}
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	if parent.Number.Uint64()+1 != header.Number.Uint64() {
		return consensus.ErrInvalidNumber
	}
	return nil
}

// VerifyHeaders implements consensus.Engine.
func (rx *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for _, header := range headers {
			select {
			case <-abort:
				return
			case results <- rx.VerifyHeader(chain, header):
			}
		}
	}()
	return abort, results
}

// VerifyUncles implements consensus.Engine.
func (rx *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return consensus.ErrUnknownAncestor
	}
	return nil
}

// Prepare implements consensus.Engine.
func (rx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	if header.Number == nil {
		header.Number = new(big.Int)
	}
	if header.Difficulty == nil {
		if parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1); parent != nil {
			header.Difficulty = rx.CalcDifficulty(chain, header.Time, parent)
		} else {
			header.Difficulty = new(big.Int).SetUint64(MinimumDifficulty)
		}
	}
	return nil
}

// Finalize implements consensus.Engine.
func (rx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {
}

// Seal implements consensus.Engine.
func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	select {
	case results <- block:
	case <-stop:
	}
	return nil
}

// SealHash implements consensus.Engine.
func (rx *RandomX) SealHash(header *types.Header) common.Hash {
	return header.Hash()
}

// CalcDifficulty implements consensus.Engine.
func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return CalcDifficulty(chain.Config(), time, parent, func(number uint64) *types.Header {
		return chain.GetHeaderByNumber(number)
	})
}

// Close implements consensus.Engine.
func (rx *RandomX) Close() error {
	return nil
}

// SetRotationInterval configures the rotating king rotation interval.
func (rx *RandomX) SetRotationInterval(interval uint64) {
	rx.lock.Lock()
	defer rx.lock.Unlock()
	rx.rotationInterval = interval
}

// AddRotatingKing adds a rotating king address to the engine.
func (rx *RandomX) AddRotatingKing(address common.Address) {
	rx.AddRotatingKingAt(address, 0)
}

// AddRotatingKingAt adds a rotating king address to the engine at the given activation height.
func (rx *RandomX) AddRotatingKingAt(address common.Address, activationHeight uint64) {
	rx.lock.Lock()
	defer rx.lock.Unlock()
	for _, existing := range rx.rotatingKings {
		if existing == address {
			return
		}
	}
	rx.rotatingKings = append(rx.rotatingKings, address)
}
