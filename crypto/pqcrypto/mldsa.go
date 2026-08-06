// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"crypto/rand"
	"errors"
	"io"

	"github.com/emmansun/gmsm/mldsa"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	AlgorithmMLDSA87 = "ML-DSA-87"

	MLDSA87SeedSize       = mldsa.SeedSize
	MLDSA87PublicKeySize  = mldsa.PublicKeySize87
	MLDSA87PrivateKeySize = mldsa.PrivateKeySize87
)

var (
	ErrUnsupportedAlgorithm = errors.New("unsupported post-quantum algorithm")
	ErrInvalidPublicKey     = errors.New("invalid post-quantum public key")
	ErrInvalidPrivateKey    = errors.New("invalid post-quantum private key")
	ErrInvalidSignature     = errors.New("invalid post-quantum signature")
)

// GenerateMLDSA87 creates a new ML-DSA-87 key pair using crypto/rand.
func GenerateMLDSA87() (*mldsa.Key87, error) {
	return mldsa.GenerateKey87(rand.Reader)
}

// NewMLDSA87FromSeed reconstructs a deterministic ML-DSA-87 key pair from a
// 32-byte seed. Store encrypted seeds in wallets, not expanded private keys.
func NewMLDSA87FromSeed(seed []byte) (*mldsa.Key87, error) {
	if len(seed) != MLDSA87SeedSize {
		return nil, ErrInvalidPrivateKey
	}
	return mldsa.NewKey87(seed)
}

// PublicKeyBytes returns the canonical FIPS 204 public-key encoding.
func PublicKeyBytes(key *mldsa.Key87) []byte {
	if key == nil {
		return nil
	}
	pub, ok := key.Public().(*mldsa.PublicKey87)
	if !ok || pub == nil {
		return nil
	}
	return pub.Bytes()
}

// Address derives an EVM-compatible account address from a PQ public key.
// The domain separator prevents collisions with legacy secp256k1 derivation.
func Address(algorithm string, publicKey []byte) (common.Address, error) {
	if algorithm != AlgorithmMLDSA87 {
		return common.Address{}, ErrUnsupportedAlgorithm
	}
	if len(publicKey) != MLDSA87PublicKeySize {
		return common.Address{}, ErrInvalidPublicKey
	}
	var addr common.Address
	digest := crypto.Keccak256([]byte("tkmchain:pq-address:v1:"), []byte(algorithm), publicKey)
	copy(addr[:], digest[12:])
	return addr, nil
}

// SignMLDSA87 signs a consensus message using ML-DSA-87.
func SignMLDSA87(key *mldsa.Key87, message []byte) ([]byte, error) {
	if key == nil {
		return nil, ErrInvalidPrivateKey
	}
	return key.PrivateKey87.Sign(rand.Reader, message, nil)
}

// SignMLDSA87WithRand signs with an explicit randomness source for tests.
func SignMLDSA87WithRand(key *mldsa.Key87, random io.Reader, message []byte) ([]byte, error) {
	if key == nil {
		return nil, ErrInvalidPrivateKey
	}
	if random == nil {
		random = rand.Reader
	}
	return key.PrivateKey87.Sign(random, message, nil)
}

// VerifyMLDSA87 verifies a consensus ML-DSA-87 signature.
func VerifyMLDSA87(publicKey, message, signature []byte) bool {
	if len(publicKey) != MLDSA87PublicKeySize || len(message) == 0 || len(signature) == 0 {
		return false
	}
	pub, err := mldsa.NewPublicKey87(publicKey)
	if err != nil {
		return false
	}
	return pub.VerifyWithOptions(signature, message, nil)
}
