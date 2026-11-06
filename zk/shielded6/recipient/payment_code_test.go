package recipient

import (
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestRecipientPaymentCodeRoundtripAndAuthorization(t *testing.T) {
	s, w := relationFixture(t)
	defer w.Clear()
	p := PaymentCode{Version: RelationVersion, Address: w.Recipient, Owner: w.Owner, StampRoot: s.StampRoot, BindingRoot: s.BindingRoot, StampIndex: w.StampIndex, StampPath: w.StampPath, BindingIndex: w.BindingIndex, BindingPath: w.BindingPath}
	encoded, err := EncodePaymentCode(p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePaymentCode(encoded, s.ChainID)
	if err != nil || decoded != p {
		t.Fatalf("payment-code roundtrip: %v", err)
	}
	if _, err := DecodePaymentCode(encoded, s.ChainID+1); err == nil {
		t.Fatal("wrong-chain code accepted")
	}
	if _, err := DecodePaymentCode(encoded+"A", s.ChainID); err == nil {
		t.Fatal("modified code accepted")
	}
	p.Owner = shielded3.Digest{99}
	if p.Validate(s.ChainID) == nil {
		t.Fatal("owner substitution accepted")
	}
	// The registry wrapper is controlled by the account owner. A commitment
	// constructed for a different stamped identity cannot use that path.
	p = decoded
	commitment, err := BindingCommitment(s.ChainID, w.Owner, w.Recipient)
	if err != nil {
		t.Fatal(err)
	}
	wrongLeaf, err := AuthorizedBindingLeaf(s.ChainID, shielded3.Digest{99}, commitment)
	if err != nil {
		t.Fatal(err)
	}
	current := wrongLeaf
	for level, sibling := range w.BindingPath {
		if w.BindingIndex>>level&1 == 0 {
			current, err = shielded3.HashPair(current, sibling)
		} else {
			current, err = shielded3.HashPair(sibling, current)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	p.BindingRoot = current
	if p.Validate(s.ChainID) == nil {
		t.Fatal("another account's authorized registry wrapper accepted")
	}
}
