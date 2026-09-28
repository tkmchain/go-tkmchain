// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package vm

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tkmasset"
)

var errInvalidTKMAssetIDInput = errors.New("invalid TKM asset ID precompile input")

// TKMAssetIDPrecompileAddr is the Antartical-native asset identity primitive.
// Its fixed address is outside Ethereum's standard precompile range and its
// domain-separated output cannot be confused with an ERC token identifier.
var TKMAssetIDPrecompileAddr = common.HexToAddress("0x00000000000000000000000000000000000000f3")

// tkmAssetIDPrecompile computes keccak256("TKM_ASSET_ID_V1" || chain ID ||
// contract || kind || manifest hash). It has no state and is safe in STATICCALL.
type tkmAssetIDPrecompile struct{}

func (c *tkmAssetIDPrecompile) RequiredGas(input []byte) uint64 {
	return 500 + uint64(len(input))*4
}

func (c *tkmAssetIDPrecompile) Run(input []byte) ([]byte, error) {
	if len(input) != 32+20+1+32 {
		return nil, errInvalidTKMAssetIDInput
	}
	id, err := tkmasset.AssetIDFromPrecompileInput(input)
	if err != nil {
		return nil, err
	}
	return id.Bytes(), nil
}

func (c *tkmAssetIDPrecompile) Name() string { return "TKMAssetID" }

var _ PrecompiledContract = (*tkmAssetIDPrecompile)(nil)
