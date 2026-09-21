package shield3wallet

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestNoteSelectionPrefersOneAndBoundsCombination(t *testing.T) {
	notes := []OwnedNote{}
	for i, value := range []string{"3", "2", "4", "5", "1"} {
		notes = append(notes, OwnedNote{Note: Note{ValueWei: value}, Commitment: shielded3.Digest{uint64(i + 1)}})
	}
	chosen, err := selectNotes(notes, big.NewInt(4))
	if err != nil || len(chosen) != 1 || chosen[0].ValueWei != "4" {
		t.Fatal("did not prefer smallest sufficient note", err)
	}
	chosen, err = selectNotes(notes, big.NewInt(13))
	if err != nil || len(chosen) != 4 {
		t.Fatal("combination", err)
	}
	if _, err = selectNotes(notes, big.NewInt(15)); err == nil {
		t.Fatal("exceeded four-input bound")
	}
	notes = append(notes, notes[0])
	if _, err = selectNotes(notes, big.NewInt(1)); err == nil {
		t.Fatal("selected duplicate commitments")
	}
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	notes = []OwnedNote{{Note: Note{ValueWei: max.String()}, Commitment: shielded3.Digest{1}}, {Note: Note{ValueWei: "1"}, Commitment: shielded3.Digest{2}}}
	if _, err = selectNotes(notes, new(big.Int).Add(max, big.NewInt(1))); err == nil {
		t.Fatal("accepted overflowing aggregate")
	}
}
