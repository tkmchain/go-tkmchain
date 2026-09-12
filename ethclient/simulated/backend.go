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

package simulated

import (
	"bytes"
	"errors"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

// Client exposes the methods provided by the Ethereum RPC client.
type Client interface {
	ethereum.BlockNumberReader
	ethereum.ChainReader
	ethereum.ChainStateReader
	ethereum.ContractCaller
	ethereum.GasEstimator
	ethereum.GasPricer
	ethereum.GasPricer1559
	ethereum.FeeHistoryReader
	ethereum.LogFilterer
	ethereum.PendingStateReader
	ethereum.PendingContractCaller
	ethereum.TransactionReader
	ethereum.TransactionSender
	ethereum.ChainIDReader
}

// simClient wraps ethclient. This exists to prevent extracting ethclient.Client
// from the Client interface returned by Backend.
type simClient struct {
	*ethclient.Client
}

// Backend is a simulated blockchain. You can use it to test your contracts or
// other code that interacts with the Ethereum chain.
type Backend struct {
	node     *node.Node
	backend  *eth.Ethereum
	client   simClient
	mu       sync.Mutex
	parent   *types.Block
	sequence uint64
}

// NewBackend creates a new simulated blockchain that can be used as a backend for
// contract bindings in unit tests.
//
// A simulated backend always uses chainID 1337.
func NewBackend(alloc types.GenesisAlloc, options ...func(nodeConf *node.Config, ethConf *ethconfig.Config)) *Backend {
	// Create the default configurations for the outer node shell and the Ethereum
	// service to mutate with the options afterwards
	nodeConf := node.DefaultConfig
	nodeConf.DataDir = ""
	nodeConf.IPCPath = ""
	nodeConf.HTTPHost = ""
	nodeConf.WSHost = ""
	nodeConf.P2P = p2p.Config{NoDiscovery: true}

	chainConfig := *params.AllDevChainProtocolChanges
	chainConfig.ChainID = big.NewInt(1337)
	chainConfig.ShanghaiTime = new(uint64)
	chainConfig.CancunTime = new(uint64)
	chainConfig.MainKingAddress = common.Address{}
	chainConfig.RandomX = nil
	ethConf := ethconfig.Defaults
	ethConf.StateScheme = rawdb.HashScheme
	ethConf.NoPruning = true
	ethConf.SnapshotCache = 0
	ethConf.Genesis = &core.Genesis{
		Config:   &chainConfig,
		GasLimit: ethconfig.Defaults.Miner.GasCeil,
		Alloc:    alloc,
	}
	ethConf.SyncMode = ethconfig.FullSync
	ethConf.TxPool.NoLocals = true
	// Disable log indexing to force unindexed log search
	ethConf.LogNoHistory = true

	for _, option := range options {
		option(&nodeConf, &ethConf)
	}
	// Assemble the Ethereum stack to run the chain with
	stack, err := node.New(&nodeConf)
	if err != nil {
		panic(err) // this should never happen
	}
	sim, err := newWithNode(stack, &ethConf, 0)
	if err != nil {
		panic(err) // this should never happen
	}
	return sim
}

// newWithNode sets up a simulated backend on an existing node. The provided node
// must not be started and will be started by this method.
func newWithNode(stack *node.Node, conf *eth.Config, blockPeriod uint64) (*Backend, error) {
	backend, err := eth.NewSimulated(stack, conf, &simulatedEngine{randomx.NewFaker()})
	if err != nil {
		return nil, err
	}
	// The simulator builds pending blocks synchronously; the live mining worker is not used.
	backend.Miner().Close()
	// Register the filter system
	filterSystem := filters.NewFilterSystem(backend.APIBackend, filters.Config{})
	stack.RegisterAPIs([]rpc.API{{
		Namespace: "eth",
		Service:   filters.NewFilterAPI(filterSystem),
	}})
	sim := &Backend{node: stack, backend: backend, client: simClient{ethclient.NewClient(stack.Attach())}, parent: backend.BlockChain().Genesis()}
	backend.Miner().SetPendingProvider(sim.pending)
	if err := stack.Start(); err != nil {
		return nil, err
	}
	return sim, nil
}

// Close shuts down the simBackend.
// The simulated backend can't be used afterwards.
func (n *Backend) Close() error {
	if n.client.Client != nil {
		n.client.Close()
		n.client = simClient{}
	}
	var err error
	if n.node != nil {
		err = errors.Join(err, n.node.Close())
		n.node = nil
	}
	n.backend = nil
	return err
}

// Commit seals a block and moves the chain forward to a new empty block.
func (n *Backend) Commit() common.Hash {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.commit(1)
}

func (n *Backend) commit(seconds int64) common.Hash {
	block, _ := n.build(seconds)
	if _, err := n.backend.BlockChain().InsertChain(types.Blocks{block}); err != nil {
		panic(err)
	}
	n.parent = block
	n.sequence++
	if err := n.backend.TxPool().Sync(); err != nil {
		panic(err)
	}
	return block.Hash()
}

func (n *Backend) pending() (*types.Block, *state.StateDB) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.build(1)
}

