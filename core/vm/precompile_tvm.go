// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tvm"
)

// TVMPrecompileAddr is the native entry point for executing validated TVM envelopes.
var TVMPrecompileAddr = common.HexToAddress("0x00000000000000000000000000000000000000f2")

type tvmPrecompileContract struct{}

type tvmHost struct {
	state    StateDB
	address  common.Address
	readOnly bool
}

func (h tvmHost) StorageLoad(key common.Hash) common.Hash {
	if h.state == nil {
		return common.Hash{}
	}
	return h.state.GetState(h.address, key)
}

func (h tvmHost) StorageStore(key common.Hash, value common.Hash) error {
	if h.readOnly {
		return tvm.ErrStaticWrite
	}
	if h.state == nil {
		return tvm.ErrInvalidProgram
	}
	h.state.SetState(h.address, key, value)
	return nil
}

func (c *tvmPrecompileContract) RequiredGas(input []byte) uint64 {
	return 15000 + uint64(len(input))*16
}

func (c *tvmPrecompileContract) Run(input []byte) ([]byte, error) {
	envelope, err := tvm.UnmarshalBinary(input)
	if err != nil {
		return nil, err
	}
	return (tvm.Runtime{}).Execute(envelope, nil)
}

func (c *tvmPrecompileContract) RunStateful(stateDB StateDB, address common.Address, input []byte, readOnly bool) ([]byte, error) {
	envelope, err := tvm.UnmarshalBinary(input)
	if err != nil {
		return nil, err
	}
	runtime := tvm.Runtime{Host: tvmHost{state: stateDB, address: address, readOnly: readOnly}, ReadOnly: readOnly}
	return runtime.Execute(envelope, nil)
}

func (c *tvmPrecompileContract) Name() string {
	return "TVM"
}

var _ StatefulPrecompiledContract = (*tvmPrecompileContract)(nil)
