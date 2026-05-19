// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
package miner

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

// KingRewardDistribution defines how block rewards are split
type KingRewardDistribution struct {
	MainKingPercent     int // 10%
	RotatingKingPercent int // 40%
	MinerPercent        int // 50%
}

// DefaultKingRewardDistribution returns the default reward split
func DefaultKingRewardDistribution() *KingRewardDistribution {
	return &KingRewardDistribution{
		MainKingPercent:     10,
		RotatingKingPercent: 40,
		MinerPercent:        50,
	}
}

// DistributeKingRewards distributes block rewards among Main King, Rotating King, and Miner
func DistributeKingRewards(
	stateDB *state.StateDB,
	mainKing common.Address,
	rotatingKing common.Address,
	miner common.Address,
	totalReward *big.Int,
	blockNumber uint64,
) *big.Int {
	if totalReward == nil || totalReward.Sign() == 0 {
		return big.NewInt(0)
	}

	distribution := DefaultKingRewardDistribution()

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
		log.Debug("Main king reward", "block", blockNumber, "address", mainKing.Hex(), "amount", mainKingReward)
	}

	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		stateDB.AddBalance(rotatingKing, uint256.MustFromBig(rotatingKingReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Rotating king reward", "block", blockNumber, "address", rotatingKing.Hex(), "amount", rotatingKingReward)
	}

	if minerReward.Sign() > 0 && miner != (common.Address{}) {
		stateDB.AddBalance(miner, uint256.MustFromBig(minerReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Miner reward", "block", blockNumber, "address", miner.Hex(), "amount", minerReward)
	}

	log.Info("Block rewards distributed (10% Main King / 40% Rotating King / 50% Miner)",
		"block", blockNumber,
		"mainKing", mainKingReward.String(),
		"rotatingKing", rotatingKingReward.String(),
		"miner", minerReward.String())

	return totalReward
}

// CalculateTotalReward calculates total reward including block reward and fees
func CalculateTotalReward(blockReward *big.Int, transactionFees *big.Int) *big.Int {
	total := new(big.Int).Set(blockReward)
	if transactionFees != nil && transactionFees.Sign() > 0 {
		total.Add(total, transactionFees)
	}
	return total
}
