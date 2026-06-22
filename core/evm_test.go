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

package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type evmTestChainContext struct {
	config *params.ChainConfig
	engine consensus.Engine
}

func (ctx evmTestChainContext) Config() *params.ChainConfig                 { return ctx.config }
func (ctx evmTestChainContext) CurrentHeader() *types.Header                { return nil }
func (ctx evmTestChainContext) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (ctx evmTestChainContext) GetHeaderByNumber(uint64) *types.Header      { return nil }
func (ctx evmTestChainContext) GetHeaderByHash(common.Hash) *types.Header   { return nil }
func (ctx evmTestChainContext) Engine() consensus.Engine                    { return ctx.engine }

func TestNewEVMBlockContextPreCancunExcessBlobGas(t *testing.T) {
	excessBlobGas := uint64(0)
	config := *params.TestChainConfig
	header := &types.Header{
		Number:        big.NewInt(1),
		Difficulty:    big.NewInt(1),
		ExcessBlobGas: &excessBlobGas,
	}
	chain := evmTestChainContext{
		config: &config,
	}
	ctx := NewEVMBlockContext(header, chain, &common.Address{})
	if ctx.BlobBaseFee != nil {
		t.Fatalf("unexpected blob base fee before Cancun: %v", ctx.BlobBaseFee)
	}
}
