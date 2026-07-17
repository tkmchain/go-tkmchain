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
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type verifySealTestChain struct {
	config *params.ChainConfig
}

func (c verifySealTestChain) Config() *params.ChainConfig               { return c.config }
func (verifySealTestChain) CurrentHeader() *types.Header                { return nil }
func (verifySealTestChain) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (verifySealTestChain) GetHeaderByNumber(uint64) *types.Header      { return nil }
func (verifySealTestChain) GetHeaderByHash(common.Hash) *types.Header   { return nil }

func TestVerifySealSkipsPreRandomXTxBlock(t *testing.T) {
	rx := &RandomX{}
	config := *params.RandomXChainConfig
	config.RandomXTxBlock = big.NewInt(2450)
	header := &types.Header{Number: big.NewInt(1), Difficulty: GenesisDifficulty}

	if err := rx.VerifySeal(verifySealTestChain{config: &config}, header); err != nil {
		t.Fatalf("unexpected pre-RandomX seal rejection: %v", err)
	}
}

func TestVerifySealRejectsZeroMixDigestAfterRandomXTxActivation(t *testing.T) {
	rx := &RandomX{}
	config := *params.RandomXChainConfig
	config.RandomXTxBlock = big.NewInt(1)
	header := &types.Header{Number: big.NewInt(21), Difficulty: GenesisDifficulty}

	err := rx.VerifySeal(verifySealTestChain{config: &config}, header)
	if err == nil || !strings.Contains(err.Error(), "invalid proof") {
		t.Fatalf("unexpected seal error: %v", err)
	}
}

func TestValidStoredMixDigestAcceptsBlock2450ProofValue(t *testing.T) {
	rx := &RandomX{}
	difficulty := new(big.Int).SetUint64(0x4a554)
	target := new(big.Int).Div(maxUint256, difficulty)
	header := &types.Header{
		Number:     big.NewInt(2450),
		Difficulty: difficulty,
		MixDigest:  common.HexToHash("0x0000242a94ce61ceba227a73e415ff4221d070ed428ae7a8f07940f70741a2a8"),
	}

	if !rx.validStoredMixDigest(header, target) {
		t.Fatal("block 2450 stored mix digest should be below target")
	}
}

func TestStoredMixDigestCompatibilityWindow(t *testing.T) {
	rx := NewFaker()
	preKyoto := &types.Header{Number: big.NewInt(int64(kyotoStoredMixDigestCompatUntil + 1))}
	if !rx.allowStoredMixDigestProof(preKyoto, false) {
		t.Fatal("pre-Kyoto stored mix digest compatibility should remain available")
	}
	kyotoHistorical := &types.Header{Number: big.NewInt(int64(kyotoStoredMixDigestCompatUntil))}
	if !rx.allowStoredMixDigestProof(kyotoHistorical, true) {
		t.Fatal("historical Kyoto stored mix digest compatibility should be available")
	}
	kyotoFuture := &types.Header{Number: big.NewInt(int64(kyotoStoredMixDigestCompatUntil + 1))}
	if rx.allowStoredMixDigestProof(kyotoFuture, true) {
		t.Fatal("future Kyoto stored mix digest compatibility should be disabled")
	}
}

func TestEpochForBlockStartsAtRandomXTxActivation(t *testing.T) {
	rx := &RandomX{config: DefaultConfig()}
	config := *params.RandomXChainConfig
	config.RandomXTxBlock = big.NewInt(2450)
	chain := verifySealTestChain{config: &config}

	if epoch := rx.epochForBlock(chain, 2449); epoch != 0 {
		t.Fatalf("pre-activation epoch = %d, want 0", epoch)
	}
	if epoch := rx.epochForBlock(chain, 2450); epoch != 0 {
		t.Fatalf("activation epoch = %d, want 0", epoch)
	}
	if epoch := rx.epochForBlock(chain, 2450+RandomXEpochLength); epoch != 1 {
		t.Fatalf("next RandomX epoch = %d, want 1", epoch)
	}
}

func TestDefaultLightModeCreatesVM(t *testing.T) {
	rx, err := New(DefaultConfig(), 1, common.Address{}, nil)
	if err != nil {
		t.Fatalf("new RandomX failed: %v", err)
	}
	defer rx.Close()

	if rx.dataset != nil {
		t.Fatal("default config unexpectedly initialized full dataset")
	}
	vm, err := rx.getVM()
	if err != nil {
		t.Fatalf("getVM failed: %v", err)
	}
	defer vm.Close()

	out := make([]byte, 32)
	vm.CalculateHash([]byte("tkm-randomx-smoke"), out)
	if common.BytesToHash(out) == (common.Hash{}) {
		t.Fatal("RandomX hash output is zero")
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
	header := &types.Header{Number: big.NewInt(1), Difficulty: GenesisDifficulty}
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

func TestKyotoEmptyBlockStillAppliesImplicitRewards(t *testing.T) {
	rx := NewFaker()
	rx.mainKing = common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2")
	rx.rotatingKings = nil
	rx.rotatingKingActivations = nil
	rotating := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rx.SetRotationInterval(100)
	rx.AddRotatingKing(rotating)
	miner := common.HexToAddress("0x4441d6fed0836b77a503e0b2788bfed6fd8c23a8")
	config := *params.RandomXChainConfig
	config.KyotoTime = new(uint64)
	chain := verifySealTestChain{config: &config}
	header := &types.Header{Number: big.NewInt(5636), Time: 1, Coinbase: miner}

	if !rx.skipImplicitRewards(chain, header, &types.Body{}) {
		t.Fatal("empty Kyoto block should use reward-tx-free compatibility path")
	}

	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	rx.Finalize(chain, header, statedb, &types.Body{})

	mainBal := statedb.GetBalance(rx.mainKing).ToBig()
	minerBal := statedb.GetBalance(miner).ToBig()
	want := new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether))
	if mainBal.Cmp(want) != 0 || minerBal.Cmp(want) != 0 {
		t.Fatalf("unexpected balances: main=%v miner=%v want=%v", mainBal, minerBal, want)
	}
	if got := statedb.GetBalance(rotating).ToBig(); got.Sign() != 0 {
		t.Fatalf("rotating king balance = %v, want 0 for empty Kyoto compatibility block", got)
	}
	if got := statedb.GetState(params.SystemAddress, rotatingKingStateSlot); got != common.BytesToHash(rotating.Bytes()) {
		t.Fatalf("rotating king state slot = %s, want %s", got, common.BytesToHash(rotating.Bytes()))
	}

	rewardTxState, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	body := &types.Body{Transactions: []*types.Transaction{types.NewBlockRewardTx(5636, types.BlockRewardMiner, header.Coinbase, big.NewInt(1))}}
	if rx.skipImplicitRewards(chain, header, body) {
		t.Fatal("Kyoto block with reward tx should not use empty-block path")
	}
	rx.Finalize(chain, header, rewardTxState, body)
	if got := rewardTxState.GetBalance(miner).ToBig(); got.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("reward transaction balance = %v, want 1", got)
	}
}

