//go:build cgo && randomx
// +build cgo,randomx

// Copyright 2026 The go-ethereum Authors

package randomx

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestCalcDifficultyWithPersistenceLinearProgressionIncreases(t *testing.T) {
	parent := &types.Header{
		Number:     big.NewInt(1),
		Time:       100,
		Difficulty: new(big.Int).Set(GenesisDifficulty),
	}

	var rx *RandomX
	difficulty := rx.CalcDifficultyWithPersistence(nil, parent.Time+TargetBlockTimeSeconds, parent)
	expected := new(big.Int).Add(GenesisDifficulty, new(big.Int).SetUint64(InitialDifficulty))
	if difficulty.Cmp(expected) != 0 {
		t.Fatalf("unexpected difficulty: got %v, want %v", difficulty, expected)
	}
	if difficulty.Cmp(parent.Difficulty) <= 0 {
		t.Fatalf("difficulty did not increase: got %v, parent %v", difficulty, parent.Difficulty)
	}
}
