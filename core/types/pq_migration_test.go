// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package types

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestPQMigrationDataRoundTrip(t *testing.T) {
	seed, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if want := common.HexToAddress("0x803e6EE61B7Ecba64eDF13ce0c4a8a65C495e5A5"); address != want {
		t.Fatalf("address mismatch: got %s want %s", address, want)
	}
	data, err := NewPQMigrationData(address, pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := ParsePQMigrationData(data)
	if err != nil {
		t.Fatal(err)
	}
	if migration.Address != address {
		t.Fatalf("migration address mismatch: got %s want %s", migration.Address, address)
	}
	if migration.Algorithm != pqcrypto.AlgorithmMLDSA87 {
		t.Fatalf("algorithm mismatch: got %q want %q", migration.Algorithm, pqcrypto.AlgorithmMLDSA87)
	}
	tx := NewTx(&LegacyTx{
		Nonce:    1,
		To:       &address,
		Value:    big.NewInt(1000),
		Gas:      100000,
		GasPrice: big.NewInt(1),
		Data:     data,
	})
	if !IsPQMigrationTx(tx) {
		t.Fatal("valid migration transaction was not recognized")
	}
}

func TestPQMigrationDataRejectsMismatchedAddress(t *testing.T) {
	seed, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatal(err)
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewPQMigrationData(common.HexToAddress("0x0000000000000000000000000000000000000001"), pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(key))
	if err != ErrPQMigrationAddress {
		t.Fatalf("error = %v, want %v", err, ErrPQMigrationAddress)
	}
}

func TestPQMigrationRejectsNonTransferTransactionTypes(t *testing.T) {
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	publicKey := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	data, err := NewPQMigrationData(address, pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	tx := NewTx(&PQTkmTx{
		ChainID:   big.NewInt(8979),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1),
		Gas:       100000,
		To:        &address,
		Value:     big.NewInt(1),
		Data:      data,
	})
	if IsPQMigrationTx(tx) {
		t.Fatal("PQ transaction envelope accepted as a legacy migration")
	}
	zeroValue := NewTransaction(0, address, new(big.Int), 100000, big.NewInt(1), data)
	if IsPQMigrationTx(zeroValue) {
		t.Fatal("zero-value marker accepted as a funds migration")
	}
}
