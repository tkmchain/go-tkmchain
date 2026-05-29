package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestRecordRotatingKingLockedDoesNotChangeActiveSchedule(t *testing.T) {
	active := common.HexToAddress("0x0000000000000000000000000000000000000001")
	pending := common.HexToAddress("0x0000000000000000000000000000000000000002")
	eth := &Ethereum{
		kingAddresses: []common.Address{active},
		rkLocks:       make(map[common.Address]time.Time),
	}

	unlock := time.Now().UTC().Add(rkLockPeriod)
	eth.recordRotatingKingLocked(pending, unlock)

	if len(eth.kingAddresses) != 1 || eth.kingAddresses[0] != active {
		t.Fatalf("active rotating king schedule changed: %v", eth.kingAddresses)
	}
	if got := eth.rkLocks[pending]; !got.Equal(unlock) {
		t.Fatalf("pending rotating king lock = %v, want %v", got, unlock)
	}
}
