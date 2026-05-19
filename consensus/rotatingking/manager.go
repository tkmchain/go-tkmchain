// Copyright 2026 The go-ethereum authors
package rotatingking

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

var (
	// EligibilityThreshold is the minimum balance required for king eligibility (100k ANTD)
	EligibilityThreshold = new(big.Int).Mul(big.NewInt(100000), big.NewInt(1e18))
)

// BlockchainStateProvider provides blockchain state access
type BlockchainStateProvider interface {
	GetBalance(common.Address) *big.Int
	GetBlockNumber() uint64
}

// RotatingKingManager manages the rotating king logic
type RotatingKingManager struct {
	mu       sync.RWMutex
	config   *RotatingKingConfig
	state    *RotatingKingState
	mainKing common.Address
	logger   log.Logger
}

// NewRotatingKingManager creates a new rotating king manager
func NewRotatingKingManager(mainKing common.Address, kingAddresses []common.Address, rotationInterval uint64) *RotatingKingManager {
	config := &RotatingKingConfig{
		RotationInterval: rotationInterval,
		RotationOffset:   0,
		KingAddresses:    kingAddresses,
		ActivationDelay:  2,
		MinStakeRequired: new(big.Int).Set(EligibilityThreshold),
	}

	state := &RotatingKingState{
		CurrentKingIndex:        0,
		RotationHeight:          0,
		NextRotationAt:          rotationInterval,
		LastUpdated:             time.Now(),
		RotationCount:           0,
		KingsHistory:            make([]KingRotation, 0),
		TotalRewardsDistributed: big.NewInt(0),
		KingRewards:             make(map[common.Address]*big.Int),
	}

	return &RotatingKingManager{
		config:   config,
		state:    state,
		mainKing: mainKing,
		logger:   log.New("module", "rotatingking"),
	}
}

// GetCurrentKing returns the current rotating king address
func (m *RotatingKingManager) GetCurrentKing() common.Address {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.config.KingAddresses) == 0 {
		return common.Address{}
	}
	return m.config.KingAddresses[m.state.CurrentKingIndex]
}

// GetMainKing returns the main king address
func (m *RotatingKingManager) GetMainKing() common.Address {
	return m.mainKing
}

// GetNextKing returns the next king in rotation
func (m *RotatingKingManager) GetNextKing() common.Address {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.config.KingAddresses) == 0 {
		return common.Address{}
	}
	nextIndex := (m.state.CurrentKingIndex + 1) % len(m.config.KingAddresses)
	return m.config.KingAddresses[nextIndex]
}

// ShouldRotate checks if rotation should occur at the given block height
func (m *RotatingKingManager) ShouldRotate(blockHeight uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return blockHeight >= m.state.NextRotationAt
}

// RotateToNextKing performs the king rotation
func (m *RotatingKingManager) RotateToNextKing(blockHeight uint64, blockHash common.Hash, stateProvider BlockchainStateProvider) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.config.KingAddresses) == 0 {
		return errors.New("no king addresses configured")
	}

	if blockHeight < m.state.NextRotationAt {
		return fmt.Errorf("rotation not due yet (next at %d)", m.state.NextRotationAt)
	}

	previousKing := m.config.KingAddresses[m.state.CurrentKingIndex]

	// Find next eligible king
	newIndex := (m.state.CurrentKingIndex + 1) % len(m.config.KingAddresses)
	newKing := m.config.KingAddresses[newIndex]

	// Check eligibility and find eligible king if needed
	if stateProvider != nil {
		balance := stateProvider.GetBalance(newKing)
		if balance.Cmp(m.config.MinStakeRequired) < 0 {
			// Search for eligible king
			for i := 1; i < len(m.config.KingAddresses); i++ {
				candidateIndex := (m.state.CurrentKingIndex + i) % len(m.config.KingAddresses)
				candidate := m.config.KingAddresses[candidateIndex]
				candidateBalance := stateProvider.GetBalance(candidate)
				if candidateBalance.Cmp(m.config.MinStakeRequired) >= 0 {
					newIndex = candidateIndex
					newKing = candidate
					m.logger.Info("Found eligible king", "address", newKing.Hex())
					break
				}
			}
		}
	}

	// Create rotation record
	rotation := KingRotation{
		BlockHeight:  blockHeight,
		PreviousKing: previousKing,
		NewKing:      newKing,
		Timestamp:    time.Now(),
		Reward:       big.NewInt(0),
		WasEligible:  true,
	}

	// Update state
	m.state.KingsHistory = append(m.state.KingsHistory, rotation)
	if len(m.state.KingsHistory) > 100 {
		m.state.KingsHistory = m.state.KingsHistory[1:]
	}

	m.state.CurrentKingIndex = newIndex
	m.state.RotationHeight = blockHeight
	m.state.NextRotationAt = blockHeight + m.config.RotationInterval
	m.state.RotationCount++
	m.state.LastUpdated = time.Now()

	m.logger.Info("King rotation executed",
		"previousKing", previousKing.Hex(),
		"newKing", newKing.Hex(),
		"blockHeight", blockHeight,
		"nextRotationAt", m.state.NextRotationAt)

	return nil
}

