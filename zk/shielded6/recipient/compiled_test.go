package recipient

import (
	"encoding/hex"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/std/math/uints"
)

func TestRecipientCompiledRelation(t *testing.T) {
	s, w := relationFixture(t)
	defer w.Clear()
	cs, hash, err := Compile()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d constraints; R1CS SHA256 %x", cs.GetNbConstraints(), hash)
	// The fingerprint must be updated deliberately with every circuit change.
	if hex.EncodeToString(hash[:]) != CircuitSHA256 {
		t.Errorf("candidate circuit fingerprint changed: %x", hash)
	}
	assignment, err := NewAssignment(s, w)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.IsSolved(encoded); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"owner", "scalar", "context"} {
		bad, err := NewAssignment(s, w)
		if err != nil {
			t.Fatal(err)
		}
		switch name {
		case "owner":
			bad.Owner[0] = emulated.ValueOf[gold](99)
		case "scalar":
			set32(&bad.EphemeralSecret, [32]byte{})
		case "context":
			bad.Claim.Intent[0] = uints.NewU8(s.Intent[0] ^ 1)
		}
		encoded, err := frontend.NewWitness(bad, ecc.BN254.ScalarField())
		if err != nil {
			t.Fatal(err)
		}
		if err := cs.IsSolved(encoded); err == nil {
			t.Fatalf("compiled circuit accepted %s", name)
		}
	}
}
