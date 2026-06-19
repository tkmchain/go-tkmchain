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
	"slices"

	"github.com/ethereum/go-ethereum/common"
)

// Checkpoint defines a hardcoded (blockNumber, blockHash) pair.
type Checkpoint struct {
	Number uint64      `json:"number"`
	Hash   common.Hash `json:"hash"`
}

// Checkpoints holds all hardcoded checkpoints for a given network.
type Checkpoints struct {
	// Map from block number to block hash
	Points map[uint64]common.Hash
}

// CheckpointValidationEnabled controls whether hardcoded checkpoints are
// enforced during block insertion.
var CheckpointValidationEnabled = true

// RandomXCheckpoints holds the globally accessible hardcoded RandomX checkpoints.
var RandomXCheckpoints = initRandomXCheckpoints()

// initRandomXCheckpoints initialises the checkpoints for the RandomX mainnet.
func initRandomXCheckpoints() *Checkpoints {
	cp := &Checkpoints{
		Points: make(map[uint64]common.Hash),
	}
	// Real checkpoint: block 0 (genesis) must match the actual genesis hash.
	cp.Points[0] = common.HexToHash("0x6bdca03e891cd028a92355065c211ead725d3e3be9f4de1047c3c5faa464a55e")
        cp.Points[1] = common.HexToHash("0x7dc591631adf306577218868d080ff4724fb46953a26a594f72ccb23e09bd7bd")
        cp.Points[2] = common.HexToHash("0x537934c4f8d992a8627afd5368b0e627d858e1623093add7620c1d66e547bb18")
        cp.Points[3] = common.HexToHash("0x71d1f30a50636ed6501ea2ce2a9516ee3f637df8f5dca480179e585f31049c64")
        cp.Points[4] = common.HexToHash("0x9ab25c0e53c87cd99e1e7049cbaad23fa3bc4d16334ee411ae4150057a28c983")
        cp.Points[10] = common.HexToHash("0xa3db81782609a6da237d32a248c039d6cfb5223b5f5532b547c841e2814a882c")
	// Add more checkpoints at strategic heights
	// cp.Points[1000] = common.HexToHash("0x...")
	// cp.Points[2000] = common.HexToHash("0x...")
	// cp.Points[10000] = common.HexToHash("0x...")

	return cp
}

// SetCheckpointValidation enables or disables hardcoded checkpoint validation.
func SetCheckpointValidation(enabled bool) {
	CheckpointValidationEnabled = enabled
}

// GetCheckpoint returns the hardcoded block hash for a given height, if any.
func (c *Checkpoints) GetCheckpoint(number uint64) (common.Hash, bool) {
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
