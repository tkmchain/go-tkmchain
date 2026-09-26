// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package core

import (
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

const (
	PrivateTVMMagic         = "TKMPRIVATEVM1"
	PrivateTVMVerifyGas     = 3_000_000
	PrivateTVMMaxDataSize   = ShieldedV3MaxTxSize
	privateTVMMaxCiphertext = 16 * 1024
)

var (
	ErrInvalidPrivateTVM = errors.New("invalid private TVM transaction")
	privateTVMRootRole   = "private-tvm/root"
)

// PrivateTVMTransaction is a consensus envelope for the bounded private TVM
// storage relation. The ciphertext is not interpreted by consensus; its hash
// is bound through the transaction intent. The native STARK proves the hidden
// storage key and values and authenticates the old/new roots.
type PrivateTVMTransaction struct {
	Version    uint64
	CodeHash   shielded3.Digest
	OldRoot    shielded3.Digest
	NewRoot    shielded3.Digest
	Operation  uint64
	Ciphertext []byte
	Proof      []byte
}

func HasPrivateTVMPrefix(data []byte) bool { return bytes.HasPrefix(data, []byte(PrivateTVMMagic)) }

func EncodePrivateTVMTransaction(e *PrivateTVMTransaction) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidPrivateTVM
	}
	payload, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(PrivateTVMMagic), payload...), nil
}

func DecodePrivateTVMTransaction(data []byte) (*PrivateTVMTransaction, bool, error) {
	if !HasPrivateTVMPrefix(data) {
		return nil, false, nil
	}
	if uint64(len(data)) > PrivateTVMMaxDataSize {
		return nil, true, ErrInvalidPrivateTVM
	}
	var e PrivateTVMTransaction
	if err := rlp.DecodeBytes(data[len(PrivateTVMMagic):], &e); err != nil {
		return nil, true, fmt.Errorf("%w: malformed envelope", ErrInvalidPrivateTVM)
	}
	return &e, true, nil
}

// PrivateTVMIntent binds every public transaction field and the envelope
// without its proof. A proof cannot be moved to another transaction or
// ciphertext without invalidating this digest.
func PrivateTVMIntent(tx *types.Transaction, e *PrivateTVMTransaction) ([64]byte, error) {
	var result [64]byte
	if tx == nil || e == nil {
		return result, ErrInvalidPrivateTVM
	}
	clean := *e
	clean.Proof = nil
	data, err := EncodePrivateTVMTransaction(&clean)
	if err != nil {
		return result, err
	}
	algorithm, publicKey, _, _ := tx.PQTkmFields()
	payload := shieldedIntentPayload{
		Domain: []byte("TKM_PRIVATE_TVM_INTENT_V1"), TxType: tx.Type(), ChainID: tx.ChainId(),
		Nonce: tx.Nonce(), GasTipCap: tx.GasTipCap(), GasFeeCap: tx.GasFeeCap(), Gas: tx.Gas(),
		To: tx.To(), Value: tx.Value(), AccessList: tx.AccessList(), PQAlgorithm: algorithm,
		PQPublicKey: publicKey, Envelope: data,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return result, err
	}
	return sha512.Sum512(encoded), nil
}

func privateTVMStatement(config *params.ChainConfig, tx *types.Transaction, e *PrivateTVMTransaction) (shielded3.PrivateTVMStatement, error) {
	if config == nil || config.ChainID == nil || !config.ChainID.IsUint64() || tx == nil || e == nil {
		return shielded3.PrivateTVMStatement{}, ErrInvalidPrivateTVM
	}
	intent, err := PrivateTVMIntent(tx, e)
	if err != nil {
		return shielded3.PrivateTVMStatement{}, err
	}
	return shielded3.PrivateTVMStatement{
		ChainID: config.ChainID.Uint64(), CodeHash: e.CodeHash, OldRoot: e.OldRoot,
		NewRoot: e.NewRoot, Intent: intent, Operation: uint8(e.Operation),
	}, nil
}

