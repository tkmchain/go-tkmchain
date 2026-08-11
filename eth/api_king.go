package eth

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// KingAPI provides RPC methods for rotating king configuration.
type KingAPI struct {
	e *Ethereum
}

var (
	// Required stake for rotating king registration (50,000 TKM)
	rkRequiredStake = new(big.Int).Mul(big.NewInt(50000), big.NewInt(params.Ether))
	// Lock period for staked funds (30 days)
	rkLockPeriod = 30 * 24 * time.Hour
	// High fee reserved for rotating king registration transactions (1 TKM)
	rkRegistrationFee = new(big.Int).SetUint64(params.Ether)
)

type rkLockInfo struct {
	Hash             common.Hash
	UnlockTime       time.Time
	UnlockHeight     uint64
	ActivationHeight uint64
	AddedHeight      uint64
}

// RKStatus represents the status of a rotating king candidate
type RKStatus struct {
	Hash                common.Hash    `json:"hash"`
	Address             common.Address `json:"address"`
	Registered          bool           `json:"registered"`
	Current             bool           `json:"current"`
	Next                bool           `json:"next"`
	NextRotationHeight  uint64         `json:"nextRotationHeight,omitempty"`
	BlocksUntilRotation uint64         `json:"blocksUntilRotation,omitempty"`
	LockedAmount        *big.Int       `json:"lockedAmount"`
	RegistrationFee     *big.Int       `json:"registrationFee"`
	UnlockTime          *time.Time     `json:"unlockTime,omitempty"`
	UnlockHeight        uint64         `json:"unlockHeight,omitempty"`
	AddedHeight         uint64         `json:"addedHeight,omitempty"`
	TotalReceived       *big.Int       `json:"totalReceived"`
}

// MainKingInfo describes the configured main king.
type MainKingInfo struct {
	Address common.Address `json:"address"`
}

// MainKingAPI provides RPC methods for the main king address.
type MainKingAPI struct {
	e *Ethereum
}

func NewMainKingAPI(e *Ethereum) *MainKingAPI {
	return &MainKingAPI{e: e}
}

func (api *MainKingAPI) Address() common.Address {
	return api.e.GetMainKingAddress()
}

func (api *MainKingAPI) GetAddress() common.Address {
	return api.Address()
}

func (api *MainKingAPI) GetInfo() MainKingInfo {
	return MainKingInfo{Address: api.Address()}
}

