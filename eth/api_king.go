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
        // Required stake for rotating king registration (50,000 ANTD)
        rkRequiredStake = new(big.Int).Mul(big.NewInt(50000), big.NewInt(params.Ether))
        // Lock period for staked funds (30 days)
        rkLockPeriod = 30 * 24 * time.Hour
)

type rkLockInfo struct {
        UnlockTime       time.Time
        UnlockHeight     uint64
        ActivationHeight uint64
}

// RKStatus represents the status of a rotating king candidate
type RKStatus struct {
        Address             common.Address `json:"address"`
        Registered          bool           `json:"registered"`
        Current             bool           `json:"current"`
        Next                bool           `json:"next"`
        NextRotationHeight  uint64         `json:"nextRotationHeight,omitempty"`
        BlocksUntilRotation uint64         `json:"blocksUntilRotation,omitempty"`
        LockedAmount        *big.Int       `json:"lockedAmount"`
        UnlockTime          *time.Time     `json:"unlockTime,omitempty"`
        UnlockHeight        uint64         `json:"unlockHeight,omitempty"`
        TotalReceived       *big.Int       `json:"totalReceived"`
}

// RotatingKingInfo describes the current rotating king schedule.
type RotatingKingInfo struct {
        MainKing            common.Address   `json:"mainKing"`
        CurrentKing         common.Address   `json:"currentKing"`
        NextKing            common.Address   `json:"nextKing"`
        KingAddresses       []common.Address `json:"kingAddresses"`
        RotationInterval    uint64           `json:"rotationInterval"`
        CurrentBlock        uint64           `json:"currentBlock"`
        NextRotationHeight  uint64           `json:"nextRotationHeight"`
        BlocksUntilRotation uint64           `json:"blocksUntilRotation"`
}

// KingStats describes the rotating king schedule and registered king status.
type KingStats struct {
        MainKing            common.Address `json:"mainKing"`
        CurrentKing         common.Address `json:"currentKing"`
        NextKing            common.Address `json:"nextKing"`
        TotalKings          int            `json:"totalKings"`
        RegisteredKings     int            `json:"registeredKings"`
        RotationInterval    uint64         `json:"rotationInterval"`
        CurrentBlock        uint64         `json:"currentBlock"`
        NextRotationHeight  uint64         `json:"nextRotationHeight"`
        BlocksUntilRotation uint64         `json:"blocksUntilRotation"`
        Kings               []RKStatus     `json:"kings"`
}

