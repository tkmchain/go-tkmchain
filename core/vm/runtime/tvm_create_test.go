package runtime

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tvm"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestCreateStoresTVMEnvelopeCode(t *testing.T) {
	envelope, err := tvm.NewEnvelope([]byte{tvm.OpReturnCodeHash}, []byte("metadata"), tvm.Limits{MemoryPages: 1, StackSlots: 16, CallDepth: 4})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	deploymentCode, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}

	ret, address, _, err := Create(deploymentCode, &Config{GasLimit: 1_000_000, State: statedb})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !bytes.Equal(ret, deploymentCode) {
		t.Fatalf("create return = %x, want deployment code %x", ret, deploymentCode)
	}
	if code := statedb.GetCode(address); !bytes.Equal(code, deploymentCode) {
		t.Fatalf("stored code = %x, want deployment code %x", code, deploymentCode)
	}
}

func TestCreateTVMEnvelopeCodeStoreOutOfGasConsumesGas(t *testing.T) {
	envelope, err := tvm.NewEnvelope([]byte{tvm.OpReturnCodeHash}, []byte("metadata"), tvm.Limits{MemoryPages: 1, StackSlots: 16, CallDepth: 4})
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	deploymentCode, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}

	_, address, leftOverGas, err := Create(deploymentCode, &Config{GasLimit: 1, State: statedb})
	if err == nil {
		t.Fatal("Create succeeded with insufficient code storage gas")
	}
	if leftOverGas != 0 {
		t.Fatalf("leftover gas = %d, want 0", leftOverGas)
	}
	if code := statedb.GetCode(address); len(code) != 0 {
		t.Fatalf("stored code after failed create = %x, want empty", code)
	}
}
