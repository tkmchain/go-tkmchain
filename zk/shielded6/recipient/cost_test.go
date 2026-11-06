package recipient

import (
	"os"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/math/uints"
)

// Explicit profiling avoids guessing which cryptographic operation dominates
// costs. It neither runs setup nor changes any accepted witness relation.
func TestRecipientConstraintCosts(t *testing.T) {
	if os.Getenv("SHIELD6_CONSTRAINT_COSTS") != "1" {
		t.Skip("set SHIELD6_CONSTRAINT_COSTS=1 to profile primitive constraint counts")
	}
	for _, item := range []struct {
		name    string
		circuit frontend.Circuit
	}{
		{"Tip5Pair", &tip5TestCircuit{Words: make([]goldElement, 10), Pair: true}},
		{"HKDFExtractHMAC", &sha512TestCircuit{Key: make([]uints.U8, 16), Data: make([]uints.U8, 32), HMAC: true}},
		{"HKDFExpandHMAC", &sha512TestCircuit{Key: make([]uints.U8, 64), Data: make([]uints.U8, 31), HMAC: true}},
		{"VariableBaseMultiplication", &multiplicationTestCircuit{}},
		{"FixedBaseMultiplication", &multiplicationTestCircuit{Fixed: true}},
	} {
		cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, item.circuit)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d constraints", item.name, cs.GetNbConstraints())
	}
}
