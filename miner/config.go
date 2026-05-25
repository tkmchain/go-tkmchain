package miner

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

// Config is the configuration for the block builder/miner.
type Config struct {
	Enabled             bool
	PendingFeeRecipient common.Address
	ExtraData           []byte
	GasFloor            uint64
	GasCeil             uint64
	GasPrice            *big.Int
	Recommit            time.Duration
	Etherbase           common.Address
	GasLimit            uint64
}

// DefaultConfig contains default settings for the miner.
var DefaultConfig = Config{
	GasCeil:  params.GenesisGasLimit,
	GasPrice: big.NewInt(params.GWei),
	Recommit: 2 * time.Second,
}
