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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

// ==================== Constants ====================

var (
	// EligibilityThreshold is the minimum balance required for king eligibility (100k ANTD)
	EligibilityThreshold = new(big.Int).Mul(big.NewInt(100_000), big.NewInt(1e18))

	// InitialBlockReward is the starting block reward (200 ANTD)
	InitialBlockReward = new(big.Int).Mul(big.NewInt(200), big.NewInt(1e18))

	// GenesisPremine is the initial supply at genesis (60,000,000 ANTD)
	GenesisPremine = new(big.Int).Mul(big.NewInt(60_000_000), big.NewInt(1e18))

	// Block timing constants
	TargetBlockTimeSeconds = 120                                                     // Default block time for RandomX chain
	BlocksPerHalving       = uint64(4 * 365 * 24 * 60 * 60 / TargetBlockTimeSeconds) // ~4 years
	GenesisTimestamp       = int64(1763731821)                                       // Dec 1, 2025
	MaxHalvings            = uint64(64)
)

// RewardDistribution defines how rewards are split among participants
type RewardDistribution struct {
	MainKingPercent     int // 10%
	RotatingKingPercent int // 40%
	MinerPercent        int // 50%
}

// DefaultRewardDistribution returns the default reward split
func DefaultRewardDistribution() *RewardDistribution {
	return &RewardDistribution{
		MainKingPercent:     10,
		RotatingKingPercent: 40,
		MinerPercent:        50,
	}
}

// ==================== Reward Calculation ====================

// CalculateBlockReward returns the current block reward with halving
func CalculateBlockReward(blockNumber uint64) *big.Int {
	halvingPeriod := blockNumber / BlocksPerHalving
	if halvingPeriod > MaxHalvings {
		halvingPeriod = MaxHalvings
	}

	reward := new(big.Int).Set(InitialBlockReward)
	for i := uint64(0); i < halvingPeriod; i++ {
		reward.Div(reward, big.NewInt(2))
		if reward.Cmp(big.NewInt(1e18)) < 0 {
			return big.NewInt(0)
		}
	}
	return reward
}

// CalculateTotalReward calculates the total reward including block reward and fees
func CalculateTotalReward(blockReward *big.Int, transactionFees *big.Int) *big.Int {
	total := new(big.Int).Set(blockReward)
	if transactionFees != nil && transactionFees.Sign() > 0 {
		total.Add(total, transactionFees)
	}
	return total
}

// GetTotalTransactionFees calculates the sum of all transaction fees in receipts
func GetTotalTransactionFees(header *types.Header, receipts []*types.Receipt) *big.Int {
	totalFees := big.NewInt(0)

	for _, receipt := range receipts {
		if receipt != nil && receipt.GasUsed > 0 {
			// Fee = GasUsed * EffectiveGasPrice
			effectiveGasPrice := receipt.EffectiveGasPrice
			if effectiveGasPrice == nil {
				effectiveGasPrice = header.BaseFee
			}
			if effectiveGasPrice != nil {
				fee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), effectiveGasPrice)
				totalFees.Add(totalFees, fee)
			}
		}
	}

	return totalFees
}

// ==================== Reward Distribution ====================

// DistributeRewards distributes block rewards among main king, rotating king, and miner
// Returns the total reward distributed
func DistributeRewards(
	stateDB *state.StateDB,
	mainKing common.Address,
	rotatingKing common.Address,
	miner common.Address,
	totalReward *big.Int,
	blockNumber uint64,
) *big.Int {
	if totalReward == nil || totalReward.Sign() == 0 {
		log.Debug("No rewards to distribute", "block", blockNumber)
		return big.NewInt(0)
	}

	distribution := DefaultRewardDistribution()

	// Calculate percentages as big.Int
	totalBig := new(big.Float).SetInt(totalReward)

	mainKingPercent := new(big.Float).SetFloat64(float64(distribution.MainKingPercent) / 100.0)
	rotatingKingPercent := new(big.Float).SetFloat64(float64(distribution.RotatingKingPercent) / 100.0)
	minerPercent := new(big.Float).SetFloat64(float64(distribution.MinerPercent) / 100.0)

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)

	new(big.Float).Mul(totalBig, mainKingPercent).Int(mainKingReward)
	new(big.Float).Mul(totalBig, rotatingKingPercent).Int(rotatingKingReward)
	new(big.Float).Mul(totalBig, minerPercent).Int(minerReward)

	// Distribute rewards
	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		stateDB.AddBalance(mainKing, uint256.MustFromBig(mainKingReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Main king reward distributed",
			"block", blockNumber,
			"address", mainKing.Hex(),
			"amount", FormatANTD(mainKingReward))
	}

	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		stateDB.AddBalance(rotatingKing, uint256.MustFromBig(rotatingKingReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Rotating king reward distributed",
			"block", blockNumber,
			"address", rotatingKing.Hex(),
			"amount", FormatANTD(rotatingKingReward))
	}

	if minerReward.Sign() > 0 && miner != (common.Address{}) {
		stateDB.AddBalance(miner, uint256.MustFromBig(minerReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Miner reward distributed",
			"block", blockNumber,
			"address", miner.Hex(),
			"amount", FormatANTD(minerReward))
	}

	log.Info("Block rewards distributed",
		"block", blockNumber,
		"total", FormatANTD(totalReward),
		"mainKing", FormatANTD(mainKingReward),
		"rotatingKing", FormatANTD(rotatingKingReward),
		"miner", FormatANTD(minerReward))

	return totalReward
}

