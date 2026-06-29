// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package tvm

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestNewEnvelope(t *testing.T) {
	code := []byte{0x01, 0x02, 0x03}
	metadata := []byte(`{"abi":[]}`)
	envelope, err := NewEnvelope(code, metadata, Limits{MemoryPages: 1, StackSlots: 16, CallDepth: 4})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	if envelope.Version != Version1 {
		t.Fatalf("version mismatch: have %d, want %d", envelope.Version, Version1)
	}
	if envelope.Target != TargetCppEVM {
		t.Fatalf("target mismatch: have %s, want %s", envelope.Target, TargetCppEVM)
	}
	if envelope.CodeHash != crypto.Keccak256Hash(code) {
		t.Fatalf("code hash mismatch")
	}
	if envelope.MetadataHash != crypto.Keccak256Hash(metadata) {
		t.Fatalf("metadata hash mismatch")
	}
	blob, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	if !bytes.HasPrefix(blob, Magic[:]) {
		t.Fatalf("deployment code missing TVM magic")
	}
}

func TestNewEnvelopeRejectsUnsafeLimits(t *testing.T) {
	_, err := NewEnvelope([]byte{0x01}, nil, Limits{MemoryPages: MaxMemoryPages + 1, StackSlots: 1, CallDepth: 1})
	if err == nil {
		t.Fatalf("expected invalid limits error")
	}
}

func TestUnmarshalBinary(t *testing.T) {
	envelope, err := NewEnvelope([]byte{OpReturnCodeHash}, nil, Limits{MemoryPages: 1, StackSlots: 1, CallDepth: 1})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	blob, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	decoded, err := UnmarshalBinary(blob)
	if err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}
	if decoded.CodeHash != envelope.CodeHash || !bytes.Equal(decoded.Code, envelope.Code) {
		t.Fatalf("decoded envelope mismatch")
	}
}

func TestRuntimeExecute(t *testing.T) {
	envelope, err := NewEnvelope([]byte{OpReturnInput}, nil, Limits{MemoryPages: 1, StackSlots: 1, CallDepth: 1})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	out, err := (Runtime{}).Execute(envelope, []byte("call-data"))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if string(out) != "call-data" {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestRuntimeRejectsStaticStore(t *testing.T) {
	code := append([]byte{OpStorageStore}, make([]byte, 64)...)
	envelope, err := NewEnvelope(code, nil, Limits{MemoryPages: 1, StackSlots: 1, CallDepth: 1})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	_, err = (Runtime{ReadOnly: true}).Execute(envelope, nil)
	if !errors.Is(err, ErrStaticWrite) {
		t.Fatalf("error mismatch: have %v, want %v", err, ErrStaticWrite)
	}
}
