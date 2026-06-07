// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
package randomx

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// RandomX difficulty constants
const (
//	TargetBlockTimeSeconds uint64 = 120 // Target block time (2 minutes)
	GenesisDifficulty      uint64 = 3   // Genesis block difficulty
	InitialDifficulty      uint64 = 50  // Starting difficulty for block 1
	MaxAdjustmentPercent   uint64 = 25  // Maximum difficulty change per block (25%)
)

// CalcDifficulty calculates the difficulty for the next block.
func CalcDifficulty(config *params.ChainConfig, blockTime uint64, parent *types.Header, getHeader func(number uint64) *types.Header) *big.Int {
	currentHeight := parent.Number.Uint64()
	nextHeight := currentHeight + 1

	// Get current timestamp - if blockTime is 0, use current system time
	currentTimestamp := blockTime
	if currentTimestamp == 0 {
		currentTimestamp = uint64(time.Now().Unix())
	}

	log.Info("========== DIFFICULTY CALC ==========",
		"currentHeight", currentHeight,
		"nextHeight", nextHeight,
		"parent_diff", parent.Difficulty.String())

	// Genesis block (block 0)
	if currentHeight == 0 {
		diff := new(big.Int).SetUint64(GenesisDifficulty)
		log.Info("Genesis difficulty", "difficulty", diff)
		return diff
	}

	// Block 1 (parent is genesis) - use initial difficulty
	if currentHeight == 1 {
		diff := new(big.Int).SetUint64(InitialDifficulty)
		log.Info("Block 1 difficulty", "difficulty", diff, "parent_difficulty", parent.Difficulty.String())
		return diff
	}

	// Calculate actual time since parent block
	parentTime := parent.Time
	var actualTime uint64
	
	if currentTimestamp > parentTime {
		actualTime = currentTimestamp - parentTime
	} else {
		// If current timestamp is less than parent time, use default
		actualTime = TargetBlockTimeSeconds
		log.Warn("Current time older than parent time, using default", 
			"currentTimestamp", currentTimestamp, 
			"parentTime", parentTime)
	}

	// Ensure actualTime is not zero to avoid division issues
	if actualTime == 0 {
		actualTime = 1
	}

	parentDiff := parent.Difficulty.Uint64()
	var newDiffVal uint64

	log.Info("Difficulty adjustment data",
		"parent_time", parentTime,
		"current_timestamp", currentTimestamp,
		"actual_time", actualTime,
		"target_time", TargetBlockTimeSeconds,
		"parent_diff", parentDiff)

	// Calculate difficulty adjustment based on actual block time
	if actualTime < TargetBlockTimeSeconds {
		// Blocks are too fast → increase difficulty
		// Calculate proportional increase
		ratio := float64(TargetBlockTimeSeconds) / float64(actualTime)
		increase := uint64(float64(parentDiff) * (ratio - 1.0))
		
		// Limit to MaxAdjustmentPercent
		maxIncrease := parentDiff * MaxAdjustmentPercent / 100
		if increase > maxIncrease {
			increase = maxIncrease
		}
		if increase < 1 {
			increase = 1
		}
		newDiffVal = parentDiff + increase
		log.Info("Increasing difficulty", 
			"ratio", ratio, 
			"increase", increase, 
			"new", newDiffVal)
	} else if actualTime > TargetBlockTimeSeconds {
		// Blocks are too slow → decrease difficulty
		ratio := float64(actualTime) / float64(TargetBlockTimeSeconds)
		decrease := uint64(float64(parentDiff) * (ratio - 1.0))
		
		// Limit to MaxAdjustmentPercent
		maxDecrease := parentDiff * MaxAdjustmentPercent / 100
		if decrease > maxDecrease {
			decrease = maxDecrease
		}
		if decrease < 1 {
			decrease = 1
		}
		if parentDiff > decrease {
			newDiffVal = parentDiff - decrease
		} else {
			newDiffVal = GenesisDifficulty
		}
		log.Info("Decreasing difficulty", 
			"ratio", ratio, 
			"decrease", decrease, 
			"new", newDiffVal)
	} else {
		// Perfect timing, keep same difficulty
		newDiffVal = parentDiff
		log.Info("Difficulty unchanged", "new", newDiffVal)
	}

	// Ensure minimum difficulty
	if newDiffVal < GenesisDifficulty {
		newDiffVal = GenesisDifficulty
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
