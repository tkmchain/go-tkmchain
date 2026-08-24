// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/hkdf"
)

const (
	ShieldedPaymentCodeVersion = 2
	shieldedViewSalt           = "TKM_SHIELDED_VIEW_SALT_V1"
	shieldedViewInfo           = "TKM_SHIELDED_VIEW_X25519_V1"
	shieldedViewBindingDomain  = "TKM_SHIELDED_VIEW_BINDING_V1"
)

var (
	ErrInvalidChainID         = errors.New("invalid shielded payment-code chain ID")
	ErrInvalidShieldedViewKey = errors.New("invalid shielded viewing public key")
)

type shieldedPaymentCodePayload struct {
	Version       uint8  `json:"v"`
	ChainID       uint64 `json:"c"`
	Address       string `json:"a"`
	ViewPublicKey string `json:"k"`
}

// ShieldedViewPrivateKey derives the private X25519 viewing key used to scan
// shielded notes. It does not permit ML-DSA signing or shielded spending, but
// callers must still treat it as secret viewing material and clear it after use.
func ShieldedViewPrivateKey(seed []byte) ([]byte, error) {
	if len(seed) != MLDSA87SeedSize {
		return nil, ErrInvalidPrivateKey
	}
	viewPrivateKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, seed, []byte(shieldedViewSalt), []byte(shieldedViewInfo)), viewPrivateKey); err != nil {
		return nil, err
	}
	return viewPrivateKey, nil
}

// ShieldedViewPublicKey derives the public X25519 viewing key used by the web
// wallet from an ML-DSA seed. The private viewing key is cleared before return.
func ShieldedViewPublicKey(seed []byte) ([]byte, error) {
	viewPrivateKey, err := ShieldedViewPrivateKey(seed)
	if err != nil {
		return nil, err
	}
	defer clear(viewPrivateKey)
	privateKey, err := ecdh.X25519().NewPrivateKey(viewPrivateKey)
	if err != nil {
		return nil, err
	}
	return privateKey.PublicKey().Bytes(), nil
}

// EncodeShieldedPaymentCode encodes the public account identity in the exact
// tkmshield2 format used by browser and exchange clients.
func EncodeShieldedPaymentCode(chainID *big.Int, address common.Address, viewPublicKey []byte) (string, error) {
	if chainID == nil || chainID.Sign() <= 0 || !chainID.IsUint64() {
		return "", ErrInvalidChainID
	}
	if len(viewPublicKey) != 32 {
		return "", ErrInvalidShieldedViewKey
	}
	payload, err := json.Marshal(shieldedPaymentCodePayload{
		Version:       ShieldedPaymentCodeVersion,
		ChainID:       chainID.Uint64(),
		Address:       address.Hex(),
		ViewPublicKey: hex.EncodeToString(viewPublicKey),
	})
	if err != nil {
		return "", err
	}
	return "tkmshield2." + base64.RawURLEncoding.EncodeToString(payload), nil
}

// ShieldedPaymentCode derives the public tkmshield2 payment code associated
// with an ML-DSA account seed. It never returns private viewing material.
func ShieldedPaymentCode(seed []byte, chainID *big.Int, address common.Address) (string, error) {
	viewPublicKey, err := ShieldedViewPublicKey(seed)
	if err != nil {
		return "", err
	}
	return EncodeShieldedPaymentCode(chainID, address, viewPublicKey)
}

// ShieldedViewBindingMessage binds a public viewing key to one PQ address.
func ShieldedViewBindingMessage(address common.Address, viewPublicKey []byte) ([]byte, error) {
	if len(viewPublicKey) != 32 {
		return nil, ErrInvalidShieldedViewKey
	}
	return crypto.Keccak256([]byte(shieldedViewBindingDomain), address.Bytes(), viewPublicKey), nil
}

// SignShieldedViewBinding authenticates public keyfile metadata so it can be
// returned by a passphrase-free local RPC without trusting mutable JSON fields.
func SignShieldedViewBinding(seed []byte, address common.Address, viewPublicKey []byte) ([]byte, error) {
	key, err := NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	message, err := ShieldedViewBindingMessage(address, viewPublicKey)
	if err != nil {
		return nil, err
	}
	return SignMLDSA87(key, message)
}

// VerifyShieldedViewBinding verifies public shielded identity metadata against
// the ML-DSA public key whose hash defines the account address.
func VerifyShieldedViewBinding(publicKey []byte, address common.Address, viewPublicKey, signature []byte) bool {
	derivedAddress, err := Address(AlgorithmMLDSA87, publicKey)
	if err != nil || derivedAddress != address {
		return false
	}
	message, err := ShieldedViewBindingMessage(address, viewPublicKey)
	return err == nil && VerifyMLDSA87(publicKey, message, signature)
}
