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
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// Difficulty constants for RandomX consensus
const (
	// TargetBlockTimeSeconds is the desired time between blocks (2 minutes)
//	TargetBlockTimeSeconds uint64 = 120

	// GenesisDifficulty is the starting difficulty for block 0
	GenesisDifficulty uint64 = 3

	// InitialDifficulty is the difficulty for block 1 (after genesis)
	InitialDifficulty uint64 = 50

	// MaxAdjustmentPercent limits difficulty change per block to prevent volatility
	MaxAdjustmentPercent uint64 = 25

	// LinearProgressionBlocks defines how many blocks use linear growth
	LinearProgressionBlocks uint64 = 100
)

// CalcDifficulty calculates the difficulty for the next block using a hybrid approach:
// - Block 0: Fixed genesis difficulty
// - Blocks 1-99: Linear progression for smooth initial growth
// - Blocks 100+: Dynamic adjustment based on actual block times
func CalcDifficulty(config *params.ChainConfig, blockTime uint64, parent *types.Header, getHeader func(number uint64) *types.Header) *big.Int {
	currentHeight := parent.Number.Uint64()
	nextHeight := currentHeight + 1

	// Use current system time if blockTime is not provided (mining case)
	currentTimestamp := blockTime
	if currentTimestamp == 0 {
		currentTimestamp = uint64(time.Now().Unix())
	}

	log.Debug("Difficulty calculation started",
		"current_height", currentHeight,
		"next_height", nextHeight,
		"parent_difficulty", parent.Difficulty.String())

	// Genesis block - minimum difficulty
	if currentHeight == 0 {
		diff := new(big.Int).SetUint64(GenesisDifficulty)
		log.Info("Genesis block difficulty", "height", nextHeight, "difficulty", diff)
		return diff
	}

	// Linear progression for early blocks (1-99)
	// This ensures smooth difficulty growth before dynamic adjustment
	if currentHeight < LinearProgressionBlocks {
		// Target difficulty at block 100 is 50
		targetMaxDiff := InitialDifficulty
		diff := GenesisDifficulty + (currentHeight*(targetMaxDiff-GenesisDifficulty))/LinearProgressionBlocks
		
		if diff < GenesisDifficulty {
			diff = GenesisDifficulty
		}
		result := new(big.Int).SetUint64(diff)
		
		log.Info("Linear difficulty progression",
			"height", nextHeight,
			"difficulty", result,
			"phase", "early_blocks")
		return result
	}

	// Dynamic adjustment for blocks 100+
	return calculateDynamicDifficulty(currentHeight, nextHeight, currentTimestamp, parent)
}

// calculateDynamicDifficulty computes difficulty based on actual block mining time
func calculateDynamicDifficulty(currentHeight, nextHeight, currentTimestamp uint64, parent *types.Header) *big.Int {
	parentTime := parent.Time
	var actualTime uint64

	// Calculate actual time since parent block
	if currentTimestamp > parentTime {
		actualTime = currentTimestamp - parentTime
	} else {
		actualTime = TargetBlockTimeSeconds
		log.Warn("Invalid timestamp, using target time",
			"current_timestamp", currentTimestamp,
			"parent_time", parentTime)
	}

	// Prevent division by zero
	if actualTime == 0 {
		actualTime = 1
	}

	parentDiff := parent.Difficulty.Uint64()
	var newDiffVal uint64

	log.Debug("Difficulty adjustment data",
		"parent_time", parentTime,
		"current_timestamp", currentTimestamp,
		"actual_time", actualTime,
		"target_time", TargetBlockTimeSeconds,
		"parent_difficulty", parentDiff)

	// Apply difficulty adjustment based on block time
	if actualTime < TargetBlockTimeSeconds {
		// Blocks too fast → increase difficulty
		newDiffVal = increaseDifficulty(parentDiff, actualTime)
	} else if actualTime > TargetBlockTimeSeconds {
		// Blocks too slow → decrease difficulty
		newDiffVal = decreaseDifficulty(parentDiff, actualTime)
	} else {
		// Perfect timing → maintain difficulty
		newDiffVal = parentDiff
		log.Debug("Difficulty unchanged - perfect timing", "difficulty", newDiffVal)
	}

	// Enforce minimum difficulty
	if newDiffVal < GenesisDifficulty {
		newDiffVal = GenesisDifficulty
		log.Warn("Difficulty below minimum, adjusting", "new_difficulty", newDiffVal)
	}

	newDiff := new(big.Int).SetUint64(newDiffVal)

	log.Info("Difficulty calculated",
		"height", nextHeight,
		"actual_time", actualTime,
		"target_time", TargetBlockTimeSeconds,
		"parent_difficulty", parentDiff,
		"new_difficulty", newDiff)

	return newDiff
}

// increaseDifficulty calculates the new difficulty when blocks are mined too quickly
func increaseDifficulty(parentDiff, actualTime uint64) uint64 {
	// Proportional increase based on time ratio
	ratio := float64(TargetBlockTimeSeconds) / float64(actualTime)
	increase := uint64(float64(parentDiff) * (ratio - 1.0))

	// Apply maximum adjustment cap
	maxIncrease := parentDiff * MaxAdjustmentPercent / 100
	if increase > maxIncrease {
		increase = maxIncrease
	}
	if increase < 1 {
		increase = 1
	}

	newDiff := parentDiff + increase
	log.Debug("Increasing difficulty",
		"ratio", ratio,
		"increase", increase,
		"new_difficulty", newDiff)

	return newDiff
}

// decreaseDifficulty calculates the new difficulty when blocks are mined too slowly
func decreaseDifficulty(parentDiff, actualTime uint64) uint64 {
	// Proportional decrease based on time ratio
	ratio := float64(actualTime) / float64(TargetBlockTimeSeconds)
	decrease := uint64(float64(parentDiff) * (ratio - 1.0))

	// Apply maximum adjustment cap
	maxDecrease := parentDiff * MaxAdjustmentPercent / 100
	if decrease > maxDecrease {
		decrease = maxDecrease
	}
	if decrease < 1 {
		decrease = 1
	}

	var newDiff uint64
	if parentDiff > decrease {
		newDiff = parentDiff - decrease
	} else {
		newDiff = GenesisDifficulty
	}

	log.Debug("Decreasing difficulty",
		"ratio", ratio,
		"decrease", decrease,
		"new_difficulty", newDiff)

	return newDiff
}

// CalculateNextDifficulty is the main exported function for external use
func CalculateNextDifficulty(parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
	return CalcDifficulty(nil, 0, parent, getHeader)
}

// GetDifficultyInfo returns comprehensive difficulty information for the current state
func GetDifficultyInfo(parent *types.Header, getHeader func(uint64) *types.Header) map[string]interface{} {
	currentHeight := parent.Number.Uint64()
	nextDifficulty := CalculateNextDifficulty(parent, getHeader)

	return map[string]interface{}{
		"current_height":          currentHeight,
		"next_height":             currentHeight + 1,
		"current_difficulty":      parent.Difficulty.String(),
		"next_difficulty":         nextDifficulty.String(),
		"target_block_time_sec":   TargetBlockTimeSeconds,
		"genesis_difficulty":      GenesisDifficulty,
		"initial_difficulty":      InitialDifficulty,
		"max_adjustment_percent":  MaxAdjustmentPercent,
		"linear_progression_blocks": LinearProgressionBlocks,
	}
}
