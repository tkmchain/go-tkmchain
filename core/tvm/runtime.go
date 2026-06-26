// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package tvm

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
)

const (
	OpReturnInput byte = iota
	OpReturnCodeHash
	OpStorageLoad
	OpStorageStore
)

var (
	ErrInvalidProgram = errors.New("invalid TVM program")
	ErrStaticWrite    = errors.New("TVM static execution cannot modify state")
)

// Host is the deterministic TVM host environment exposed by the EVM.
type Host interface {
	StorageLoad(key common.Hash) common.Hash
	StorageStore(key common.Hash, value common.Hash) error
}

// Runtime executes validated TVM modules against a bounded host environment.
type Runtime struct {
	Host     Host
	ReadOnly bool
}

// Execute runs the compiled TVM module. This initial runtime supports a small,
// deterministic instruction set used by C++ tooling conformance tests.
func (r Runtime) Execute(envelope *Envelope, input []byte) ([]byte, error) {
	if envelope == nil || len(envelope.Code) == 0 {
		return nil, ErrEmptyModule
	}
	switch envelope.Code[0] {
	case OpReturnInput:
		return append([]byte(nil), input...), nil
	case OpReturnCodeHash:
		return envelope.CodeHash.Bytes(), nil
	case OpStorageLoad:
		if r.Host == nil || len(envelope.Code) != 33 {
			return nil, ErrInvalidProgram
		}
		return r.Host.StorageLoad(common.BytesToHash(envelope.Code[1:33])).Bytes(), nil
	case OpStorageStore:
		if r.ReadOnly {
			return nil, ErrStaticWrite
		}
		if r.Host == nil || len(envelope.Code) != 65 {
			return nil, ErrInvalidProgram
		}
		key := common.BytesToHash(envelope.Code[1:33])
		value := common.BytesToHash(envelope.Code[33:65])
		if err := r.Host.StorageStore(key, value); err != nil {
			return nil, err
		}
		return value.Bytes(), nil
	default:
		return nil, ErrInvalidProgram
	}
}
