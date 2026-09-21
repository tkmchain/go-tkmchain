// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"bytes"
	"crypto/mlkem"
	"crypto/sha512"
	"testing"
)

func TestShieldedV3KEMInteroperability(t *testing.T) {
	seed, pub, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(seed)
	ek, err := mlkem.NewEncapsulationKey1024(pub)
	if err != nil {
		t.Fatal(err)
	}
	shared, ct := ek.Encapsulate()
	defer clear(shared)
	dk, err := mlkem.NewDecapsulationKey1024(seed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := dk.Decapsulate(ct)
	defer clear(got)
	if err != nil || !bytes.Equal(shared, got) {
		t.Fatalf("stdlib interoperability: %v", err)
	}
	derived, err := ShieldedV3ViewPublicKey(seed)
	if err != nil || !bytes.Equal(derived, pub) {
		t.Fatalf("public key reconstruction: %v", err)
	}
}

func TestShieldedV3EncryptionAndTampering(t *testing.T) {
	seed, pub, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(seed)
	ctx := ShieldedV3Context{ChainID: 8979, Purpose: ShieldedV3Incoming, Commitment: sha512.Sum512([]byte("test hiding commitment"))}
	for _, size := range []int{0, 1, 123, ShieldedV3MaxPlaintextSize} {
		plain := bytes.Repeat([]byte{42}, size)
		ct, err := SealShieldedV3(pub, plain, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ct) != ShieldedV3CiphertextSize {
			t.Fatal("plaintext length disclosed")
		}
		opened, err := OpenShieldedV3(seed, ct, ctx)
		if err != nil || !bytes.Equal(opened, plain) {
			t.Fatalf("roundtrip: %v", err)
		}
		clear(opened)
		other, err := SealShieldedV3(pub, plain, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(ct, other) {
			t.Fatal("randomness reused")
		}
	}
	ct, err := SealShieldedV3(pub, []byte("private amount and recipient"), ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise every byte, including implicit-rejection KEM ciphertexts.
	for i := range ct {
		modified := bytes.Clone(ct)
		modified[i] ^= 1
		if _, err := OpenShieldedV3(seed, modified, ctx); err != ErrInvalidShieldedV3Ciphertext {
			t.Fatalf("byte %d: %v", i, err)
		}
	}
	for _, bad := range [][]byte{nil, ct[:len(ct)-1], append(bytes.Clone(ct), 0)} {
		if _, err := OpenShieldedV3(seed, bad, ctx); err != ErrInvalidShieldedV3Ciphertext {
			t.Fatalf("malformed size: %v", err)
		}
	}
	wrong, _, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(wrong)
	if _, err := OpenShieldedV3(wrong, ct, ctx); err != ErrInvalidShieldedV3Ciphertext {
		t.Fatalf("wrong key: %v", err)
	}
	for _, changed := range []ShieldedV3Context{
		{ChainID: 8980, Purpose: ctx.Purpose, Commitment: ctx.Commitment},
		{ChainID: ctx.ChainID, Purpose: ShieldedV3Stamp, Commitment: ctx.Commitment},
		{ChainID: ctx.ChainID, Purpose: ctx.Purpose, Commitment: sha512.Sum512([]byte("different commitment"))},
	} {
		if _, err := OpenShieldedV3(seed, ct, changed); err != ErrInvalidShieldedV3Ciphertext {
			t.Fatalf("context substitution: %v", err)
		}
	}
}

func TestShieldedV3SeparationAndBounds(t *testing.T) {
	master := bytes.Repeat([]byte{7}, 32)
	seen := make(map[string]bool)
	for _, chain := range []uint64{8979, 8980} {
		for _, purpose := range []ShieldedV3Purpose{ShieldedV3Incoming, ShieldedV3Outgoing, ShieldedV3Stamp} {
			seed, err := DeriveShieldedV3ViewKey(master, chain, purpose)
			if err != nil {
				t.Fatal(err)
			}
			if seen[string(seed)] {
				t.Fatal("key reused across purpose or chain")
			}
			seen[string(seed)] = true
			again, err := DeriveShieldedV3ViewKey(master, chain, purpose)
			if err != nil || !bytes.Equal(seed, again) {
				t.Fatal("unstable derivation")
			}
			clear(seed)
			clear(again)
		}
	}
	valid := ShieldedV3Context{ChainID: 8979, Purpose: ShieldedV3Incoming, Commitment: sha512.Sum512([]byte("test hiding commitment"))}
	seed, pub, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(seed)
	for _, ctx := range []ShieldedV3Context{{}, {ChainID: 8979, Purpose: 0, Commitment: valid.Commitment}, {ChainID: 8979, Purpose: 5, Commitment: valid.Commitment}, {ChainID: 8979, Purpose: ShieldedV3Incoming}} {
		if _, err := SealShieldedV3(pub, nil, ctx); err != ErrInvalidShieldedV3Context {
			t.Fatalf("invalid context: %v", err)
		}
	}
	if _, err := SealShieldedV3(pub, make([]byte, ShieldedV3MaxPlaintextSize+1), valid); err != ErrShieldedV3PlaintextTooLarge {
		t.Fatalf("oversize: %v", err)
	}
	if _, err := SealShieldedV3(pub[:len(pub)-1], nil, valid); err != ErrInvalidShieldedViewKey {
		t.Fatalf("invalid public key: %v", err)
	}
	if _, err := DeriveShieldedV3ViewKey(master[:31], 8979, ShieldedV3Incoming); err != ErrInvalidPrivateKey {
		t.Fatalf("invalid master: %v", err)
	}
	if _, err := ShieldedV3ViewPublicKey(make([]byte, 63)); err != ErrInvalidPrivateKey {
		t.Fatalf("invalid seed: %v", err)
	}
}

func TestShield3DisclosureKeyIsScopedAndAuthenticated(t *testing.T) {
	seed, pub, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(seed)
	ctx := ShieldedV3Context{ChainID: 8979, Purpose: ShieldedV3Outgoing, Commitment: sha512.Sum512([]byte("selected output"))}
	one, err := SealShieldedV3(pub, []byte("payment one"), ctx)
	if err != nil {
		t.Fatal(err)
	}
	two, err := SealShieldedV3(pub, []byte("payment two"), ctx)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ShieldedV3RecordKey(seed, one, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	plain, err := OpenShieldedV3RecordKey(key, one, ctx)
	if err != nil || string(plain) != "payment one" {
		t.Fatal("selected payment", err)
	}
	clear(plain)
	if _, err = OpenShieldedV3RecordKey(key, two, ctx); err == nil {
		t.Fatal("disclosure opened another record")
	}
	bad := bytes.Clone(one)
	bad[len(bad)-1] ^= 1
	if _, err = ShieldedV3RecordKey(seed, bad, ctx); err == nil {
		t.Fatal("exported unauthenticated record key")
	}
	wrong := bytes.Clone(seed)
	wrong[0] ^= 1
	if _, err = ShieldedV3RecordKey(wrong, one, ctx); err == nil {
		t.Fatal("wrong wallet exported record key")
	}
	ctx.Purpose = ShieldedV3Incoming
	if _, err = OpenShieldedV3RecordKey(key, one, ctx); err == nil {
		t.Fatal("ignored disclosure role")
	}
}
