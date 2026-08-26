// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"crypto/ecdh"
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	emailVMKeySalt = "TKM_EMAILVM_X25519_SALT_V1"
	emailVMKeyInfo = "TKM_EMAILVM_X25519_KEY_V1"
)

// EmailVMPrivateKey derives the deterministic X25519 mail-decryption key used
// by the web wallet. It is separate from both the ML-DSA spending key and the
// shielded viewing key and must be cleared by its caller after use.
func EmailVMPrivateKey(seed []byte) ([]byte, error) {
	if len(seed) != MLDSA87SeedSize {
		return nil, ErrInvalidPrivateKey
	}
	privateKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, seed, []byte(emailVMKeySalt), []byte(emailVMKeyInfo)), privateKey); err != nil {
		return nil, err
	}
	return privateKey, nil
}

// EmailVMPublicKey derives the public X25519 encryption key published by an
// EmailVM mailbox. No private key material is returned.
func EmailVMPublicKey(seed []byte) ([]byte, error) {
	privateBytes, err := EmailVMPrivateKey(seed)
	if err != nil {
		return nil, err
	}
	defer clear(privateBytes)
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return nil, err
	}
	return privateKey.PublicKey().Bytes(), nil
}