func (n *Backend) build(seconds int64) (*types.Block, *state.StateDB) {
	pool := n.backend.TxPool()
	if err := pool.Sync(); err != nil {
		panic(err)
	}
	txs, _ := pool.Pending(txpool.PendingFilter{})
	blobs, _ := pool.Pending(txpool.PendingFilter{BlobTxs: true})
	for addr, list := range blobs {
		txs[addr] = list
	}
	addresses := make([]common.Address, 0, len(txs))
	for addr := range txs {
		addresses = append(addresses, addr)
	}
	slices.SortFunc(addresses, func(a, b common.Address) int { return bytes.Compare(a[:], b[:]) })
	blocks, _ := core.GenerateChain(n.backend.BlockChain().Config(), n.parent, n.backend.Engine(), n.backend.ChainDb(), 1, func(_ int, gen *core.BlockGen) {
		gen.OffsetTime(seconds - 10)
		gen.SetExtra(new(big.Int).SetUint64(n.sequence).Bytes())
		for _, addr := range addresses {
			for _, lazy := range txs[addr] {
				tx := lazy.Resolve()
				if tx == nil || tx.Nonce() < gen.TxNonce(addr) {
					continue
				}
				if tx.Nonce() != gen.TxNonce(addr) || tx.Gas() > gen.Gas() {
					break
				}
				gen.AddTx(tx.WithoutBlobTxSidecar())
			}
		}
	})
	st, err := n.backend.BlockChain().StateAt(blocks[0].Header())
	if err != nil {
		panic(err)
	}
	return blocks[0], st
}

// Rollback drops pending transactions without advancing the chain.
func (n *Backend) Rollback() { n.mu.Lock(); defer n.mu.Unlock(); n.backend.TxPool().Clear() }

// Fork creates a side-chain that can be used to simulate reorgs.
//
// This function should be called with the ancestor block where the new side
// chain should be started. Transactions (old and new) can then be applied on
// top and Commit-ed.
//
// Note, the side-chain will only become canonical (and trigger the events) when
// it becomes longer. Until then CallContract will still operate on the current
// canonical chain.
//
// There is a % chance that the side chain becomes canonical at the same length
// to simulate live network behavior.
func (n *Backend) Fork(parentHash common.Hash) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	parent := n.backend.BlockChain().GetBlockByHash(parentHash)
	if parent == nil {
		return errors.New("fork parent not found")
	}
	if _, count := n.backend.TxPool().Pending(txpool.PendingFilter{}); count != 0 {
		return errors.New("pending transactions must be committed or rolled back before forking")
	}
	if _, err := n.backend.BlockChain().SetCanonical(parent); err != nil {
		return err
	}
	n.parent = parent
	return n.backend.TxPool().Sync()
}

// AdjustTime changes the block timestamp and creates a new block.
// It can only be called on empty blocks.
func (n *Backend) AdjustTime(adjustment time.Duration) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if adjustment < time.Second {
		return errors.New("time adjustment must advance by at least one second")
	}
	if _, count := n.backend.TxPool().Pending(txpool.PendingFilter{}); count != 0 {
		return errors.New("cannot adjust time with pending transactions")
	}
	n.commit(int64(adjustment / time.Second))
	return nil
}

// Client returns a client that accesses the simulated chain.
func (n *Backend) Client() Client {
	return n.client
}
