package rawdb

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestRotatingKingAddressStorage(t *testing.T) {
	db := NewMemoryDatabase()
	addresses := []common.Address{
		common.HexToAddress("0x0000000000000000000000000000000000000001"),
		common.HexToAddress("0x0000000000000000000000000000000000000002"),
	}

	if got := ReadRotatingKingAddresses(db); got != nil {
		t.Fatalf("empty rotating king addresses = %v, want nil", got)
	}
	WriteRotatingKingAddresses(db, addresses)
	if got := ReadRotatingKingAddresses(db); !reflect.DeepEqual(got, addresses) {
		t.Fatalf("rotating king addresses = %v, want %v", got, addresses)
	}
}

func TestRotatingKingLockStorageIncludesHash(t *testing.T) {
	db := NewMemoryDatabase()
	locks := []RotatingKingLock{{
		Address:          common.HexToAddress("0x0000000000000000000000000000000000000001"),
		UnlockTime:       123,
		UnlockHeight:     456,
		ActivationHeight: 400,
		AddedHeight:      300,
		Hash:             common.HexToHash("0x1234"),
	}}

	WriteRotatingKingLocks(db, locks)
	if got := ReadRotatingKingLocks(db); !reflect.DeepEqual(got, locks) {
		t.Fatalf("rotating king locks = %+v, want %+v", got, locks)
	}
}
