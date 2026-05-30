package rotatingking

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestGetKingAtHeightActivatesAddedKingAtRotation(t *testing.T) {
	active := common.HexToAddress("0x0000000000000000000000000000000000000001")
	pending := common.HexToAddress("0x0000000000000000000000000000000000000002")
	manager := NewRotatingKingManager(common.Address{}, []common.Address{active}, 100)
	manager.AddKingAddressAt(pending, 400)

	if got := manager.GetKingAtHeight(399); got != active {
		t.Fatalf("king at 399 = %v, want %v", got, active)
	}
	if got := manager.GetKingAtHeight(400); got != pending {
		t.Fatalf("king at 400 = %v, want %v", got, pending)
	}
	if got := manager.GetKingAtHeight(500); got != active {
		t.Fatalf("king at 500 = %v, want %v", got, active)
	}
}

func TestGetKingAtHeightActivatesPendingOnlyKing(t *testing.T) {
	pending := common.HexToAddress("0x0000000000000000000000000000000000000002")
	manager := NewRotatingKingManager(common.Address{}, nil, 100)
	manager.AddKingAddressAt(pending, 400)

	if got := manager.GetKingAtHeight(399); got != (common.Address{}) {
		t.Fatalf("king at 399 = %v, want zero address", got)
	}
	if got := manager.GetKingAtHeight(400); got != pending {
		t.Fatalf("king at 400 = %v, want %v", got, pending)
	}
}
