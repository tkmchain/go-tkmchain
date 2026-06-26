// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package vm

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tvm"
	"github.com/ethereum/go-ethereum/params"
)

func TestTVMPrecompileRegistered(t *testing.T) {
	if _, ok := PrecompiledContractsPrague[TVMPrecompileAddr]; !ok {
		t.Fatalf("TVM precompile missing from Prague precompiles")
	}
	if _, ok := PrecompiledContractsOsaka[TVMPrecompileAddr]; !ok {
		t.Fatalf("TVM precompile missing from Osaka precompiles")
	}
}

func TestTVMPrecompileRun(t *testing.T) {
	envelope, err := tvm.NewEnvelope([]byte{tvm.OpReturnCodeHash}, nil, tvm.Limits{MemoryPages: 1, StackSlots: 1, CallDepth: 1})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	input, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	out, _, err := RunPrecompiledContract(nil, &tvmPrecompileContract{}, TVMPrecompileAddr, input, NewGasBudget(100000), nil, params.Rules{}, false)
	if err != nil {
		t.Fatalf("RunPrecompiledContract failed: %v", err)
	}
	if common.BytesToHash(out) != envelope.CodeHash {
		t.Fatalf("output hash mismatch")
	}
}

func TestTVMPrecompileRejectsStaticStore(t *testing.T) {
	code := append([]byte{tvm.OpStorageStore}, make([]byte, 64)...)
	envelope, err := tvm.NewEnvelope(code, nil, tvm.Limits{MemoryPages: 1, StackSlots: 1, CallDepth: 1})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	input, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	_, _, err = RunPrecompiledContract(nil, &tvmPrecompileContract{}, TVMPrecompileAddr, input, NewGasBudget(100000), nil, params.Rules{}, true)
	if !errors.Is(err, tvm.ErrStaticWrite) {
		t.Fatalf("error mismatch: have %v, want %v", err, tvm.ErrStaticWrite)
	}
}
