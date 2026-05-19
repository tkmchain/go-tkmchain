package eth

import "github.com/ethereum/go-ethereum/common"

// KingAPI provides RPC methods for rotating king configuration.
type KingAPI struct {
	e *Ethereum
}

// NewKingAPI creates a new king RPC API service.
func NewKingAPI(e *Ethereum) *KingAPI {
	return &KingAPI{e: e}
}

// MainAddress returns the configured main king address.
func (api *KingAPI) MainAddress() common.Address {
	return api.e.GetMainKingAddress()
}

// Addresses returns the configured rotating king addresses.
func (api *KingAPI) Addresses() []common.Address {
	return api.e.GetKingAddresses()
}
