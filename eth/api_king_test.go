package eth

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func newTestKingAPI(t *testing.T, alloc types.GenesisAlloc) (*KingAPI, *Ethereum) {
	t.Helper()
	db := rawdb.NewMemoryDatabase()
	genesis := &core.Genesis{
		Config: params.TestChainConfig,
		Alloc:  alloc,
	}
	chain, err := core.NewBlockChain(db, genesis, randomx.NewFaker(), nil)
	if err != nil {
		t.Fatalf("failed to create test chain: %v", err)
	}
	t.Cleanup(chain.Stop)
	eth := &Ethereum{
		chainDb:       db,
		blockchain:    chain,
		kingAddresses: nil,
		rkLocks:       make(map[common.Address]rkLockInfo),
	}
	return NewKingAPI(eth), eth
}

func TestKingAPIAddRejectsIneligibleAddresses(t *testing.T) {
	eligible := common.HexToAddress("0x0000000000000000000000000000000000000001")
	underfunded := common.HexToAddress("0x0000000000000000000000000000000000000002")
	emptyBalance := common.HexToAddress("0x0000000000000000000000000000000000000003")
	api, eth := newTestKingAPI(t, types.GenesisAlloc{
		(common.Address{}): {Balance: new(big.Int).Add(rkRequiredStake, rkRegistrationFee)},
		eligible:           {Balance: new(big.Int).Add(rkRequiredStake, rkRegistrationFee)},
		underfunded:        {Balance: new(big.Int).Sub(new(big.Int).Add(rkRequiredStake, rkRegistrationFee), big.NewInt(1))},
	})

	for _, address := range []common.Address{common.Address{}, underfunded, emptyBalance} {
		if _, err := api.Add(address); err == nil {
			t.Fatalf("Add(%s) succeeded for ineligible address", address.Hex())
		}
	}
	if len(eth.kingAddresses) != 0 {
		t.Fatalf("ineligible addresses changed rotating king schedule: %v", eth.kingAddresses)
	}
	if len(eth.rkLocks) != 0 {
		t.Fatalf("ineligible addresses created rotating king locks: %v", eth.rkLocks)
	}

	status, err := api.Add(eligible)
	if err != nil {
		t.Fatalf("Add(%s) failed: %v", eligible.Hex(), err)
	}
	if !status.Registered || status.Address != eligible {
		t.Fatalf("eligible address status = %+v, want registered %s", status, eligible.Hex())
	}
	if len(eth.kingAddresses) != 1 || eth.kingAddresses[0] != eligible {
		t.Fatalf("rotating king schedule = %v, want [%v]", eth.kingAddresses, eligible)
	}
	if _, ok := eth.rkLocks[eligible]; !ok {
		t.Fatalf("eligible address missing lock entry")
	}
}

func TestNoteRotatingKingRejectsIneligibleUpdate(t *testing.T) {
	underfunded := common.HexToAddress("0x0000000000000000000000000000000000000002")
	_, eth := newTestKingAPI(t, types.GenesisAlloc{
		underfunded: {Balance: new(big.Int).Sub(new(big.Int).Add(rkRequiredStake, rkRegistrationFee), big.NewInt(1))},
	})

	eth.noteRotatingKing(underfunded, time.Now().UTC().Add(rkLockPeriod))
	if len(eth.kingAddresses) != 0 {
		t.Fatalf("ineligible update changed rotating king schedule: %v", eth.kingAddresses)
	}
	if len(eth.rkLocks) != 0 {
		t.Fatalf("ineligible update created rotating king locks: %v", eth.rkLocks)
	}
}

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
