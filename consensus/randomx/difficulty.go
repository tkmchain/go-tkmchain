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
//	TargetBlockTimeSeconds uint64 = 120 // Target block time (2 minutes)
	MinimumDifficulty      uint64 = 3   // Minimum difficulty (genesis)
	MaxAdjustmentPercent   uint64 = 25  // Maximum difficulty change per block (25%)
	LinearIncreaseBlocks   uint64 = 100 // Number of blocks for linear progression
)

// CalcDifficulty calculates the difficulty for the next block.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, getHeader func(number uint64) *types.Header) *big.Int {
	currentHeight := parent.Number.Uint64()
	nextHeight := currentHeight + 1

	log.Info("========== DIFFICULTY CALC ==========",
		"currentHeight", currentHeight,
		"nextHeight", nextHeight,
		"parent_diff", parent.Difficulty.String())

	// Genesis block (block 0) - minimum difficulty
	if currentHeight == 0 {
		diff := new(big.Int).SetUint64(MinimumDifficulty)
		log.Info("Genesis difficulty", "difficulty", diff)
		return diff
	}

	// Blocks 1-99: Linear progression to reach reasonable difficulty by block 100
	if currentHeight < LinearIncreaseBlocks {
		// Target difficulty at block 100 = 50
		targetMaxDiff := uint64(50)
		diff := MinimumDifficulty + (currentHeight * (targetMaxDiff - MinimumDifficulty) / LinearIncreaseBlocks)
		if diff < MinimumDifficulty {
			diff = MinimumDifficulty
		}
		result := new(big.Int).SetUint64(diff)
		log.Info("Linear difficulty progression",
			"height", nextHeight,
			"difficulty", result,
			"phase", "early_blocks")
		return result
	}

	// For blocks >= 100, use simple adjustment based on parent block time
	// This avoids complex array operations that could cause issues
	parentTime := parent.Time
	var actualTime uint64
	if time > parentTime {
		actualTime = time - parentTime
	} else {
		actualTime = TargetBlockTimeSeconds
	}

	parentDiff := parent.Difficulty.Uint64()
	var newDiffVal uint64

	log.Info("Difficulty adjustment data",
		"parent_time", parentTime,
		"current_time", time,
		"actual_time", actualTime,
		"target_time", TargetBlockTimeSeconds,
		"parent_diff", parentDiff)

	if actualTime < TargetBlockTimeSeconds {
		// Too fast, increase difficulty
		increase := parentDiff * (TargetBlockTimeSeconds - actualTime) / TargetBlockTimeSeconds
		if increase < 1 {
			increase = 1
		}
		maxIncrease := parentDiff * MaxAdjustmentPercent / 100
		if increase > maxIncrease {
			increase = maxIncrease
		}
		newDiffVal = parentDiff + increase
		log.Info("Increasing difficulty", "increase", increase, "new", newDiffVal)
	} else if actualTime > TargetBlockTimeSeconds {
		// Too slow, decrease difficulty
		decrease := parentDiff * (actualTime - TargetBlockTimeSeconds) / TargetBlockTimeSeconds
		if decrease < 1 {
			decrease = 1
		}
		maxDecrease := parentDiff * MaxAdjustmentPercent / 100
		if decrease > maxDecrease {
			decrease = maxDecrease
		}
		if parentDiff > decrease {
			newDiffVal = parentDiff - decrease
		} else {
			newDiffVal = MinimumDifficulty
		}
		log.Info("Decreasing difficulty", "decrease", decrease, "new", newDiffVal)
	} else {
		newDiffVal = parentDiff
		log.Info("Difficulty unchanged", "new", newDiffVal)
	}

	// Ensure minimum difficulty
	if newDiffVal < MinimumDifficulty {
		newDiffVal = MinimumDifficulty
	}

	newDiff := new(big.Int).SetUint64(newDiffVal)

	log.Info("Difficulty calculated",
		"height", nextHeight,
		"actual_time", actualTime,
		"target_time", TargetBlockTimeSeconds,
		"parent_diff", parentDiff,
		"new_diff", newDiff)

	return newDiff
}

// CalculateNextDifficulty is the main exported function
func CalculateNextDifficulty(parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
	return CalcDifficulty(nil, 0, parent, getHeader)
}
