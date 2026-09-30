package state

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/holiman/uint256"
)

func TestSpeculativeDeltaRoundTrip(t *testing.T) {
	db := NewDatabaseForTesting()
	speculative, err := New(common.Hash{}, db)
	if err != nil {
		t.Fatal(err)
	}
	addr := common.HexToAddress("0x1234")
	speculative.SetBalance(addr, uint256.NewInt(42), tracing.BalanceChangeUnspecified)
	speculative.SetNonce(addr, 7, tracing.NonceChangeUnspecified)
	delta, err := speculative.BuildSpeculativeDelta()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := New(common.Hash{}, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := canonical.ApplySpeculativeDelta(delta); err != nil {
		t.Fatal(err)
	}
	if canonical.GetBalance(addr).Uint64() != 42 || canonical.GetNonce(addr) != 7 {
		t.Fatalf("speculative delta was not applied: balance=%v nonce=%d", canonical.GetBalance(addr), canonical.GetNonce(addr))
	}
	if err := canonical.CanApplySpeculativeDelta(delta); err == nil {
		t.Fatal("accepted a delta with an already changed account")
	}
}
