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
	"sort"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// RandomX difficulty constants
const (
	DifficultyWindow       = 720
	DifficultyCut          = 60
	TargetBlockTime        = 120
	MinimumDifficultyValue = 1310
	MaxAdjustmentPercent   = 10
)

// MinimumDifficulty as *big.Int
var MinimumDifficulty = new(big.Int).SetUint64(MinimumDifficultyValue)

// MaxUint256 is the maximum 256-bit integer (2^256 - 1)
var MaxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

// CalcDifficulty calculates the difficulty for the next block.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, getHeader func(number uint64) *types.Header) *big.Int {
	next := new(big.Int).Add(parent.Number, big.NewInt(1))
	
	// For first 100 blocks, use simple linear progression
	if next.Uint64() < 100 {
		diff := MinimumDifficultyValue + (next.Uint64() / 2)
		if diff < MinimumDifficultyValue {
			diff = MinimumDifficultyValue
		}
		return new(big.Int).SetUint64(diff)
	}
	
	// Get timestamps and difficulties for the last DifficultyWindow blocks
	var timestamps []uint64
	var difficulties []*big.Int
	
	// Collect parent blocks
	current := parent
	for i := 0; i < DifficultyWindow && current != nil; i++ {
		timestamps = append([]uint64{current.Time}, timestamps...)
		difficulties = append([]*big.Int{current.Difficulty}, difficulties...)
		if current.Number.Uint64() == 0 {
			break
		}
		current = getHeader(current.Number.Uint64() - 1)
	}
	
	if len(timestamps) < 2 {
		return new(big.Int).Set(parent.Difficulty)
	}
	
	// Calculate median timestamp (remove outliers)
	sortedTimestamps := make([]uint64, len(timestamps))
	copy(sortedTimestamps, timestamps)
	sort.Slice(sortedTimestamps, func(i, j int) bool { return sortedTimestamps[i] < sortedTimestamps[j] })
	
	cut := len(sortedTimestamps) / 10
	if cut < 1 {
		cut = 1
	}
	trimmedTimestamps := sortedTimestamps[cut : len(sortedTimestamps)-cut]
	
	// Calculate average block time
	var totalTime uint64
	for i := 1; i < len(trimmedTimestamps); i++ {
		totalTime += trimmedTimestamps[i] - trimmedTimestamps[i-1]
	}
	avgBlockTime := totalTime / uint64(len(trimmedTimestamps)-1)
	if avgBlockTime == 0 {
		avgBlockTime = 1
	}
	
	// Calculate difficulty adjustment
	parentDiff := parent.Difficulty.Uint64()
	var newDiffVal uint64
	
	if avgBlockTime < TargetBlockTime {
		increase := parentDiff * (TargetBlockTime - avgBlockTime) / TargetBlockTime
		maxIncrease := parentDiff * MaxAdjustmentPercent / 100
		if increase > maxIncrease {
			increase = maxIncrease
		}
		if increase < 1 {
			increase = 1
		}
		newDiffVal = parentDiff + increase
	} else if avgBlockTime > TargetBlockTime {
		decrease := parentDiff * (avgBlockTime - TargetBlockTime) / TargetBlockTime
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
	} else {
		newDiffVal = parentDiff
	}
	
	if newDiffVal < MinimumDifficultyValue {
		newDiffVal = MinimumDifficultyValue
	}
	
	return new(big.Int).SetUint64(newDiffVal)
}

// CalculateNextDifficulty is the main exported function for difficulty calculation
func CalculateNextDifficulty(parent *types.Header, getHeader func(uint64) *types.Header) *big.Int {
	return CalcDifficulty(nil, 0, parent, getHeader)
}

// CheckHash64 checks if a hash meets the difficulty requirement (64-bit version)
func CheckHash64(hash []byte, difficulty uint64) bool {
	if len(hash) < 32 {
		return false
	}
	
	hashVal := new(big.Int)
	for i := 31; i >= 0; i-- {
		hashVal.Lsh(hashVal, 8)
		hashVal.Or(hashVal, new(big.Int).SetUint64(uint64(hash[i])))
	}
	
	target := new(big.Int).Div(MaxUint256, new(big.Int).SetUint64(difficulty))
	return hashVal.Cmp(target) <= 0
}

// CheckHash128 checks if a hash meets the difficulty requirement (128-bit version)
func CheckHash128(hash []byte, difficulty *big.Int) bool {
	if len(hash) < 32 {
		return false
	}
	
	hashVal := new(big.Int)
	for i := 31; i >= 0; i-- {
		hashVal.Lsh(hashVal, 8)
		hashVal.Or(hashVal, new(big.Int).SetUint64(uint64(hash[i])))
	}
	
	product := new(big.Int).Mul(hashVal, difficulty)
	return product.Cmp(MaxUint256) <= 0
}

// CheckHash checks if a hash meets the difficulty requirement
func CheckHash(hash []byte, difficulty *big.Int) bool {
	if difficulty.IsUint64() {
		return CheckHash64(hash, difficulty.Uint64())
	}
	return CheckHash128(hash, difficulty)
}

// DifficultyToTarget converts a difficulty value to a target hash
func DifficultyToTarget(difficulty *big.Int) *big.Int {
	if difficulty.Sign() == 0 {
		return new(big.Int).Set(MaxUint256)
	}
	return new(big.Int).Div(MaxUint256, difficulty)
}

// TargetToDifficulty converts a target hash to a difficulty value
func TargetToDifficulty(target *big.Int) *big.Int {
	if target.Sign() == 0 {
		return new(big.Int).Set(MaxUint256)
	}
	return new(big.Int).Div(MaxUint256, target)
}

// DifficultyHex returns the hexadecimal representation of a difficulty value
func DifficultyHex(diff *big.Int) string {
	if diff.Sign() == 0 {
		return "0x0"
	}
	return "0x" + diff.Text(16)
}

// ParseDifficulty parses a hexadecimal string to a difficulty big.Int
func ParseDifficulty(hexStr string) (*big.Int, error) {
	if len(hexStr) >= 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}
	if hexStr == "" {
		return big.NewInt(0), nil
	}
	diff := new(big.Int)
	_, ok := diff.SetString(hexStr, 16)
	if !ok {
		return nil, fmt.Errorf("invalid difficulty hex string: %s", hexStr)
	}
	return diff, nil
}
