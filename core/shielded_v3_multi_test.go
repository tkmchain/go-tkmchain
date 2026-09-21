package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShield3MultiNullifiersAndRelayExpiry(t *testing.T) {
	e := &ShieldedV3Transaction{Nullifier: shielded3.Digest{1}, InputCount: 4, AdditionalNullifiers: [3]shielded3.Digest{{2}, {3}, {4}}, WithdrawalValue: new(big.Int), Relayed: true, ValidUntil: 200}
	ns, err := ShieldedV3Nullifiers(e)
	if err != nil || len(ns) != 4 {
		t.Fatal(err)
	}
	e.AdditionalNullifiers[2] = e.Nullifier
	if _, err = ShieldedV3Nullifiers(e); err == nil {
		t.Fatal("accepted duplicate input")
	}
	e.AdditionalNullifiers[2] = shielded3.Digest{4}
	e.InputCount = 3
	if _, err = ShieldedV3Nullifiers(e); err == nil {
		t.Fatal("accepted inactive nullifier")
	}
	e.InputCount = 4
	e.AdditionalNullifiers[1] = shielded3.Digest{}
	if _, err = ShieldedV3Nullifiers(e); err == nil {
		t.Fatal("accepted zero active nullifier")
	}
	e.AdditionalNullifiers[1] = shielded3.Digest{3}
	e.InputCount = 5
	if _, err = ShieldedV3Nullifiers(e); err == nil {
		t.Fatal("exceeded input bound")
	}
	e.InputCount = 4
	if err = ValidateShieldedV3Time(e, 100); err != nil {
		t.Fatal(err)
	}
	if err = ValidateShieldedV3Time(e, 200); err == nil {
		t.Fatal("accepted expired relay")
	}
	e.ValidUntil = 100 + AntarticalStampSponsorshipLifetime + 1
	if err = ValidateShieldedV3Time(e, 100); err == nil {
		t.Fatal("accepted excessive lifetime")
	}
	e.ValidUntil = 200
	e.Deposit = true
	if err = ValidateShieldedV3Time(e, 100); err == nil {
		t.Fatal("accepted public-deposit relay")
	}
	e.Deposit = false
	e.WithdrawalValue.SetInt64(1)
	if err = ValidateShieldedV3Time(e, 100); err == nil {
		t.Fatal("accepted public-withdrawal relay")
	}
	e.WithdrawalValue.SetInt64(0)
	e.Relayed = false
	if err = ValidateShieldedV3Time(e, 100); err == nil {
		t.Fatal("accepted direct transaction with relay expiry")
	}
}
