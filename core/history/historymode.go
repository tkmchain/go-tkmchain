// Copyright 2025 The go-ethereum Authors
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

package history

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// HistoryMode configures history pruning.
type HistoryMode uint32

const (
	// KeepAll (default) means that all chain history down to genesis block will be kept.
	KeepAll HistoryMode = iota

	// KeepPostMerge sets the history pruning point to the merge activation block.
	// Note: In RandomX-only chain, this is not applicable but kept for compatibility
	KeepPostMerge

	// KeepPostPrague sets the history pruning point to the Prague (Pectra) activation block.
	KeepPostPrague
)

func (m HistoryMode) IsValid() bool {
	return m <= KeepPostPrague
}

func (m HistoryMode) String() string {
	switch m {
	case KeepAll:
		return "all"
	case KeepPostMerge:
		return "postmerge"
	case KeepPostPrague:
		return "postprague"
	default:
		return fmt.Sprintf("invalid HistoryMode(%d)", m)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (m HistoryMode) MarshalText() ([]byte, error) {
	if m.IsValid() {
		return []byte(m.String()), nil
	}
	return nil, fmt.Errorf("unknown history mode %d", m)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *HistoryMode) UnmarshalText(text []byte) error {
	switch string(text) {
	case "all":
		*m = KeepAll
	case "postmerge":
		*m = KeepPostMerge
	case "postprague":
		*m = KeepPostPrague
	default:
		return fmt.Errorf(`unknown history mode %q, want "all", "postmerge", or "postprague"`, text)
	}
	return nil
}

// PrunePoint identifies a specific block for history pruning.
type PrunePoint struct {
	BlockNumber uint64
	BlockHash   common.Hash
}

// ============================================================
// STATIC PRUNE POINTS - DISABLED FOR RANDOMX
// ============================================================
// RandomX chains don't need history pruning. The static prune points
// are only relevant for Ethereum mainnet. For RandomX, we keep all history.
// If you want to enable pruning for your RandomX chain, you can add
// your own prune points here using params.RandomXGenesisHash.

var staticPrunePoints = map[HistoryMode]map[common.Hash]*PrunePoint{
	// For RandomX, we don't use pruning. The KeepPostMerge and KeepPostPrague
	// modes will fall back to KeepAll for RandomX chains.
	//
	// To add pruning for RandomX, uncomment and set your own prune blocks:
	// KeepPostMerge: {
	// 	params.RandomXGenesisHash: {
	// 		BlockNumber: 100000, // Your desired prune block
	// 		BlockHash:   common.HexToHash("0x..."), // Block hash at that height
	// 	},
	// },
	// KeepPostPrague: {
	// 	params.RandomXGenesisHash: {
	// 		BlockNumber: 200000, // Your desired prune block
	// 		BlockHash:   common.HexToHash("0x..."), // Block hash at that height
	// 	},
	// },
}

// HistoryPolicy describes the configured history pruning strategy.
type HistoryPolicy struct {
	Mode HistoryMode
	// Static prune point for PostMerge/PostPrague, nil otherwise.
	Target *PrunePoint
}

// NewPolicy constructs a HistoryPolicy from the given mode and genesis hash.
func NewPolicy(mode HistoryMode, genesisHash common.Hash) (HistoryPolicy, error) {
	switch mode {
	case KeepAll:
		return HistoryPolicy{Mode: KeepAll}, nil

	case KeepPostMerge, KeepPostPrague:
		point := staticPrunePoints[mode][genesisHash]
		if point == nil {
			// For unknown networks (including RandomX), default to KeepAll
			return HistoryPolicy{Mode: KeepAll}, nil
		}
		return HistoryPolicy{Mode: mode, Target: point}, nil

	default:
		return HistoryPolicy{}, fmt.Errorf("invalid history mode: %d", mode)
	}
}

// PrunedHistoryError is returned by APIs when the requested history is pruned.
type PrunedHistoryError struct{}

func (e *PrunedHistoryError) Error() string  { return "pruned history unavailable" }
func (e *PrunedHistoryError) ErrorCode() int { return 4444 }
