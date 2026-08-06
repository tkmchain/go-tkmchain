// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package types

import (
	"errors"

	"github.com/emmansun/gmsm/mldsa"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

var ErrNotPQTkmTx = errors.New("transaction is not a post-quantum transaction")

// SignPQTkmTx signs a PQ transaction with an ML-DSA-87 key.
func SignPQTkmTx(tx *Transaction, s Signer, key *mldsa.Key87) (*Transaction, error) {
	pqtx, ok := tx.inner.(*PQTkmTx)
	if !ok {
		return nil, ErrNotPQTkmTx
	}
	cpy := pqtx.copy().(*PQTkmTx)
	cpy.Algorithm = pqcrypto.AlgorithmMLDSA87
	cpy.PublicKey = pqcrypto.PublicKeyBytes(key)
	cpy.ChainID = s.ChainID()
	hash := cpy.sigHash(s.ChainID())
	sig, err := pqcrypto.SignMLDSA87(key, hash[:])
	if err != nil {
		return nil, err
	}
	cpy.Signature = sig
	return &Transaction{inner: cpy, time: tx.time}, nil
}

// SignNewPQTkmTx creates and signs a PQ transaction.
func SignNewPQTkmTx(key *mldsa.Key87, s Signer, txdata *PQTkmTx) (*Transaction, error) {
	return SignPQTkmTx(NewTx(txdata), s, key)
}
