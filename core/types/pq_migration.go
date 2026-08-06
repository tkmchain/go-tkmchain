// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package types

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidPQMigrationData = errors.New("invalid post-quantum migration data")
	ErrPQMigrationAddress     = errors.New("post-quantum migration address mismatch")
)

var pqMigrationDataMagic = []byte("TKMPQMIG1")

// PQMigration records the public PQ account identity that a legacy account is
// moving funds to before the quantum-resistant transaction fork activates.
type PQMigration struct {
	Address   common.Address
	Algorithm string
	PublicKey []byte
}

// NewPQMigrationData returns calldata that marks a normal value transfer as a
// PQ account migration. The recipient must be the address derived from publicKey.
func NewPQMigrationData(address common.Address, algorithm string, publicKey []byte) ([]byte, error) {
	derived, err := pqcrypto.Address(algorithm, publicKey)
	if err != nil {
		return nil, err
	}
	if derived != address {
		return nil, ErrPQMigrationAddress
	}
	payload, err := rlp.EncodeToBytes(&PQMigration{
		Address:   address,
		Algorithm: algorithm,
		PublicKey: append([]byte(nil), publicKey...),
	})
	if err != nil {
		return nil, err
	}
	data := make([]byte, 0, len(pqMigrationDataMagic)+len(payload))
	data = append(data, pqMigrationDataMagic...)
	data = append(data, payload...)
	return data, nil
}

// ParsePQMigrationData parses and validates a PQ migration calldata marker.
func ParsePQMigrationData(data []byte) (*PQMigration, error) {
	if !bytes.HasPrefix(data, pqMigrationDataMagic) {
		return nil, ErrInvalidPQMigrationData
	}
	var migration PQMigration
	if err := rlp.DecodeBytes(data[len(pqMigrationDataMagic):], &migration); err != nil {
		return nil, err
	}
	derived, err := pqcrypto.Address(migration.Algorithm, migration.PublicKey)
	if err != nil {
		return nil, err
	}
	if derived != migration.Address {
		return nil, ErrPQMigrationAddress
	}
	return &migration, nil
}

// HasPQMigrationDataPrefix reports whether data claims to carry a PQ migration marker.
func HasPQMigrationDataPrefix(data []byte) bool {
	return bytes.HasPrefix(data, pqMigrationDataMagic)
}

// ValidPQMigrationDataForRecipient reports whether data carries a valid PQ
// migration marker bound to recipient.
func ValidPQMigrationDataForRecipient(data []byte, recipient common.Address) bool {
	migration, err := ParsePQMigrationData(data)
	if err != nil {
		return false
	}
	return migration.Address == recipient
}

// IsPQMigrationTx reports whether tx is a normal transfer that carries a valid
// PQ migration marker for its recipient.
func IsPQMigrationTx(tx *Transaction) bool {
	if tx == nil || tx.To() == nil {
		return false
	}
	return ValidPQMigrationDataForRecipient(tx.Data(), *tx.To())
}
