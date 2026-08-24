// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestShieldedPaymentCodeMatchesWebWallet(t *testing.T) {
	seed := make([]byte, MLDSA87SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	viewPublicKey, err := ShieldedViewPublicKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	if have, want := hex.EncodeToString(viewPublicKey), "37895750d443bea81eb17a07e247bdccd6578b60f2ccaca22fe10713e9cf3d78"; have != want {
		t.Fatalf("viewing public key = %s, want %s", have, want)
	}
	viewPrivateKey, err := ShieldedViewPrivateKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(viewPrivateKey)
	if have, want := hex.EncodeToString(viewPrivateKey), "1f5fac0ccd5e66e1094d61510cc6b4dd61612442da69b15cd8c0d76415efccf3"; have != want {
		t.Fatalf("viewing private key = %s, want %s", have, want)
	}
	code, err := ShieldedPaymentCode(seed, big.NewInt(8979), common.HexToAddress("0x1111111111111111111111111111111111111111"))
	if err != nil {
		t.Fatal(err)
	}
	want := "tkmshield2.eyJ2IjoyLCJjIjo4OTc5LCJhIjoiMHgxMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExIiwiayI6IjM3ODk1NzUwZDQ0M2JlYTgxZWIxN2EwN2UyNDdiZGNjZDY1NzhiNjBmMmNjYWNhMjJmZTEwNzEzZTljZjNkNzgifQ"
	if code != want {
		t.Fatalf("payment code = %s, want %s", code, want)
	}
}

func TestEncodeShieldedPaymentCodeRejectsInvalidPublicInputs(t *testing.T) {
	if _, err := EncodeShieldedPaymentCode(nil, common.Address{}, make([]byte, 32)); err != ErrInvalidChainID {
		t.Fatalf("nil chain ID error = %v, want %v", err, ErrInvalidChainID)
	}
	if _, err := EncodeShieldedPaymentCode(big.NewInt(8979), common.Address{}, make([]byte, 31)); err != ErrInvalidShieldedViewKey {
		t.Fatalf("short viewing key error = %v, want %v", err, ErrInvalidShieldedViewKey)
	}
}

func TestShieldedViewBinding(t *testing.T) {
	seed := make([]byte, MLDSA87SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	key, err := NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := PublicKeyBytes(key)
	address, err := Address(AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	viewPublicKey, err := ShieldedViewPublicKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := SignShieldedViewBinding(seed, address, viewPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyShieldedViewBinding(publicKey, address, viewPublicKey, signature) {
		t.Fatal("valid shielded viewing-key binding was rejected")
	}
	tamperedViewPublicKey := append([]byte(nil), viewPublicKey...)
	tamperedViewPublicKey[0] ^= 0x01
	if VerifyShieldedViewBinding(publicKey, address, tamperedViewPublicKey, signature) {
		t.Fatal("tampered shielded viewing key was accepted")
	}
	if VerifyShieldedViewBinding(publicKey, common.Address{}, viewPublicKey, signature) {
		t.Fatal("shielded viewing-key binding was accepted for another address")
	}
}
