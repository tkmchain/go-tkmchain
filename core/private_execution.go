// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or
// modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

package core

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// ErrPublicExecutionDisabled is returned when a transaction tries to use the
// transparent EVM or a non-Shield3/Shield4 envelope after Antartical. Every
// user transaction must use a consensus-verified Shield3 or Shield4 envelope.
//
// This is deliberately a consensus error rather than a wallet-only policy. A
// node must not accept a transaction into its pool and later discover that a
// different node rejects it while processing a block.
var ErrPublicExecutionDisabled = errors.New("transparent EVM/TVM execution is disabled after Antartical; use a Shield3 or Shield4 private envelope")

// ValidatePrivateExecutionPolicy applies the Antartical transaction boundary.
//
// Before Antartical the existing EVM compatibility rules remain unchanged. At
// Antartical, user transactions must use a Shield3 or Shield4 envelope.
// Synthetic block rewards remain protocol-generated transactions. This helper
// is shared by txpool admission and block execution so both paths enforce the
// same rule.
func ValidatePrivateExecutionPolicy(config *params.ChainConfig, number *big.Int, blockTime uint64, tx *types.Transaction) error {
	if config == nil || tx == nil || !config.IsAntartical(number, blockTime) {
		return nil
	}
	if tx.Type() == types.PQTkmTxType && HasAntarticalStampPrefix(tx.Data()) {
		return ValidateAntarticalStampBasics(config, number, blockTime, tx)
	}
	return validatePrivateExecutionFields(config, number, blockTime, tx.Type(), tx.To(), tx.Value(), tx.Data(), types.IsBlockRewardTx(tx))
}

// ValidatePrivateExecutionMessage is the state-transition equivalent of
// ValidatePrivateExecutionPolicy. Messages do not retain the complete signed
// transaction, so the policy is checked from the consensus fields carried into
// the EVM.
func ValidatePrivateExecutionMessage(config *params.ChainConfig, number *big.Int, blockTime uint64, txType uint8, to *common.Address, value *big.Int, data []byte) error {
	return validatePrivateExecutionFields(config, number, blockTime, txType, to, value, data, false)
}

func validatePrivateExecutionFields(config *params.ChainConfig, number *big.Int, blockTime uint64, txType uint8, _ *common.Address, _ *big.Int, data []byte, blockReward bool) error {
	if config == nil || !config.IsAntartical(number, blockTime) || blockReward {
		return nil
	}
	if txType != types.PQTkmTxType {
		return ErrPublicExecutionDisabled
	}
	if HasAntarticalStampPrefix(data) {
		// Stamp registration is a consensus protocol envelope rather than a
		// transparent EVM/TVM call. The full transaction path validates its
		// stateless shape and ownership proof before applying registry state.
		return nil
	}
	if HasShieldedV3Prefix(data) || HasShieldedV4Prefix(data) {
		return nil
	}
	return ErrPublicExecutionDisabled
}
