// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package utils

import (
	"math/big"

	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/urfave/cli/v2"
)

// RandomX mining flags - fully restored and active
var (
	// Mining control flags
	MiningEnabledFlag = &cli.BoolFlag{
		Name:     "mine",
		Usage:    "Enable RandomX CPU mining",
		Category: flags.MinerCategory,
	}
	
	// Thread configuration
	MinerThreadsFlag = &cli.IntFlag{
		Name:     "miner.threads",
		Usage:    "Number of CPU threads for RandomX mining (0 = auto-detect all cores)",
		Value:    0,
		Category: flags.MinerCategory,
	}
	
	// Reward recipient
	MinerEtherbaseFlag = &cli.StringFlag{
		Name:     "miner.etherbase",
		Usage:    "0x prefixed public address to receive mining rewards (default = first account)",
		Category: flags.MinerCategory,
	}
	
	// Block configuration
	MinerExtraDataFlag = &cli.StringFlag{
		Name:     "miner.extradata",
		Usage:    "Block extra data set by the miner (max 32 bytes)",
		Category: flags.MinerCategory,
	}
	
	// Gas configuration
	MinerGasPriceFlag = &cli.BigFlag{
		Name:     "miner.gasprice",
		Usage:    "Minimum gas price (in wei) for accepting transactions in mined blocks",
		Value:    big.NewInt(1e9), // 1 Gwei
		Category: flags.MinerCategory,
	}
	
	MinerGasLimitFlag = &cli.Uint64Flag{
		Name:     "miner.gaslimit",
		Usage:    "Target gas limit for mined blocks",
		Value:    8000000, // 8 million gas
		Category: flags.MinerCategory,
	}
	
	// RandomX specific flags
	RandomXCacheSizeFlag = &cli.Uint64Flag{
		Name:     "randomx.cache-size",
		Usage:    "RandomX cache size in MB (light verification mode)",
		Value:    256,
		Category: flags.MinerCategory,
	}
	
	RandomXDatasetSizeFlag = &cli.Uint64Flag{
		Name:     "randomx.dataset-size",
		Usage:    "RandomX dataset size in GB (full mining mode)",
		Value:    2,
		Category: flags.MinerCategory,
	}
	
	RandomXEpochLengthFlag = &cli.Uint64Flag{
		Name:     "randomx.epoch-length",
		Usage:    "Number of blocks between RandomX epoch resets",
		Value:    2048,
		Category: flags.MinerCategory,
	}
	
	RandomXMinMemoryFlag = &cli.Uint64Flag{
		Name:     "randomx.min-memory",
		Usage:    "Minimum memory required for RandomX mining in GB",
		Value:    4,
		Category: flags.MinerCategory,
	}
	
	// King mining rewards
	MainKingAddressFlag = &cli.StringFlag{
		Name:     "king.main",
		Usage:    "Main king address (receives 10% of block rewards)",
		Category: flags.MinerCategory,
	}
	
	RotatingKingAddressesFlag = &cli.StringFlag{
		Name:     "king.rotating",
		Usage:    "Comma-separated list of rotating king addresses (receive 40% of block rewards)",
		Category: flags.MinerCategory,
	}
	
	KingRotationIntervalFlag = &cli.Uint64Flag{
		Name:     "king.rotation-interval",
		Usage:    "Number of blocks between rotating king changes",
		Value:    100,
		Category: flags.MinerCategory,
	}
)

// RandomXMiningFlags groups all RandomX mining flags
var RandomXMiningFlags = []cli.Flag{
	MiningEnabledFlag,
	MinerThreadsFlag,
	MinerEtherbaseFlag,
	MinerExtraDataFlag,
	MinerGasPriceFlag,
	MinerGasLimitFlag,
	RandomXCacheSizeFlag,
	RandomXDatasetSizeFlag,
	RandomXEpochLengthFlag,
	RandomXMinMemoryFlag,
	MainKingAddressFlag,
	RotatingKingAddressesFlag,
	KingRotationIntervalFlag,
}
