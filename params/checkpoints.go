// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
// or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser General Public License
// for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

// Checkpoint defines a hardcoded (blockNumber, blockHash) pair.
type Checkpoint struct {
	Number uint64      `json:"number"`
	Hash   common.Hash `json:"hash"`
}

// Checkpoints holds all hardcoded checkpoints for a given network.
type Checkpoints struct {
	lock sync.RWMutex

	// Map from block number to block hash
	Points map[uint64]common.Hash
}

// CheckpointValidationEnabled controls whether hardcoded checkpoints are
// enforced during block insertion.
var CheckpointValidationEnabled = true

var (
	mandatoryRandomXCheckpoints = map[uint64]common.Hash{
		2370: common.HexToHash("0xe10ff3179cc30f911c29326a822e6a24206f819dcaff2edfeeb5b2078dd95b17"),
		6000: common.HexToHash("0x4d3cd743aec4b40c276174d6582049189901a0a78fa6fc280b8c5cfd946fa660"),
		7165: common.HexToHash("0xcd0abe2c94903b0a7584ac5892ff812d2f2450853fc6a055cbb09807ce8c9f53"),
	}
	mandatoryEgyptCheckpoints  = map[uint64]common.Hash{}
	activeMandatoryCheckpoints = mandatoryRandomXCheckpoints
)

// RandomXCheckpoints holds the globally accessible hardcoded RandomX checkpoints.
var RandomXCheckpoints = initRandomXCheckpoints()

// initRandomXCheckpoints initialises the checkpoints for the RandomX mainnet.
func initRandomXCheckpoints() *Checkpoints {
	cp := &Checkpoints{
		Points: make(map[uint64]common.Hash),
	}
	// Real checkpoint: block 0 (genesis) must match the actual genesis hash.
	cp.Points[0] = MainnetGenesisHash
	for number, hash := range mandatoryRandomXCheckpoints {
		cp.Points[number] = hash
	}

	// Add more checkpoints at strategic heights
	// cp.Points[1000] = common.HexToHash("0x...")
	// cp.Points[2000] = common.HexToHash("0x...")
	// cp.Points[10000] = common.HexToHash("0x...")

	return cp
}

// initEgyptCheckpoints initialises the checkpoints for the Egypt RandomX testnet.
func initEgyptCheckpoints() *Checkpoints {
	return &Checkpoints{
		Points: map[uint64]common.Hash{
			0: EgyptGenesisHash,
		},
	}
}

// SetActiveCheckpointGenesis selects the hardcoded checkpoints for a genesis hash.
func SetActiveCheckpointGenesis(genesis common.Hash) {
	switch genesis {
	case EgyptGenesisHash:
		RandomXCheckpoints = initEgyptCheckpoints()
		activeMandatoryCheckpoints = mandatoryEgyptCheckpoints
	default:
		RandomXCheckpoints = initRandomXCheckpoints()
		activeMandatoryCheckpoints = mandatoryRandomXCheckpoints
	}
}

// SetCheckpointValidation enables or disables hardcoded checkpoint validation.
func SetCheckpointValidation(enabled bool) {
	CheckpointValidationEnabled = enabled
}

// HasMandatoryCheckpoints reports whether any checkpoint is always enforced.
func HasMandatoryCheckpoints() bool {
	return len(activeMandatoryCheckpoints) > 0
}

// ShouldValidateCheckpoint reports whether a checkpoint at number must be enforced.
func ShouldValidateCheckpoint(number uint64) bool {
	if CheckpointValidationEnabled {
		return true
	}
	_, ok := activeMandatoryCheckpoints[number]
	return ok
}

// AddCheckpoint adds an immutable checkpoint. Existing checkpoints cannot be changed.
func (c *Checkpoints) AddCheckpoint(number uint64, hash common.Hash) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if existing, ok := c.Points[number]; ok {
		if existing == hash {
			return nil
		}
		return fmt.Errorf("checkpoint already set at block %d: have %s, want %s", number, existing, hash)
	}
	c.Points[number] = hash
	return nil
}

// AddCheckpoint adds a globally configured immutable checkpoint.
func AddCheckpoint(number uint64, hash common.Hash) error {
	return RandomXCheckpoints.AddCheckpoint(number, hash)
}

// GetCheckpoint returns the hardcoded block hash for a given height, if any.
func (c *Checkpoints) GetCheckpoint(number uint64) (common.Hash, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	hash, ok := c.Points[number]
	return hash, ok
}

// GetCheckpoint returns the globally configured hardcoded block hash for a given
// height, if any.
func GetCheckpoint(number uint64) (common.Hash, bool) {
	return RandomXCheckpoints.GetCheckpoint(number)
}

// All returns all checkpoints sorted by block number.
func (c *Checkpoints) All() []Checkpoint {
	c.lock.RLock()
	defer c.lock.RUnlock()

	checkpoints := make([]Checkpoint, 0, len(c.Points))
	for number, hash := range c.Points {
		checkpoints = append(checkpoints, Checkpoint{Number: number, Hash: hash})
	}
	slices.SortFunc(checkpoints, func(a, b Checkpoint) int {
		return cmp.Compare(a.Number, b.Number)
	})
	return checkpoints
}

// AllCheckpoints returns all globally configured checkpoints sorted by block number.
func AllCheckpoints() []Checkpoint {
	return RandomXCheckpoints.All()
}
