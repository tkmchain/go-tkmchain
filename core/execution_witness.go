// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or
// modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.

package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
)

// BuildExecutionWitness executes block with witness collection enabled. When
// recoverState is true, a missing parent state is rebuilt from the nearest
// available ancestor by re-executing the blocks already stored in the local
// database. This is deliberately opt-in because rebuilding from genesis can
// be expensive on a pruned node.
//
// The caller must provide a block already present in the local chain database.
// The method never changes the canonical head and only writes recovered state
// needed to make the requested execution reproducible.
func (bc *BlockChain) BuildExecutionWitness(ctx context.Context, block *types.Block, recoverState bool) (*stateless.Witness, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	if block.NumberU64() == 0 {
		return nil, errors.New("genesis block has no parent execution witness")
	}
	if !bc.chainmu.TryLock() {
		return nil, errors.New("blockchain is busy or stopping")
	}
	defer bc.chainmu.Unlock()

	parent := bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		return nil, fmt.Errorf("parent block %s is not available", block.ParentHash())
	}
	if !bc.HasState(parent.Root()) {
		if !recoverState {
			return nil, fmt.Errorf("parent state %s is not available; retry with state recovery enabled", parent.Root())
		}
		if _, err := bc.recoverAncestors(ctx, parent, false); err != nil {
			return nil, fmt.Errorf("recover parent state %s: %w", parent.Root(), err)
		}
		if !bc.HasState(parent.Root()) {
			return nil, fmt.Errorf("parent state %s remains unavailable after recovery", parent.Root())
		}
	}

	result, err := bc.ProcessBlock(ctx, parent.Root(), block, ExecuteConfig{
		WriteState:         false,
		EnableTracer:       false,
		MakeWitness:        true,
		EnableWitnessStats: false,
	})
	if err != nil {
		return nil, fmt.Errorf("execute block %d: %w", block.NumberU64(), err)
	}
	if result == nil || result.Witness() == nil {
		return nil, fmt.Errorf("execution witness was not produced for block %d", block.NumberU64())
	}
	return result.Witness(), nil
}
