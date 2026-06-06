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
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// Difficulty calculator bomb delays for Ethereum proof-of-work hard forks.
var (
	byzantiumDifficultyBombDelay      = big.NewInt(3000000)
	constantinopleDifficultyBombDelay = big.NewInt(5000000)
	muirGlacierDifficultyBombDelay    = big.NewInt(9000000)
	arrowGlacierDifficultyBombDelay   = big.NewInt(10700000)
	grayGlacierDifficultyBombDelay    = big.NewInt(11400000)
)

// CalcDifficulty calculates the difficulty of a header at the given time.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header, _ func(number uint64) *types.Header) *big.Int {
	next := new(big.Int).Add(parent.Number, big.NewInt(1))
	if config != nil {
		switch {
		case config.IsGrayGlacier(next):
			return DynamicDifficultyCalculator(grayGlacierDifficultyBombDelay)(time, parent)
		case config.IsArrowGlacier(next):
			return DynamicDifficultyCalculator(arrowGlacierDifficultyBombDelay)(time, parent)
		case config.IsMuirGlacier(next):
			return DynamicDifficultyCalculator(muirGlacierDifficultyBombDelay)(time, parent)
		case config.IsConstantinople(next):
			return DynamicDifficultyCalculator(constantinopleDifficultyBombDelay)(time, parent)
		case config.IsByzantium(next):
			return DynamicDifficultyCalculator(byzantiumDifficultyBombDelay)(time, parent)
		case config.IsHomestead(next):
			return HomesteadDifficultyCalculator(time, parent)
		}
	}
	return FrontierDifficultyCalculator(time, parent)
}

// FrontierDifficultyCalculator is the Frontier difficulty adjustment algorithm.
func FrontierDifficultyCalculator(time uint64, parent *types.Header) *big.Int {
	adjust := new(big.Int).Div(parent.Difficulty, params.DifficultyBoundDivisor)
	difficulty := new(big.Int).Set(parent.Difficulty)
	if time-parent.Time < params.DurationLimit.Uint64() {
		difficulty.Add(difficulty, adjust)
	} else {
		difficulty.Sub(difficulty, adjust)
	}
	return addDifficultyBomb(ensureMinimumDifficulty(difficulty), new(big.Int).Add(parent.Number, big.NewInt(1)))
}

// HomesteadDifficultyCalculator is the Homestead difficulty adjustment algorithm.
func HomesteadDifficultyCalculator(time uint64, parent *types.Header) *big.Int {
	return calcDifficultyHomestead(time, parent, new(big.Int).Add(parent.Number, big.NewInt(1)))
}

// DynamicDifficultyCalculator returns a Byzantium-style calculator with a delayed difficulty bomb.
func DynamicDifficultyCalculator(bombDelay *big.Int) func(time uint64, parent *types.Header) *big.Int {
	return func(time uint64, parent *types.Header) *big.Int {
		fakeBlockNumber := new(big.Int).Add(parent.Number, big.NewInt(1))
		if fakeBlockNumber.Cmp(bombDelay) >= 0 {
			fakeBlockNumber.Sub(fakeBlockNumber, bombDelay)
		} else {
			fakeBlockNumber.SetUint64(0)
		}
		return calcDifficultyByzantium(time, parent, fakeBlockNumber)
	}
}

// CalcDifficultyFrontierU256 calculates Frontier difficulty.
func CalcDifficultyFrontierU256(time uint64, parent *types.Header) *big.Int {
	return FrontierDifficultyCalculator(time, parent)
}

// CalcDifficultyHomesteadU256 calculates Homestead difficulty.
func CalcDifficultyHomesteadU256(time uint64, parent *types.Header) *big.Int {
	return HomesteadDifficultyCalculator(time, parent)
}

// MakeDifficultyCalculatorU256 returns a dynamic difficulty calculator.
func MakeDifficultyCalculatorU256(bombDelay *big.Int) func(time uint64, parent *types.Header) *big.Int {
	return DynamicDifficultyCalculator(bombDelay)
}

func calcDifficultyHomestead(time uint64, parent *types.Header, bombBlockNumber *big.Int) *big.Int {
	x := new(big.Int).SetUint64((time - parent.Time) / 10)
	x.Sub(big.NewInt(1), x)
	return calcDifficultyWithAdjustment(parent, x, bombBlockNumber)
}

func calcDifficultyByzantium(time uint64, parent *types.Header, bombBlockNumber *big.Int) *big.Int {
	x := new(big.Int).SetUint64((time - parent.Time) / 9)
	if parent.UncleHash == types.EmptyUncleHash {
		x.Sub(big.NewInt(1), x)
	} else {
		x.Sub(big.NewInt(2), x)
	}
	return calcDifficultyWithAdjustment(parent, x, bombBlockNumber)
}

func calcDifficultyWithAdjustment(parent *types.Header, adjustmentFactor *big.Int, bombBlockNumber *big.Int) *big.Int {
	if adjustmentFactor.Cmp(big.NewInt(-99)) < 0 {
		adjustmentFactor.SetInt64(-99)
	}
	adjust := new(big.Int).Div(parent.Difficulty, params.DifficultyBoundDivisor)
	adjust.Mul(adjust, adjustmentFactor)
	difficulty := new(big.Int).Add(parent.Difficulty, adjust)
	return addDifficultyBomb(ensureMinimumDifficulty(difficulty), bombBlockNumber)
}

func ensureMinimumDifficulty(difficulty *big.Int) *big.Int {
	if difficulty.Cmp(params.MinimumDifficulty) < 0 {
		return new(big.Int).Set(params.MinimumDifficulty)
	}
	return difficulty
}

