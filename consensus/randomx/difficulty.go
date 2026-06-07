// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
package randomx

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// RandomX difficulty constants
const (
//	TargetBlockTimeSeconds = 120
	MinimumDifficultyValue = 3
	MaxAdjustmentPercent   = 10
)

// CalcDifficulty calculates the difficulty for the next block.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, getHeader func(number uint64) *types.Header) *big.Int {
	currentHeight := parent.Number.Uint64()
	nextHeight := currentHeight + 1

	log.Info("========== DIFFICULTY CALC ==========",
		"currentHeight", currentHeight,
		"nextHeight", nextHeight,
		"parent_diff", parent.Difficulty.String())

	// Genesis block
	if currentHeight == 0 {
		diff := new(big.Int).SetUint64(MinimumDifficultyValue)
		log.Info("Genesis difficulty", "difficulty", diff)
		return diff
	}

	// Calculate actual time since parent block
	parentTime := parent.Time
	var actualTime uint64
	if time > parentTime {
		actualTime = time - parentTime
	} else {
		actualTime = uint64(TargetBlockTimeSeconds)  // Convert to uint64
	}

	targetTime := uint64(TargetBlockTimeSeconds)  // Convert to uint64
	parentDiff := parent.Difficulty.Uint64()

	log.Info("Difficulty adjustment data",
		"parent_time", parentTime,
		"current_time", time,
		"actual_time", actualTime,
		"target_time", targetTime,
		"parent_diff", parentDiff)

	var newDiffVal uint64

	if actualTime < targetTime {
		// Blocks too fast → increase difficulty
		// Use a smaller increase to avoid dramatic jumps
		increase := parentDiff * (targetTime - actualTime) / (targetTime * 2)
		if increase < 1 {
			increase = 1
		}
		// Limit to 10% max increase
		maxIncrease := parentDiff * MaxAdjustmentPercent / 100
		if increase > maxIncrease {
			increase = maxIncrease
		}
		newDiffVal = parentDiff + increase
		log.Info("Increasing difficulty", "increase", increase, "new", newDiffVal)
	} else if actualTime > targetTime {
		// Blocks too slow → decrease difficulty
		decrease := parentDiff * (actualTime - targetTime) / (targetTime * 2)
		if decrease < 1 {
			decrease = 1
		}
		// Limit to 10% max decrease
		maxDecrease := parentDiff * MaxAdjustmentPercent / 100
		if decrease > maxDecrease {
			decrease = maxDecrease
		}
		if parentDiff > decrease {
			newDiffVal = parentDiff - decrease
		} else {
			newDiffVal = MinimumDifficultyValue
		}
		log.Info("Decreasing difficulty", "decrease", decrease, "new", newDiffVal)
	} else {
		newDiffVal = parentDiff
		log.Info("Difficulty unchanged", "new", newDiffVal)
	}

	// Ensure minimum difficulty
	if newDiffVal < MinimumDifficultyValue {
		newDiffVal = MinimumDifficultyValue
	}

	newDiff := new(big.Int).SetUint64(newDiffVal)

	log.Info("Difficulty calculated", "height", nextHeight, "new_diff", newDiff)
	return newDiff
}

// CalculateNextDifficulty is the main exported function
func CalculateNextDifficulty(parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
	return CalcDifficulty(nil, 0, parent, getHeader)
}
