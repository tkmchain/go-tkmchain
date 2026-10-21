package shield3wallet

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShield4ConsolidationUsesLargestFourAndPreservesTargetReserve(t *testing.T) {
	notes := make([]OwnedNote, 0, 6)
	for i, value := range []string{"1", "2", "3", "4", "5", "6"} {
		notes = append(notes, OwnedNote{Note: Note{ValueWei: value}, Commitment: shielded3.Digest{uint64(i + 1)}})
	}
	amount, err := shield4ConsolidationAmount(notes, big.NewInt(10), big.NewInt(2))
	if err != nil {
		t.Fatal(err)
	}
	// The largest four sum to 18; reserve 2 for the possible fee-sponsored
	// input, leaving a self-payment of 16.
	if amount.Cmp(big.NewInt(16)) != 0 {
		t.Fatalf("consolidation amount = %s, want 16", amount)
	}
}

func TestShield4ConsolidationRefusesInsufficientAndAlreadyBoundedNotes(t *testing.T) {
	tooSmall := []OwnedNote{
		{Note: Note{ValueWei: "3"}, Commitment: shielded3.Digest{1}},
		{Note: Note{ValueWei: "2"}, Commitment: shielded3.Digest{2}},
		{Note: Note{ValueWei: "1"}, Commitment: shielded3.Digest{3}},
		{Note: Note{ValueWei: "1"}, Commitment: shielded3.Digest{4}},
		{Note: Note{ValueWei: "1"}, Commitment: shielded3.Digest{5}},
	}
	if _, err := shield4ConsolidationAmount(tooSmall, big.NewInt(9), big.NewInt(1)); err == nil {
		t.Fatal("consolidation accepted notes that cannot cover the send and reserve")
	}
	bounded := []OwnedNote{
		{Note: Note{ValueWei: "3"}, Commitment: shielded3.Digest{1}},
		{Note: Note{ValueWei: "3"}, Commitment: shielded3.Digest{2}},
		{Note: Note{ValueWei: "3"}, Commitment: shielded3.Digest{3}},
		{Note: Note{ValueWei: "3"}, Commitment: shielded3.Digest{4}},
	}
	if _, err := shield4ConsolidationAmount(bounded, big.NewInt(10), big.NewInt(1)); err == nil {
		t.Fatal("consolidation should not loop when all notes already fit within the input bound")
	}
}
