// Copyright 2026 The go-ethereum Authors
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

package types

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

// RandomXTx is the native RandomX transaction envelope.
type RandomXTx struct {
	ChainID    *big.Int
	Nonce      uint64
	GasTipCap  *big.Int
	GasFeeCap  *big.Int
	Gas        uint64
	To         *common.Address `rlp:"nil"`
	Value      *big.Int
	Data       []byte
	AccessList AccessList

	// Signature values
	V *big.Int
	R *big.Int
	S *big.Int
}

// copy creates a deep copy of the transaction data and initializes all fields.
func (tx *RandomXTx) copy() TxData {
	cpy := &RandomXTx{
		Nonce:      tx.Nonce,
		To:         copyAddressPtr(tx.To),
		Data:       common.CopyBytes(tx.Data),
		Gas:        tx.Gas,
		AccessList: make(AccessList, len(tx.AccessList)),
		Value:      new(big.Int),
		ChainID:    new(big.Int),
		GasTipCap:  new(big.Int),
		GasFeeCap:  new(big.Int),
		V:          new(big.Int),
		R:          new(big.Int),
		S:          new(big.Int),
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
	if tx.V != nil {
		cpy.V.Set(tx.V)
	}
	if tx.R != nil {
		cpy.R.Set(tx.R)
	}
	if tx.S != nil {
		cpy.S.Set(tx.S)
	}
	return cpy
}

func (tx *RandomXTx) txType() byte           { return RandomXTxType }
func (tx *RandomXTx) chainID() *big.Int      { return tx.ChainID }
func (tx *RandomXTx) accessList() AccessList { return tx.AccessList }
func (tx *RandomXTx) data() []byte           { return tx.Data }
func (tx *RandomXTx) gas() uint64            { return tx.Gas }
func (tx *RandomXTx) gasFeeCap() *big.Int    { return tx.GasFeeCap }
func (tx *RandomXTx) gasTipCap() *big.Int    { return tx.GasTipCap }
func (tx *RandomXTx) gasPrice() *big.Int     { return tx.GasFeeCap }
func (tx *RandomXTx) value() *big.Int        { return tx.Value }
func (tx *RandomXTx) nonce() uint64          { return tx.Nonce }
func (tx *RandomXTx) to() *common.Address    { return tx.To }

func (tx *RandomXTx) effectiveGasPrice(dst *big.Int, baseFee *big.Int) *big.Int {
	if baseFee == nil {
		return dst.Set(tx.GasFeeCap)
	}
	tip := dst.Sub(tx.GasFeeCap, baseFee)
	if tip.Cmp(tx.GasTipCap) > 0 {
		tip.Set(tx.GasTipCap)
	}
	return tip.Add(tip, baseFee)
}

func (tx *RandomXTx) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *RandomXTx) setSignatureValues(chainID, v, r, s *big.Int) {
	tx.ChainID, tx.V, tx.R, tx.S = chainID, v, r, s
}

func (tx *RandomXTx) encode(b *bytes.Buffer) error {
	return rlp.Encode(b, tx)
}

func (tx *RandomXTx) decode(input []byte) error {
	return rlp.DecodeBytes(input, tx)
}

func (tx *RandomXTx) sigHash(chainID *big.Int) common.Hash {
	return prefixedRlpHash(
		RandomXTxType,
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
		})
}
