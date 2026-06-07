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
//	TargetBlockTimeSeconds = 120
	MinimumDifficultyValue = 3
	MaxAdjustmentPercent   = 10
)

// CalcDifficulty calculates the difficulty for the next block.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
	currentHeight := parent.Number.Uint64()
	nextHeight := currentHeight + 1

	log.Info("========== DIFFICULTY CALC ==========", 
		"currentHeight", currentHeight, 
		"nextHeight", nextHeight,
		"parent_diff", parent.Difficulty.String())

	if currentHeight == 0 {
		diff := new(big.Int).SetUint64(MinimumDifficultyValue)
		log.Info("Genesis difficulty", "difficulty", diff)
		return diff
	}

	if currentHeight < 100 {
		diff := MinimumDifficultyValue + (currentHeight / 2)
		if diff < MinimumDifficultyValue {
			diff = MinimumDifficultyValue
		}
		result := new(big.Int).SetUint64(diff)
		log.Info("Early block difficulty (linear)", "height", nextHeight, "difficulty", result)
		return result
	}

	parentTime := parent.Time
	var actualTime uint64
	if time > parentTime {
		actualTime = time - parentTime
	} else {
		actualTime = uint64(TargetBlockTimeSeconds)
	}

	targetTime := uint64(TargetBlockTimeSeconds)
	parentDiff := parent.Difficulty.Uint64()
	
	log.Info("Difficulty adjustment data",
		"parent_time", parentTime,
		"current_time", time,
		"actual_time", actualTime,
		"target_time", targetTime,
		"parent_diff", parentDiff)

	var newDiffVal uint64
	
	if actualTime < targetTime {
		var increase uint64
		if actualTime > 0 {
			calculated := uint64(float64(parentDiff) * float64(targetTime) / float64(actualTime))
			if calculated > parentDiff {
				increase = calculated - parentDiff
			}
		} else {
			increase = parentDiff * MaxAdjustmentPercent / 100
		}
		
		maxIncrease := parentDiff * MaxAdjustmentPercent / 100
		if increase > maxIncrease {
			increase = maxIncrease
		}
		if increase < 1 {
			increase = 1
		}
		newDiffVal = parentDiff + increase
		log.Info("Increasing difficulty", "increase", increase, "new", newDiffVal)
	} else if actualTime > targetTime {
		var decrease uint64
		calculated := uint64(float64(parentDiff) * float64(targetTime) / float64(actualTime))
		if calculated < parentDiff {
			decrease = parentDiff - calculated
		}
		
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
			newDiffVal = MinimumDifficultyValue
		}
		log.Info("Decreasing difficulty", "decrease", decrease, "new", newDiffVal)
	} else {
		newDiffVal = parentDiff
		log.Info("Difficulty unchanged", "new", newDiffVal)
	}

	if newDiffVal < MinimumDifficultyValue {
		newDiffVal = MinimumDifficultyValue
	}

	newDiff := new(big.Int).SetUint64(newDiffVal)

	log.Info("Difficulty calculated", "height", nextHeight, "new_diff", newDiff)
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
