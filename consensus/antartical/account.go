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
		if n == nil || n.Sign() < 0 {
			return ErrInvalidUserOperation
		}
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
	// Hash dynamic fields before the tuple is encoded. This is the canonical
	// EIP-4337/RIP-7560 domain-separated operation digest format.
	return rlp.EncodeToBytes([]interface{}{
		common.BytesToHash([]byte("TKM-AA-1")), op.Sender, op.Nonce,
		crypto.Keccak256Hash(op.InitCode), crypto.Keccak256Hash(op.CallData),
		op.CallGasLimit, op.VerificationGasLimit, op.PreVerificationGas,
		op.MaxFeePerGas, op.MaxPriorityFeePerGas, crypto.Keccak256Hash(op.PaymasterAndData),
		entryPoint, chainID,
	})
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
