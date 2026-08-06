package rawdb

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRotatingKingAddressStorage(t *testing.T) {
	db := NewMemoryDatabase()
	addresses := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000001"),
		common.HexToAddress("0x0000000000000000000000000000000000000002"),
	}

	if got := ReadRotatingKingAddresses(db); got != nil {
		t.Fatalf("empty rotating king addresses = %v, want nil", got)
	}
	WriteRotatingKingAddresses(db, addresses)
	if got := ReadRotatingKingAddresses(db); !reflect.DeepEqual(got, addresses) {
		t.Fatalf("rotating king addresses = %v, want %v", got, addresses)
	}
}

func TestPrivacyActivationStorage(t *testing.T) {
	db := NewMemoryDatabase()
	activations := []PrivacyActivation{{
		Address:        common.HexToAddress("0x0000000000000000000000000000000000000001"),
		PaymentHash:    common.HexToHash("0x1234"),
		PaidHeight:     10,
		ActivateHeight: 15,
		Amount:         big.NewInt(1_000_000_000_000_000_000),
	}}

	if got := ReadPrivacyActivations(db); got != nil {
		t.Fatalf("empty privacy activations = %v, want nil", got)
	}
	WritePrivacyActivations(db, activations)
	if got := ReadPrivacyActivations(db); !reflect.DeepEqual(got, activations) {
		t.Fatalf("privacy activations = %+v, want %+v", got, activations)
	}
}

func TestPrivacyCommitmentStorage(t *testing.T) {
	db := NewMemoryDatabase()
	commitments := []PrivacyCommitment{{
		Commitment:       common.HexToHash("0x1234"),
		EncryptedPayload: []byte("ciphertext"),
		Nonce:            []byte("nonce"),
		PaidHeight:       10,
		ActivateHeight:   15,
		Amount:           big.NewInt(1_000_000_000_000_000_000),
		PayloadHash:      common.HexToHash("0x5678"),
		EphemeralPubKey:  []byte("ephemeral"),
		ViewTag:          []byte{0x7},
	}}

	if got := ReadPrivacyCommitments(db); got != nil {
		t.Fatalf("empty privacy commitments = %v, want nil", got)
	}
	WritePrivacyCommitments(db, commitments)
	if got := ReadPrivacyCommitments(db); !reflect.DeepEqual(got, commitments) {
		t.Fatalf("privacy commitments = %+v, want %+v", got, commitments)
	}
}

func TestPrivacyNullifierStorage(t *testing.T) {
	db := NewMemoryDatabase()
	nullifiers := []PrivacyNullifier{{
		Nullifier:          common.HexToHash("0x99"),
		ProofHash:          common.HexToHash("0x77"),
		EncryptedSpendData: []byte("spend"),
		SpentHeight:        20,
	}}

	if got := ReadPrivacyNullifiers(db); got != nil {
		t.Fatalf("empty privacy nullifiers = %v, want nil", got)
	}
	WritePrivacyNullifiers(db, nullifiers)
	if got := ReadPrivacyNullifiers(db); !reflect.DeepEqual(got, nullifiers) {
		t.Fatalf("privacy nullifiers = %+v, want %+v", got, nullifiers)
	}
}

func TestRotatingKingLockStorageIncludesHash(t *testing.T) {
	db := NewMemoryDatabase()
	locks := []RotatingKingLock{{
		Address:          common.HexToAddress("0x0000000000000000000000000000000000000001"),
		UnlockTime:       123,
		UnlockHeight:     456,
		ActivationHeight: 400,
		AddedHeight:      300,
		Hash:             common.HexToHash("0x1234"),
	}}

	WriteRotatingKingLocks(db, locks)
	if got := ReadRotatingKingLocks(db); !reflect.DeepEqual(got, locks) {
		t.Fatalf("rotating king locks = %+v, want %+v", got, locks)
	}
}