// RotatingKingInfo describes the current rotating king schedule.
type RotatingKingInfo struct {
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
	api.e.lock.Lock()
	defer api.e.lock.Unlock()
	api.e.loadRotatingKingStateLocked()
	api.e.releaseUnlockedRotatingKingsLocked()

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
	api.e.lock.Lock()
	defer api.e.lock.Unlock()
	api.e.loadRotatingKingStateLocked()
	api.e.releaseUnlockedRotatingKingsLocked()
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
	api.e.lock.Lock()
	defer api.e.lock.Unlock()
	api.e.loadRotatingKingStateLocked()
	api.e.releaseUnlockedRotatingKingsLocked()

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

// AddCheckpoint adds an immutable checkpoint after the main king node verifies the local block hash.
func (api *KingAPI) AddCheckpoint(number hexutil.Uint64, hash common.Hash) (bool, error) {
	if err := api.e.addCheckpoint(uint64(number), hash); err != nil {
		return false, err
	}
	return true, nil
}

// AddSignedCheckpoint adds and broadcasts a checkpoint signed by the main king address.
func (api *KingAPI) AddSignedCheckpoint(number hexutil.Uint64, hash common.Hash, signature hexutil.Bytes) (bool, error) {
	if err := api.e.addSignedCheckpoint(uint64(number), hash, []byte(signature)); err != nil {
		return false, err
	}
	api.e.broadcastCheckpoint(uint64(number), hash, []byte(signature))
	return true, nil
}

// CheckpointSigningHash returns the 32-byte digest the main king must sign for a checkpoint.
func (api *KingAPI) CheckpointSigningHash(number hexutil.Uint64, hash common.Hash) common.Hash {
	return api.e.checkpointSigningHash(uint64(number), hash)
}

// Add registers an address as rotating king if stake requirement is met.
func (api *KingAPI) Add(address common.Address) (RKStatus, error) {
	api.e.lock.Lock()
	api.e.loadRotatingKingStateLocked()
	api.e.removeUnderfundedRotatingKingsLocked()
	if api.e.isRotatingKingRegisteredLocked(address) {
		api.e.lock.Unlock()
		return RKStatus{}, fmt.Errorf("rotating king address already registered: %s", address.Hex())
	}
	api.e.lock.Unlock()
	if err := api.e.ensureRotatingKingEligible(address); err != nil {
		return RKStatus{}, err
	}
	unlock := time.Now().UTC().Add(rkLockPeriod)
	api.e.lock.Lock()
	api.e.loadRotatingKingStateLocked()
	if api.e.isRotatingKingRegisteredLocked(address) {
		api.e.lock.Unlock()
		return RKStatus{}, fmt.Errorf("rotating king address already registered: %s", address.Hex())
	}
	api.e.recordRotatingKingLocked(address, unlock, api.e.unlockHeightForTime(unlock))
	status := api.statusLocked(address)
	api.e.lock.Unlock()

	api.e.broadcastRotatingKing(address, unlock)
	return status, nil
}

// List returns all registered rotating king addresses with status.
func (api *KingAPI) List() []RKStatus {
	api.e.lock.Lock()
	defer api.e.lock.Unlock()
	api.e.loadRotatingKingStateLocked()
	api.e.releaseUnlockedRotatingKingsLocked()

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
	api.e.lock.Lock()
	defer api.e.lock.Unlock()
	api.e.loadRotatingKingStateLocked()
	api.e.releaseUnlockedRotatingKingsLocked()
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
		Address:         address,
		Registered:      registered,
		Current:         api.e.getCurrentRotatingKing() == address,
		Next:            api.e.getNextRotatingKing() == address,
		LockedAmount:    new(big.Int),
		RegistrationFee: new(big.Int),
		// Historical reward accounting must not run while holding the global king
		// lock. It previously made rk_add/rk_status exceed the HTTP RPC deadline.
		TotalReceived: new(big.Int),
	}

	if height, ok := api.e.nextRotationHeight(address); ok {
		status.NextRotationHeight = height
		if head := api.e.blockchain.CurrentBlock(); head != nil && height > head.Number.Uint64() {
			status.BlocksUntilRotation = height - head.Number.Uint64()
		}
	}

	if locked {
		status.LockedAmount.Set(rkRequiredStake)
		status.RegistrationFee.Set(rkRegistrationFee)
	}

	if locked {
		status.Hash = lockInfo.Hash
		unlockCopy := lockInfo.UnlockTime
		status.UnlockTime = &unlockCopy
		status.UnlockHeight = lockInfo.UnlockHeight
		status.AddedHeight = lockInfo.AddedHeight
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
	required := new(big.Int).Add(rkRequiredStake, rkRegistrationFee)
	if balance.Sign() == 0 {
		return fmt.Errorf("insufficient balance: address has no balance, need at least %s wei", required.String())
	}
	if balance.Cmp(required) < 0 {
		return fmt.Errorf("insufficient balance: have %s wei, need at least %s wei", balance.String(), required.String())
	}
	return nil
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

func (s *Ethereum) storeCheckpoint(number uint64, hash common.Hash) error {
	if s.checkpointDb == nil {
		return fmt.Errorf("checkpoint database is not available")
	}
	if err := rawdb.WriteTrustedCheckpoint(s.checkpointDb, number, hash); err != nil {
		return err
	}
	return params.AddCheckpoint(number, hash)
}

func (s *Ethereum) loadCheckpoints() error {
	if s.checkpointDb == nil {
		return fmt.Errorf("checkpoint database is not available")
	}
	hardcoded := make(map[uint64]common.Hash)
	for _, checkpoint := range params.AllCheckpoints() {
		hardcoded[checkpoint.Number] = checkpoint.Hash
		if err := rawdb.WriteCheckpoint(s.checkpointDb, checkpoint.Number, checkpoint.Hash); err != nil {
			return err
		}
	}
	trustedLoaded := 0
	legacyPruned := 0
	for _, checkpoint := range rawdb.ReadCheckpoints(s.checkpointDb) {
		if hash, ok := hardcoded[checkpoint.Number]; !ok || hash != checkpoint.Hash {
			if !rawdb.ReadTrustedCheckpoint(s.checkpointDb, checkpoint.Number) {
				legacyPruned++
				if err := rawdb.DeleteCheckpoint(s.checkpointDb, checkpoint.Number); err != nil {
					log.Warn("Failed to prune legacy unsigned checkpoint", "number", checkpoint.Number, "hash", checkpoint.Hash, "err", err)
				}
				continue
			}
			trustedLoaded++
		}
		if err := params.AddCheckpoint(checkpoint.Number, checkpoint.Hash); err != nil {
			return err
		}
	}
	if legacyPruned > 0 {
		log.Info("Pruned legacy unsigned checkpoints", "count", legacyPruned)
	}
	if trustedLoaded > 0 {
		log.Info("Loaded trusted persisted checkpoints", "count", trustedLoaded)
	}
	return nil
}

func (s *Ethereum) addCheckpoint(number uint64, hash common.Hash) error {
	mainKing := s.GetMainKingAddress()
	if mainKing == (common.Address{}) {
		return fmt.Errorf("main king address is not configured")
	}
	if s.config.Miner.Etherbase != mainKing {
		return fmt.Errorf("checkpoint can only be added by the main king wallet %s", mainKing.Hex())
	}
	return s.validateAndStoreCheckpoint(number, hash)
}

func (s *Ethereum) addSignedCheckpoint(number uint64, hash common.Hash, signature []byte) error {
	if err := s.verifyCheckpointSignature(number, hash, signature); err != nil {
		return err
	}
	return s.validateAndStoreCheckpoint(number, hash)
}

func (s *Ethereum) validateAndStoreCheckpoint(number uint64, hash common.Hash) error {
	block := s.blockchain.GetBlockByNumber(number)
	if block == nil {
		return fmt.Errorf("block %d is not available locally", number)
	}
	if block.Hash() != hash {
		return fmt.Errorf("checkpoint hash mismatch at block %d: have %s, want %s", number, hash, block.Hash())
	}
	return s.storeCheckpoint(number, hash)
}

func (s *Ethereum) checkpointSigningHash(number uint64, hash common.Hash) common.Hash {
	chainID := (*big.Int)(nil)
	if s.blockchain != nil && s.blockchain.Config() != nil {
		chainID = s.blockchain.Config().ChainID
	}
	return checkpointSigningHash(chainID, number, hash)
}

func checkpointSigningHash(chainID *big.Int, number uint64, hash common.Hash) common.Hash {
	chainBytes := []byte(nil)
	if chainID != nil {
		chainBytes = chainID.Bytes()
	}
	payload := make([]byte, 0, len("TKMCHAIN_SIGNED_CHECKPOINT_V1")+2+len(chainBytes)+8+common.HashLength)
	payload = append(payload, []byte("TKMCHAIN_SIGNED_CHECKPOINT_V1")...)
	var chainLen [2]byte
	binary.BigEndian.PutUint16(chainLen[:], uint16(len(chainBytes)))
	payload = append(payload, chainLen[:]...)
	payload = append(payload, chainBytes...)
	var numberBytes [8]byte
	binary.BigEndian.PutUint64(numberBytes[:], number)
	payload = append(payload, numberBytes[:]...)
	payload = append(payload, hash.Bytes()...)
	return crypto.Keccak256Hash(payload)
}

func (s *Ethereum) verifyCheckpointSignature(number uint64, hash common.Hash, signature []byte) error {
	mainKing := s.GetMainKingAddress()
	if mainKing == (common.Address{}) {
		return fmt.Errorf("main king address is not configured")
	}
	if len(signature) != crypto.SignatureLength {
		return fmt.Errorf("checkpoint signature must be %d bytes", crypto.SignatureLength)
	}
	sig := append([]byte(nil), signature...)
	if sig[crypto.RecoveryIDOffset] >= 27 {
		sig[crypto.RecoveryIDOffset] -= 27
	}
	if sig[crypto.RecoveryIDOffset] > 1 {
		return fmt.Errorf("invalid checkpoint signature recovery id %d", sig[crypto.RecoveryIDOffset])
	}
	chainID := (*big.Int)(nil)
	if s.blockchain != nil && s.blockchain.Config() != nil {
		chainID = s.blockchain.Config().ChainID
	}
	digest := checkpointSigningHash(chainID, number, hash)
	if signer, err := recoverCheckpointSigner(digest, sig); err == nil && signer == mainKing {
		return nil
	}
	prefixed := crypto.Keccak256Hash([]byte("\x19Ethereum Signed Message:\n32"), digest.Bytes())
	signer, err := recoverCheckpointSigner(prefixed, sig)
	if err != nil {
		return fmt.Errorf("invalid checkpoint signature: %w", err)
	}
	if signer != mainKing {
		return fmt.Errorf("checkpoint signed by %s, want main king %s", signer.Hex(), mainKing.Hex())
	}
	return nil
}

func recoverCheckpointSigner(digest common.Hash, signature []byte) (common.Address, error) {
	pub, err := crypto.SigToPub(digest.Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pub), nil
}

func (s *Ethereum) banCheckpointPeer(peerID string, duration time.Duration) {
	if s.handler == nil {
		return
	}
	if s.handler.downloader != nil {
		s.handler.downloader.BanPeer(peerID, duration)
	}
	s.handler.removePeer(peerID)
}

func (s *Ethereum) noteCheckpointFromPeer(number uint64, hash common.Hash, signature []byte, peerID string) {
	if existing, ok := params.GetCheckpoint(number); ok {
		if existing != hash {
			log.Warn("Banning peer for conflicting checkpoint", "number", number, "hash", hash, "existing", existing, "peer", peerID)
			s.banCheckpointPeer(peerID, 365*24*time.Hour)
		}
		return
	}
	if err := s.verifyCheckpointSignature(number, hash, signature); err != nil {
		log.Warn("Banning peer for invalid signed checkpoint", "number", number, "hash", hash, "peer", peerID, "err", err)
		s.banCheckpointPeer(peerID, 365*24*time.Hour)
		return
	}
	block := s.blockchain.GetBlockByNumber(number)
	if block == nil {
		log.Warn("Ignoring signed checkpoint for unavailable block", "number", number, "hash", hash, "peer", peerID)
		return
	}
	if block.Hash() != hash {
		log.Warn("Banning peer for checkpoint with mismatched local block hash", "number", number, "announced", hash, "local", block.Hash(), "peer", peerID)
		s.banCheckpointPeer(peerID, 365*24*time.Hour)
		return
	}
	if err := s.storeCheckpoint(number, hash); err != nil {
		log.Warn("Banning peer for conflicting checkpoint", "number", number, "hash", hash, "peer", peerID, "err", err)
		s.banCheckpointPeer(peerID, 365*24*time.Hour)
		return
	}
	s.broadcastCheckpointExcept(number, hash, signature, peerID)
}

func (s *Ethereum) broadcastCheckpoint(number uint64, hash common.Hash, signature []byte) {
	s.broadcastCheckpointExcept(number, hash, signature, "")
}

func (s *Ethereum) broadcastCheckpointExcept(number uint64, hash common.Hash, signature []byte, skip string) {
	if s.handler == nil {
		return
	}
	peers := s.handler.peers.all()
	if len(peers) == 0 {
		return
	}
	msg := ethproto.CheckpointUpdatePacket{Number: number, Hash: hash, Signature: append([]byte(nil), signature...)}
	for _, peer := range peers {
		if skip != "" && peer.ID() == skip {
			continue
		}
		if err := peer.SendCheckpointUpdate(msg); err != nil {
			log.Debug("Failed to announce checkpoint", "peer", peer.ID(), "number", number, "hash", hash, "err", err)
		}
	}
}
