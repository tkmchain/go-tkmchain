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

package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/tvm"
)

// TVMAPI provides RPC helpers for preparing secure TVM C++ contract deployments.
type TVMAPI struct{}

// NewTVMAPI creates a new TVM RPC API instance.
func NewTVMAPI() *TVMAPI {
	return new(TVMAPI)
}

// TVMBuildRequest contains a compiled deterministic C++ module and deployment limits.
type TVMBuildRequest struct {
	Code        hexutil.Bytes `json:"code"`
	Metadata    hexutil.Bytes `json:"metadata"`
	MemoryPages uint32        `json:"memoryPages"`
	StackSlots  uint32        `json:"stackSlots"`
	CallDepth   uint16        `json:"callDepth"`
}

// TVMBuildResult returns the deployment bytecode and hashes committed by the envelope.
type TVMBuildResult struct {
	Version        uint16        `json:"version"`
	Target         string        `json:"target"`
	CodeHash       common.Hash   `json:"codeHash"`
	MetadataHash   common.Hash   `json:"metadataHash"`
	DeploymentCode hexutil.Bytes `json:"deploymentCode"`
}

// BuildDeployment wraps a compiled C++ TVM module in a validated deployment envelope.
func (api *TVMAPI) BuildDeployment(req TVMBuildRequest) (*TVMBuildResult, error) {
	envelope, err := tvm.NewEnvelope(req.Code, req.Metadata, tvm.Limits{
		MemoryPages: req.MemoryPages,
		StackSlots:  req.StackSlots,
		CallDepth:   req.CallDepth,
	})
	if err != nil {
		return nil, err
	}
	blob, err := envelope.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &TVMBuildResult{
		Version:        envelope.Version,
		Target:         envelope.Target,
		CodeHash:       envelope.CodeHash,
		MetadataHash:   envelope.MetadataHash,
		DeploymentCode: blob,
	}, nil
}

// ValidateDeployment validates a compiled C++ TVM module without returning deployment code.
func (api *TVMAPI) ValidateDeployment(req TVMBuildRequest) (bool, error) {
	_, err := tvm.NewEnvelope(req.Code, req.Metadata, tvm.Limits{
		MemoryPages: req.MemoryPages,
		StackSlots:  req.StackSlots,
		CallDepth:   req.CallDepth,
	})
	return err == nil, err
}
