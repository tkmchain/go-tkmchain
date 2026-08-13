// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package txpool

import (
	"crypto/ecdsa"
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
)

func TestValidateTransactionEIP2681(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	head := &types.Header{
		Number:     big.NewInt(1),
		GasLimit:   5000000,
		Time:       1,
		Difficulty: big.NewInt(1),
	}

	signer := types.LatestSigner(params.TestChainConfig)

	// Create validation options
	opts := &ValidationOptions{
		Config:       params.TestChainConfig,
		Accept:       0xFF, // Accept all transaction types
		MaxSize:      32 * 1024,
		MaxBlobCount: 6,
		MinTip:       big.NewInt(0),
	}

	tests := []struct {
		name    string
		nonce   uint64
		wantErr error
	}{
		{
			name:    "normal nonce",
			nonce:   42,
			wantErr: nil,
		},
		{
			name:    "max allowed nonce (2^64-2)",
			nonce:   math.MaxUint64 - 1,
			wantErr: nil,
		},
		{
			name:    "EIP-2681 nonce overflow (2^64-1)",
			nonce:   math.MaxUint64,
			wantErr: core.ErrNonceMax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := createTestTransaction(key, tt.nonce)
			err := ValidateTransaction(tx, head, signer, opts)

			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateTransaction() error = %v, wantErr nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateTransaction() error = nil, wantErr %v", tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateTransaction() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateTransactionRejectsUnprotectedLegacyAfterEIP155(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	head := &types.Header{
		Number:     big.NewInt(1),
		GasLimit:   5000000,
		Time:       1,
		Difficulty: big.NewInt(1),
	}
	opts := &ValidationOptions{
		Config:       params.TestChainConfig,
		Accept:       0xFF,
		MaxSize:      32 * 1024,
		MaxBlobCount: 6,
		MinTip:       big.NewInt(0),
	}

	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	tx := types.NewTransaction(0, to, big.NewInt(1000), 21000, big.NewInt(1), nil)
	unprotected, err := types.SignTx(tx, types.HomesteadSigner{}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(unprotected, head, types.LatestSigner(params.TestChainConfig), opts); !errors.Is(err, ErrInvalidSender) {
		t.Fatalf("ValidateTransaction() error = %v, want %v", err, ErrInvalidSender)
	}

	protected, err := types.SignTx(tx, types.LatestSigner(params.TestChainConfig), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(protected, head, types.LatestSigner(params.TestChainConfig), opts); err != nil {
		t.Fatalf("ValidateTransaction() protected error = %v", err)
	}
}

func TestValidateTransactionQuantumResistantFork(t *testing.T) {
	ecdsaKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pqKey, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	config := *params.TestChainConfig
	config.QuantumResistantTime = new(uint64)
	*config.QuantumResistantTime = 10
	migrationRecovery := uint64(20)
	config.PQMigrationRecoveryTime = &migrationRecovery
	head := &types.Header{
		Number:     big.NewInt(1),
		GasLimit:   5000000,
		Time:       10,
		Difficulty: big.NewInt(1),
	}
	opts := &ValidationOptions{
		Config:       &config,
		Accept:       0xFF,
		MaxSize:      64 * 1024,
		MaxBlobCount: 6,
		MinTip:       big.NewInt(0),
	}
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	legacy, err := types.SignTx(types.NewTransaction(0, to, big.NewInt(1), 21000, big.NewInt(10), nil), types.NewEIP155Signer(config.ChainID), ecdsaKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(legacy, head, types.MakeSigner(&config, head.Number, head.Time), opts); !errors.Is(err, core.ErrTxTypeNotSupported) {
		t.Fatalf("legacy tx post-quantum error = %v, want %v", err, core.ErrTxTypeNotSupported)
	}
	pqAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(pqKey))
	if err != nil {
		t.Fatal(err)
	}
	migrationData, err := types.NewPQMigrationData(pqAddress, pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(pqKey))
	if err != nil {
		t.Fatal(err)
	}
	migration, err := types.SignTx(types.NewTransaction(0, pqAddress, big.NewInt(1), 100000, big.NewInt(10), migrationData), types.NewEIP155Signer(config.ChainID), ecdsaKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(migration, head, types.MakeSigner(&config, head.Number, head.Time), opts); !errors.Is(err, core.ErrTxTypeNotSupported) {
		t.Fatalf("migration before recovery error = %v, want %v", err, core.ErrTxTypeNotSupported)
	}
	head.Time = migrationRecovery
	if err := ValidateTransaction(migration, head, types.MakeSigner(&config, head.Number, head.Time), opts); err != nil {
		t.Fatalf("migration at recovery fork error = %v", err)
	}
	if err := ValidateTransaction(legacy, head, types.MakeSigner(&config, head.Number, head.Time), opts); !errors.Is(err, core.ErrTxTypeNotSupported) {
		t.Fatalf("ordinary legacy tx at recovery fork error = %v, want %v", err, core.ErrTxTypeNotSupported)
	}
	pqtx, err := types.SignNewPQTkmTx(pqKey, types.MakeSigner(&config, head.Number, head.Time), &types.PQTkmTx{
		ChainID:   config.ChainID,
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(10),
		Gas:       21000,
		To:        &to,
		Value:     big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(pqtx, head, types.MakeSigner(&config, head.Number, head.Time), opts); err != nil {
		t.Fatalf("PQ tx post-quantum error = %v", err)
	}
	head.Time = 9
	if err := ValidateTransaction(pqtx, head, types.MakeSigner(&config, head.Number, head.Time), opts); !errors.Is(err, core.ErrTxTypeNotSupported) {
		t.Fatalf("PQ tx pre-quantum error = %v, want %v", err, core.ErrTxTypeNotSupported)
	}
}

func TestValidateTransactionPQMigrationRecoveryWithPrivacy(t *testing.T) {
	legacyKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pqKey, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	activation := uint64(10)
	recovery := uint64(20)
	config := *params.TestChainConfig
	config.QuantumResistantTime = &activation
	config.PrivacyCommitmentTime = &activation
	config.PQMigrationRecoveryTime = &recovery
	head := &types.Header{
		Number:     big.NewInt(1),
		GasLimit:   5000000,
		Time:       recovery,
		Difficulty: big.NewInt(1),
	}
	opts := &ValidationOptions{
		Config:       &config,
		Accept:       0xFF,
		MaxSize:      64 * 1024,
		MaxBlobCount: 6,
		MinTip:       big.NewInt(0),
	}
	publicKey := pqcrypto.PublicKeyBytes(pqKey)
	pqAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	data, err := types.NewPQMigrationData(pqAddress, pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := types.SignTx(
		types.NewTransaction(0, pqAddress, big.NewInt(1), 100000, big.NewInt(10), data),
		types.NewEIP155Signer(config.ChainID),
		legacyKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(tx, head, types.MakeSigner(&config, head.Number, head.Time), opts); err != nil {
		t.Fatalf("privacy-era PQ migration recovery error = %v", err)
	}
}

func TestValidateTransactionRejectsTransparentPQAfterPrivacyCommitments(t *testing.T) {
	pqKey, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	config := *params.TestChainConfig
	activation := uint64(10)
	config.QuantumResistantTime = &activation
	config.PrivacyCommitmentTime = &activation
	head := &types.Header{
		Number:     big.NewInt(1),
		GasLimit:   5000000,
		Time:       activation,
		Difficulty: big.NewInt(1),
	}
	opts := &ValidationOptions{
		Config:       &config,
		Accept:       0xFF,
		MaxSize:      64 * 1024,
		MaxBlobCount: 6,
		MinTip:       big.NewInt(0),
	}
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	pqtx, err := types.SignNewPQTkmTx(pqKey, types.MakeSigner(&config, head.Number, head.Time), &types.PQTkmTx{
		ChainID:   config.ChainID,
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(10),
		Gas:       21000,
		To:        &to,
		Value:     new(big.Int),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(pqtx, head, types.MakeSigner(&config, head.Number, head.Time), opts); !errors.Is(err, core.ErrInvalidShieldedTx) {
		t.Fatalf("transparent PQ tx post-privacy error = %v, want %v", err, core.ErrInvalidShieldedTx)
	}
}

func TestValidateTransactionRejectsInvalidPQMigrationMarker(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	head := &types.Header{
		Number:     big.NewInt(1),
		GasLimit:   5000000,
		Time:       1,
		Difficulty: big.NewInt(1),
	}
	opts := &ValidationOptions{
		Config:       params.TestChainConfig,
		Accept:       0xFF,
		MaxSize:      32 * 1024,
		MaxBlobCount: 6,
		MinTip:       big.NewInt(0),
	}
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    0,
		To:       &to,
		Value:    big.NewInt(1),
		Gas:      100000,
		GasPrice: big.NewInt(1),
		Data:     []byte("TKMPQMIG1invalid"),
	})
	signed, err := types.SignTx(tx, types.LatestSigner(params.TestChainConfig), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransaction(signed, head, types.LatestSigner(params.TestChainConfig), opts); err == nil || err.Error() != "invalid post-quantum migration transaction" {
		t.Fatalf("ValidateTransaction() error = %v, want invalid post-quantum migration transaction", err)
	}
}

// createTestTransaction creates a basic transaction for testing
func createTestTransaction(key *ecdsa.PrivateKey, nonce uint64) *types.Transaction {
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")

	txdata := &types.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    big.NewInt(1000),
		Gas:      21000,
		GasPrice: big.NewInt(1),
		Data:     nil,
	}

	tx := types.NewTx(txdata)
	signedTx, _ := types.SignTx(tx, types.LatestSigner(params.TestChainConfig), key)
	return signedTx
}
