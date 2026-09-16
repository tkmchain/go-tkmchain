// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"crypto/mlkem"
	"crypto/sha512"
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
)

// ShieldedV3ViewBindingMessage authenticates a canonical ML-KEM viewing key
// against a chain, role and PQ signing account. This is metadata authentication,
// not a zero-knowledge spend proof. It does not disclose a private viewing key.
func ShieldedV3ViewBindingMessage(chainID uint64, purpose ShieldedV3Purpose, address common.Address, viewPublicKey []byte) ([]byte, error) {
	if chainID == 0 || !validShieldedV3Purpose(purpose) {
		return nil, ErrInvalidShieldedV3Context
	}
	if _, err := mlkem.NewEncapsulationKey1024(viewPublicKey); err != nil {
		return nil, ErrInvalidShieldedViewKey
	}
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD3_MLKEM1024_VIEW_BINDING_V1"))
	h.Write([]byte{ShieldedV3Version, byte(purpose)})
	var chain [8]byte
	binary.BigEndian.PutUint64(chain[:], chainID)
	h.Write(chain[:])
	h.Write(address[:])
	h.Write(viewPublicKey)
	return h.Sum(nil), nil
}

func SignShieldedV3ViewBinding(signingSeed []byte, chainID uint64, purpose ShieldedV3Purpose, address common.Address, viewPublicKey []byte) ([]byte, error) {
	key, err := NewMLDSA87FromSeed(signingSeed)
	if err != nil {
		return nil, err
	}
	derived, err := Address(AlgorithmMLDSA87, PublicKeyBytes(key))
	if err != nil || derived != address {
		return nil, ErrInvalidPublicKey
	}
	message, err := ShieldedV3ViewBindingMessage(chainID, purpose, address, viewPublicKey)
	if err != nil {
		return nil, err
	}
	return SignMLDSA87(key, message)
}

func VerifyShieldedV3ViewBinding(signingPublicKey []byte, chainID uint64, purpose ShieldedV3Purpose, address common.Address, viewPublicKey, signature []byte) bool {
	derived, err := Address(AlgorithmMLDSA87, signingPublicKey)
	if err != nil || derived != address {
		return false
	}
	message, err := ShieldedV3ViewBindingMessage(chainID, purpose, address, viewPublicKey)
	return err == nil && VerifyMLDSA87(signingPublicKey, message, signature)
}
