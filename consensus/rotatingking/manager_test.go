package rotatingking

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

type registrationState struct {
	balances map[common.Address]*big.Int
	height   uint64
}

func (s registrationState) GetBalance(address common.Address) *big.Int {
	if balance := s.balances[address]; balance != nil {
		return new(big.Int).Set(balance)
	}
	return new(big.Int)
}

func (s registrationState) GetBlockNumber() uint64 { return s.height }

func TestRegisterKingAtRequiresStakeAndSchedulesActivation(t *testing.T) {
	main := common.HexToAddress("0x100")
	candidate := common.HexToAddress("0x101")
	manager := NewRotatingKingManager(main, nil, 10)
	if _, err := manager.RegisterKingAt(candidate, 98, new(big.Int).Sub(EligibilityThreshold, big.NewInt(1))); !errors.Is(err, ErrInsufficientStake) {
		t.Fatalf("underfunded registration error = %v, want %v", err, ErrInsufficientStake)
	}
	registration, err := manager.RegisterKingAt(candidate, 98, EligibilityThreshold)
	if err != nil {
		t.Fatalf("funded registration failed: %v", err)
	}
	if registration.AddedHeight != 98 || registration.ActivationHeight != 100 {
		t.Fatalf("registration heights = %+v, want added 98 activation 100", registration)
	}
	if registration.RegistrationHash == (common.Hash{}) {
		t.Fatal("registration hash is zero")
	}
	if got := manager.GetKingAtHeight(99); got != (common.Address{}) {
		t.Fatalf("king before activation = %s, want zero address", got.Hex())
	}
	if got := manager.GetKingAtHeight(100); got != candidate {
		t.Fatalf("king at activation = %s, want %s", got.Hex(), candidate.Hex())
	}
	if _, err := manager.RegisterKingAt(candidate, 101, EligibilityThreshold); !errors.Is(err, ErrDuplicateRegistration) {
		t.Fatalf("duplicate registration error = %v, want %v", err, ErrDuplicateRegistration)
	}
}

func TestRegistrationHashIsChainBound(t *testing.T) {
	address := common.HexToAddress("0x301")
	hashEgypt := RegistrationHashForChain(big.NewInt(8980), address, EligibilityThreshold, 8, 10)
	hashMainnet := RegistrationHashForChain(big.NewInt(8979), address, EligibilityThreshold, 8, 10)
	if hashEgypt == hashMainnet {
		t.Fatalf("registration hash was replayable across chain IDs: %s", hashEgypt.Hex())
	}
}

func TestRotateToNextKingRefusesIneligibleSchedule(t *testing.T) {
	first := common.HexToAddress("0x201")
	second := common.HexToAddress("0x202")
	manager := NewRotatingKingManager(common.Address{}, []common.Address{first, second}, 10)
	state := registrationState{balances: map[common.Address]*big.Int{first: EligibilityThreshold, second: new(big.Int)}, height: 10}
	if err := manager.RotateToNextKing(10, common.HexToHash("0x1"), state); err == nil {
		t.Fatal("rotation succeeded with no eligible candidate")
	}
	if got := manager.GetCurrentKing(); got != first {
		t.Fatalf("current king changed after rejected rotation: %s", got.Hex())
	}
	state.balances[second] = new(big.Int).Set(EligibilityThreshold)
	if err := manager.RotateToNextKing(10, common.HexToHash("0x1"), state); err != nil {
		t.Fatalf("eligible rotation failed: %v", err)
	}
	if got := manager.GetCurrentKing(); got != second {
		t.Fatalf("current king = %s, want %s", got.Hex(), second.Hex())
	}
}

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
