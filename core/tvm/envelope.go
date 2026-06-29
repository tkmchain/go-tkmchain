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

// Package tvm defines the consensus-neutral TVM deployment envelope used by
// RPC tooling to prepare deterministic C++ contract modules for EVM accounts.
package tvm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	// Magic is the TVM envelope prefix. It makes TVM deployment payloads
	// distinguishable from raw EVM bytecode before any runtime is enabled.
	Magic = [4]byte{'T', 'V', 'M', 0}

	ErrEmptyModule    = errors.New("empty TVM module")
	ErrModuleTooLarge = errors.New("TVM module exceeds maximum size")
	ErrInvalidVersion = errors.New("unsupported TVM version")
	ErrInvalidTarget  = errors.New("unsupported TVM target")
	ErrInvalidLimits  = errors.New("invalid TVM resource limits")
)

const (
	Version1     uint16 = 1
	TargetCppEVM        = "cpp-evm-v1"

	MaxModuleSize   = 24 * 1024
	MaxMetadataSize = 8 * 1024
	MaxMemoryPages  = 256
	MaxStackSlots   = 1024
	MaxCallDepth    = 1024
)

// Limits declares the bounded resources a TVM module may use.
type Limits struct {
	MemoryPages uint32 `json:"memoryPages"`
	StackSlots  uint32 `json:"stackSlots"`
	CallDepth   uint16 `json:"callDepth"`
}

// Envelope describes a deterministic C++ TVM module deployment.
type Envelope struct {
	Version      uint16      `json:"version"`
	Target       string      `json:"target"`
	CodeHash     common.Hash `json:"codeHash"`
	MetadataHash common.Hash `json:"metadataHash"`
	Limits       Limits      `json:"limits"`
	Code         []byte      `json:"-"`
	Metadata     []byte      `json:"-"`
}

// NewEnvelope validates module data and returns a TVM deployment envelope.
func NewEnvelope(code, metadata []byte, limits Limits) (*Envelope, error) {
	if len(code) == 0 {
		return nil, ErrEmptyModule
	}
	if len(code) > MaxModuleSize || len(metadata) > MaxMetadataSize {
		return nil, ErrModuleTooLarge
	}
	if err := ValidateLimits(limits); err != nil {
		return nil, err
	}
	return &Envelope{
		Version:      Version1,
		Target:       TargetCppEVM,
		CodeHash:     crypto.Keccak256Hash(code),
		MetadataHash: crypto.Keccak256Hash(metadata),
		Limits:       limits,
		Code:         append([]byte(nil), code...),
		Metadata:     append([]byte(nil), metadata...),
	}, nil
}

// ValidateLimits checks resource limits against the TVM safety bounds.
func ValidateLimits(limits Limits) error {
	if limits.MemoryPages == 0 || limits.MemoryPages > MaxMemoryPages {
		return fmt.Errorf("%w: memory pages must be in [1,%d]", ErrInvalidLimits, MaxMemoryPages)
	}
	if limits.StackSlots == 0 || limits.StackSlots > MaxStackSlots {
		return fmt.Errorf("%w: stack slots must be in [1,%d]", ErrInvalidLimits, MaxStackSlots)
	}
	if limits.CallDepth == 0 || limits.CallDepth > MaxCallDepth {
		return fmt.Errorf("%w: call depth must be in [1,%d]", ErrInvalidLimits, MaxCallDepth)
	}
	return nil
}

// MarshalBinary encodes the envelope and module bytes into deployable account code.
func (e *Envelope) MarshalBinary() ([]byte, error) {
	if e == nil || e.Version != Version1 {
		return nil, ErrInvalidVersion
	}
	if e.Target != TargetCppEVM {
		return nil, ErrInvalidTarget
	}
	if len(e.Code) == 0 {
		return nil, ErrEmptyModule
	}
	if len(e.Code) > MaxModuleSize || len(e.Metadata) > MaxMetadataSize {
		return nil, ErrModuleTooLarge
	}
	if err := ValidateLimits(e.Limits); err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	buf.Write(Magic[:])
	_ = binary.Write(buf, binary.BigEndian, e.Version)
	_ = binary.Write(buf, binary.BigEndian, uint16(len(e.Target)))
	buf.WriteString(e.Target)
	buf.Write(e.CodeHash[:])
	buf.Write(e.MetadataHash[:])
	_ = binary.Write(buf, binary.BigEndian, e.Limits.MemoryPages)
	_ = binary.Write(buf, binary.BigEndian, e.Limits.StackSlots)
	_ = binary.Write(buf, binary.BigEndian, e.Limits.CallDepth)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(e.Code)))
	_ = binary.Write(buf, binary.BigEndian, uint32(len(e.Metadata)))
	buf.Write(e.Code)
	buf.Write(e.Metadata)
	return buf.Bytes(), nil
}

// UnmarshalBinary decodes and validates a TVM deployment envelope.
func UnmarshalBinary(blob []byte) (*Envelope, error) {
	if len(blob) < len(Magic)+2+2+32+32+4+4+2+4+4 || !bytes.Equal(blob[:len(Magic)], Magic[:]) {
		return nil, ErrInvalidTarget
	}
	off := len(Magic)
	version := binary.BigEndian.Uint16(blob[off:])
	off += 2
	if version != Version1 {
		return nil, ErrInvalidVersion
	}
	targetLen := int(binary.BigEndian.Uint16(blob[off:]))
	off += 2
	if targetLen == 0 || len(blob) < off+targetLen+32+32+4+4+2+4+4 {
		return nil, ErrInvalidTarget
	}
	target := string(blob[off : off+targetLen])
	off += targetLen
	if target != TargetCppEVM {
		return nil, ErrInvalidTarget
	}
	var codeHash, metadataHash common.Hash
	copy(codeHash[:], blob[off:off+32])
	off += 32
	copy(metadataHash[:], blob[off:off+32])
	off += 32
	limits := Limits{
		MemoryPages: binary.BigEndian.Uint32(blob[off:]),
		StackSlots:  binary.BigEndian.Uint32(blob[off+4:]),
		CallDepth:   binary.BigEndian.Uint16(blob[off+8:]),
	}
	off += 10
	codeLen := int(binary.BigEndian.Uint32(blob[off:]))
	metadataLen := int(binary.BigEndian.Uint32(blob[off+4:]))
	off += 8
	if codeLen == 0 {
		return nil, ErrEmptyModule
	}
	if codeLen > MaxModuleSize || metadataLen > MaxMetadataSize || len(blob) != off+codeLen+metadataLen {
		return nil, ErrModuleTooLarge
	}
	if err := ValidateLimits(limits); err != nil {
		return nil, err
	}
	code := append([]byte(nil), blob[off:off+codeLen]...)
	off += codeLen
	metadata := append([]byte(nil), blob[off:off+metadataLen]...)
	if crypto.Keccak256Hash(code) != codeHash || crypto.Keccak256Hash(metadata) != metadataHash {
		return nil, errors.New("TVM envelope hash mismatch")
	}
	return &Envelope{
		Version:      version,
		Target:       target,
		CodeHash:     codeHash,
		MetadataHash: metadataHash,
		Limits:       limits,
		Code:         code,
		Metadata:     metadata,
	}, nil
}
