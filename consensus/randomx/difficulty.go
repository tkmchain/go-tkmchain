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

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// RandomX difficulty constants
const (
	// MinimumDifficulty is the absolute lowest allowed difficulty
	MinimumDifficulty = 3

	// MaxDifficultyChange is the maximum percentage change per block (25%)
	MaxDifficultyChange = 25
)

// CalcDifficulty calculates the difficulty for the next block using a simple,
// deterministic algorithm that will always produce the same result for the same inputs.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
	currentHeight := parent.Number.Uint64()
	nextHeight := currentHeight + 1

	// Genesis block or block 1: use minimum difficulty
	if currentHeight == 0 {
		log.Info("Initializing difficulty for genesis block", "difficulty", MinimumDifficulty)
		return new(big.Int).SetUint64(MinimumDifficulty)
	}

	// For early blocks (first 100 blocks), use a simple progression
	if currentHeight < 100 {
		// Gradual difficulty increase: start at 3, reach ~100 by block 100
		diff := uint64(3) + (currentHeight / 2)
		if diff < MinimumDifficulty {
			diff = MinimumDifficulty
		}
		result := new(big.Int).SetUint64(diff)
		log.Debug("Early block difficulty", "height", nextHeight, "difficulty", result)
		return result
	}

	// For normal operation, adjust based on actual block time
	parentTime := parent.Time
	
	// Calculate the actual time since parent block
	var actualTime uint64
	if time > parentTime {
		actualTime = time - parentTime
	} else {
		actualTime = TargetBlockTimeSeconds
	}
	
	// Get parent difficulty
	parentDiff := parent.Difficulty
	
	// Calculate adjustment factor based on how far from target we are
	var adjustmentPercent int64
	
	if actualTime < TargetBlockTimeSeconds/2 {
		// Block was extremely fast (less than 1 minute) - increase difficulty by 25%
		adjustmentPercent = MaxDifficultyChange
	} else if actualTime < TargetBlockTimeSeconds {
		// Block was somewhat fast - increase difficulty by 10%
		adjustmentPercent = MaxDifficultyChange / 2
	} else if actualTime > TargetBlockTimeSeconds*2 {
		// Block was extremely slow - decrease difficulty by 25%
		adjustmentPercent = -MaxDifficultyChange
	} else if actualTime > TargetBlockTimeSeconds {
		// Block was somewhat slow - decrease difficulty by 10%
		adjustmentPercent = -(MaxDifficultyChange / 2)
	} else {
		// Perfect timing - no change
		adjustmentPercent = 0
	}
	
	// Calculate new difficulty
	newDiff := new(big.Int).Set(parentDiff)
	
	if adjustmentPercent > 0 {
		// Increase difficulty
		increase := new(big.Int).Mul(parentDiff, big.NewInt(adjustmentPercent))
		increase.Div(increase, big.NewInt(100))
		newDiff.Add(parentDiff, increase)
	} else if adjustmentPercent < 0 {
		// Decrease difficulty
		decrease := new(big.Int).Mul(parentDiff, big.NewInt(-adjustmentPercent))
		decrease.Div(decrease, big.NewInt(100))
		newDiff.Sub(parentDiff, decrease)
	}
	
	// Ensure minimum difficulty
	minDiff := new(big.Int).SetUint64(MinimumDifficulty)
	if newDiff.Cmp(minDiff) < 0 {
		newDiff = minDiff
	}
	
	// Log the adjustment
	log.Debug("Difficulty adjusted",
		"height", nextHeight,
		"parent_time", parentTime,
		"block_time", actualTime,
		"target_time", TargetBlockTimeSeconds,
		"parent_diff", parentDiff,
		"new_diff", newDiff,
		"adjustment_percent", adjustmentPercent,
	)
	
	return newDiff
}

// CalculateNextDifficulty is the main exported function for difficulty calculation
func CalculateNextDifficulty(parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
	return CalcDifficulty(nil, 0, parent, getHeader)
}

// Legacy functions kept for compatibility
func makeDifficultyCalculator(bombDelay *big.Int) func(time uint64, parent *types.Header) *big.Int {
	return func(time uint64, parent *types.Header) *big.Int {
		return new(big.Int).Set(parent.Difficulty)
	}
}

func calcDifficultyHomestead(time uint64, parent *types.Header) *big.Int {
	return new(big.Int).Set(parent.Difficulty)
}

func calcDifficultyFrontier(time uint64, parent *types.Header) *big.Int {
	return new(big.Int).Set(parent.Difficulty)
}

var (
	calcDifficultyEip5133        = makeDifficultyCalculator(big.NewInt(0))
	calcDifficultyEip4345        = makeDifficultyCalculator(big.NewInt(0))
	calcDifficultyEip3554        = makeDifficultyCalculator(big.NewInt(0))
	calcDifficultyConstantinople = makeDifficultyCalculator(big.NewInt(0))
	calcDifficultyByzantium      = makeDifficultyCalculator(big.NewInt(0))
)
