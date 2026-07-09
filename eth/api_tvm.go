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
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tvm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// TVMAPI provides RPC helpers for preparing secure TVM C++ contract deployments and inspecting TVM contract code.
type TVMAPI struct {
	b tvmStateBackend
}

type tvmStateBackend interface {
	StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error)
}

// NewTVMAPI creates a new TVM RPC API instance.
func NewTVMAPI(b tvmStateBackend) *TVMAPI {
	return &TVMAPI{b: b}
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

// TVMCodeResult describes TVM code stored at a contract account.
type TVMCodeResult struct {
	Address      common.Address `json:"address"`
	Version      uint16         `json:"version"`
	Target       string         `json:"target"`
	CodeHash     common.Hash    `json:"codeHash"`
	MetadataHash common.Hash    `json:"metadataHash"`
	MemoryPages  uint32         `json:"memoryPages"`
	StackSlots   uint32         `json:"stackSlots"`
	CallDepth    uint16         `json:"callDepth"`
	Code         hexutil.Bytes  `json:"code"`
	Metadata     hexutil.Bytes  `json:"metadata"`
	Envelope     hexutil.Bytes  `json:"envelope"`
}

var (
	errTVMBackendUnavailable = errors.New("TVM state backend is not available")
	errTVMCodeNotFound       = errors.New("TVM contract code not found")
)

func tvmBlockNumberOrHash(blockNrOrHash *rpc.BlockNumberOrHash) rpc.BlockNumberOrHash {
	if blockNrOrHash == nil {
		return rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	}
	return *blockNrOrHash
}

// GetCode returns the decoded TVM module stored at a contract account.
func (api *TVMAPI) GetCode(ctx context.Context, address common.Address, blockNrOrHash *rpc.BlockNumberOrHash) (*TVMCodeResult, error) {
	if api.b == nil {
		return nil, errTVMBackendUnavailable
	}
	state, _, err := api.b.StateAndHeaderByNumberOrHash(ctx, tvmBlockNumberOrHash(blockNrOrHash))
	if state == nil || err != nil {
		return nil, err
	}
	envelopeBytes := state.GetCode(address)
	if err := state.Error(); err != nil {
		return nil, err
	}
	if len(envelopeBytes) == 0 {
		return nil, fmt.Errorf("%w: %s", errTVMCodeNotFound, address.Hex())
	}
	envelope, err := tvm.UnmarshalBinary(envelopeBytes)
	if err != nil {
		return nil, fmt.Errorf("account %s does not contain TVM envelope code: %w", address.Hex(), err)
	}
	return &TVMCodeResult{
		Address:      address,
		Version:      envelope.Version,
		Target:       envelope.Target,
		CodeHash:     envelope.CodeHash,
		MetadataHash: envelope.MetadataHash,
		MemoryPages:  envelope.Limits.MemoryPages,
		StackSlots:   envelope.Limits.StackSlots,
		CallDepth:    envelope.Limits.CallDepth,
		Code:         append([]byte(nil), envelope.Code...),
		Metadata:     append([]byte(nil), envelope.Metadata...),
		Envelope:     append([]byte(nil), envelopeBytes...),
	}, nil
}

// GetContractCode is an explicit alias for GetCode for explorer/client readability.
func (api *TVMAPI) GetContractCode(ctx context.Context, address common.Address, blockNrOrHash *rpc.BlockNumberOrHash) (*TVMCodeResult, error) {
	return api.GetCode(ctx, address, blockNrOrHash)
}

// GetWasm returns only the decoded TVM module bytes for a contract account.
func (api *TVMAPI) GetWasm(ctx context.Context, address common.Address, blockNrOrHash *rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	result, err := api.GetCode(ctx, address, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	return result.Code, nil
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
