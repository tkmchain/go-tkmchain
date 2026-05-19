// Copyright 2026 The go-ethereum Authors
// Package rotatingking implements a rotating king consensus mechanism
package rotatingking

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// KingRotation represents a king rotation event
type KingRotation struct {
	BlockHeight  uint64         `json:"blockHeight"`
	PreviousKing common.Address `json:"previousKing"`
	NewKing      common.Address `json:"newKing"`
	Timestamp    time.Time      `json:"timestamp"`
	Reward       *big.Int       `json:"reward"`
	WasEligible  bool           `json:"wasEligible"`
	Reason       string         `json:"reason,omitempty"`
}

// RotatingKingState holds the current state of the rotating king system
type RotatingKingState struct {
	CurrentKingIndex        int                       `json:"currentKingIndex"`
	RotationHeight          uint64                    `json:"rotationHeight"`
	NextRotationAt          uint64                    `json:"nextRotationAt"`
	LastUpdated             time.Time                 `json:"lastUpdated"`
	RotationCount           uint64                    `json:"rotationCount"`
	KingsHistory            []KingRotation            `json:"kingsHistory"`
	TotalRewardsDistributed *big.Int                  `json:"totalRewardsDistributed"`
	KingRewards             map[common.Address]*big.Int `json:"kingRewards"`
}

// RotatingKingConfig holds configuration for the rotating king system
type RotatingKingConfig struct {
	RotationInterval  uint64           `json:"rotationInterval"`
	RotationOffset    uint64           `json:"rotationOffset"`
	KingAddresses     []common.Address `json:"kingAddresses"`
	ActivationDelay   uint64           `json:"activationDelay"`
	MinStakeRequired  *big.Int         `json:"minStakeRequired"`
}

// RewardDistribution defines how rewards are split
type RewardDistribution struct {
	MainKingPercent  int // 10%
	RotatingKingPercent int // 40%
	MinerPercent     int // 50%
}

// DefaultRewardDistribution returns the default reward split
func DefaultRewardDistribution() *RewardDistribution {
	return &RewardDistribution{
		MainKingPercent:    10,
		RotatingKingPercent: 40,
		MinerPercent:       50,
	}
}
