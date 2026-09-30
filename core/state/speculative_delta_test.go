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

func TestDynamicAccessSummary(t *testing.T) {
	db := NewDatabaseForTesting()
	st, err := New(common.Hash{}, db)
	if err != nil {
		t.Fatal(err)
	}
	read := common.HexToAddress("0x1")
	write := common.HexToAddress("0x2")
	st.GetBalance(read)
	st.SetNonce(write, 1, tracing.NonceChangeUnspecified)
	reads, writes := st.AccessSummary()
	if len(reads) == 0 || reads[0] != read || len(writes) == 0 || writes[0] != write {
		t.Fatalf("unexpected dynamic access summary: reads=%v writes=%v", reads, writes)
	}
}

func TestSpeculativeDeltaValidatesSlotReads(t *testing.T) {
	db := NewDatabaseForTesting()
	st, err := New(common.Hash{}, db)
	if err != nil {
		t.Fatal(err)
	}
	addr := common.HexToAddress("0x1234")
	key := common.HexToHash("0x42")
	value := common.HexToHash("0x99")
	st.SetBalance(addr, uint256.NewInt(1), tracing.BalanceChangeUnspecified)
	st.SetState(addr, key, value)
	st.Finalise(true)
	st.ResetAccessSummary()
	if got := st.GetState(addr, key); got != value {
		t.Fatalf("unexpected slot value: got %s want %s", got, value)
	}
	delta, err := st.BuildSpeculativeDelta()
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Reads) != 1 || delta.Reads[0].Address != addr || delta.Reads[0].Key != key || delta.Reads[0].Value != (common.Hash{}) {
		t.Fatalf("missing slot witness: %+v", delta.Reads)
	}
	st.SetState(addr, key, common.HexToHash("0xaa"))
	if err := st.CanApplySpeculativeDelta(delta); err == nil {
		t.Fatal("accepted a slot witness after the canonical value changed")
	}
}
