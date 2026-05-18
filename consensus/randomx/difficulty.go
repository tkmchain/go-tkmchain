// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package randomx

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// Difficulty calculation constants
const (
	difficultyBoundDivisor = 2048
	minimumDifficulty      = 131072
	expDiffPeriod          = 100000
	durationLimit          = 13
)

// CalcDifficulty computes the difficulty for a new block based on parent block.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.Header) *big.Int {
	next := new(big.Int).Add(parent.Number, big.NewInt(1))

	// Use appropriate difficulty calculator based on fork
	switch {
	case config.IsGrayGlacier(next):
		return calcDifficultyEip5133(time, parent)
	case config.IsArrowGlacier(next):
		return calcDifficultyEip4345(time, parent)
	case config.IsLondon(next):
		return calcDifficultyEip3554(time, parent)
	case config.IsMuirGlacier(next):
		return calcDifficultyEip2384(time, parent)
	case config.IsConstantinople(next):
		return calcDifficultyConstantinople(time, parent)
	case config.IsByzantium(next):
		return calcDifficultyByzantium(time, parent)
	case config.IsHomestead(next):
		return calcDifficultyHomestead(time, parent)
	default:
		return calcDifficultyFrontier(time, parent)
	}
}

// makeDifficultyCalculator creates a difficulty calculator with bomb delay.
func makeDifficultyCalculator(bombDelay *big.Int) func(time uint64, parent *types.Header) *big.Int {
	bombDelayFromParent := new(big.Int).Sub(bombDelay, big.NewInt(1))

	return func(time uint64, parent *types.Header) *big.Int {
		bigTime := new(big.Int).SetUint64(time)
		bigParentTime := new(big.Int).SetUint64(parent.Time)

		// Calculate adjustment factor
		x := new(big.Int).Sub(bigTime, bigParentTime)
		x.Div(x, big.NewInt(9))

		if parent.UncleHash == types.EmptyUncleHash {
			x.Sub(big.NewInt(1), x)
		} else {
			x.Sub(big.NewInt(2), x)
		}

		// Bound adjustment factor
		if x.Cmp(big.NewInt(-99)) < 0 {
			x.Set(big.NewInt(-99))
		}

		// Calculate new difficulty
		y := new(big.Int).Div(parent.Difficulty, big.NewInt(difficultyBoundDivisor))
		x.Mul(y, x)
		x.Add(parent.Difficulty, x)

		// Ensure minimum difficulty
		if x.Cmp(big.NewInt(minimumDifficulty)) < 0 {
			x.Set(big.NewInt(minimumDifficulty))
		}

		// Apply exponential difficulty bomb
		fakeBlockNumber := new(big.Int)
		if parent.Number.Cmp(bombDelayFromParent) >= 0 {
			fakeBlockNumber.Sub(parent.Number, bombDelayFromParent)
		}

		periodCount := new(big.Int).Div(fakeBlockNumber, big.NewInt(expDiffPeriod))
		if periodCount.Cmp(big.NewInt(1)) > 0 {
			expFactor := new(big.Int).Sub(periodCount, big.NewInt(2))
			expFactor.Exp(big.NewInt(2), expFactor, nil)
			x.Add(x, expFactor)
		}

		return x
	}
}

