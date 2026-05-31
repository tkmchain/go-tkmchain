// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package randomx

import (
	"math/big"
	"testing"
)

func TestCalculateBlockRewardHalvesAfterFourYears(t *testing.T) {
	fourYearsInBlocks := uint64(4 * 365 * 24 * 60 * 60 / TargetBlockTimeSeconds)
	if BlocksPerHalving != fourYearsInBlocks {
		t.Fatalf("halving period mismatch: have %d blocks, want %d", BlocksPerHalving, fourYearsInBlocks)
	}

	if reward := CalculateBlockReward(BlocksPerHalving - 1); reward.Cmp(InitialBlockReward) != 0 {
		t.Fatalf("reward before halving block mismatch: have %v, want %v", reward, InitialBlockReward)
	}

	expected := new(big.Int).Div(new(big.Int).Set(InitialBlockReward), big.NewInt(2))
	if reward := CalculateBlockReward(BlocksPerHalving); reward.Cmp(expected) != 0 {
		t.Fatalf("reward at first halving block mismatch: have %v, want %v", reward, expected)
	}
}