func addDifficultyBomb(difficulty *big.Int, blockNumber *big.Int) *big.Int {
	periodCount := new(big.Int).Div(blockNumber, big.NewInt(100000))
	if periodCount.Cmp(big.NewInt(1)) > 0 {
		bomb := new(big.Int).Sub(periodCount, big.NewInt(2))
		bomb.Exp(big.NewInt(2), bomb, nil)
		difficulty.Add(difficulty, bomb)
	}
	return difficulty
}

// Difficulty constants
const (
	DifficultyWindow = 720 // DIFFICULTY_WINDOW - number of blocks for difficulty calculation
	DifficultyCut    = 60  // DIFFICULTY_CUT - number of blocks to cut from each end
	TargetSeconds    = 120 // Target block time in seconds
)

// MaxUint256 is the maximum 256-bit integer (2^256 - 1)
var MaxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

// CheckHash64 checks if a hash meets the difficulty requirement (64-bit version)
func CheckHash64(hash []byte, difficulty uint64) bool {
	if len(hash) < 32 {
		return false
	}

	// Convert hash words to little-endian
	hashWords := make([]uint64, 4)
	for i := 0; i < 4; i++ {
		hashWords[i] = binary.LittleEndian.Uint64(hash[i*8 : (i+1)*8])
	}

	var low, high, top, cur uint64

	// Check the highest word first
	top, high = mul128(hashWords[3], difficulty)
	if high != 0 {
		return false
	}

	// Process each word
	low, cur = mul128(hashWords[0], difficulty)
	low, high = mul128(hashWords[1], difficulty)

	carry := addWithCarry(cur, low, false)
	cur = high

	low, high = mul128(hashWords[2], difficulty)
	carry = addWithCarry(cur, low, carry)
	carry = addWithCarry(high, top, carry)

	return !carry
}

// CheckHash128 checks if a hash meets the difficulty requirement (128-bit version)
func CheckHash128(hash []byte, difficulty *big.Int) bool {
	if len(hash) < 32 {
		return false
	}

	// Convert hash to big integer (little-endian)
	hashVal := new(big.Int)
	for i := 3; i >= 0; i-- {
		word := binary.LittleEndian.Uint64(hash[i*8 : (i+1)*8])
		hashVal.Lsh(hashVal, 64)
		hashVal.Or(hashVal, new(big.Int).SetUint64(word))
	}

	// Check if hash * difficulty <= MaxUint256
	product := new(big.Int).Mul(hashVal, difficulty)
	return product.Cmp(MaxUint256) <= 0
}

// CheckHash checks if a hash meets the difficulty requirement
func CheckHash(hash []byte, difficulty *big.Int) bool {
	// If difficulty fits in 64 bits, use faster check
	if difficulty.IsUint64() {
		return CheckHash64(hash, difficulty.Uint64())
	}
	return CheckHash128(hash, difficulty)
}

// NextDifficulty calculates the next difficulty based on timestamps and cumulative difficulties
func NextDifficulty(timestamps []uint64, cumulativeDifficulties []*big.Int, targetSeconds uint64) *big.Int {
	// Ensure we don't have more than DIFFICULTY_WINDOW entries
	if len(timestamps) > DifficultyWindow {
		timestamps = timestamps[len(timestamps)-DifficultyWindow:]
		cumulativeDifficulties = cumulativeDifficulties[len(cumulativeDifficulties)-DifficultyWindow:]
	}

	length := len(timestamps)
	if length <= 1 {
		return big.NewInt(1)
	}

	if length > DifficultyWindow {
		length = DifficultyWindow
	}

	// Sort timestamps for median calculation
	sortedTimestamps := make([]uint64, length)
	copy(sortedTimestamps, timestamps)
	sort.Slice(sortedTimestamps, func(i, j int) bool { return sortedTimestamps[i] < sortedTimestamps[j] })

	// Determine cut boundaries
	cutBegin, cutEnd := 0, length
	if length > DifficultyWindow-2*DifficultyCut {
		cutBegin = (length - (DifficultyWindow - 2*DifficultyCut) + 1) / 2
		cutEnd = cutBegin + (DifficultyWindow - 2*DifficultyCut)
	}

	if cutBegin+2 > cutEnd || cutEnd > length {
		return big.NewInt(1)
	}

	// Calculate time span
	timeSpan := sortedTimestamps[cutEnd-1] - sortedTimestamps[cutBegin]
	if timeSpan == 0 {
		timeSpan = 1
	}

	// Calculate total work
	totalWork := new(big.Int).Sub(cumulativeDifficulties[cutEnd-1], cumulativeDifficulties[cutBegin])
	if totalWork.Sign() <= 0 {
		return big.NewInt(1)
	}

	// Calculate new difficulty: (totalWork * targetSeconds + timeSpan - 1) / timeSpan
	temp := new(big.Int).Mul(totalWork, new(big.Int).SetUint64(targetSeconds))
	temp.Add(temp, new(big.Int).SetUint64(timeSpan-1))
	newDiff := new(big.Int).Div(temp, new(big.Int).SetUint64(timeSpan))

	// Ensure difficulty is at least 1
	if newDiff.Sign() == 0 {
		return big.NewInt(1)
	}

	return newDiff
}

// mul128 multiplies two 64-bit numbers and returns low and high 64-bit parts
func mul128(a, b uint64) (low, high uint64) {
	// Use big integer for multiplication
	temp := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
	low = temp.Uint64()
	high = temp.Rsh(temp, 64).Uint64()
	return
}

// addWithCarry adds two numbers with a carry flag
func addWithCarry(a, b uint64, carry bool) bool {
	sum := a + b
	if carry {
		sum++
	}
	return sum < a || (carry && sum == a)
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

// Hex returns the hexadecimal representation of a difficulty value
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
