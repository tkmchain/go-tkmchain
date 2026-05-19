package rotatingking

import (
	"github.com/ethereum/go-ethereum/common"
)

// API is the RPC API for the rotating king system
type API struct {
	manager *RotatingKingManager
}

// NewAPI creates a new rotating king API
func NewAPI(manager *RotatingKingManager) *API {
	return &API{manager: manager}
}

// GetCurrentKing returns the current rotating king
func (api *API) GetCurrentKing() common.Address {
	return api.manager.GetCurrentKing()
}

// GetMainKing returns the main king
func (api *API) GetMainKing() common.Address {
	return api.manager.GetMainKing()
}

// GetNextKing returns the next king in rotation
func (api *API) GetNextKing() common.Address {
	return api.manager.GetNextKing()
}

// GetRotationInfo returns rotation information
func (api *API) GetRotationInfo(height uint64) map[string]interface{} {
	return api.manager.GetRotationInfo(height)
}

// GetKingAddresses returns all rotating king addresses
func (api *API) GetKingAddresses() []common.Address {
	return api.manager.GetKingAddresses()
}

// IsKing checks if an address is a king
func (api *API) IsKing(address common.Address) bool {
	return api.manager.IsKing(address)
}
