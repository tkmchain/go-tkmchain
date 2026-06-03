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

	PoolMiningFlag = &cli.BoolFlag{
		Name:     "pool",
		Usage:    "Enable RandomX pool mining mode (generate external work without local CPU sealing)",
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

	// Block and gas flags are shared with cmd/utils/flags.go

	// RandomX specific flags
	RandomXCacheSizeFlag = &cli.Uint64Flag{
		Name:     "randomx.cache-size",
		Usage:    "RandomX cache size in MB (light verification mode)",
		Value:    256,
		Category: flags.MinerCategory,
	}

	RandomXDatasetSizeFlag = &cli.StringFlag{
		Name:     "randomx.dataset-size",
		Usage:    "RandomX dataset size in GB (full mining mode, decimals allowed and rounded up)",
		Value:    "2",
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

	// RandomX RAM cache flag
	RandomXRAMCacheFlag = &cli.BoolFlag{
		Name:     "randomx.ram-cache",
		Usage:    "Store RandomX dataset in RAM instead of disk (faster but uses more memory)",
		Category: flags.MinerCategory,
	}

	RandomXNoPersistFlag = &cli.BoolFlag{
		Name:     "randomx.no-persist",
		Usage:    "Don't persist RandomX dataset to disk (generate fresh on each start)",
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
	PoolMiningFlag,
	MinerThreadsFlag,
	MinerEtherbaseFlag,
	MinerExtraDataFlag,
	MinerGasPriceFlag,
	MinerGasLimitFlag,
	RandomXCacheSizeFlag,
	RandomXDatasetSizeFlag,
	RandomXEpochLengthFlag,
	RandomXMinMemoryFlag,
	RandomXRAMCacheFlag,
	RandomXNoPersistFlag,
	MainKingAddressFlag,
	RotatingKingAddressesFlag,
	KingRotationIntervalFlag,
}
