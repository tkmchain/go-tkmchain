//go:build cgo && randomx
// +build cgo,randomx

// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
// or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package randomx

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestVerifySealAcceptsZeroMixDigestDuringBootstrap(t *testing.T) {
	rx := &RandomX{}
	header := &types.Header{Number: big.NewInt(1)}

	if err := rx.VerifySeal(nil, header); err != nil {
		t.Fatalf("unexpected bootstrap seal rejection: %v", err)
	}
}

func TestVerifySealRejectsZeroMixDigestAfterBootstrap(t *testing.T) {
	rx := &RandomX{}
	header := &types.Header{Number: big.NewInt(21)}

	if err := rx.VerifySeal(nil, header); err != errInvalidMixHash {
		t.Fatalf("unexpected seal error: have %v, want %v", err, errInvalidMixHash)
	}
}

func TestFinalizeWritesRotatingKingWithoutCoinbase(t *testing.T) {
	rotating := common.HexToAddress("0x0000000000000000000000000000000000000002")

	rootWithout := randomxFinalizedRoot(t, nil, common.Address{})
	rootWith := randomxFinalizedRoot(t, []common.Address{rotating}, common.Address{})

	if rootWith == rootWithout {
		t.Fatalf("state root did not include rotating king without coinbase: %s", rootWith)
	}
}

func TestPrepareWithoutParentFallsBackToGenesisDifficulty(t *testing.T) {
	rx := NewFaker()
	header := &types.Header{Number: big.NewInt(1)}
	if err := rx.Prepare(nil, header); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if header.Difficulty == nil || header.Difficulty.Cmp(GenesisDifficulty) != 0 {
		t.Fatalf("difficulty mismatch: have %v, want %v", header.Difficulty, GenesisDifficulty)
	}
	if header.TxHash != types.EmptyTxsHash {
		t.Fatalf("tx hash mismatch: have %s, want %s", header.TxHash, types.EmptyTxsHash)
	}
}

func TestFinalizeAndAssembleKeepsUserTransactionsAndMatchesFinalize(t *testing.T) {
	rx := NewFaker()
	miner := common.HexToAddress("0x0000000000000000000000000000000000000003")
	userTx := types.NewTransaction(0, common.HexToAddress("0x0000000000000000000000000000000000000004"), big.NewInt(1), 21000, big.NewInt(2), nil)
	receipts := []*types.Receipt{{
		Type:              userTx.Type(),
		Status:            types.ReceiptStatusSuccessful,
		CumulativeGasUsed: 21000,
		TxHash:            userTx.Hash(),
		GasUsed:           21000,
		EffectiveGasPrice: big.NewInt(2),
	}}

	finalizeDB := state.NewDatabaseForTesting()
	finalizeState, err := state.New(types.EmptyRootHash, finalizeDB)
	if err != nil {
		t.Fatalf("failed to create finalize state: %v", err)
	}
	assembleDB := state.NewDatabaseForTesting()
	assembleState, err := state.New(types.EmptyRootHash, assembleDB)
	if err != nil {
		t.Fatalf("failed to create assemble state: %v", err)
	}

	finalizeHeader := &types.Header{Number: big.NewInt(1), Coinbase: miner}
	rx.Finalize(nil, finalizeHeader, finalizeState, &types.Body{Transactions: []*types.Transaction{userTx}})

	assembleHeader := &types.Header{Number: big.NewInt(1), Coinbase: miner, GasUsed: 21000}
	block, err := rx.FinalizeAndAssemble(nil, assembleHeader, assembleState, &types.Body{Transactions: []*types.Transaction{userTx}}, receipts)
	if err != nil {
		t.Fatalf("failed to finalize and assemble: %v", err)
	}
	if len(block.Transactions()) != 4 {
		t.Fatalf("transaction count mismatch: have %d, want %d", len(block.Transactions()), 4)
	}
	if block.Transactions()[0].Hash() != userTx.Hash() {
		t.Fatalf("user transaction was not preserved at the front of the block")
	}
	if finalizeState.IntermediateRoot(false) != assembleState.IntermediateRoot(false) {
		t.Fatalf("finalize and assemble roots differ: finalize %s assemble %s", finalizeState.IntermediateRoot(false), assembleState.IntermediateRoot(false))
	}
}

