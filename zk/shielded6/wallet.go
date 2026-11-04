// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package shielded6

import (
	"crypto/hkdf"
	cryptorand "crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"github.com/ethereum/go-ethereum/rlp"
)

const PaymentCodePrefix = "tkmshield6:"

var ErrInvalidWalletOutput = errors.New("shield6: invalid wallet output")

// Address is a chain-bound pair of independent spend and view public keys.
type Address struct {
	ChainID     uint64
	SpendPublic [PointSize]byte
	ViewPublic  [PointSize]byte
}

// WalletKeys are derived from a high-entropy wallet seed and must be cleared
// after use. SpendSecret is the scalar used by CLSAG for newly received notes.
type WalletKeys struct {
	ChainID     uint64
	SpendSecret [ScalarSize]byte
	ViewSecret  [ScalarSize]byte
	SpendPublic [PointSize]byte
	ViewPublic  [PointSize]byte
}

func (k *WalletKeys) Clear() {
	if k != nil {
		clear(k.SpendSecret[:])
		clear(k.ViewSecret[:])
	}
}

func (k *WalletKeys) Address() Address {
	if k == nil {
		return Address{}
	}
	return Address{ChainID: k.ChainID, SpendPublic: k.SpendPublic, ViewPublic: k.ViewPublic}
}

// CreateWallet generates a fresh 256-bit backup seed and derives its
// chain-separated spend and view keys. The caller owns and must securely back
// up seed before clearing it; losing the seed permanently loses note access.
// random may be nil to use the operating system CSPRNG.
func CreateWallet(random io.Reader, chainID uint64) (*WalletKeys, [32]byte, error) {
	var seed [32]byte
	if chainID == 0 {
		return nil, seed, ErrInvalidWalletOutput
	}
	if random == nil {
		random = cryptorand.Reader
	}
	if _, err := io.ReadFull(random, seed[:]); err != nil {
		clear(seed[:])
		return nil, seed, err
	}
	keys, err := DeriveWalletKeys(seed[:], chainID)
	if err != nil {
		clear(seed[:])
		return nil, seed, err
	}
	return keys, seed, nil
}

func DeriveWalletKeys(seed []byte, chainID uint64) (*WalletKeys, error) {
	if len(seed) < 32 || chainID == 0 {
		return nil, ErrInvalidWalletOutput
	}
	spend, err := deriveWalletScalar(seed, chainID, "TKM_SHIELD6_SPEND_SCALAR_V1")
	if err != nil {
		return nil, err
	}
	view, err := deriveWalletScalar(seed, chainID, "TKM_SHIELD6_VIEW_SCALAR_V1")
	if err != nil {
		return nil, err
	}
	defer spend.Set(new(curve25519.Scalar))
	defer view.Set(new(curve25519.Scalar))
	keys := &WalletKeys{ChainID: chainID, SpendSecret: scalarBytes(*spend), ViewSecret: scalarBytes(*view)}
	copy(keys.SpendPublic[:], new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(spend).Bytes())
	copy(keys.ViewPublic[:], new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(view).Bytes())
	return keys, nil
}

func deriveWalletScalar(seed []byte, chainID uint64, domain string) (*curve25519.Scalar, error) {
	var salt [12]byte
	binary.BigEndian.PutUint64(salt[:8], chainID)
	for counter := uint32(0); ; counter++ {
		binary.BigEndian.PutUint32(salt[8:], counter)
		material, err := hkdf.Key(sha512.New, seed, salt[:], domain, ScalarSize)
		if err != nil {
			return nil, err
		}
		scalar, parseErr := new(curve25519.Scalar).SetCanonicalBytes(material)
		clear(material)
		if parseErr == nil && scalar.Equal(new(curve25519.Scalar)) != 1 {
			return scalar, nil
		}
		if counter == ^uint32(0) {
			return nil, ErrInvalidWalletOutput
		}
	}
}

type paymentCodePayload struct {
	Version     uint64
	ChainID     uint64
	SpendPublic [PointSize]byte
	ViewPublic  [PointSize]byte
	Checksum    [4]byte
}

func EncodePaymentCode(address Address) (string, error) {
	if err := address.Validate(); err != nil {
		return "", err
	}
	payload := paymentCodePayload{Version: 1, ChainID: address.ChainID, SpendPublic: address.SpendPublic, ViewPublic: address.ViewPublic}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return "", err
	}
	digest := crypto.Keccak256(append([]byte("TKM_SHIELD6_PAYMENT_CODE_CHECKSUM_V1"), encoded...))
	copy(payload.Checksum[:], digest[:4])
	encoded, err = rlp.EncodeToBytes(payload)
	if err != nil {
		return "", err
	}
	return PaymentCodePrefix + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func DecodePaymentCode(code string, expectedChainID uint64) (Address, error) {
	var address Address
	if len(code) < len(PaymentCodePrefix) || code[:len(PaymentCodePrefix)] != PaymentCodePrefix || expectedChainID == 0 {
		return address, ErrInvalidWalletOutput
	}
	encoded, err := base64.RawURLEncoding.DecodeString(code[len(PaymentCodePrefix):])
	if err != nil || base64.RawURLEncoding.EncodeToString(encoded) != code[len(PaymentCodePrefix):] {
		return address, ErrInvalidWalletOutput
	}
	var payload paymentCodePayload
	if err := rlp.DecodeBytes(encoded, &payload); err != nil || payload.Version != 1 || payload.ChainID != expectedChainID {
		return address, ErrInvalidWalletOutput
	}
	checksum := payload.Checksum
	payload.Checksum = [4]byte{}
	unsigned, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return address, ErrInvalidWalletOutput
	}
	digest := crypto.Keccak256(append([]byte("TKM_SHIELD6_PAYMENT_CODE_CHECKSUM_V1"), unsigned...))
	if !bytesEqual(checksum[:], digest[:4]) {
		return address, ErrInvalidWalletOutput
	}
	address = Address{ChainID: payload.ChainID, SpendPublic: payload.SpendPublic, ViewPublic: payload.ViewPublic}
	return address, address.Validate()
}

func (a Address) Validate() error {
	if a.ChainID == 0 {
		return ErrInvalidWalletOutput
	}
	if _, err := decodePoint(a.SpendPublic[:], false); err != nil {
		return err
	}
	if _, err := decodePoint(a.ViewPublic[:], false); err != nil {
		return err
	}
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
