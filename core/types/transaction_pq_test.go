// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package types

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestPQTkmTransactionSigningAndSender(t *testing.T) {
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	to := common.HexToAddress("0x0000000000000000000000000000000000001234")
	signer := NewQuantumSigner(big.NewInt(8979))
	tx, err := SignNewPQTkmTx(key, signer, &PQTkmTx{
		ChainID:   big.NewInt(8979),
		Nonce:     7,
		GasTipCap: big.NewInt(2),
		GasFeeCap: big.NewInt(10),
		Gas:       21000,
		To:        &to,
		Value:     big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	want, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(key))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Sender(signer, tx)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("sender mismatch: got %s want %s", got, want)
	}
}

func TestPQTkmTransactionTamperFails(t *testing.T) {
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	to := common.HexToAddress("0x0000000000000000000000000000000000001234")
	signer := NewQuantumSigner(big.NewInt(8979))
	tx, err := SignNewPQTkmTx(key, signer, &PQTkmTx{
		ChainID:   big.NewInt(8979),
		Nonce:     7,
		GasTipCap: big.NewInt(2),
		GasFeeCap: big.NewInt(10),
		Gas:       21000,
		To:        &to,
		Value:     big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := tx.inner.copy().(*PQTkmTx)
	tampered.Value = big.NewInt(2)
	if _, err := Sender(signer, NewTx(tampered)); err == nil {
		t.Fatal("tampered PQ transaction unexpectedly verified")
	}
}

func TestPQTkmTransactionRequiresQuantumSigner(t *testing.T) {
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	to := common.HexToAddress("0x0000000000000000000000000000000000001234")
	quantumSigner := NewQuantumSigner(big.NewInt(8979))
	tx, err := SignNewPQTkmTx(key, quantumSigner, &PQTkmTx{
		ChainID:   big.NewInt(8979),
		Nonce:     7,
		GasTipCap: big.NewInt(2),
		GasFeeCap: big.NewInt(10),
		Gas:       21000,
		To:        &to,
		Value:     big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sender(NewLondonSigner(big.NewInt(8979)), tx); err != ErrTxTypeNotSupported {
		t.Fatalf("pre-quantum signer error = %v, want %v", err, ErrTxTypeNotSupported)
	}
}