// DistributeUncleReward distributes rewards for uncle blocks
func DistributeUncleReward(stateDB *state.StateDB, uncleCoinbase common.Address, uncleNumber, blockNumber uint64, blockReward *big.Int) {
	// Uncle reward: (uncle_number + 8 - block_number) * block_reward / 8
	uncleReward := new(big.Int).Set(blockReward)

	// Calculate adjustment factor
	adjustment := new(big.Int).SetUint64(uncleNumber)
	adjustment.Add(adjustment, big.NewInt(8))
	adjustment.Sub(adjustment, new(big.Int).SetUint64(blockNumber))

	if adjustment.Sign() > 0 {
		uncleReward.Mul(uncleReward, adjustment)
		uncleReward.Div(uncleReward, big.NewInt(8))
		stateDB.AddBalance(uncleCoinbase, uint256.MustFromBig(uncleReward), tracing.BalanceIncreaseRewardMineUncle)

		log.Debug("Uncle reward distributed",
			"uncleNumber", uncleNumber,
			"blockNumber", blockNumber,
			"reward", FormatANTD(uncleReward))
	}
}

// ==================== Helper Functions ====================

// GetNextHalvingInfo returns information about the next halving event
func GetNextHalvingInfo(blockNumber uint64) map[string]interface{} {
	currentPeriod := blockNumber / BlocksPerHalving
	nextHalvingBlock := (currentPeriod + 1) * BlocksPerHalving

	currentReward := CalculateBlockReward(blockNumber)
	nextReward := CalculateBlockReward(nextHalvingBlock)

	return map[string]interface{}{
		"currentBlock":           blockNumber,
		"nextHalvingBlock":       nextHalvingBlock,
		"blocksUntil":            nextHalvingBlock - blockNumber,
		"currentReward":          currentReward.String(),
		"nextReward":             nextReward.String(),
		"halvingPeriod":          currentPeriod,
		"currentRewardFormatted": FormatANTD(currentReward),
		"nextRewardFormatted":    FormatANTD(nextReward),
	}
}

// CalculateTotalSupplyCap calculates the theoretical maximum supply
func CalculateTotalSupplyCap() *big.Int {
	total := new(big.Int).Set(GenesisPremine)
	currentReward := new(big.Int).Set(InitialBlockReward)

	for h := uint64(0); h <= MaxHalvings; h++ {
		periodReward := new(big.Int).Mul(currentReward, new(big.Int).SetUint64(BlocksPerHalving))
		total.Add(total, periodReward)

		currentReward.Div(currentReward, big.NewInt(2))
		if currentReward.Cmp(big.NewInt(1e18)) < 0 {
			break
		}
	}
	return total
}

// CalculateCirculatingSupply calculates the approximate circulating supply up to a block
func CalculateCirculatingSupply(blockNumber uint64) *big.Int {
	// Sum of all block rewards up to current block
	total := new(big.Int).Set(GenesisPremine)
	currentReward := new(big.Int).Set(InitialBlockReward)

	remainingBlocks := blockNumber
	for halving := uint64(0); halving <= MaxHalvings && remainingBlocks > 0; halving++ {
		blocksInPeriod := BlocksPerHalving
		if remainingBlocks < blocksInPeriod {
			blocksInPeriod = remainingBlocks
		}

		periodReward := new(big.Int).Mul(currentReward, new(big.Int).SetUint64(blocksInPeriod))
		total.Add(total, periodReward)

		remainingBlocks -= blocksInPeriod
		currentReward.Div(currentReward, big.NewInt(2))

		if currentReward.Cmp(big.NewInt(1e18)) < 0 {
			break
		}
	}

	return total
}

