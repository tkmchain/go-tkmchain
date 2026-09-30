package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
)

type validatorTestState struct{ values map[common.Hash]common.Hash }

func (s *validatorTestState) GetState(_ common.Address, key common.Hash) common.Hash {
	return s.values[key]
}

func TestValidatorRewardMarkerBindsToSelectedValidator(t *testing.T) {
	st := &validatorTestState{values: make(map[common.Hash]common.Hash)}
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	pub := pqcrypto.PublicKeyBytes(key)
	addr, _ := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	putValidatorTestRecord(t, st, 0, &ValidatorRecord{Version: 1, Address: addr, PublicKey: pub, RewardAddress: addr, Stake: ValidatorBondWei(), ActivationHeight: 1})
	header := &types.Header{Number: new(big.Int).SetUint64(1000), ParentHash: common.HexToHash("0x1234")}
	tx, err := BuildValidatorRewardTx(st, header)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateValidatorRewardTransaction(st, header, tx); err != nil {
		t.Fatal(err)
	}
	tx = types.NewValidatorRewardTx(header.Number.Uint64(), common.HexToAddress("0xdead"), tx.Value())
	if err := ValidateValidatorRewardTransaction(st, header, tx); err == nil {
		t.Fatal("accepted reward for a non-selected validator")
	}
}
func (s *validatorTestState) SetState(_ common.Address, key common.Hash, value common.Hash) common.Hash {
	old := s.values[key]
	s.values[key] = value
	return old
}

func putValidatorTestRecord(t *testing.T, st *validatorTestState, index uint64, record *ValidatorRecord) {
	t.Helper()
	if err := writeValidatorRecord(st, record); err != nil {
		t.Fatal(err)
	}
	st.SetState(params.ShieldedPoolAddress, validatorIndexSlot(index), common.BytesToHash(record.Address.Bytes()))
	st.SetState(params.ShieldedPoolAddress, validatorCountSlot(), uint64Hash(index+1))
}

func TestValidatorRegistryActivationAndDeterministicSelection(t *testing.T) {
	st := &validatorTestState{values: make(map[common.Hash]common.Hash)}
	keyA, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	pubA, pubB := pqcrypto.PublicKeyBytes(keyA), pqcrypto.PublicKeyBytes(keyB)
	addrA, _ := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pubA)
	addrB, _ := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pubB)
	putValidatorTestRecord(t, st, 0, &ValidatorRecord{Version: 1, Address: addrB, PublicKey: pubB, RewardAddress: addrB, Stake: new(big.Int).Set(ValidatorBondWei()), ActivationHeight: 20})
	putValidatorTestRecord(t, st, 1, &ValidatorRecord{Version: 1, Address: addrA, PublicKey: pubA, RewardAddress: addrA, Stake: new(big.Int).Set(ValidatorBondWei()), ActivationHeight: 10})
	active, err := ActiveValidatorRecords(st, 19)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Address != addrA {
		t.Fatalf("active set = %+v", active)
	}
	first, err := SelectValidator(st, 20, common.HexToHash("0x42"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := SelectValidator(st, 20, common.HexToHash("0x42"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Address != second.Address {
		t.Fatal("validator selection is not deterministic")
	}
	if first.Stake.Cmp(ValidatorBondWei()) != 0 {
		t.Fatal("validator stake was not persisted")
	}
}

func TestValidatorExitAndWithdrawalStateMachine(t *testing.T) {
	st := &validatorTestState{values: make(map[common.Hash]common.Hash)}
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	pub := pqcrypto.PublicKeyBytes(key)
	addr, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	if err != nil {
		t.Fatal(err)
	}
	putValidatorTestRecord(t, st, 0, &ValidatorRecord{
		Version: 1, Address: addr, PublicKey: pub, RewardAddress: addr,
		Stake: new(big.Int).Set(ValidatorBondWei()), ActivationHeight: 1,
	})
	exit, err := EncodeValidatorExit(&ValidatorAction{Version: ValidatorEnvelopeVersion})
	if err != nil {
		t.Fatal(err)
	}
	if !HasValidatorExitPrefix(exit) || HasValidatorWithdrawalPrefix(exit) {
		t.Fatal("validator exit envelope prefix is invalid")
	}
	if err := ValidateValidatorActionState(st, addr, exit, 100); err != nil {
		t.Fatal(err)
	}
	record, err := readValidatorRecord(st, addr)
	if err != nil {
		t.Fatal(err)
	}
	record.ExitHeight = 100
	record.JailedUntil = 100 + ValidatorUnbondingPeriod
	if err := writeValidatorRecord(st, record); err != nil {
		t.Fatal(err)
	}
	withdrawal, err := EncodeValidatorWithdrawal(&ValidatorAction{Version: ValidatorEnvelopeVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateValidatorActionState(st, addr, withdrawal, 100+ValidatorUnbondingPeriod-1); err == nil {
		t.Fatal("accepted a withdrawal before the unbonding period elapsed")
	}
	if err := ValidateValidatorActionState(st, addr, withdrawal, 100+ValidatorUnbondingPeriod); err != nil {
		t.Fatal(err)
	}
	record.Stake.SetUint64(0)
	if err := writeValidatorRecord(st, record); err != nil {
		t.Fatal(err)
	}
	if err := ValidateValidatorActionState(st, addr, withdrawal, 100+ValidatorUnbondingPeriod); err == nil {
		t.Fatal("accepted a second withdrawal after the bond was cleared")
	}
}
