package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func TestRecordRotatingKingLockedAddsPendingAddress(t *testing.T) {
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

	if len(eth.kingAddresses) != 2 || eth.kingAddresses[0] != active || eth.kingAddresses[1] != pending {
		t.Fatalf("active rotating king schedule = %v, want [%v %v]", eth.kingAddresses, active, pending)
	}
	if got := eth.rkLocks[pending]; !got.UnlockTime.Equal(unlock) || got.UnlockHeight != unlockHeight {
		t.Fatalf("pending rotating king lock = %v, want unlock %v height %d", got, unlock, unlockHeight)
	}
	reloaded := &Ethereum{
		chainDb:       rawdb.NewMemoryDatabase(),
		kingAddresses: []common.Address{active},
		rkLocks:       make(map[common.Address]rkLockInfo),
	}
	reloaded.chainDb = eth.chainDb
	reloaded.loadRotatingKingLocks()
	if got := reloaded.rkLocks[pending]; !got.UnlockTime.Equal(unlock) || got.UnlockHeight != unlockHeight {
		t.Fatalf("reloaded rotating king lock = %v, want unlock %v height %d", got, unlock, unlockHeight)
	}
	if len(reloaded.kingAddresses) != 2 || reloaded.kingAddresses[0] != active || reloaded.kingAddresses[1] != pending {
		t.Fatalf("reloaded rotating king schedule = %v, want [%v %v]", reloaded.kingAddresses, active, pending)
	}
}

func TestRotatingKingAtActivatesLockedAddressAtNextRotation(t *testing.T) {
	active := common.HexToAddress("0x0000000000000000000000000000000000000001")
	pending := common.HexToAddress("0x0000000000000000000000000000000000000002")
	eth := &Ethereum{
		kingAddresses: []common.Address{active, pending},
		rkLocks: map[common.Address]rkLockInfo{
			pending: {ActivationHeight: 400},
		},
	}

	if got := eth.rotatingKingAt(399); got != active {
		t.Fatalf("rotating king at 399 = %v, want %v", got, active)
	}
	if got := eth.rotatingKingAt(400); got != pending {
		t.Fatalf("rotating king at 400 = %v, want %v", got, pending)
	}
	if got := eth.rotatingKingAt(500); got != active {
		t.Fatalf("rotating king at 500 = %v, want %v", got, active)
	}
}

func TestNextRotationHeight(t *testing.T) {
	kings := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000001"),
		common.HexToAddress("0x0000000000000000000000000000000000000002"),
		common.HexToAddress("0x0000000000000000000000000000000000000003"),
	}

	tests := []struct {
		name    string
		height  uint64
		address common.Address
		want    uint64
		wantOK  bool
	}{
		{
			name:    "next king starts at next interval",
			height:  25,
			address: kings[1],
			want:    100,
			wantOK:  true,
		},
		{
			name:    "later king starts two intervals out",
			height:  25,
			address: kings[2],
			want:    200,
			wantOK:  true,
		},
		{
			name:    "current king reports next future round",
			height:  25,
			address: kings[0],
			want:    300,
			wantOK:  true,
		},
		{
			name:    "boundary uses active round",
			height:  100,
			address: kings[2],
			want:    200,
			wantOK:  true,
		},
		{
			name:    "unregistered king has no scheduled round",
			height:  25,
			address: common.HexToAddress("0x0000000000000000000000000000000000000004"),
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := nextRotationHeight(tt.height, 100, kings, tt.address)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("height = %d, want %d", got, tt.want)
			}
		})
	}
}