// FormatANTD formats ANTD amount with decimals (for logs/RPC)
func FormatANTD(amount *big.Int) string {
	if amount == nil {
		return "0"
	}
	oneANTD := big.NewInt(1e18)
	whole := new(big.Int).Div(amount, oneANTD)
	remainder := new(big.Int).Mod(amount, oneANTD)

	if remainder.Sign() == 0 {
		return whole.String()
	}

	// Format with up to 6 decimal places
	remainderStr := remainder.String()
	for len(remainderStr) < 18 {
		remainderStr = "0" + remainderStr
	}
	// Trim trailing zeros
	for len(remainderStr) > 6 && remainderStr[len(remainderStr)-1] == '0' {
		remainderStr = remainderStr[:len(remainderStr)-1]
	}

	return whole.String() + "." + remainderStr[:min(6, len(remainderStr))]
}

// ParseANTD parses an ANTD amount string to big.Int
func ParseANTD(amountStr string) (*big.Int, error) {
	oneANTD := big.NewInt(1e18)
	result := big.NewInt(0)

	// Find decimal point
	decimalIdx := -1
	for i, c := range amountStr {
		if c == '.' {
			decimalIdx = i
			break
		}
	}

	if decimalIdx == -1 {
		// No decimal point, parse as whole ANTD
		whole, ok := new(big.Int).SetString(amountStr, 10)
		if !ok {
			return nil, fmt.Errorf("invalid number format: %s", amountStr)
		}
		return new(big.Int).Mul(whole, oneANTD), nil
	}

	// Split whole and fractional parts
	wholeStr := amountStr[:decimalIdx]
	fracStr := amountStr[decimalIdx+1:]

	// Parse whole part
	whole, ok := new(big.Int).SetString(wholeStr, 10)
	if !ok {
		return nil, fmt.Errorf("invalid whole number: %s", wholeStr)
	}
	result.Mul(whole, oneANTD)

	// Parse fractional part (pad to 18 digits)
	for len(fracStr) < 18 {
		fracStr += "0"
	}
	if len(fracStr) > 18 {
		fracStr = fracStr[:18]
	}
	frac, ok := new(big.Int).SetString(fracStr, 10)
	if !ok {
		return nil, fmt.Errorf("invalid fractional part: %s", fracStr)
	}
	result.Add(result, frac)

	return result, nil
}

// helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ==================== Reward Info for RPC ====================

// GetRewardInfo returns comprehensive reward information for a block
func GetRewardInfo(blockNumber uint64, blockReward *big.Int, transactionFees *big.Int) map[string]interface{} {
	distribution := DefaultRewardDistribution()
	totalReward := CalculateTotalReward(blockReward, transactionFees)

	totalBig := new(big.Float).SetInt(totalReward)
	mainKingPercent := new(big.Float).SetFloat64(float64(distribution.MainKingPercent) / 100.0)
	rotatingKingPercent := new(big.Float).SetFloat64(float64(distribution.RotatingKingPercent) / 100.0)
	minerPercent := new(big.Float).SetFloat64(float64(distribution.MinerPercent) / 100.0)

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)

	new(big.Float).Mul(totalBig, mainKingPercent).Int(mainKingReward)
	new(big.Float).Mul(totalBig, rotatingKingPercent).Int(rotatingKingReward)
	new(big.Float).Mul(totalBig, minerPercent).Int(minerReward)

	return map[string]interface{}{
		"blockNumber":              blockNumber,
		"blockReward":              blockReward.String(),
		"blockRewardFormatted":     FormatANTD(blockReward),
		"transactionFees":          transactionFees.String(),
		"transactionFeesFormatted": FormatANTD(transactionFees),
		"totalReward":              totalReward.String(),
		"totalRewardFormatted":     FormatANTD(totalReward),
		"distribution": map[string]interface{}{
			"mainKingPercent":     distribution.MainKingPercent,
			"rotatingKingPercent": distribution.RotatingKingPercent,
			"minerPercent":        distribution.MinerPercent,
		},
		"rewards": map[string]interface{}{
			"mainKing":     mainKingReward.String(),
			"rotatingKing": rotatingKingReward.String(),
			"miner":        minerReward.String(),
		},
		"rewardsFormatted": map[string]string{
			"mainKing":     FormatANTD(mainKingReward),
			"rotatingKing": FormatANTD(rotatingKingReward),
			"miner":        FormatANTD(minerReward),
		},
	}
}
