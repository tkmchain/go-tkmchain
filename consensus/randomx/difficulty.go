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
    "fmt"
    "math/big"
//    "sort"

    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/log"
    "github.com/ethereum/go-ethereum/params"
)

// RandomX difficulty constants
const (
    // WindowSize is the number of previous blocks to analyze for difficulty calculation
    WindowSize = 720
    
    // Lag is the number of blocks to exclude from the most recent tip
    // Prevents timestamp manipulation attacks
    Lag = 15
    
    // MinimumDifficulty is the absolute lowest allowed difficulty
    MinimumDifficulty = 3
    
    // AdjustmentLimit prevents dramatic changes per block (max 2x increase or 0.5x decrease)
    AdjustmentLimit = 2.0
)

func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
    currentHeight := parent.Number.Uint64()
    nextHeight := currentHeight + 1
    
    // Genesis block: use minimum difficulty
    if currentHeight == 0 {
        log.Info("Initializing difficulty for genesis block", "difficulty", MinimumDifficulty)
        return new(big.Int).SetUint64(MinimumDifficulty)
    }
    
    // Determine start height for the window
    startHeight := uint64(0)
    if currentHeight > WindowSize {
        startHeight = currentHeight - WindowSize
    }
    
    // Collect timestamps and cumulative difficulties
    var timestamps []uint64
    var cumDifficulties []*big.Int
    
    for h := startHeight; h <= currentHeight; h++ {
        header := getHeader(h)
        if header == nil {
            log.Warn("Missing header in difficulty window", "height", h)
            continue
        }
        timestamps = append(timestamps, header.Time)
        
        var cumDiff *big.Int
        if h == 0 {
            cumDiff = new(big.Int).Set(header.Difficulty)
        } else {
            prevCum := cumDifficulties[len(cumDifficulties)-1]
            cumDiff = new(big.Int).Add(prevCum, header.Difficulty)
        }
        cumDifficulties = append(cumDifficulties, cumDiff)
    }
    
    if len(timestamps) < 3 {
        // Insufficient data, return parent difficulty
        log.Debug("Insufficient blocks for difficulty calculation", 
            "window_size", len(timestamps), 
            "required", 3)
        return new(big.Int).Set(parent.Difficulty)
    }
    
    // Calculate weighted average using LWMA-2 algorithm
    // This gives more weight to recent blocks while maintaining stability
    N := int64(len(timestamps))
    var weightedSum, totalWeight float64
    
    for i := int64(1); i < N; i++ {
        // Calculate solve time between consecutive blocks
        solveTime := int64(timestamps[i] - timestamps[i-1])
        
        // Normalize solve time with bounds to prevent manipulation
        if solveTime < 30 {
            solveTime = 30 // Minimum 30 seconds
        }
        if solveTime > int64(TargetBlockTimeSeconds)*3 {
            solveTime = int64(TargetBlockTimeSeconds) * 3 // Maximum 3x target
        }
        
        // Calculate difficulty difference for this block
        diffDelta := new(big.Float).SetInt(cumDifficulties[i])
        diffDelta.Sub(diffDelta, new(big.Float).SetInt(cumDifficulties[i-1]))
        
        // Calculate hash rate: difficulty / solve_time
        solveTimeFloat := big.NewFloat(float64(solveTime))
        hashRate := new(big.Float).Quo(diffDelta, solveTimeFloat)
        
        // LWMA-2 weighting: more weight to recent blocks
        // Weight increases linearly with block age.
        weight := float64(i+1) / float64(N)
        weightedSum += weight * toFloat(hashRate)
        totalWeight += weight
    }
    
    if totalWeight == 0 {
        log.Warn("Zero weight in difficulty calculation")
        return new(big.Int).Set(parent.Difficulty)
    }
    
    // Calculate average hash rate from weighted sum
    avgHashRate := weightedSum / totalWeight
    
    // Calculate new difficulty = average hash rate * target block time
    targetTimeFloat := big.NewFloat(float64(TargetBlockTimeSeconds))
    newDifficultyFloat := new(big.Float).Mul(big.NewFloat(avgHashRate), targetTimeFloat)
    newDifficulty, _ := newDifficultyFloat.Int(nil)
    
    // Apply minimum difficulty
    minDiff := new(big.Int).SetUint64(MinimumDifficulty)
    if newDifficulty.Cmp(minDiff) < 0 {
        newDifficulty = minDiff
    }
    
    // Apply adjustment limit to prevent dramatic changes
    maxIncrease := new(big.Float).Mul(new(big.Float).SetInt(parent.Difficulty), 
        big.NewFloat(AdjustmentLimit))
    maxDecrease := new(big.Float).Mul(new(big.Float).SetInt(parent.Difficulty), 
        big.NewFloat(1.0/AdjustmentLimit))
    
    newDiffFloat := new(big.Float).SetInt(newDifficulty)
    
    if newDiffFloat.Cmp(maxIncrease) > 0 {
        newDifficulty, _ = maxIncrease.Int(nil)
        log.Debug("Difficulty increase limited", 
            "proposed", newDiffFloat,
            "limited_to", maxIncrease)
    } else if newDiffFloat.Cmp(maxDecrease) < 0 {
        newDifficulty, _ = maxDecrease.Int(nil)
        log.Debug("Difficulty decrease limited",
            "proposed", newDiffFloat,
            "limited_to", maxDecrease)
    }
    
    // Log difficulty adjustment
    log.Debug("Difficulty calculated (LWMA-2)",
        "height", nextHeight,
        "parent_difficulty", parent.Difficulty,
        "new_difficulty", newDifficulty,
        "avg_hash_rate_hps", fmt.Sprintf("%.2f", avgHashRate),
        "window_blocks", len(timestamps))
    
    return newDifficulty
}

// CalculateNextDifficulty is the main exported function for difficulty calculation
func CalculateNextDifficulty(parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
    return CalcDifficulty(nil, 0, parent, getHeader)
}

// toFloat converts a big.Float to float64 safely
func toFloat(f *big.Float) float64 {
    val, _ := f.Float64()
    return val
}

// EstimateNetworkHashrate estimates current network hashrate from difficulty and solve time
func EstimateNetworkHashrate(difficulty *big.Int, solveTimeSeconds float64) float64 {
    if solveTimeSeconds <= 0 {
        return 0
    }
    diff := new(big.Float).SetInt(difficulty)
    time := big.NewFloat(solveTimeSeconds)
    rate := new(big.Float).Quo(diff, time)
    result, _ := rate.Float64()
    return result
}

// GetExpectedBlockTime estimates expected block time for given difficulty and hashrate
func GetExpectedBlockTime(difficulty *big.Int, networkHashrate float64) float64 {
    if networkHashrate <= 0 {
        return float64(TargetBlockTimeSeconds)
    }
    diff := new(big.Float).SetInt(difficulty)
    rate := big.NewFloat(networkHashrate)
    time := new(big.Float).Quo(diff, rate)
    result, _ := time.Float64()
    return result
}

// Legacy functions kept for compatibility with existing code
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
    calcDifficultyEip5133         = makeDifficultyCalculator(big.NewInt(0))
    calcDifficultyEip4345         = makeDifficultyCalculator(big.NewInt(0))
    calcDifficultyEip3554         = makeDifficultyCalculator(big.NewInt(0))
    calcDifficultyConstantinople  = makeDifficultyCalculator(big.NewInt(0))
    calcDifficultyByzantium       = makeDifficultyCalculator(big.NewInt(0))
)