// IsCurrentKing checks if the given address is the current rotating king
func (m *RotatingKingManager) IsCurrentKing(address common.Address) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.config.KingAddresses) == 0 {
		return false
	}
	return m.config.KingAddresses[m.state.CurrentKingIndex] == address
}

// IsMainKing checks if the given address is the main king
func (m *RotatingKingManager) IsMainKing(address common.Address) bool {
	return m.mainKing == address
}

// IsKing checks if the address is either main king or in rotation
func (m *RotatingKingManager) IsKing(address common.Address) bool {
	if m.IsMainKing(address) {
		return true
	}
	return m.IsCurrentKing(address)
}

// GetKingAddresses returns all rotating king addresses
func (m *RotatingKingManager) GetKingAddresses() []common.Address {
	m.mu.RLock()
	defer m.mu.RUnlock()

	addresses := make([]common.Address, len(m.config.KingAddresses))
	copy(addresses, m.config.KingAddresses)
	return addresses
}

// GetCurrentKingIndex returns the current king index
func (m *RotatingKingManager) GetCurrentKingIndex() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.CurrentKingIndex
}

// GetRotationInfo returns rotation information
func (m *RotatingKingManager) GetRotationInfo(height uint64) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info := make(map[string]interface{})
	if len(m.config.KingAddresses) == 0 {
		return info
	}

	currentKing := m.config.KingAddresses[m.state.CurrentKingIndex]
	nextKing := m.config.KingAddresses[(m.state.CurrentKingIndex+1)%len(m.config.KingAddresses)]

	blocksUntilRotation := uint64(0)
	if m.state.NextRotationAt > height {
		blocksUntilRotation = m.state.NextRotationAt - height
	}

	info["mainKing"] = m.mainKing.Hex()
	info["currentKing"] = currentKing.Hex()
	info["nextKing"] = nextKing.Hex()
	info["blocksUntilRotation"] = blocksUntilRotation
	info["rotationHeight"] = m.state.RotationHeight
	info["nextRotationAt"] = m.state.NextRotationAt
	info["rotationInterval"] = m.config.RotationInterval
	info["kingCount"] = len(m.config.KingAddresses)
	info["rotationCount"] = m.state.RotationCount

	return info
}

// GetMonitoringResponsibilities returns the chain monitoring responsibilities for rotating kings.
func (m *RotatingKingManager) GetMonitoringResponsibilities() []MonitoringCategory {
	return []MonitoringCategory{
		{Name: "Block Production & Chain Health", Metrics: []string{"New Block Height", "Block Inclusion Distance", "Orphaned/Forked Blocks"}},
		{Name: "Node & Peer Connectivity", Metrics: []string{"Peer Count", "Peer Quality Scores", "Connection Stability"}},
		{Name: "Reward & Performance Metrics", Metrics: []string{"Reward Trend", "Reward Percentage Change", "Balance Changes"}},
		{Name: "Slashing & Violation Detection", Metrics: []string{"Attestation Violations", "Unavailable Node Detection", "Balance Below Eligibility Threshold"}},
		{Name: "Chain Statistics", Metrics: []string{"Total Active Kings", "Total Staked/Delegated Amount", "RKs Entering/Exiting Rotation", "Pending Activation/Exit Queue"}},
		{Name: "Rotation-Specific Monitoring", Metrics: []string{"Missed Rotation Window", "Rotation Consensus", "Upcoming Rotation Block", "Pending Proposal Status"}},
		{Name: "Transaction Performance", Metrics: []string{"Transaction Inclusion Time", "Pending Transaction Count", "Gas Price Trends"}},
		{Name: "Infrastructure Health (Critical)", Metrics: []string{"Node Resource Health", "RPC Availability", "Disk/Database Health"}},
	}
}
