package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

func TestTKMBlockHashAnchorPredeploymentAdvances(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	firstParent := common.HexToHash("0x1111")
	if err := EnsureTKMBlockHashAnchor(statedb, params.EgyptChainConfig, big.NewInt(1), 0, firstParent); err != nil {
		t.Fatal(err)
	}
	runtime, err := loadTKMBlockHashAnchorRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if got := crypto.Keccak256Hash(statedb.GetCode(params.TKMBlockHashAnchorAddress)); got != crypto.Keccak256Hash(runtime) {
		t.Fatalf("predeployed runtime hash = %s, want %s", got, crypto.Keccak256Hash(runtime))
	}
	if got := statedb.GetState(params.TKMBlockHashAnchorAddress, tkmAnchorMappingSlot(0)); got != firstParent {
		t.Fatalf("genesis anchor = %s, want %s", got, firstParent)
	}
	if got := new(big.Int).SetBytes(statedb.GetState(params.TKMBlockHashAnchorAddress, tkmAnchorSlot(4)).Bytes()); got.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("anchor count after first block = %s, want 1", got)
	}

	secondParent := common.HexToHash("0x2222")
	if err := EnsureTKMBlockHashAnchor(statedb, params.EgyptChainConfig, big.NewInt(2), 1, secondParent); err != nil {
		t.Fatal(err)
	}
	if got := statedb.GetState(params.TKMBlockHashAnchorAddress, tkmAnchorMappingSlot(1)); got != secondParent {
		t.Fatalf("height-one anchor = %s, want %s", got, secondParent)
	}
	if got := new(big.Int).SetBytes(statedb.GetState(params.TKMBlockHashAnchorAddress, tkmAnchorSlot(4)).Bytes()); got.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("anchor count after second block = %s, want 2", got)
	}
	if err := EnsureTKMBlockHashAnchor(statedb, params.EgyptChainConfig, big.NewInt(2), 1, secondParent); err != nil {
		t.Fatal(err)
	}
	if got := new(big.Int).SetBytes(statedb.GetState(params.TKMBlockHashAnchorAddress, tkmAnchorSlot(4)).Bytes()); got.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("replaying block transition changed anchor count to %s", got)
	}
}
