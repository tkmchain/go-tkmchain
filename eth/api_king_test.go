package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func TestRecordRotatingKingLockedDoesNotChangeActiveSchedule(t *testing.T) {
	active := common.HexToAddress("0x0000000000000000000000000000000000000001")
	pending := common.HexToAddress("0x0000000000000000000000000000000000000002")
	eth := &Ethereum{
		chainDb:       rawdb.NewMemoryDatabase(),
		kingAddresses: []common.Address{active},
		rkLocks:       make(map[common.Address]rkLockInfo),
	}

	unlock := time.Unix(time.Now().UTC().Add(rkLockPeriod).Unix(), 0).UTC()
	unlockHeight := uint64(100)
	eth.recordRotatingKingLocked(pending, unlock, unlockHeight)

	if len(eth.kingAddresses) != 1 || eth.kingAddresses[0] != active {
		t.Fatalf("active rotating king schedule changed: %v", eth.kingAddresses)
	}
	if got := eth.rkLocks[pending]; !got.UnlockTime.Equal(unlock) || got.UnlockHeight != unlockHeight {
		t.Fatalf("pending rotating king lock = %v, want unlock %v height %d", got, unlock, unlockHeight)
	}
	reloaded := &Ethereum{
		chainDb: rawdb.NewMemoryDatabase(),
		rkLocks: make(map[common.Address]rkLockInfo),
	}
	reloaded.chainDb = eth.chainDb
	reloaded.loadRotatingKingLocks()
	if got := reloaded.rkLocks[pending]; !got.UnlockTime.Equal(unlock) || got.UnlockHeight != unlockHeight {
		t.Fatalf("reloaded rotating king lock = %v, want unlock %v height %d", got, unlock, unlockHeight)
	}
}
