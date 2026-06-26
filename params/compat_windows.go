//go:build windows

package params

import "github.com/ethereum/go-ethereum/common"

// Windows compatibility layer for RandomX
// These are symbols that exist in Ethereum but not in RandomX

var (
    // DepositContractAddress is not used in RandomX
    DepositContractAddress = common.Address{}
    
    // MainnetChainConfig is aliased to RandomXChainConfig
    MainnetChainConfig = RandomXChainConfig
    SepoliaChainConfig = RandomXChainConfig
    HoleskyChainConfig = RandomXChainConfig
    HoodiChainConfig   = RandomXChainConfig
)
