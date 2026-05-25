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
        // TargetBlockTimeSeconds is the desired time between blocks
//        TargetBlockTimeSeconds = 120

        // MinimumDifficulty is the absolute lowest allowed difficulty
        MinimumDifficulty = 3

        // MaxAdjustmentPercent is the maximum difficulty change per block (10%)
        MaxAdjustmentPercent = 10
)

// CalcDifficulty calculates the difficulty for the next block.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
        currentHeight := parent.Number.Uint64()
        nextHeight := currentHeight + 1

        // Genesis block: use minimum difficulty
        if currentHeight == 0 {
                log.Info("Initializing difficulty for genesis block", "difficulty", MinimumDifficulty)
                return new(big.Int).SetUint64(MinimumDifficulty)
        }

        // For first 100 blocks, use simple linear progression
        if currentHeight < 100 {
                diff := MinimumDifficulty + (currentHeight / 2)
                if diff < MinimumDifficulty {
                        diff = MinimumDifficulty
                }
                result := new(big.Int).SetUint64(diff)
                log.Debug("Early block difficulty (linear)", "height", nextHeight, "difficulty", result)
                return result
        }

        // Calculate actual time since parent block
        parentTime := parent.Time
        var actualTime uint64
        if time > parentTime {
                actualTime = time - parentTime
        } else {
                actualTime = uint64(TargetBlockTimeSeconds) // Convert to uint64
        }

        targetTime := uint64(TargetBlockTimeSeconds) // Convert to uint64
        parentDiff := parent.Difficulty.Uint64()

        // Calculate difficulty adjustment based on time ratio
        var newDiffVal uint64
        
        if actualTime < targetTime {
                // Block too fast → increase difficulty
                // Calculate proportional increase
                var increase uint64
                if actualTime > 0 {
                        // new_diff = parent_diff * target_time / actual_time
                        // But limit to 10% max
                        calculated := uint64(float64(parentDiff) * float64(targetTime) / float64(actualTime))
                        if calculated > parentDiff {
                                increase = calculated - parentDiff
                        }
                } else {
                        increase = parentDiff * MaxAdjustmentPercent / 100
                }
                
                // Limit to 10% max increase
                maxIncrease := parentDiff * MaxAdjustmentPercent / 100
                if increase > maxIncrease {
                        increase = maxIncrease
                }
                if increase < 1 {
                        increase = 1
                }
                newDiffVal = parentDiff + increase
                log.Debug("Increasing difficulty", 
                        "actual", actualTime, "target", targetTime, 
                        "increase", increase, "new", newDiffVal)
        } else if actualTime > targetTime {
                // Block too slow → decrease difficulty
                // Calculate proportional decrease
                var decrease uint64
                // new_diff = parent_diff * target_time / actual_time
                calculated := uint64(float64(parentDiff) * float64(targetTime) / float64(actualTime))
                if calculated < parentDiff {
                        decrease = parentDiff - calculated
                }
                
                // Limit to 10% max decrease
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
                        newDiffVal = MinimumDifficulty
                }
                log.Debug("Decreasing difficulty", 
                        "actual", actualTime, "target", targetTime,
                        "decrease", decrease, "new", newDiffVal)
        } else {
                // Perfect timing, keep same difficulty
                newDiffVal = parentDiff
        }

        if newDiffVal < MinimumDifficulty {
                newDiffVal = MinimumDifficulty
        }

        newDiff := new(big.Int).SetUint64(newDiffVal)

        log.Info("Difficulty calculated",
                "height", nextHeight,
                "actual_time", actualTime,
                "target_time", targetTime,
                "parent_diff", parentDiff,
                "new_diff", newDiff)

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
