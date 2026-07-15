// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package randomx

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRewardTransactionsKeepVisibleSplitWithoutRotatingKing(t *testing.T) {
	mainKing := common.HexToAddress("0x0000000000000000000000000000000000000001")
	miner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rx := NewFaker()
	rx.mainKing = mainKing
	rx.rotatingKings = nil
	rx.rotatingKingActivations = nil
	rx.SetRotationInterval(100)

	rewards := rx.RewardTransactions(&types.Header{Number: big.NewInt(1), Coinbase: miner}, nil)
	if len(rewards) != 3 {
		t.Fatalf("reward tx count = %d, want 3", len(rewards))
	}
	wantMain := types.NewBlockRewardTx(1, types.BlockRewardMainKing, mainKing, new(big.Int).Mul(big.NewInt(20), big.NewInt(1e18)))
	wantRotating := types.NewBlockRewardTx(1, types.BlockRewardRotatingKing, common.Address{}, new(big.Int).Mul(big.NewInt(80), big.NewInt(1e18)))
	wantMiner := types.NewBlockRewardTx(1, types.BlockRewardMiner, miner, new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18)))
	for i, want := range []*types.Transaction{wantMain, wantRotating, wantMiner} {
		if rewards[i].Hash() != want.Hash() {
			t.Fatalf("reward tx %d hash = %s, want %s", i, rewards[i].Hash(), want.Hash())
		}
	}
}

func TestRewardTransactionsKeepMinerOnlyWhenNoKingAddressesExist(t *testing.T) {
	miner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rx := NewFaker()
	rx.mainKing = common.Address{}
	rx.rotatingKings = nil
	rx.rotatingKingActivations = nil
	rx.SetRotationInterval(100)

	rewards := rx.RewardTransactions(&types.Header{Number: big.NewInt(1), Coinbase: miner}, nil)
	if len(rewards) != 1 {
		t.Fatalf("reward tx count = %d, want only miner tx", len(rewards))
	}
	wantMiner := types.NewBlockRewardTx(1, types.BlockRewardMiner, miner, new(big.Int).Mul(big.NewInt(200), big.NewInt(1e18)))
	if rewards[0].Hash() != wantMiner.Hash() {
		t.Fatalf("reward tx hash = %s, want miner reward %s", rewards[0].Hash(), wantMiner.Hash())
	}
}

func TestCompatibleRewardTransactionsIncludePreviousFallbackMarkers(t *testing.T) {
	mainKing := common.HexToAddress("0x0000000000000000000000000000000000000001")
	miner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rx := NewFaker()
	rx.mainKing = mainKing
	rx.rotatingKings = nil
	rx.rotatingKingActivations = nil
	rx.SetRotationInterval(100)

	sets := rx.CompatibleRewardTransactions(&types.Header{Number: big.NewInt(1), Coinbase: miner}, nil)
	if len(sets) != 2 {
		t.Fatalf("compatible reward set count = %d, want canonical and previous fallback", len(sets))
	}
	legacy := sets[0]
	wantMain := types.NewBlockRewardTx(1, types.BlockRewardMainKing, mainKing, new(big.Int).Mul(big.NewInt(20), big.NewInt(1e18)))
	wantRotating := types.NewBlockRewardTx(1, types.BlockRewardRotatingKing, common.Address{}, new(big.Int).Mul(big.NewInt(80), big.NewInt(1e18)))
	wantMiner := types.NewBlockRewardTx(1, types.BlockRewardMiner, miner, new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18)))
	for i, want := range []*types.Transaction{wantMain, wantRotating, wantMiner} {
		if legacy[i].Hash() != want.Hash() {
			t.Fatalf("legacy reward tx %d hash = %s, want %s", i, legacy[i].Hash(), want.Hash())
		}
	}
	fallback := sets[1]
	wantFallbackMain := types.NewBlockRewardTx(1, types.BlockRewardMainKing, mainKing, new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18)))
	wantFallbackRotating := types.NewBlockRewardTx(1, types.BlockRewardRotatingKing, common.Address{}, new(big.Int))
	wantFallbackMiner := types.NewBlockRewardTx(1, types.BlockRewardMiner, miner, new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18)))
	for i, want := range []*types.Transaction{wantFallbackMain, wantFallbackRotating, wantFallbackMiner} {
		if fallback[i].Hash() != want.Hash() {
			t.Fatalf("fallback reward tx %d hash = %s, want %s", i, fallback[i].Hash(), want.Hash())
		}
	}
}

func TestLegacyRewardTransactionMatchesBlock2462Marker(t *testing.T) {
	mainKing := common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2")
	amount := new(big.Int).Mul(big.NewInt(20), big.NewInt(1e18))
	tx := types.NewBlockRewardTx(2462, types.BlockRewardMainKing, mainKing, amount)
	want := common.HexToHash("0x3798f342e7d81c8f62c0f7be285fe48ad9fd44c1d92ee3726e57c85738f3db58")
	if tx.Hash() != want {
		t.Fatalf("legacy block 2462 main king marker = %s, want %s", tx.Hash(), want)
	}
}
