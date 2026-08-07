// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package types

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// PQTkmTx is the post-quantum transaction envelope.
//
// The sender is not recovered from an ECDSA recovery id. Consensus verifies the
// ML-DSA signature against PublicKey and derives the sender from that public key.
type PQTkmTx struct {
	ChainID    *big.Int
	Nonce      uint64
	GasTipCap  *big.Int
	GasFeeCap  *big.Int
	Gas        uint64
	To         *common.Address `rlp:"nil"`
	Value      *big.Int
	Data       []byte
	AccessList AccessList

	Algorithm string
	PublicKey []byte
	Signature []byte
}

func (tx *PQTkmTx) copy() TxData {
	cpy := &PQTkmTx{
		Nonce:      tx.Nonce,
		To:         copyAddressPtr(tx.To),
		Data:       common.CopyBytes(tx.Data),
		Gas:        tx.Gas,
		AccessList: make(AccessList, len(tx.AccessList)),
		Value:      new(big.Int),
		ChainID:    new(big.Int),
		GasTipCap:  new(big.Int),
		GasFeeCap:  new(big.Int),
		Algorithm:  tx.Algorithm,
		PublicKey:  common.CopyBytes(tx.PublicKey),
		Signature:  common.CopyBytes(tx.Signature),
	}
	copy(cpy.AccessList, tx.AccessList)
	if tx.Value != nil {
		cpy.Value.Set(tx.Value)
	}
	if tx.ChainID != nil {
		cpy.ChainID.Set(tx.ChainID)
	}
	if tx.GasTipCap != nil {
		cpy.GasTipCap.Set(tx.GasTipCap)
	}
	if tx.GasFeeCap != nil {
		cpy.GasFeeCap.Set(tx.GasFeeCap)
	}
	return cpy
}

func (tx *PQTkmTx) txType() byte           { return PQTkmTxType }
func (tx *PQTkmTx) chainID() *big.Int      { return tx.ChainID }
func (tx *PQTkmTx) accessList() AccessList { return tx.AccessList }
func (tx *PQTkmTx) data() []byte           { return tx.Data }
func (tx *PQTkmTx) gas() uint64            { return tx.Gas }
func (tx *PQTkmTx) gasFeeCap() *big.Int    { return tx.GasFeeCap }
func (tx *PQTkmTx) gasTipCap() *big.Int    { return tx.GasTipCap }
func (tx *PQTkmTx) gasPrice() *big.Int     { return tx.GasFeeCap }
func (tx *PQTkmTx) value() *big.Int        { return tx.Value }
func (tx *PQTkmTx) nonce() uint64          { return tx.Nonce }
func (tx *PQTkmTx) to() *common.Address    { return tx.To }

func (tx *PQTkmTx) effectiveGasPrice(dst *big.Int, baseFee *big.Int) *big.Int {
	if baseFee == nil {
		return dst.Set(tx.GasFeeCap)
	}
	tip := dst.Sub(tx.GasFeeCap, baseFee)
	if tip.Cmp(tx.GasTipCap) > 0 {
		tip.Set(tx.GasTipCap)
	}
	return tip.Add(tip, baseFee)
}

func (tx *PQTkmTx) rawSignatureValues() (v, r, s *big.Int) {
	return new(big.Int), new(big.Int), new(big.Int)
}

func (tx *PQTkmTx) setSignatureValues(chainID, v, r, s *big.Int) {
	if chainID != nil {
		tx.ChainID = new(big.Int).Set(chainID)
	}
}

func (tx *PQTkmTx) encode(b *bytes.Buffer) error {
	return rlp.Encode(b, tx)
}

func (tx *PQTkmTx) decode(input []byte) error {
	return rlp.DecodeBytes(input, tx)
}

func (tx *PQTkmTx) sigHash(chainID *big.Int) common.Hash {
	return prefixedRlpHash(
		PQTkmTxType,
		[]any{
			chainID,
			tx.Nonce,
			tx.GasTipCap,
			tx.GasFeeCap,
			tx.Gas,
			tx.To,
			tx.Value,
			tx.Data,
			tx.AccessList,
			tx.Algorithm,
			tx.PublicKey,
		})
}

func (tx *PQTkmTx) validateSignature(hash common.Hash) (common.Address, error) {
	if tx.Algorithm != pqcrypto.AlgorithmMLDSA87 {
		return common.Address{}, pqcrypto.ErrUnsupportedAlgorithm
	}
	addr, err := pqcrypto.Address(tx.Algorithm, tx.PublicKey)
	if err != nil {
		return common.Address{}, err
	}
	if !pqcrypto.VerifyMLDSA87(tx.PublicKey, hash[:], tx.Signature) {
		return common.Address{}, pqcrypto.ErrInvalidSignature
	}
	return addr, nil
}

// PQTkmFields returns the public post-quantum signature metadata for RPC callers.
func (tx *Transaction) PQTkmFields() (algorithm string, publicKey []byte, signature []byte, ok bool) {
	pqtx, ok := tx.inner.(*PQTkmTx)
	if !ok {
		return "", nil, nil, false
	}
	return pqtx.Algorithm, common.CopyBytes(pqtx.PublicKey), common.CopyBytes(pqtx.Signature), true
}
