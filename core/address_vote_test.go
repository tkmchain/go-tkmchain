package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

func stampedVoteState(t *testing.T) *state.StateDB {
	t.Helper()
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func addVoteStamp(st *state.StateDB, voter common.Address, seed byte) {
	owner := shielded3.Digest{}
	owner[0] = uint64(seed)
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", voter.Bytes()), common.BytesToHash([]byte{seed}))
	v3WriteDigest(st, "stamp/address-owner", voter.Bytes(), owner)
	st.SetBalance(voter, uint256.MustFromBig(new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether))), tracing.BalanceChangeUnspecified)
}

func TestAddressVoteThresholdAndCallerOwnedUnvote(t *testing.T) {
	st := stampedVoteState(t)
	target := common.HexToAddress("0x1234")
	for i := byte(1); i <= byte(AddressVoteThreshold); i++ {
		voter := common.BytesToAddress([]byte{i})
		addVoteStamp(st, voter, i)
		balanceBefore := st.GetBalance(voter).ToBig()
		data, err := EncodeAddressVote(&AddressVote{Version: AddressVoteVersion, Target: target, Reason: "fraud"})
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateAddressVoteState(st, voter, data); err != nil {
			t.Fatalf("vote %d rejected: %v", i, err)
		}
		if err := ProcessAddressVote(st, voter, data, common.BigToHash(big.NewInt(int64(i)))); err != nil {
			t.Fatalf("process vote %d: %v", i, err)
		}
		if want := new(big.Int).Sub(balanceBefore, AddressVoteBurnWei()); st.GetBalance(voter).ToBig().Cmp(want) != 0 {
			t.Fatalf("vote %d burn = %s, want %s", i, st.GetBalance(voter), want)
		}
	}
	if got := AddressVoteCount(st, target); got != AddressVoteThreshold || !IsAddressSuspended(st, target) {
		t.Fatalf("threshold state = count %d suspended %v", got, IsAddressSuspended(st, target))
	}

	duplicate, _ := EncodeAddressVote(&AddressVote{Version: AddressVoteVersion, Target: target, Reason: "fraud"})
	if err := ValidateAddressVoteState(st, common.BytesToAddress([]byte{1}), duplicate); !errors.Is(err, ErrAddressVoteAlreadyCast) {
		t.Fatalf("duplicate vote error = %v", err)
	}

	unvote, _ := EncodeAddressVote(&AddressVote{Version: AddressVoteVersion, Unvote: true, Target: target})
	if err := ValidateAddressVoteState(st, common.BytesToAddress([]byte{1}), unvote); err != nil {
		t.Fatalf("caller unvote rejected: %v", err)
	}
	if err := ProcessAddressVote(st, common.BytesToAddress([]byte{1}), unvote, common.HexToHash("0x99")); err != nil {
		t.Fatal(err)
	}
	if want := new(big.Int).Sub(new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether)), new(big.Int).Mul(big.NewInt(2), AddressVoteBurnWei())); st.GetBalance(common.BytesToAddress([]byte{1})).ToBig().Cmp(want) != 0 {
		t.Fatalf("unvote burn = %s, want %s", st.GetBalance(common.BytesToAddress([]byte{1})), want)
	}
	if got := AddressVoteCount(st, target); got != AddressVoteThreshold-1 || IsAddressSuspended(st, target) {
		t.Fatalf("unvote state = count %d suspended %v", got, IsAddressSuspended(st, target))
	}
}

func TestAddressVoteRequiresBurnBalance(t *testing.T) {
	st := stampedVoteState(t)
	voter := common.HexToAddress("0x99")
	addVoteStamp(st, voter, 1)
	st.SetBalance(voter, new(uint256.Int), tracing.BalanceChangeUnspecified)
	target := common.HexToAddress("0x1234")
	data, err := EncodeAddressVote(&AddressVote{Version: AddressVoteVersion, Target: target, Reason: "fraud"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ProcessAddressVote(st, voter, data, common.HexToHash("0x1")); !errors.Is(err, ErrAddressVoteInsufficientBalance) {
		t.Fatalf("insufficient balance error = %v", err)
	}
	if got := AddressVoteCount(st, target); got != 0 {
		t.Fatalf("vote count changed after rejected burn: %d", got)
	}
}

func TestAddressVoteRequiresStampAndBoundedReason(t *testing.T) {
	st := stampedVoteState(t)
	target := common.HexToAddress("0x1234")
	vote, err := EncodeAddressVote(&AddressVote{Version: AddressVoteVersion, Target: target, Reason: "fraud"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAddressVoteState(st, common.HexToAddress("0x99"), vote); !errors.Is(err, ErrUnstampedAddress) {
		t.Fatalf("unstamped voter error = %v", err)
	}
	longReason := make([]byte, AddressVoteMaxReason+1)
	for i := range longReason {
		longReason[i] = 'x'
	}
	longVote, err := EncodeAddressVote(&AddressVote{Version: AddressVoteVersion, Target: target, Reason: string(longReason)})
	if err != nil {
		t.Fatal(err)
	}
	tx := types.NewTx(&types.PQTkmTx{ChainID: params.MainnetChainConfig.ChainID, To: &params.ShieldedPoolAddress, Value: new(big.Int), Data: longVote})
	if err := ValidateAddressVoteBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); err == nil {
		t.Fatal("oversized vote reason accepted")
	}
	shortVote, err := EncodeAddressVote(&AddressVote{Version: AddressVoteVersion, Target: target, Reason: "fraud"})
	if err != nil {
		t.Fatal(err)
	}
	voteTx := types.NewTx(&types.PQTkmTx{ChainID: params.MainnetChainConfig.ChainID, To: &params.ShieldedPoolAddress, Value: new(big.Int), Data: shortVote})
	if cost := ShieldedTransactionPreBalanceCost(voteTx); cost.Cmp(AddressVoteBurnWei()) != 0 {
		t.Fatalf("vote pre-balance cost = %s, want %s", cost, AddressVoteBurnWei())
	}
}
