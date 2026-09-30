// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

// Package antartical contains the deterministic consensus primitives enabled
// by the Antartical protocol upgrade.
package antartical

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidUserOperation = errors.New("invalid Antartical user operation")
	ErrInvalidOperationSig  = errors.New("invalid Antartical user operation signature")
)

// UserOperation is the consensus representation shared by EIP-4337 and
// RIP-7560 style account abstraction. Signature is intentionally excluded from
// the signing tuple and included only in the operation envelope.
type UserOperation struct {
	Sender               common.Address
	Nonce                *big.Int
	InitCode             []byte
	CallData             []byte
	CallGasLimit         *big.Int
	VerificationGasLimit *big.Int
	PreVerificationGas   *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	PaymasterAndData     []byte
	Algorithm            string
	PublicKey            []byte
	Signature            []byte
}

type userOperationWire struct {
	Magic                []byte
	Sender               common.Address
	Nonce                *big.Int
	InitCode             []byte
	CallData             []byte
	CallGasLimit         *big.Int
	VerificationGasLimit *big.Int
	PreVerificationGas   *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	PaymasterAndData     []byte
	Algorithm            string
	PublicKey            []byte
	Signature            []byte
}

func (op *UserOperation) validate() error {
	if op == nil || op.Sender == (common.Address{}) || op.Nonce == nil || op.Nonce.Sign() < 0 {
		return ErrInvalidUserOperation
	}
	for _, n := range []*big.Int{op.CallGasLimit, op.VerificationGasLimit, op.PreVerificationGas, op.MaxFeePerGas, op.MaxPriorityFeePerGas} {
		if n == nil || n.Sign() < 0 || n.BitLen() > 256 {
			return ErrInvalidUserOperation
		}
	}
	if op.Nonce.BitLen() > 256 {
		return ErrInvalidUserOperation
	}
	if op.MaxPriorityFeePerGas.Cmp(op.MaxFeePerGas) > 0 {
		return ErrInvalidUserOperation
	}
	return nil
}

func (op *UserOperation) signingTuple(chainID *big.Int, entryPoint common.Address) ([]byte, error) {
	if err := op.validate(); err != nil || chainID == nil || chainID.Sign() <= 0 || entryPoint == (common.Address{}) {
		return nil, ErrInvalidUserOperation
	}
	// EIP-4337 EntryPoint.getUserOpHash first hashes the packed operation and
	// then ABI-encodes that hash with the EntryPoint and chain ID. Every value
	// below is a single 32-byte ABI word; dynamic fields are represented by
	// their keccak256 hash exactly as in the canonical EntryPoint contract.
	packed := make([]byte, 0, 11*32)
	packed = append(packed, abiAddressWord(op.Sender)...)
	packed = append(packed, abiBigWord(op.Nonce)...)
	packed = append(packed, crypto.Keccak256(op.InitCode)...)
	packed = append(packed, crypto.Keccak256(op.CallData)...)
	packed = append(packed, abiBigWord(op.CallGasLimit)...)
	packed = append(packed, abiBigWord(op.VerificationGasLimit)...)
	packed = append(packed, abiBigWord(op.PreVerificationGas)...)
	packed = append(packed, abiBigWord(op.MaxFeePerGas)...)
	packed = append(packed, abiBigWord(op.MaxPriorityFeePerGas)...)
	packed = append(packed, crypto.Keccak256(op.PaymasterAndData)...)
	inner := crypto.Keccak256(packed)
	outer := make([]byte, 0, 96)
	outer = append(outer, inner...)
	outer = append(outer, abiAddressWord(entryPoint)...)
	outer = append(outer, abiBigWord(chainID)...)
	return outer, nil
}

func abiAddressWord(address common.Address) []byte {
	word := make([]byte, 32)
	copy(word[12:], address[:])
	return word
}

func abiBigWord(value *big.Int) []byte {
	word := make([]byte, 32)
	if value != nil {
		copy(word[32-len(value.Bytes()):], value.Bytes())
	}
	return word
}

// Hash returns the operation hash signed by the account owner.
func (op *UserOperation) Hash(chainID *big.Int, entryPoint common.Address) (common.Hash, error) {
	tuple, err := op.signingTuple(chainID, entryPoint)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(tuple), nil
}

// Encode is the canonical network envelope for a UserOperation. It is kept
// separate from legacy transaction RLP so old clients cannot reinterpret an
// account-abstraction operation as an ordinary EVM transaction.
func (op *UserOperation) Encode() ([]byte, error) {
	if err := op.validate(); err != nil {
		return nil, err
	}
	return rlp.EncodeToBytes(userOperationWire{
		Magic: []byte("TKM_USER_OPERATION_V1"), Sender: op.Sender, Nonce: op.Nonce,
		InitCode: op.InitCode, CallData: op.CallData, CallGasLimit: op.CallGasLimit,
		VerificationGasLimit: op.VerificationGasLimit, PreVerificationGas: op.PreVerificationGas,
		MaxFeePerGas: op.MaxFeePerGas, MaxPriorityFeePerGas: op.MaxPriorityFeePerGas,
		PaymasterAndData: op.PaymasterAndData, Algorithm: op.Algorithm, PublicKey: op.PublicKey, Signature: op.Signature,
	})
}

func DecodeUserOperation(encoded []byte) (*UserOperation, error) {
	var wire userOperationWire
	if err := rlp.DecodeBytes(encoded, &wire); err != nil || string(wire.Magic) != "TKM_USER_OPERATION_V1" {
		return nil, ErrInvalidUserOperation
	}
	op := &UserOperation{
		Sender: wire.Sender, Nonce: wire.Nonce, InitCode: wire.InitCode, CallData: wire.CallData,
		CallGasLimit: wire.CallGasLimit, VerificationGasLimit: wire.VerificationGasLimit, PreVerificationGas: wire.PreVerificationGas,
		MaxFeePerGas: wire.MaxFeePerGas, MaxPriorityFeePerGas: wire.MaxPriorityFeePerGas,
		PaymasterAndData: wire.PaymasterAndData, Algorithm: wire.Algorithm, PublicKey: wire.PublicKey, Signature: wire.Signature,
	}
	if err := op.validate(); err != nil {
		return nil, err
	}
	return op, nil
}

// VerifySignature validates a canonical 65-byte secp256k1 signature against
// Sender. Native ML-DSA account signatures are carried by the PQ transaction
// envelope and can use the same operation hash as their domain.
func (op *UserOperation) VerifySignature(chainID *big.Int, entryPoint common.Address) error {
	hash, err := op.Hash(chainID, entryPoint)
	if err != nil {
		return ErrInvalidOperationSig
	}
	if op.Algorithm == pqcrypto.AlgorithmMLDSA87 {
		if len(op.PublicKey) == 0 || !pqcrypto.VerifyMLDSA87(op.PublicKey, hash[:], op.Signature) {
			return ErrInvalidOperationSig
		}
		address, err := pqcrypto.Address(op.Algorithm, op.PublicKey)
		if err != nil || address != op.Sender {
			return ErrInvalidOperationSig
		}
		return nil
	}
	if op.Algorithm != "" || len(op.PublicKey) != 0 || len(op.Signature) != crypto.SignatureLength {
		return ErrInvalidOperationSig
	}
	pub, err := crypto.SigToPub(hash.Bytes(), op.Signature)
	if err != nil || crypto.PubkeyToAddress(*pub) != op.Sender {
		return ErrInvalidOperationSig
	}
	return nil
}
