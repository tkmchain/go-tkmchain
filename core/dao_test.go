// Copyright 2016 The go-ethereum Authors
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

	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/params"
)

// Tests that DAO-fork enabled clients can properly filter out fork-commencing
// blocks based on their extradata fields.
func TestDAOForkRangeExtradata(t *testing.T) {
	forkBlock := big.NewInt(32)
	chainConfig := *params.TestChainConfig
	chainConfig.HomesteadBlock = big.NewInt(0)
	chainConfig.EIP150Block = new(big.Int).Set(forkBlock)
	chainConfig.EIP155Block = new(big.Int).Set(forkBlock)
	chainConfig.EIP158Block = new(big.Int).Set(forkBlock)
	chainConfig.ByzantiumBlock = new(big.Int).Set(forkBlock)
	chainConfig.ConstantinopleBlock = new(big.Int).Set(forkBlock)
	chainConfig.PetersburgBlock = new(big.Int).Set(forkBlock)
	chainConfig.IstanbulBlock = new(big.Int).Set(forkBlock)
	chainConfig.BerlinBlock = new(big.Int).Set(forkBlock)
	chainConfig.LondonBlock = new(big.Int).Set(forkBlock)

	// Generate a common prefix for both pro-forkers and non-forkers
	gspec := &Genesis{
		BaseFee: big.NewInt(params.InitialBaseFee),
		Config:  &chainConfig,
	}
	genDb, prefix, _ := GenerateChainWithGenesis(gspec, randomx.NewFaker(), int(forkBlock.Int64()-1), func(i int, gen *BlockGen) {})

	// Create the concurrent, conflicting two nodes
	proDb := rawdb.NewMemoryDatabase()
	proConf := *params.TestChainConfig
	proConf.HomesteadBlock = big.NewInt(0)
	// The current protocol configuration validates fork ordering strictly. Keep
	// the synthetic DAO fork at the same height as the subsequent block forks
	// so this legacy extra-data test remains a valid chain configuration.
	proConf.EIP150Block = new(big.Int).Set(forkBlock)
	proConf.EIP155Block = new(big.Int).Set(forkBlock)
	proConf.EIP158Block = new(big.Int).Set(forkBlock)
	proConf.ByzantiumBlock = new(big.Int).Set(forkBlock)
	proConf.ConstantinopleBlock = new(big.Int).Set(forkBlock)
	proConf.PetersburgBlock = new(big.Int).Set(forkBlock)
	proConf.IstanbulBlock = new(big.Int).Set(forkBlock)
	proConf.BerlinBlock = new(big.Int).Set(forkBlock)
	proConf.LondonBlock = new(big.Int).Set(forkBlock)
	proConf.DAOForkBlock = forkBlock
	proConf.DAOForkSupport = true
	progspec := &Genesis{
		BaseFee: big.NewInt(params.InitialBaseFee),
		Config:  &proConf,
	}
	proBc, err := NewBlockChain(proDb, progspec, randomx.NewFaker(), nil)
	if err != nil {
		t.Fatalf("pro-fork: failed to create blockchain: %v", err)
	}
	defer stopCanonicalFixture(t, proBc)

	conDb := rawdb.NewMemoryDatabase()
	conConf := *params.TestChainConfig
	conConf.HomesteadBlock = big.NewInt(0)
	conConf.EIP150Block = new(big.Int).Set(forkBlock)
	conConf.EIP155Block = new(big.Int).Set(forkBlock)
	conConf.EIP158Block = new(big.Int).Set(forkBlock)
	conConf.ByzantiumBlock = new(big.Int).Set(forkBlock)
	conConf.ConstantinopleBlock = new(big.Int).Set(forkBlock)
	conConf.PetersburgBlock = new(big.Int).Set(forkBlock)
	conConf.IstanbulBlock = new(big.Int).Set(forkBlock)
	conConf.BerlinBlock = new(big.Int).Set(forkBlock)
	conConf.LondonBlock = new(big.Int).Set(forkBlock)
	conConf.DAOForkBlock = forkBlock
	conConf.DAOForkSupport = false
	congspec := &Genesis{
		BaseFee: big.NewInt(params.InitialBaseFee),
		Config:  &conConf,
	}
	conBc, err := NewBlockChain(conDb, congspec, randomx.NewFaker(), nil)
	if err != nil {
		t.Fatalf("contra-fork: failed to create blockchain: %v", err)
	}
	defer stopCanonicalFixture(t, conBc)

	if _, err := proBc.InsertChain(prefix); err != nil {
		t.Fatalf("pro-fork: failed to import chain prefix: %v", err)
	}
	if _, err := conBc.InsertChain(prefix); err != nil {
		t.Fatalf("con-fork: failed to import chain prefix: %v", err)
	}
	// Try to expand both pro-fork and non-fork chains iteratively with other camp's blocks
	for i := int64(0); i < params.DAOForkExtraRange.Int64(); i++ {
		// Create a pro-fork block, and try to feed into the no-fork chain
		bc, _ := NewBlockChain(rawdb.NewMemoryDatabase(), congspec, randomx.NewFaker(), nil)

		blocks := conBc.GetBlocksFromHash(conBc.CurrentBlock().Hash(), int(conBc.CurrentBlock().Number.Uint64()))
		for j := 0; j < len(blocks)/2; j++ {
			blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
		}
		if _, err := bc.InsertChain(blocks); err != nil {
			t.Fatalf("failed to import contra-fork chain for expansion: %v", err)
		}
		if err := bc.triedb.Commit(bc.CurrentHeader().Root, false); err != nil {
			t.Fatalf("failed to commit contra-fork head for expansion: %v", err)
		}
		stopCanonicalFixture(t, bc)
		blocks, _ = GenerateChain(&proConf, conBc.GetBlockByHash(conBc.CurrentBlock().Hash()), randomx.NewFaker(), genDb, 1, func(i int, gen *BlockGen) {})
		if _, err := conBc.InsertChain(blocks); err == nil {
			t.Fatalf("contra-fork chain accepted pro-fork block: %v", blocks[0])
		}
		// Create a proper no-fork block for the contra-forker
		blocks, _ = GenerateChain(&conConf, conBc.GetBlockByHash(conBc.CurrentBlock().Hash()), randomx.NewFaker(), genDb, 1, func(i int, gen *BlockGen) {})
		if _, err := conBc.InsertChain(blocks); err != nil {
			t.Fatalf("contra-fork chain didn't accepted no-fork block: %v", err)
		}
		// Create a no-fork block, and try to feed into the pro-fork chain
		bc, _ = NewBlockChain(rawdb.NewMemoryDatabase(), progspec, randomx.NewFaker(), nil)

		blocks = proBc.GetBlocksFromHash(proBc.CurrentBlock().Hash(), int(proBc.CurrentBlock().Number.Uint64()))
		for j := 0; j < len(blocks)/2; j++ {
			blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
		}
		if _, err := bc.InsertChain(blocks); err != nil {
			t.Fatalf("failed to import pro-fork chain for expansion: %v", err)
		}
		if err := bc.triedb.Commit(bc.CurrentHeader().Root, false); err != nil {
			t.Fatalf("failed to commit pro-fork head for expansion: %v", err)
		}
		stopCanonicalFixture(t, bc)
		blocks, _ = GenerateChain(&conConf, proBc.GetBlockByHash(proBc.CurrentBlock().Hash()), randomx.NewFaker(), genDb, 1, func(i int, gen *BlockGen) {})
		if _, err := proBc.InsertChain(blocks); err == nil {
			t.Fatalf("pro-fork chain accepted contra-fork block: %v", blocks[0])
		}
		// Create a proper pro-fork block for the pro-forker
		blocks, _ = GenerateChain(&proConf, proBc.GetBlockByHash(proBc.CurrentBlock().Hash()), randomx.NewFaker(), genDb, 1, func(i int, gen *BlockGen) {})
		if _, err := proBc.InsertChain(blocks); err != nil {
			t.Fatalf("pro-fork chain didn't accepted pro-fork block: %v", err)
		}
	}
	// Verify that contra-forkers accept pro-fork extra-datas after forking finishes
	bc, _ := NewBlockChain(rawdb.NewMemoryDatabase(), congspec, randomx.NewFaker(), nil)
	defer stopCanonicalFixture(t, bc)

	blocks := conBc.GetBlocksFromHash(conBc.CurrentBlock().Hash(), int(conBc.CurrentBlock().Number.Uint64()))
	for j := 0; j < len(blocks)/2; j++ {
		blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatalf("failed to import contra-fork chain for expansion: %v", err)
	}
	if err := bc.triedb.Commit(bc.CurrentHeader().Root, false); err != nil {
		t.Fatalf("failed to commit contra-fork head for expansion: %v", err)
	}
	blocks, _ = GenerateChain(&proConf, conBc.GetBlockByHash(conBc.CurrentBlock().Hash()), randomx.NewFaker(), genDb, 1, func(i int, gen *BlockGen) {})
	if _, err := conBc.InsertChain(blocks); err != nil {
		t.Fatalf("contra-fork chain didn't accept pro-fork block post-fork: %v", err)
	}
	// Verify that pro-forkers accept contra-fork extra-datas after forking finishes
	bc, _ = NewBlockChain(rawdb.NewMemoryDatabase(), progspec, randomx.NewFaker(), nil)
	defer stopCanonicalFixture(t, bc)

	blocks = proBc.GetBlocksFromHash(proBc.CurrentBlock().Hash(), int(proBc.CurrentBlock().Number.Uint64()))
	for j := 0; j < len(blocks)/2; j++ {
		blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatalf("failed to import pro-fork chain for expansion: %v", err)
	}
	if err := bc.triedb.Commit(bc.CurrentHeader().Root, false); err != nil {
		t.Fatalf("failed to commit pro-fork head for expansion: %v", err)
	}
	blocks, _ = GenerateChain(&conConf, proBc.GetBlockByHash(proBc.CurrentBlock().Hash()), randomx.NewFaker(), genDb, 1, func(i int, gen *BlockGen) {})
	if _, err := proBc.InsertChain(blocks); err != nil {
		t.Fatalf("pro-fork chain didn't accept contra-fork block post-fork: %v", err)
	}
}