func ValidatePrivateTVMBasics(config *params.ChainConfig, number *big.Int, blockTime uint64, tx *types.Transaction) (*PrivateTVMTransaction, error) {
	if config == nil || !config.IsAntartical(number, blockTime) || !config.IsPrivacyCommitments(number, blockTime) {
		return nil, fmt.Errorf("%w: not active until Antartical", ErrInvalidPrivateTVM)
	}
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 {
		return nil, fmt.Errorf("%w: requires a PQ transaction to the shielded pool", ErrInvalidPrivateTVM)
	}
	if config.ChainID == nil || tx.ChainId().Cmp(config.ChainID) != 0 {
		return nil, fmt.Errorf("%w: chain ID mismatch", ErrInvalidPrivateTVM)
	}
	e, ok, err := DecodePrivateTVMTransaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if !ok || e.Version != 1 || e.Operation > 1 || e.CodeHash == (shielded3.Digest{}) || e.OldRoot == (shielded3.Digest{}) || e.NewRoot == (shielded3.Digest{}) {
		return nil, fmt.Errorf("%w: invalid version, operation, code hash, or root", ErrInvalidPrivateTVM)
	}
	if len(e.Ciphertext) == 0 || len(e.Ciphertext) > privateTVMMaxCiphertext || !shielded3.ValidProofEncoding(e.Proof) {
		return nil, fmt.Errorf("%w: invalid ciphertext or proof", ErrInvalidPrivateTVM)
	}
	if _, err := privateTVMStatement(config, tx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func ValidatePrivateTVMProof(config *params.ChainConfig, number *big.Int, blockTime uint64, tx *types.Transaction) error {
	e, err := ValidatePrivateTVMBasics(config, number, blockTime, tx)
	if err != nil {
		return err
	}
	statement, err := privateTVMStatement(config, tx, e)
	if err != nil {
		return err
	}
	if err := (shielded3.NativeBackend{}).VerifyPrivateTVM(context.Background(), statement, e.Proof); err != nil {
		return fmt.Errorf("%w: STARK proof: %v", ErrInvalidPrivateTVM, err)
	}
	return nil
}

func PrivateTVMStateRoot(st shieldedStateReader) (shielded3.Digest, error) {
	return v3ReadDigest(st, privateTVMRootRole, nil)
}

func processPrivateTVM(config *params.ChainConfig, number *big.Int, blockTime uint64, st *state.StateDB, tx *types.Transaction) error {
	if err := ValidatePrivateTVMProof(config, number, blockTime, tx); err != nil {
		return err
	}
	e, _, _ := DecodePrivateTVMTransaction(tx.Data())
	current, err := PrivateTVMStateRoot(st)
	if err != nil {
		return err
	}
	if current == (shielded3.Digest{}) && e.Operation == 0 {
		return fmt.Errorf("%w: private TVM read requires an initialized root", ErrInvalidPrivateTVM)
	}
	if current != (shielded3.Digest{}) && current != e.OldRoot {
		return fmt.Errorf("%w: old private TVM root is not canonical", ErrInvalidPrivateTVM)
	}
	intent, err := PrivateTVMIntent(tx, e)
	if err != nil {
		return err
	}
	intentKey := ShieldedV3StateSlot("private-tvm/intent", intent[:])
	if st.GetState(params.ShieldedPoolAddress, intentKey) != (common.Hash{}) {
		return fmt.Errorf("%w: intent already spent", ErrInvalidPrivateTVM)
	}
	v3WriteDigest(st, privateTVMRootRole, nil, e.NewRoot)
	st.SetState(params.ShieldedPoolAddress, intentKey, tx.Hash())
	return nil
}

func PrivateTVMGasData(data []byte) ([]byte, uint64, error) {
	e, ok, err := DecodePrivateTVMTransaction(data)
	if err != nil || !ok {
		return data, 0, err
	}
	if len(e.Proof) > shielded3.MaxProofSize {
		return nil, 0, ErrInvalidPrivateTVM
	}
	proofLen := len(e.Proof)
	e.Proof = nil
	encoded, err := EncodePrivateTVMTransaction(e)
	return encoded, PrivateTVMVerifyGas + uint64(proofLen), err
}
