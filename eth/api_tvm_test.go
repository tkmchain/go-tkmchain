package eth

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/tvm"
	"github.com/ethereum/go-ethereum/core/types"
	vmruntime "github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/triedb"
)

type testTVMBackend struct {
	state  *state.StateDB
	header *types.Header
}

func (b testTVMBackend) StateAndHeaderByNumberOrHash(context.Context, rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	return b.state, b.header, nil
}

func TestTVMGetCodeDecodesEnvelope(t *testing.T) {
	address := common.HexToAddress("0x0000000000000000000000000000000000001234")
	module := []byte{tvm.OpReturnCodeHash, 0x01, 0x02}
	metadata := []byte("contract metadata")
	envelope, err := tvm.NewEnvelope(module, metadata, tvm.Limits{MemoryPages: 2, StackSlots: 16, CallDepth: 4})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	blob, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	db := rawdb.NewMemoryDatabase()
	tdb := triedb.NewDatabase(db, nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabase(tdb, nil))
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}
	statedb.CreateAccount(address)
	statedb.SetCode(address, blob, tracing.CodeChangeUnspecified)

	api := NewTVMAPI(testTVMBackend{state: statedb, header: &types.Header{Root: types.EmptyRootHash}})
	result, err := api.GetCode(context.Background(), address, nil)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}
	if result.Address != address || result.Version != envelope.Version || result.Target != envelope.Target {
		t.Fatalf("unexpected result metadata: %+v", result)
	}
	if result.CodeHash != envelope.CodeHash || result.MetadataHash != envelope.MetadataHash {
		t.Fatalf("unexpected hashes: %+v", result)
	}
	if !bytes.Equal(result.Code, module) {
		t.Fatalf("module code = %x, want %x", []byte(result.Code), module)
	}
	if !bytes.Equal(result.Metadata, metadata) {
		t.Fatalf("metadata = %x, want %x", []byte(result.Metadata), metadata)
	}
	if !bytes.Equal(result.Envelope, blob) {
		t.Fatalf("envelope = %x, want %x", []byte(result.Envelope), blob)
	}

	wasm, err := api.GetWasm(context.Background(), address, nil)
	if err != nil {
		t.Fatalf("GetWasm failed: %v", err)
	}
	if !bytes.Equal(wasm, module) {
		t.Fatalf("wasm = %x, want %x", []byte(wasm), module)
	}
}

func TestTVMCppFixtureDeployAndReadFullMinedCode(t *testing.T) {
	source, err := os.ReadFile("../contracts/tvm/test_return_code_hash.cpp")
	if err != nil {
		t.Fatalf("failed to read C++ fixture: %v", err)
	}
	if !strings.Contains(string(source), "ReturnCodeHash") {
		t.Fatalf("C++ fixture does not declare the expected TVM operation")
	}

	module := []byte{tvm.OpReturnCodeHash}
	metadata := []byte(`{"name":"TestReturnCodeHash","language":"cpp","source":"contracts/tvm/test_return_code_hash.cpp","target":"cpp-evm-v1"}`)
	api := NewTVMAPI(nil)
	build, err := api.BuildDeployment(TVMBuildRequest{
		Code:        module,
		Metadata:    metadata,
		MemoryPages: 1,
		StackSlots:  16,
		CallDepth:   4,
	})
	if err != nil {
		t.Fatalf("BuildDeployment failed: %v", err)
	}

	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}
	ret, address, _, err := vmruntime.Create(build.DeploymentCode, &vmruntime.Config{GasLimit: 1_000_000, State: statedb})
	if err != nil {
		t.Fatalf("TVM fixture deployment failed: %v", err)
	}
	if !bytes.Equal(ret, build.DeploymentCode) {
		t.Fatalf("create return = %x, want deployment code %x", ret, []byte(build.DeploymentCode))
	}
	if code := statedb.GetCode(address); !bytes.Equal(code, build.DeploymentCode) {
		t.Fatalf("mined code = %x, want full deployment envelope %x", code, []byte(build.DeploymentCode))
	}

	readAPI := NewTVMAPI(testTVMBackend{state: statedb, header: &types.Header{Root: types.EmptyRootHash}})
	decoded, err := readAPI.GetContractCode(context.Background(), address, nil)
	if err != nil {
		t.Fatalf("GetContractCode failed: %v", err)
	}
	if decoded.Address != address {
		t.Fatalf("decoded address = %s, want %s", decoded.Address, address)
	}
	if decoded.Target != tvm.TargetCppEVM || decoded.CodeHash != build.CodeHash || decoded.MetadataHash != build.MetadataHash {
		t.Fatalf("unexpected decoded TVM metadata: %+v", decoded)
	}
	if !bytes.Equal(decoded.Code, module) {
		t.Fatalf("decoded module = %x, want %x", []byte(decoded.Code), module)
	}
	if !bytes.Equal(decoded.Metadata, metadata) {
		t.Fatalf("decoded metadata = %s, want %s", []byte(decoded.Metadata), metadata)
	}
	if !bytes.Equal(decoded.Envelope, build.DeploymentCode) {
		t.Fatalf("decoded envelope does not match full mined code")
	}
}
