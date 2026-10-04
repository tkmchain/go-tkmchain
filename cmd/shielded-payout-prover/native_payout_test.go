package main

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestNativePayoutCapacityUsesFourLargestAndFees(t *testing.T) {
	var notes []shield3wallet.OwnedNote
	for i, amount := range []int64{1, 2, 3, 4, 100} {
		n := shield3wallet.OwnedNote{Note: shield3wallet.Note{ValueWei: big.NewInt(amount).String()}}
		n.Commitment[0] = uint64(i + 1)
		notes = append(notes, n)
	}
	got, err := nativeNoteCapacity(notes, big.NewInt(9))
	if err != nil || got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("capacity %v, %v", got, err)
	}
	got, err = nativeNoteCapacity(notes, big.NewInt(1000))
	if err != nil || got.Sign() != 0 {
		t.Fatalf("insufficient gas %v, %v", got, err)
	}
	if _, err = nativeNoteCapacity(append(notes, notes[0]), new(big.Int)); err == nil {
		t.Fatal("duplicate note accepted")
	}
}

func TestLegacyProofBuilderEndpointsAreDisabled(t *testing.T) {
	p := (*Prover)(nil)
	checks := []struct {
		name string
		call func() error
	}{
		{"deposit", func() error { _, err := p.BuildDeposit(context.Background(), DepositRequest{}); return err }},
		{"transfer", func() error { _, err := p.BuildTransfer(context.Background(), BuildTransferRequest{}); return err }},
		{"withdrawal", func() error { _, err := p.BuildWithdrawal(context.Background(), BuildWithdrawalRequest{}); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.call()
			if err == nil || !strings.Contains(err.Error(), "legacy proof-only") {
				t.Fatalf("legacy builder was not explicitly rejected: %v", err)
			}
		})
	}
}

func TestNativeAmountsAboveLegacyUint64(t *testing.T) {
	max := shielded3.MaxSendWei()
	for _, v := range []*big.Int{new(big.Int).Mul(big.NewInt(25), big.NewInt(1e18)), max} {
		got, err := nativeDepositAmount(DepositRequest{AmountWei: v.String()})
		if err != nil || got.Cmp(v) != 0 {
			t.Fatalf("valid native amount %s: %v", v, err)
		}
		got, err = nativeAmount(v.String())
		if err != nil || got.Cmp(v) != 0 {
			t.Fatalf("valid payout %s: %v", v, err)
		}
	}
	for _, v := range []string{"0", "-1", "bad", new(big.Int).Add(max, big.NewInt(1)).String()} {
		if _, err := nativeAmount(v); err == nil {
			t.Fatalf("accepted %s", v)
		}
	}
	note := shield3wallet.OwnedNote{Note: shield3wallet.Note{ValueWei: new(big.Int).Mul(max, big.NewInt(2)).String()}}
	got, err := nativeNoteCapacity([]shield3wallet.OwnedNote{note}, new(big.Int))
	if err != nil || got.Cmp(max) != 0 {
		t.Fatalf("limit %v: %v", got, err)
	}
}
