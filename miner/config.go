// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package miner

import (
        "math/big"
        "time"

        "github.com/ethereum/go-ethereum/common"
)

// Config contains the miner configuration
type Config struct {
        Enabled             bool
        Etherbase           common.Address
        PendingFeeRecipient common.Address   // Address for pending block producer
        ExtraData           []byte
        GasPrice            *big.Int
        GasLimit            uint64
        Recommit            time.Duration    // Time interval to recreate the block being mined
        GasFloor            uint64
        GasCeil             uint64
        Threads             int
}

// DefaultConfig is the default miner configuration
var DefaultConfig = Config{
        Enabled:    false,
        GasPrice:   big.NewInt(1e9),
        GasLimit:   8000000,
        Recommit:   3 * time.Second,  // Default recommit interval
        Threads:    1,
}
