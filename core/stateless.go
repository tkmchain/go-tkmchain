// Copyright 2024 The go-ethereum Authors
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
        "context"
        "fmt"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethereum/go-ethereum/common/lru"
        "github.com/ethereum/go-ethereum/consensus/randomx"
        "github.com/ethereum/go-ethereum/core/state"
        "github.com/ethereum/go-ethereum/core/stateless"
        "github.com/ethereum/go-ethereum/ethdb"
        "github.com/ethereum/go-ethereum/core/types"
        "github.com/ethereum/go-ethereum/core/vm"
        "github.com/ethereum/go-ethereum/log"
        "github.com/ethereum/go-ethereum/params"
        "github.com/ethereum/go-ethereum/trie"
        "github.com/ethereum/go-ethereum/triedb"
)

// ExecuteStateless runs a stateless execution based on a witness, verifies
// everything it can locally and returns the state root and receipt root, that
// need the other side to explicitly check.
func ExecuteStateless(ctx context.Context, config *params.ChainConfig, vmconfig vm.Config, block *types.Block, witness *stateless.Witness) (common.Hash, common.Hash, error) {
        // Sanity check if the supplied block accidentally contains a set root or
        // receipt hash.
        if block.Root() != (common.Hash{}) {
                log.Error("stateless runner received state root it's expected to calculate (faulty consensus client)", "block", block.Number())
        }
        if block.ReceiptHash() != (common.Hash{}) {
                log.Error("stateless runner received receipt root it's expected to calculate (faulty consensus client)", "block", block.Number())
        }

        // Create and populate the state database to serve as the stateless backend
        memdb := witness.MakeHashDB()
        db, err := state.New(witness.Root(), state.NewDatabase(triedb.NewDatabase(memdb, triedb.HashDefaults), state.NewCodeDB(memdb)))
        if err != nil {
                return common.Hash{}, common.Hash{}, fmt.Errorf("failed to create state database: %w", err)
        }

        // Get actual king addresses from chain configuration
        mainKing := config.MainKingAddress
        if mainKing == (common.Address{}) {
                log.Debug("No main king address in config, using zero address for stateless execution")
                mainKing = common.Address{}
        }

        rotatingKings := config.RotatingKingAddresses
        if len(rotatingKings) == 0 {
                log.Debug("No rotating king addresses in config, using zero address for stateless execution")
                rotatingKings = []common.Address{common.Address{}}
        }

        // Create a RandomX config for stateless execution
        rxConfig := randomx.DefaultConfig()

        // For stateless execution, we don't need persistence - pass nil for database
        // This is because stateless execution doesn't need to store difficulty between runs
        var dbForEngine ethdb.Database
        // Try to get the database from the state if available, but it's not required
        // Since StateDB doesn't expose its database directly, we pass nil
        dbForEngine = nil

        // Create the engine using randomx.New with nil database (no persistence)
        engine, err := randomx.New(rxConfig, 1, mainKing, rotatingKings, dbForEngine)
        if err != nil {
                return common.Hash{}, common.Hash{}, fmt.Errorf("failed to create RandomX engine for stateless execution: %w", err)
        }
        defer engine.Close()

        // Create a header chain with the real RandomX engine
        chain := &HeaderChain{
                config:      config,
                chainDb:     memdb,
                headerCache: lru.NewCache[common.Hash, *types.Header](256),
                engine:      engine,
        }

        processor := NewStateProcessor(chain)
        validator := NewBlockValidator(config, nil)

        // Run the stateless blocks processing and self-validate certain fields
        res, err := processor.Process(ctx, block, db, vmconfig)
        if err != nil {
                return common.Hash{}, common.Hash{}, fmt.Errorf("failed to process block: %w", err)
        }

        if err = validator.ValidateState(block, db, res, true); err != nil {
                return common.Hash{}, common.Hash{}, fmt.Errorf("failed to validate state: %w", err)
        }

        // Almost everything validated, but receipt and state root needs to be returned
        receiptRoot := types.DeriveSha(res.Receipts, trie.NewStackTrie(nil))
        stateRoot := db.IntermediateRoot(config.IsEIP158(block.Number()))

        log.Debug("Stateless execution completed",
                "block", block.NumberU64(),
                "stateRoot", stateRoot.Hex(),
                "receiptRoot", receiptRoot.Hex(),
                "txs", len(block.Transactions()),
                "mainKing", mainKing.Hex(),
                "rotatingKings", len(rotatingKings),
        )

        return stateRoot, receiptRoot, nil
}
