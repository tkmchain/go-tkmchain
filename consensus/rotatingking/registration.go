// Copyright 2026 The go-ethereum Authors
// Package rotatingking contains the consensus rules for rotating-king
// registration and rotation.
package rotatingking

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	// ErrInvalidRegistration is returned when a registration does not satisfy
	// the deterministic consensus registration rules.
	ErrInvalidRegistration = errors.New("invalid rotating king registration")
	// ErrInsufficientStake is returned when the candidate cannot satisfy the
	// configured minimum stake.
	ErrInsufficientStake = errors.New("insufficient rotating king stake")
	// ErrDuplicateRegistration is returned when an address is already in the
	// rotating-king schedule.
	ErrDuplicateRegistration = errors.New("rotating king is already registered")
)

// KingRegistration is the deterministic registration record used by the
// consensus manager.  The registration hash is domain separated and commits
// to every field that affects activation, so peers cannot reinterpret a
// registration at a different height.
type KingRegistration struct {
	Address          common.Address
	Stake            *big.Int
	AddedHeight      uint64
	ActivationHeight uint64
	RegistrationHash common.Hash
}

// RegistrationHash returns the canonical commitment for a registration.
// It deliberately uses fixed-width integers and a chain-independent domain;
// the chain ID is supplied by the caller through the surrounding transaction
// or block domain when this record is persisted.
func RegistrationHash(address common.Address, stake *big.Int, addedHeight, activationHeight uint64) common.Hash {
	return RegistrationHashForChain(nil, address, stake, addedHeight, activationHeight)
}

// RegistrationHashForChain returns a registration commitment bound to a
// particular chain. A chain ID is required for registrations that cross a
// network boundary (for example, Egypt and mainnet); nil preserves the
// legacy chain-independent commitment used by older callers.
func RegistrationHashForChain(chainID *big.Int, address common.Address, stake *big.Int, addedHeight, activationHeight uint64) common.Hash {
	if stake == nil {
		stake = new(big.Int)
	}
	stakeBytes := stake.Bytes()
	if len(stakeBytes) > 32 {
		stakeBytes = stakeBytes[len(stakeBytes)-32:]
	}
	chainWord := make([]byte, 32)
	if chainID != nil && chainID.Sign() >= 0 {
		chainBytes := chainID.Bytes()
		if len(chainBytes) > len(chainWord) {
			chainBytes = chainBytes[len(chainBytes)-len(chainWord):]
		}
		copy(chainWord[len(chainWord)-len(chainBytes):], chainBytes)
	}
	payload := make([]byte, 0, len("TKM-ROTATING-KING-REGISTRATION-V2")+len(chainWord)+common.AddressLength+32+16)
	payload = append(payload, []byte("TKM-ROTATING-KING-REGISTRATION-V2")...)
	payload = append(payload, chainWord...)
	payload = append(payload, address.Bytes()...)
	var stakeWord [32]byte
	copy(stakeWord[32-len(stakeBytes):], stakeBytes)
	payload = append(payload, stakeWord[:]...)
	var height [8]byte
	binary.BigEndian.PutUint64(height[:], addedHeight)
	payload = append(payload, height[:]...)
	binary.BigEndian.PutUint64(height[:], activationHeight)
	payload = append(payload, height[:]...)
	return crypto.Keccak256Hash(payload)
}

// ValidateRegistration checks the rules that must be identical on every
// node.  The activation delay is measured in blocks and overflow is rejected
// rather than silently wrapping to an already-active registration.
func ValidateRegistration(address common.Address, stake *big.Int, currentHeight, activationDelay uint64, minimumStake *big.Int) error {
	if address == (common.Address{}) || stake == nil || stake.Sign() < 0 || minimumStake == nil || minimumStake.Sign() < 0 {
		return ErrInvalidRegistration
	}
	if stake.Cmp(minimumStake) < 0 {
		return ErrInsufficientStake
	}
	if currentHeight > ^uint64(0)-activationDelay {
		return ErrInvalidRegistration
	}
	return nil
}