func randomxFinalizedRoot(t *testing.T, rotatingKings []common.Address, miner common.Address) common.Hash {
	t.Helper()
	rxdb := state.NewDatabaseForTesting()
	statedb, err := state.New(types.EmptyRootHash, rxdb)
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}
	rx := NewFaker()
	rx.mainKing = common.Address{}
	rx.rotatingKings = nil
	rx.rotatingKingActivations = nil
	rx.SetRotationInterval(100)
	for _, king := range rotatingKings {
		rx.AddRotatingKing(king)
	}
	header := &types.Header{Number: big.NewInt(1), Coinbase: miner}
	rx.Finalize(nil, header, statedb, &types.Body{})
	return statedb.IntermediateRoot(false)
}

func TestRotatingKingRewardGoesToCurrentIntervalKing(t *testing.T) {
	first := common.HexToAddress("0x0000000000000000000000000000000000000001")
	second := common.HexToAddress("0x0000000000000000000000000000000000000002")
	miner := common.HexToAddress("0x0000000000000000000000000000000000000003")
	rxdb := state.NewDatabaseForTesting()
	statedb, err := state.New(types.EmptyRootHash, rxdb)
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}
	rx := NewFaker()
	rx.mainKing = common.Address{}
	rx.rotatingKings = nil
	rx.rotatingKingActivations = nil
	rx.SetRotationInterval(100)
	rx.AddRotatingKing(first)
	rx.AddRotatingKing(second)

	header := &types.Header{Number: big.NewInt(1263), Coinbase: miner}
	rx.Finalize(nil, header, statedb, &types.Body{})

	if statedb.GetBalance(first).Sign() != 0 {
		t.Fatalf("previous rotating king received reward outside its interval: %v", statedb.GetBalance(first))
	}
	if statedb.GetBalance(second).Sign() == 0 {
		t.Fatalf("current rotating king did not receive reward for covered interval")
	}
}

func TestRotatingKingRewardsCurrentKingAfterRotation(t *testing.T) {
	first := common.HexToAddress("0x0000000000000000000000000000000000000001")
	second := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rx := NewFaker()
	rx.rotatingKings = nil
	rx.SetRotationInterval(100)
	rx.AddRotatingKing(first)
	rx.AddRotatingKing(second)

	if got := rx.getRotatingKing(99); got != first {
		t.Fatalf("rotating king before first rotation = %v, want %v", got, first)
	}
	if got := rx.getRotatingKing(100); got != second {
		t.Fatalf("rotating king at first rotation = %v, want %v", got, second)
	}
	if got := rx.getRotatingKing(199); got != second {
		t.Fatalf("rotating king before second rotation = %v, want %v", got, second)
	}
	if got := rx.getRotatingKing(200); got != first {
		t.Fatalf("rotating king at second rotation = %v, want %v", got, first)
	}
}

func TestRotatingKingActivationStartsAtRotationBoundary(t *testing.T) {
	first := common.HexToAddress("0x0000000000000000000000000000000000000001")
	second := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rx := NewFaker()
	rx.rotatingKings = nil
	rx.SetRotationInterval(100)
	rx.AddRotatingKing(first)
	rx.AddRotatingKingAt(second, 200)

	if got := rx.getRotatingKing(199); got != first {
		t.Fatalf("rotating king before activation = %v, want %v", got, first)
	}
	if got := rx.getRotatingKing(200); got != second {
		t.Fatalf("rotating king at activation = %v, want %v", got, second)
	}
	if got := rx.getRotatingKing(300); got != first {
		t.Fatalf("rotating king after activation = %v, want %v", got, first)
	}
}
