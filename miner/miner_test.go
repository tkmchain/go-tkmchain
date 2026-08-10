// Copyright 2020 The go-ethereum Authors
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

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/clique"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/ethereum/go-ethereum/triedb"
)

type mockBackend struct {
	bc     *core.BlockChain
	txPool *txpool.TxPool
}

var minerTestTxPoolConfig = func() legacypool.Config {
	config := legacypool.DefaultConfig
	config.Journal = ""
	return config
}()

func NewMockBackend(bc *core.BlockChain, txPool *txpool.TxPool) *mockBackend {
	return &mockBackend{
		bc:     bc,
		txPool: txPool,
	}
}

func (m *mockBackend) BlockChain() *core.BlockChain {
	return m.bc
}

func (m *mockBackend) TxPool() *txpool.TxPool {
	return m.txPool
}

type testBlockChain struct {
	root          common.Hash
	config        *params.ChainConfig
	statedb       *state.StateDB
	gasLimit      uint64
	chainHeadFeed *event.Feed
}

func (bc *testBlockChain) Config() *params.ChainConfig {
	return bc.config
}

func (bc *testBlockChain) CurrentBlock() *types.Header {
	return &types.Header{
		Number:   new(big.Int),
		GasLimit: bc.gasLimit,
	}
}

func (bc *testBlockChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return types.NewBlock(bc.CurrentBlock(), nil, nil, trie.NewStackTrie(nil))
}

func (bc *testBlockChain) StateAt(header *types.Header) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *testBlockChain) Genesis() *types.Block {
	return types.NewBlock(bc.CurrentBlock(), nil, nil, trie.NewStackTrie(nil))
}

func (bc *testBlockChain) HasState(root common.Hash) bool {
	return bc.root == root
}

func (bc *testBlockChain) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func TestBuildPendingBlocks(t *testing.T) {
	miner := createMiner(t)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		block, _ := miner.Pending()
		if block == nil {
			t.Error("Pending failed")
		}
	}()
	wg.Wait()
}

func minerTestGenesisBlock(period uint64, gasLimit uint64, faucet common.Address) *core.Genesis {
	config := *params.AllCliqueProtocolChanges
	config.Clique = &params.CliqueConfig{
		Period: period,
		Epoch:  config.Clique.Epoch,
	}

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	return &core.Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, crypto.SignatureLength)...),
		GasLimit:   gasLimit,
		BaseFee:    big.NewInt(params.InitialBaseFee),
		Difficulty: big.NewInt(1),
		Alloc: map[common.Address]types.Account{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			common.BytesToAddress([]byte{9}): {Balance: big.NewInt(1)}, // BLAKE2b
			faucet:                           {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
}

func TestMakeCurrentUsesForkSigner(t *testing.T) {
	config := *params.RandomXChainConfig
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	from := crypto.PubkeyToAddress(key.PublicKey)
	to := common.HexToAddress("0x000000000000000000000000000000000000c0de")
	db := rawdb.NewMemoryDatabase()
	genesis := &core.Genesis{
		Config:   &config,
		GasLimit: 11_500_000,
		BaseFee:  big.NewInt(params.InitialBaseFee),
		Alloc: map[common.Address]types.Account{
			from: {Balance: new(big.Int).Lsh(big.NewInt(1), 128)},
		},
	}
	engine := clique.New(&params.CliqueConfig{Period: 15, Epoch: 30000}, db)
	bc, err := core.NewBlockChain(db, genesis, engine, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Stop()
	parentNumber := new(big.Int).Sub(config.RandomXTxBlock, common.Big1)
	parentHeader := &types.Header{Number: parentNumber, Root: bc.Genesis().Root(), GasLimit: 11_500_000}
	header := &types.Header{Number: new(big.Int).Set(config.RandomXTxBlock), GasLimit: 11_500_000, BaseFee: big.NewInt(params.InitialBaseFee)}
	w := &worker{
		chain:  bc,
		config: &config,
	}
	if err := w.makeCurrent(types.NewBlock(parentHeader, nil, nil, trie.NewStackTrie(nil)), header); err != nil {
		t.Fatal(err)
	}
	tx := types.MustSignNewTx(key, w.current.signer, &types.RandomXTx{
		ChainID:   config.ChainID,
		GasTipCap: big.NewInt(2),
		GasFeeCap: big.NewInt(params.InitialBaseFee + 2),
		Gas:       params.TxGas,
		To:        &to,
		Value:     big.NewInt(1),
	})
	sender, err := types.Sender(w.current.signer, tx)
	if err != nil {
		t.Fatalf("miner signer rejected randomx tx: %v", err)
	}
	if sender != from {
		t.Fatalf("sender = %v, want %v", sender, from)
	}
}

func createMiner(t *testing.T) *Miner {
	// Create miner config.
	config := DefaultConfig
	config.PendingFeeRecipient = common.HexToAddress("123456789")
	// Create chainConfig
	chainDB := rawdb.NewMemoryDatabase()
	triedb := triedb.NewDatabase(chainDB, nil)
	genesis := minerTestGenesisBlock(15, 11_500_000, common.HexToAddress("12345"))
	chainConfig, _, _, err := core.SetupGenesisBlock(chainDB, triedb, genesis)
	if err != nil {
		t.Fatalf("can't create new chain config: %v", err)
	}
	// Create consensus engine
	engine := clique.New(chainConfig.Clique, chainDB)
	// Create Ethereum backend
	bc, err := core.NewBlockChain(chainDB, genesis, engine, nil)
	if err != nil {
		t.Fatalf("can't create new chain %v", err)
	}
	statedb, _ := state.New(bc.Genesis().Root(), state.NewDatabase(bc.TrieDB(), bc.CodeDB()))
	blockchain := &testBlockChain{bc.Genesis().Root(), chainConfig, statedb, 10000000, new(event.Feed)}

	pool := legacypool.New(minerTestTxPoolConfig, blockchain)
	txpool, _ := txpool.New(minerTestTxPoolConfig.PriceLimit, blockchain, []txpool.SubPool{pool})

	// Create Miner
	backend := NewMockBackend(bc, txpool)
	miner := New(backend, chainConfig, new(event.TypeMux), engine, config.Recommit, config.GasFloor, config.GasCeil, nil)
	miner.SetEtherbase(config.PendingFeeRecipient)
	miner.worker.generateWorkForExternal()
	return miner
}