func TestFinalizeAndAssembleKeepsUserTransactionsAndMatchesFinalize(t *testing.T) {
	rx := NewFaker()
	rx.mainKing = common.HexToAddress("0x0000000000000000000000000000000000000001")
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

	header := &types.Header{Number: big.NewInt(1363), Coinbase: miner}
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

func TestRewardSharesFallbackRotatingKingRewardToMainKing(t *testing.T) {
	mainKing := common.HexToAddress("0x0000000000000000000000000000000000000001")
	miner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rx := NewFaker()
	rx.mainKing = mainKing
	rx.rotatingKings = nil
	rx.rotatingKingActivations = nil
	rx.SetRotationInterval(100)

	totalReward := big.NewInt(1000)
	header := &types.Header{Number: big.NewInt(1), Coinbase: miner}
	gotMainKing, mainKingReward, rotatingKing, rotatingKingReward, gotMiner, minerReward := rx.rewardShares(header, totalReward)

	if gotMainKing != mainKing {
		t.Fatalf("main king = %s, want %s", gotMainKing.Hex(), mainKing.Hex())
	}
	if rotatingKing != (common.Address{}) {
		t.Fatalf("rotating king = %s, want zero address", rotatingKing.Hex())
	}
	if gotMiner != miner {
		t.Fatalf("miner = %s, want %s", gotMiner.Hex(), miner.Hex())
	}
	if mainKingReward.Cmp(big.NewInt(500)) != 0 {
		t.Fatalf("main king reward = %s, want 500", mainKingReward)
	}
	if rotatingKingReward.Sign() != 0 {
		t.Fatalf("rotating king reward = %s, want 0", rotatingKingReward)
	}
	if minerReward.Cmp(big.NewInt(500)) != 0 {
		t.Fatalf("miner reward = %s, want 500", minerReward)
	}
}

type edaTestChain struct {
	config *params.ChainConfig
}

func (c edaTestChain) Config() *params.ChainConfig                             { return c.config }
func (c edaTestChain) CurrentHeader() *types.Header                            { return nil }
func (c edaTestChain) GetHeader(hash common.Hash, number uint64) *types.Header { return nil }
func (c edaTestChain) GetHeaderByNumber(number uint64) *types.Header           { return nil }
func (c edaTestChain) GetHeaderByHash(hash common.Hash) *types.Header          { return nil }

func TestCalcDifficultyAppliesFastEDAOnEgypt(t *testing.T) {
	chain := edaTestChain{config: params.EgyptChainConfig}
	rx := NewFaker()
	parent := &types.Header{
		Number:     big.NewInt(63),
		Time:       1_000,
		Difficulty: big.NewInt(7424),
	}

	oneStep := rx.CalcDifficulty(chain, parent.Time+EgyptEDAThreshold, parent)
	if want := big.NewInt(1856); oneStep.Cmp(want) != 0 {
		t.Fatalf("one Egypt EDA step difficulty = %v, want %v", oneStep, want)
	}

	manySteps := rx.CalcDifficulty(chain, parent.Time+6*EgyptEDAThreshold, parent)
	if manySteps.Cmp(MinDifficulty) != 0 {
		t.Fatalf("many Egypt EDA steps difficulty = %v, want min %v", manySteps, MinDifficulty)
	}
}

func TestCalcDifficultyAppliesGentleMainnetEDAOncePerBlock(t *testing.T) {
	edaTime := uint64(0)
	config := &params.ChainConfig{
		ChainID:     big.NewInt(1),
		LondonBlock: big.NewInt(0),
		EDATime:     &edaTime,
	}
	chain := edaTestChain{config: config}
	rx := NewFaker()
	parent := &types.Header{
		Number:     big.NewInt(1),
		Time:       1_000,
		Difficulty: big.NewInt(1024),
	}

	oneStep := rx.CalcDifficulty(chain, parent.Time+EDAThreshold, parent)
	if want := big.NewInt(768); oneStep.Cmp(want) != 0 {
		t.Fatalf("one EDA step difficulty = %v, want %v", oneStep, want)
	}

	longGap := rx.CalcDifficulty(chain, parent.Time+5*EDAThreshold, parent)
	if want := big.NewInt(768); longGap.Cmp(want) != 0 {
		t.Fatalf("long gap EDA difficulty = %v, want one gentle reduction %v", longGap, want)
	}
}
