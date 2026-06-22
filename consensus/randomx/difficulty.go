// Copyright 2026 The go-ethereum Authors

package randomx

import (
    "math/big"
    "time"

    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/log"
    "github.com/ethereum/go-ethereum/params"
)

// Difficulty constants
const (
    // TargetBlockTimeSeconds is the desired time between blocks (e.g. 120 = 2 minutes)
//    TargetBlockTimeSeconds uint64 = 120

    // InitialDifficulty is the difficulty for early blocks
    InitialDifficulty uint64 = 50

    // MaxAdjustmentPercent limits difficulty change per block
    MaxAdjustmentPercent uint64 = 25

    // LinearProgressionBlocks defines how many blocks use linear growth
    LinearProgressionBlocks uint64 = 100
)

var GenesisDifficulty = big.NewInt(2440)


// CalcDifficulty calculates the difficulty for the next block
func CalcDifficulty(config *params.ChainConfig, blockTime uint64, parent *types.Header, getHeader func(number uint64) *types.Header) *big.Int {
    currentHeight := parent.Number.Uint64()
    nextHeight := currentHeight + 1

    // Use current system time if blockTime is not provided
    currentTimestamp := blockTime
    if currentTimestamp == 0 {
        currentTimestamp = uint64(time.Now().Unix())
    }

    log.Debug("Difficulty calculation started",
        "current_height", currentHeight,
        "next_height", nextHeight,
        "parent_difficulty", parent.Difficulty)

    // Genesis block
    if currentHeight == 0 {
        log.Info("Genesis block difficulty", "height", nextHeight, "difficulty", GenesisDifficulty)
        return new(big.Int).Set(GenesisDifficulty)
    }

    // Linear progression for early blocks (smooth ramp-up)
    if currentHeight < LinearProgressionBlocks {
        targetMaxDiff := new(big.Int).SetUint64(InitialDifficulty)

        // Linear interpolation: GenesisDifficulty → InitialDifficulty
        diff := new(big.Int).Sub(targetMaxDiff, GenesisDifficulty)
        diff.Mul(diff, new(big.Int).SetUint64(currentHeight))
        diff.Div(diff, new(big.Int).SetUint64(LinearProgressionBlocks))
        diff.Add(diff, GenesisDifficulty)

        // Ensure we don't go below genesis difficulty
        if diff.Cmp(GenesisDifficulty) < 0 {
            diff.Set(GenesisDifficulty)
        }

        log.Info("Linear difficulty progression",
            "height", nextHeight,
            "difficulty", diff,
            "phase", "early_blocks")

        return diff
    }

    // Dynamic adjustment after linear phase
    return calculateDynamicDifficulty(currentHeight, nextHeight, currentTimestamp, parent)
}

// calculateDynamicDifficulty computes difficulty based on actual block time
func calculateDynamicDifficulty(currentHeight, nextHeight, currentTimestamp uint64, parent *types.Header) *big.Int {
    parentTime := parent.Time
    actualTime := uint64(0)

    if currentTimestamp > parentTime {
        actualTime = currentTimestamp - parentTime
    }

    if actualTime == 0 {
        actualTime = TargetBlockTimeSeconds
        log.Warn("Invalid timestamp, using target time", "timestamp", currentTimestamp)
    }

    parentDiff := new(big.Int).Set(parent.Difficulty)

    log.Debug("Difficulty adjustment",
        "height", nextHeight,
        "actual_time", actualTime,
        "target_time", TargetBlockTimeSeconds,
        "parent_diff", parentDiff)

    var newDiff *big.Int

    if actualTime < TargetBlockTimeSeconds {
        // Blocks too fast → increase difficulty
        newDiff = increaseDifficulty(parentDiff, actualTime)
    } else if actualTime > TargetBlockTimeSeconds {
        // Blocks too slow → decrease difficulty
        newDiff = decreaseDifficulty(parentDiff, actualTime)
    } else {
        newDiff = new(big.Int).Set(parentDiff)
        log.Debug("Difficulty unchanged - perfect timing")
    }

    // Enforce minimum difficulty
    if newDiff.Cmp(GenesisDifficulty) < 0 {
        newDiff.Set(GenesisDifficulty)
        log.Warn("Difficulty below minimum, clamped", "new_diff", newDiff)
    }

    log.Info("Difficulty calculated",
        "height", nextHeight,
        "actual_time", actualTime,
        "parent_diff", parentDiff,
        "new_difficulty", newDiff)

    return newDiff
}

// increaseDifficulty when blocks are mined too fast
func increaseDifficulty(parentDiff *big.Int, actualTime uint64) *big.Int {
    ratio := float64(TargetBlockTimeSeconds) / float64(actualTime)
    increase := new(big.Int).Set(parentDiff)
    increase = big.NewInt(int64(float64(increase.Int64()) * (ratio - 1.0)))

    maxIncrease := new(big.Int).Div(
        new(big.Int).Mul(parentDiff, big.NewInt(int64(MaxAdjustmentPercent))),
        big.NewInt(100),
    )

    if increase.Cmp(maxIncrease) > 0 {
        increase.Set(maxIncrease)
    }
    if increase.Sign() <= 0 {
        increase.SetInt64(1)
    }

    return new(big.Int).Add(parentDiff, increase)
}

// decreaseDifficulty when blocks are mined too slowly
func decreaseDifficulty(parentDiff *big.Int, actualTime uint64) *big.Int {
    ratio := float64(actualTime) / float64(TargetBlockTimeSeconds)
    decrease := new(big.Int).Set(parentDiff)
    decrease = big.NewInt(int64(float64(decrease.Int64()) * (ratio - 1.0)))

    maxDecrease := new(big.Int).Div(
        new(big.Int).Mul(parentDiff, big.NewInt(int64(MaxAdjustmentPercent))),
        big.NewInt(100),
    )

    if decrease.Cmp(maxDecrease) > 0 {
        decrease.Set(maxDecrease)
    }
    if decrease.Sign() <= 0 {
        decrease.SetInt64(1)
    }

    newDiff := new(big.Int).Sub(parentDiff, decrease)
    if newDiff.Cmp(GenesisDifficulty) < 0 {
        newDiff.Set(GenesisDifficulty)
    }

    return newDiff
}

// CalculateNextDifficulty is the main exported function used by the engine
/*func CalculateNextDifficulty(parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
    return CalcDifficulty(nil, 0, parent, getHeader)
}*/

func CalculateNextDifficulty(parent *types.Header, getHeaderByNumber func(uint64) *types.Header) *big.Int {
        if parent == nil {
                return GenesisDifficulty
        }
        return GenesisDifficulty
}

// GetDifficultyInfo returns debug information
func GetDifficultyInfo(parent *types.Header, getHeader func(uint64) *types.Header) map[string]interface{} {
    nextDifficulty := CalculateNextDifficulty(parent, getHeader)

    return map[string]interface{}{
        "current_height":            parent.Number.Uint64(),
        "next_height":               parent.Number.Uint64() + 1,
        "current_difficulty":        parent.Difficulty.String(),
        "next_difficulty":           nextDifficulty.String(),
        "target_block_time_sec":     TargetBlockTimeSeconds,
        "genesis_difficulty":        GenesisDifficulty.String(),
        "initial_difficulty":        InitialDifficulty,
        "max_adjustment_percent":    MaxAdjustmentPercent,
        "linear_progression_blocks": LinearProgressionBlocks,
    }
}
