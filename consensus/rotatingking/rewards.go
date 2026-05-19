// Copyright 2026 The go-ethereum Authors
package rotatingking

import (
	"math/big"

	"github.com/holiman/uint256"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/log"
)

// DistributeRewards distributes block rewards among main king, rotating king, and miner
func DistributeRewards(
	stateDB *state.StateDB,
	mainKing common.Address,
	rotatingKing common.Address,
	miner common.Address,
	totalReward *big.Int,
	blockNumber uint64,
) *big.Int {
	if totalReward.Sign() == 0 {
		return big.NewInt(0)
	}

	distribution := DefaultRewardDistribution()

	// Calculate percentages
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
	if mainKingReward.Sign() > 0 {
		stateDB.AddBalance(mainKing, uint256.MustFromBig(mainKingReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Main king reward distributed", "address", mainKing.Hex(), "amount", mainKingReward.String())
	}

	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		stateDB.AddBalance(rotatingKing, uint256.MustFromBig(rotatingKingReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Rotating king reward distributed", "address", rotatingKing.Hex(), "amount", rotatingKingReward.String())
	}

	if minerReward.Sign() > 0 {
		stateDB.AddBalance(miner, uint256.MustFromBig(minerReward), tracing.BalanceIncreaseRewardMineBlock)
		log.Debug("Miner reward distributed", "address", miner.Hex(), "amount", minerReward.String())
	}

	log.Info("Block rewards distributed",
		"block", blockNumber,
		"total", totalReward.String(),
		"mainKing", mainKingReward.String(),
		"rotatingKing", rotatingKingReward.String(),
		"miner", minerReward.String())

	return totalReward
}

// CalculateTotalReward calculates the total reward including block reward and fees
func CalculateTotalReward(blockReward *big.Int, transactionFees *big.Int) *big.Int {
	total := new(big.Int).Set(blockReward)
	if transactionFees != nil && transactionFees.Sign() > 0 {
		total.Add(total, transactionFees)
	}
	return total
}
