// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package keystore

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/google/uuid"
)

const pqKeyVersion = 4

// PQKey is a plaintext post-quantum account key.
//
// Seed contains the compact ML-DSA seed. The expanded private key is derived
// only when signing and is not stored in the keystore file.
type PQKey struct {
	Id        uuid.UUID
	Address   common.Address
	Algorithm string
	PublicKey []byte
	Seed      []byte
}

type encryptedPQKeyJSONV4 struct {
	Address               string     `json:"address"`
	Algorithm             string     `json:"algorithm"`
	PublicKey             string     `json:"publicKey"`
	ShieldedViewPublicKey string     `json:"shieldedViewPublicKey,omitempty"`
	ShieldedViewSignature string     `json:"shieldedViewSignature,omitempty"`
	Crypto                CryptoJSON `json:"crypto"`
	Id                    string     `json:"id"`
	Version               int        `json:"version"`
}

// NewPQKey creates a new ML-DSA-87 key.
func NewPQKey() (*PQKey, error) {
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		return nil, err
	}
	return NewPQKeyFromSeed(key.Seed())
}

// NewPQKeyFromSeed creates an ML-DSA-87 key from a raw 32-byte seed.
func NewPQKeyFromSeed(seed []byte) (*PQKey, error) {
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	pub := pqcrypto.PublicKeyBytes(key)
	addr, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("could not create random uuid: %w", err)
	}
	return &PQKey{
		Id:        id,
		Address:   addr,
		Algorithm: pqcrypto.AlgorithmMLDSA87,
		PublicKey: pub,
		Seed:      common.CopyBytes(seed),
	}, nil
}

func zeroPQKey(key *PQKey) {
	if key == nil {
		return
	}
	clear(key.Seed)
}

func EncryptPQKey(key *PQKey, auth string, scryptN, scryptP int) ([]byte, error) {
	if key == nil || key.Algorithm != pqcrypto.AlgorithmMLDSA87 || len(key.Seed) != pqcrypto.MLDSA87SeedSize {
		return nil, pqcrypto.ErrInvalidPrivateKey
	}
	cryptoStruct, err := EncryptDataV3(key.Seed, []byte(auth), scryptN, scryptP)
	if err != nil {
		return nil, err
	}
	viewPublicKey, err := pqcrypto.ShieldedViewPublicKey(key.Seed)
	if err != nil {
		return nil, err
	}
	viewSignature, err := pqcrypto.SignShieldedViewBinding(key.Seed, key.Address, viewPublicKey)
	if err != nil {
		return nil, err
	}
	encrypted := encryptedPQKeyJSONV4{
		Address:               hex.EncodeToString(key.Address[:]),
		Algorithm:             key.Algorithm,
		PublicKey:             hex.EncodeToString(key.PublicKey),
		ShieldedViewPublicKey: hex.EncodeToString(viewPublicKey),
		ShieldedViewSignature: hex.EncodeToString(viewSignature),
		Crypto:                cryptoStruct,
		Id:                    key.Id.String(),
		Version:               pqKeyVersion,
	}
	return json.Marshal(encrypted)
}

func DecryptPQKey(keyjson []byte, auth string) (*PQKey, error) {
	var encrypted encryptedPQKeyJSONV4
	if err := json.Unmarshal(keyjson, &encrypted); err != nil {
		return nil, err
	}
	if encrypted.Version != pqKeyVersion {
		return nil, fmt.Errorf("unsupported PQ key version: %d", encrypted.Version)
	}
	if encrypted.Algorithm != pqcrypto.AlgorithmMLDSA87 {
		return nil, pqcrypto.ErrUnsupportedAlgorithm
	}
	seed, err := DecryptDataV3(encrypted.Crypto, auth)
	if err != nil {
		return nil, err
	}
	key, err := NewPQKeyFromSeed(seed)
	clear(seed)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(encrypted.Id)
	if err != nil {
		zeroPQKey(key)
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}
	pub, err := hex.DecodeString(encrypted.PublicKey)
	if err != nil {
		zeroPQKey(key)
		return nil, err
	}
	addrBytes, err := hex.DecodeString(encrypted.Address)
	if err != nil {
		zeroPQKey(key)
		return nil, err
	}
	addr := common.BytesToAddress(addrBytes)
	if key.Address != addr || !bytes.Equal(key.PublicKey, pub) {
		zeroPQKey(key)
		return nil, fmt.Errorf("PQ key content mismatch")
	}
	if encrypted.ShieldedViewPublicKey != "" {
		viewPublicKey, err := hex.DecodeString(encrypted.ShieldedViewPublicKey)
		if err != nil {
			zeroPQKey(key)
			return nil, err
		}
		expectedViewPublicKey, err := pqcrypto.ShieldedViewPublicKey(key.Seed)
		if err != nil || !bytes.Equal(viewPublicKey, expectedViewPublicKey) {
			zeroPQKey(key)
			return nil, fmt.Errorf("PQ shielded viewing key mismatch")
		}
		viewSignature, err := hex.DecodeString(encrypted.ShieldedViewSignature)
		if err != nil || !pqcrypto.VerifyShieldedViewBinding(key.PublicKey, key.Address, viewPublicKey, viewSignature) {
			zeroPQKey(key)
			return nil, fmt.Errorf("PQ shielded viewing key signature mismatch")
		}
	}
	key.Id = id
	return key, nil
}
