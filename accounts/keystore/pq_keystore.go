// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package keystore

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

const AlgorithmECDSA = "ECDSA-secp256k1"

// PQMigrationAccount contains the local key material metadata needed to move a
// legacy account balance into a newly created PQ account.
type PQMigrationAccount struct {
	LegacyAccount accounts.Account
	PQAccount     accounts.Account
	PQAlgorithm   string
	PQPublicKey   []byte
	MigrationData []byte
}

func isPQKeyJSON(keyjson []byte) bool {
	var header struct {
		Version   int    `json:"version"`
		Algorithm string `json:"algorithm"`
	}
	if err := json.Unmarshal(keyjson, &header); err != nil {
		return false
	}
	return header.Version == pqKeyVersion && header.Algorithm == pqcrypto.AlgorithmMLDSA87
}

func signPQTkmTxWithKey(tx *types.Transaction, signer types.Signer, key *PQKey) (*types.Transaction, error) {
	if key == nil || key.Algorithm != pqcrypto.AlgorithmMLDSA87 {
		return nil, pqcrypto.ErrInvalidPrivateKey
	}
	mldsaKey, err := pqcrypto.NewMLDSA87FromSeed(key.Seed)
	if err != nil {
		return nil, err
	}
	return types.SignPQTkmTx(tx, signer, mldsaKey)
}

// AccountAlgorithm reports the local keystore algorithm for an account.
func (ks *KeyStore) AccountAlgorithm(a accounts.Account) (string, error) {
	a, err := ks.Find(a)
	if err != nil {
		return "", err
	}
	keyjson, err := os.ReadFile(a.URL.Path)
	if err != nil {
		return "", err
	}
	if isPQKeyJSON(keyjson) {
		return pqcrypto.AlgorithmMLDSA87, nil
	}
	return AlgorithmECDSA, nil
}

// AccountAlgorithms reports algorithm metadata for every indexed keystore account.
func (ks *KeyStore) AccountAlgorithms() map[common.Address]string {
	accounts := ks.Accounts()
	out := make(map[common.Address]string, len(accounts))
	for _, account := range accounts {
		algorithm, err := ks.AccountAlgorithm(account)
		if err != nil {
			continue
		}
		out[account.Address] = algorithm
	}
	return out
}

// PreparePQMigration creates a new ML-DSA-87 account and returns the migration
// calldata needed for a legacy-signed value transfer into that PQ account.
func (ks *KeyStore) PreparePQMigration(a accounts.Account, legacyPassphrase, pqPassphrase string) (*PQMigrationAccount, error) {
	legacy, key, pqKey, err := ks.getDecryptedAnyKey(a, legacyPassphrase)
	if err != nil {
		return nil, err
	}
	if pqKey != nil {
		zeroPQKey(pqKey)
		return nil, errors.New("source account is already post-quantum")
	}
	if key != nil {
		zeroKey(key.PrivateKey)
	}

	newKey, err := NewPQKey()
	if err != nil {
		return nil, err
	}
	defer zeroPQKey(newKey)
	migrationData, err := types.NewPQMigrationData(newKey.Address, newKey.Algorithm, newKey.PublicKey)
	if err != nil {
		return nil, err
	}

	ks.importMu.Lock()
	defer ks.importMu.Unlock()
	if ks.cache.hasAddress(newKey.Address) {
		return nil, ErrAccountAlreadyExists
	}
	pqAccount, err := ks.importPQKey(newKey, pqPassphrase)
	if err != nil {
		return nil, err
	}
	return &PQMigrationAccount{
		LegacyAccount: legacy,
		PQAccount:     pqAccount,
		PQAlgorithm:   newKey.Algorithm,
		PQPublicKey:   common.CopyBytes(newKey.PublicKey),
		MigrationData: migrationData,
	}, nil
}
