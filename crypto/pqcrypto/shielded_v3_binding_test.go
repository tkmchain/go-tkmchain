// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestShieldedV3AuthenticatedViewingMetadata(t *testing.T) {
	signingSeed := bytes.Repeat([]byte{5}, MLDSA87SeedSize)
	signingKey, err := NewMLDSA87FromSeed(signingSeed)
	if err != nil {
		t.Fatal(err)
	}
	pub := PublicKeyBytes(signingKey)
	address, err := Address(AlgorithmMLDSA87, pub)
	if err != nil {
		t.Fatal(err)
	}
	seed, viewPub, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(seed)
	signature, err := SignShieldedV3ViewBinding(signingSeed, 8979, ShieldedV3Incoming, address, viewPub)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyShieldedV3ViewBinding(pub, 8979, ShieldedV3Incoming, address, viewPub, signature) {
		t.Fatal("valid viewing-key binding rejected")
	}
	otherSeed, otherPub, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(otherSeed)
	if VerifyShieldedV3ViewBinding(pub, 8979, ShieldedV3Incoming, address, otherPub, signature) {
		t.Fatal("substituted recipient key accepted")
	}
	if VerifyShieldedV3ViewBinding(pub, 8980, ShieldedV3Incoming, address, viewPub, signature) {
		t.Fatal("cross-chain binding replay accepted")
	}
	if VerifyShieldedV3ViewBinding(pub, 8979, ShieldedV3Stamp, address, viewPub, signature) {
		t.Fatal("stamp-key role substitution accepted")
	}
	if VerifyShieldedV3ViewBinding(pub, 8979, ShieldedV3Incoming, common.Address{}, viewPub, signature) {
		t.Fatal("account substitution accepted")
	}
	changed := bytes.Clone(signature)
	changed[0] ^= 1
	if VerifyShieldedV3ViewBinding(pub, 8979, ShieldedV3Incoming, address, viewPub, changed) {
		t.Fatal("altered PQ signature accepted")
	}
	if _, err := SignShieldedV3ViewBinding(signingSeed, 8979, ShieldedV3Incoming, common.Address{}, viewPub); err != ErrInvalidPublicKey {
		t.Fatalf("incorrect signing account: %v", err)
	}
	if _, err := ShieldedV3ViewBindingMessage(8979, ShieldedV3Incoming, address, make([]byte, 32)); err != ErrInvalidShieldedViewKey {
		t.Fatalf("legacy X25519 key accepted: %v", err)
	}
}
