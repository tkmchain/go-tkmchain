package eth

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// KingAPI provides RPC methods for rotating king configuration.
type KingAPI struct {
	e *Ethereum
}

var (
	rkRequiredStake = new(big.Int).Mul(big.NewInt(50000), big.NewInt(params.Ether))
	rkLockPeriod    = 30 * 24 * time.Hour
)

type RKStatus struct {
	Address       common.Address `json:"address"`
	Registered    bool           `json:"registered"`
	Current       bool           `json:"current"`
	Next          bool           `json:"next"`
	LockedAmount  *big.Int       `json:"lockedAmount"`
	UnlockTime    *time.Time     `json:"unlockTime,omitempty"`
	TotalReceived *big.Int       `json:"totalReceived"`
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
	unlock := time.Now().UTC().Add(rkLockPeriod)
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
	seen := make(map[common.Address]struct{})
	list := make([]RKStatus, 0, len(api.e.kingAddresses)+len(api.e.rkLocks))
	for _, addr := range api.e.kingAddresses {
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		list = append(list, api.statusLocked(addr))
	}
	for addr := range api.e.rkLocks {
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
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
	unlock, locked := api.e.rkLocks[address]
	registered := locked
	for _, addr := range api.e.kingAddresses {
		if addr == address {
			registered = true
			break
		}
	}
	status := RKStatus{
		Address:       address,
		Registered:    registered,
		Current:       api.e.getCurrentRotatingKing() == address,
		Next:          api.e.getNextRotatingKing() == address,
		LockedAmount:  new(big.Int),
		TotalReceived: api.e.totalRotatingKingReward(address),
	}
	if registered {
		status.LockedAmount.Set(rkRequiredStake)
	}
	if locked {
		unlockCopy := unlock
		status.UnlockTime = &unlockCopy
	}
	return status
}

func (s *Ethereum) totalRotatingKingReward(address common.Address) *big.Int {
	total := new(big.Int)
	head := s.blockchain.CurrentBlock()
	if head == nil || len(s.kingAddresses) == 0 {
		return total
	}
	distribution := randomx.DefaultRewardDistribution()
	for block := uint64(1); block <= head.Number.Uint64(); block++ {
		if s.rotatingKingAt(block) != address {
			continue
		}
		reward := randomx.CalculateBlockReward(block)
		reward.Mul(reward, big.NewInt(int64(distribution.RotatingKingPercent)))
		reward.Div(reward, big.NewInt(100))
		total.Add(total, reward)
	}
	return total
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
