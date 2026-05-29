package eth

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// KingAPI provides RPC methods for rotating king configuration.
type KingAPI struct {
	e *Ethereum
}

var rkRequiredStake = new(big.Int).Mul(big.NewInt(50000), big.NewInt(params.Ether))

type RKStatus struct {
	Address      common.Address `json:"address"`
	Registered   bool           `json:"registered"`
	Current      bool           `json:"current"`
	LockedAmount *big.Int       `json:"lockedAmount"`
	UnlockTime   *time.Time     `json:"unlockTime,omitempty"`
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

// Add registers an address as rotating king if stake requirement is met.
func (api *KingAPI) Add(address common.Address) (RKStatus, error) {
	header := api.e.blockchain.CurrentBlock()
	if header == nil {
		return RKStatus{}, fmt.Errorf("no head block available")
	}
	statedb, err := api.e.blockchain.StateAt(header)
	if err != nil {
		return RKStatus{}, err
	}
	balance := statedb.GetBalance(address).ToBig()
	if balance.Cmp(rkRequiredStake) < 0 {
		return RKStatus{}, fmt.Errorf("insufficient balance: need at least %s wei", rkRequiredStake.String())
	}
	unlock := time.Now().UTC().Add(30 * 24 * time.Hour)
	api.e.lock.Lock()
	api.e.recordRotatingKingLocked(address, unlock)
	status := api.statusLocked(address)
	api.e.lock.Unlock()

	api.e.broadcastRotatingKing(address, unlock)
	return status, nil
}

// List returns all registered rotating king addresses with status.
func (api *KingAPI) List() []RKStatus {
	api.e.lock.RLock()
	defer api.e.lock.RUnlock()
	list := make([]RKStatus, 0, len(api.e.rkLocks))
	for addr := range api.e.rkLocks {
		list = append(list, api.statusLocked(addr))
	}
	return list
}

// Status returns registration details for one address.
func (api *KingAPI) Status(address common.Address) RKStatus {
	api.e.lock.RLock()
	defer api.e.lock.RUnlock()
	return api.statusLocked(address)
}

func (api *KingAPI) statusLocked(address common.Address) RKStatus {
	unlock, ok := api.e.rkLocks[address]
	status := RKStatus{
		Address:      address,
		Registered:   ok,
		Current:      api.e.getCurrentRotatingKing() == address,
		LockedAmount: new(big.Int),
	}
	if ok {
		status.LockedAmount.Set(rkRequiredStake)
		unlockCopy := unlock
		status.UnlockTime = &unlockCopy
	}
	return status
}

func (s *Ethereum) broadcastRotatingKing(address common.Address, unlock time.Time) {
	if s.handler == nil {
		return
	}
	peers := s.handler.peers.all()
	if len(peers) == 0 {
		return
	}
	msg := ethproto.RotatingKingUpdatePacket{
		Address:    address,
		UnlockTime: uint64(unlock.Unix()),
	}
	for _, peer := range peers {
		if err := peer.SendRotatingKingUpdate(msg); err != nil {
			log.Debug("Failed to announce rotating king", "peer", peer.ID(), "address", address.Hex(), "err", err)
		}
	}
}