// calcDifficultyHomestead implements Homestead difficulty rules.
func calcDifficultyHomestead(time uint64, parent *types.Header) *big.Int {
	bigTime := new(big.Int).SetUint64(time)
	bigParentTime := new(big.Int).SetUint64(parent.Time)

	// Calculate adjustment
	x := new(big.Int).Sub(bigTime, bigParentTime)
	x.Div(x, big.NewInt(10))
	x.Sub(big.NewInt(1), x)

	if x.Cmp(big.NewInt(-99)) < 0 {
		x.Set(big.NewInt(-99))
	}

	// Apply adjustment
	y := new(big.Int).Div(parent.Difficulty, big.NewInt(difficultyBoundDivisor))
	x.Mul(y, x)
	x.Add(parent.Difficulty, x)

	// Ensure minimum
	if x.Cmp(big.NewInt(minimumDifficulty)) < 0 {
		x.Set(big.NewInt(minimumDifficulty))
	}

	// Apply bomb
	periodCount := new(big.Int).Add(parent.Number, big.NewInt(1))
	periodCount.Div(periodCount, big.NewInt(expDiffPeriod))

	if periodCount.Cmp(big.NewInt(1)) > 0 {
		y.Sub(periodCount, big.NewInt(2))
		y.Exp(big.NewInt(2), y, nil)
		x.Add(x, y)
	}

	return x
}

// calcDifficultyFrontier implements Frontier difficulty rules.
func calcDifficultyFrontier(time uint64, parent *types.Header) *big.Int {
	diff := new(big.Int)
	adjust := new(big.Int).Div(parent.Difficulty, big.NewInt(difficultyBoundDivisor))

	bigTime := new(big.Int).SetUint64(time)
	bigParentTime := new(big.Int).SetUint64(parent.Time)

	if new(big.Int).Sub(bigTime, bigParentTime).Cmp(big.NewInt(durationLimit)) < 0 {
		diff.Add(parent.Difficulty, adjust)
	} else {
		diff.Sub(parent.Difficulty, adjust)
	}

	if diff.Cmp(big.NewInt(minimumDifficulty)) < 0 {
		diff.Set(big.NewInt(minimumDifficulty))
	}

	// Apply bomb
	periodCount := new(big.Int).Add(parent.Number, big.NewInt(1))
	periodCount.Div(periodCount, big.NewInt(expDiffPeriod))

	if periodCount.Cmp(big.NewInt(1)) > 0 {
		expDiff := new(big.Int).Sub(periodCount, big.NewInt(2))
		expDiff.Exp(big.NewInt(2), expDiff, nil)
		diff.Add(diff, expDiff)
		if diff.Cmp(big.NewInt(minimumDifficulty)) < 0 {
			diff.Set(big.NewInt(minimumDifficulty))
		}
	}

	return diff
}

// Difficulty calculators for various EIPs
var (
	calcDifficultyEip5133        = makeDifficultyCalculator(big.NewInt(11_400_000))
	calcDifficultyEip4345        = makeDifficultyCalculator(big.NewInt(10_700_000))
	calcDifficultyEip3554        = makeDifficultyCalculator(big.NewInt(9_700_000))
	calcDifficultyEip2384        = makeDifficultyCalculator(big.NewInt(9_000_000))
	calcDifficultyConstantinople = makeDifficultyCalculator(big.NewInt(5_000_000))
	calcDifficultyByzantium      = makeDifficultyCalculator(big.NewInt(3_000_000))
)

// Exported for fuzzing compatibility.
var FrontierDifficultyCalculator = calcDifficultyFrontier
var HomesteadDifficultyCalculator = calcDifficultyHomestead
var DynamicDifficultyCalculator = makeDifficultyCalculator

// CalcDifficultyFrontierU256 mirrors calcDifficultyFrontier for fuzzing compatibility.
func CalcDifficultyFrontierU256(time uint64, parent *types.Header) *big.Int {
	return calcDifficultyFrontier(time, parent)
}

// CalcDifficultyHomesteadU256 mirrors calcDifficultyHomestead for fuzzing compatibility.
func CalcDifficultyHomesteadU256(time uint64, parent *types.Header) *big.Int {
	return calcDifficultyHomestead(time, parent)
}

// MakeDifficultyCalculatorU256 mirrors makeDifficultyCalculator for fuzzing compatibility.
func MakeDifficultyCalculatorU256(bombDelay *big.Int) func(time uint64, parent *types.Header) *big.Int {
	return makeDifficultyCalculator(bombDelay)
}