// RotationHistoryEntry describes one historical rotating king slot.
type RotationHistoryEntry struct {
        BlockHeight  uint64         `json:"blockHeight"`
        PreviousKing common.Address `json:"previousKing"`
        NewKing      common.Address `json:"newKing"`
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

// GetInfo returns the current rotating king schedule.
func (api *KingAPI) GetInfo() RotatingKingInfo {
        api.e.lock.RLock()
        defer api.e.lock.RUnlock()

        var currentBlock uint64
        if head := api.e.blockchain.CurrentBlock(); head != nil {
                currentBlock = head.Number.Uint64()
        }
        interval := api.e.rotatingKingInterval()
        nextRotation := uint64(0)
        blocksUntilRotation := uint64(0)
        if interval > 0 {
                nextRotation = ((currentBlock / interval) + 1) * interval
                if nextRotation > currentBlock {
                        blocksUntilRotation = nextRotation - currentBlock
                }
        }

        return RotatingKingInfo{
                MainKing:            api.e.GetMainKingAddress(),
                CurrentKing:         api.e.getCurrentRotatingKing(),
                NextKing:            api.e.getNextRotatingKing(),
                KingAddresses:       api.e.GetKingAddresses(),
                RotationInterval:    interval,
                CurrentBlock:        currentBlock,
                NextRotationHeight:  nextRotation,
                BlocksUntilRotation: blocksUntilRotation,
        }
}

// GetKingAddresses returns all configured rotating king addresses.
func (api *KingAPI) GetKingAddresses() []common.Address {
        return api.Addresses()
}

// GetCurrentKing returns the current rotating king address.
func (api *KingAPI) GetCurrentKing() common.Address {
        api.e.lock.RLock()
        defer api.e.lock.RUnlock()
        return api.e.getCurrentRotatingKing()
}

// GetRotationHistory returns recent rotating king changes derived from the chain height.
func (api *KingAPI) GetRotationHistory(limit *uint64) []RotationHistoryEntry {
        api.e.lock.RLock()
        defer api.e.lock.RUnlock()

        head := api.e.blockchain.CurrentBlock()
        if head == nil || len(api.e.kingAddresses) == 0 {
                return nil
        }
        interval := api.e.rotatingKingInterval()
        if interval == 0 {
                return nil
        }
        currentBlock := head.Number.Uint64()
        if currentBlock < interval {
                return nil
        }

        maxEntries := uint64(100)
        if limit != nil && *limit > 0 && *limit < maxEntries {
                maxEntries = *limit
        }
        firstHeight := interval
        lastHeight := (currentBlock / interval) * interval
        if rotations := lastHeight / interval; rotations > maxEntries {
                firstHeight = (rotations - maxEntries + 1) * interval
        }

        history := make([]RotationHistoryEntry, 0, (lastHeight-firstHeight)/interval+1)
        for height := firstHeight; height <= lastHeight; height += interval {
                history = append(history, RotationHistoryEntry{
                        BlockHeight:  height,
                        PreviousKing: api.e.rotatingKingAt(height - 1),
                        NewKing:      api.e.rotatingKingAt(height),
                })
        }
        return history
}

// GetKingStats returns the current rotating king schedule and registered king statuses.
func (api *KingAPI) GetKingStats(_ *interface{}) KingStats {
        api.e.lock.RLock()
        defer api.e.lock.RUnlock()

        var currentBlock uint64
        if head := api.e.blockchain.CurrentBlock(); head != nil {
                currentBlock = head.Number.Uint64()
        }
        interval := api.e.rotatingKingInterval()
        nextRotation := uint64(0)
        blocksUntilRotation := uint64(0)
        if interval > 0 {
                nextRotation = ((currentBlock / interval) + 1) * interval
                if nextRotation > currentBlock {
                        blocksUntilRotation = nextRotation - currentBlock
                }
        }
        
        seen := make(map[common.Address]struct{})
        kings := make([]RKStatus, 0, len(api.e.kingAddresses)+len(api.e.rkLocks))
        
        for _, addr := range api.e.kingAddresses {
                if _, ok := seen[addr]; ok {
                        continue
                }
                seen[addr] = struct{}{}
                kings = append(kings, api.statusLocked(addr))
        }
        for addr := range api.e.rkLocks {
                if _, ok := seen[addr]; ok {
                        continue
                }
                seen[addr] = struct{}{}
                kings = append(kings, api.statusLocked(addr))
        }

        return KingStats{
                MainKing:            api.e.GetMainKingAddress(),
                CurrentKing:         api.e.getCurrentRotatingKing(),
                NextKing:            api.e.getNextRotatingKing(),
                TotalKings:          len(api.e.kingAddresses),
                RegisteredKings:     len(kings),
                RotationInterval:    interval,
                CurrentBlock:        currentBlock,
                NextRotationHeight:  nextRotation,
                BlocksUntilRotation: blocksUntilRotation,
                Kings:               kings,
        }
}

// Add registers an address as rotating king if stake requirement is met.
func (api *KingAPI) Add(address common.Address) (RKStatus, error) {
        if err := api.e.ensureRotatingKingEligible(address); err != nil {
                return RKStatus{}, err
        }
        unlock := time.Now().UTC().Add(rkLockPeriod)
        api.e.lock.Lock()
        api.e.recordRotatingKingLocked(address, unlock, api.e.unlockHeightForTime(unlock))
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
        lockInfo, locked := api.e.rkLocks[address]
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
        
        if height, ok := api.e.nextRotationHeight(address); ok {
                status.NextRotationHeight = height
                if head := api.e.blockchain.CurrentBlock(); head != nil && height > head.Number.Uint64() {
                        status.BlocksUntilRotation = height - head.Number.Uint64()
                }
        }
        
        if registered {
                status.LockedAmount.Set(rkRequiredStake)
        }
        
        if locked {
                unlockCopy := lockInfo.UnlockTime
                status.UnlockTime = &unlockCopy
                status.UnlockHeight = lockInfo.UnlockHeight
        }
        
        return status
}

// ==================== Helper methods for Ethereum backend ====================

func (s *Ethereum) ensureRotatingKingEligible(address common.Address) error {
        if address == (common.Address{}) {
                return fmt.Errorf("invalid rotating king address: zero address")
        }
        if s.blockchain == nil {
                return fmt.Errorf("no blockchain available")
        }
        header := s.blockchain.CurrentBlock()
        if header == nil {
                return fmt.Errorf("no head block available")
        }
        statedb, err := s.blockchain.StateAt(header)
        if err != nil {
                return err
        }
        balance := statedb.GetBalance(address).ToBig()
        if balance.Sign() == 0 {
                return fmt.Errorf("insufficient balance: address has no balance, need at least %s wei", rkRequiredStake.String())
        }
        if balance.Cmp(rkRequiredStake) < 0 {
                return fmt.Errorf("insufficient balance: have %s wei, need at least %s wei", balance.String(), rkRequiredStake.String())
        }
        return nil
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
        s.broadcastRotatingKingExcept(address, unlock, "")
}

func (s *Ethereum) broadcastRotatingKingExcept(address common.Address, unlock time.Time, skip string) {
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
                if skip != "" && peer.ID() == skip {
                        continue
                }
                if err := peer.SendRotatingKingUpdate(msg); err != nil {
                        log.Debug("Failed to announce rotating king", "peer", peer.ID(), "address", address.Hex(), "err", err)
                }
        }
}
