package recipient

import (
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/test"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type goldReductionCircuit struct{ Value, Expected frontend.Variable }

func (c *goldReductionCircuit) Define(api frontend.API) error {
	api.ToBinary(c.Value, 192)
	api.AssertIsEqual(reduceGold(api, c.Value, 128), c.Expected)
	return nil
}

func TestRecipientGoldReductionRejectsMaliciousHints(t *testing.T) {
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &goldReductionCircuit{})
	if err != nil {
		t.Fatal(err)
	}
	p := new(big.Int).SetUint64(goldPrime)
	max := new(big.Int).Sub(p, big.NewInt(1))
	max.Exp(max, big.NewInt(3), nil)
	for _, value := range []*big.Int{big.NewInt(0), big.NewInt(1), new(big.Int).Sub(p, big.NewInt(1)), new(big.Int).Set(p), new(big.Int).Mul(p, p), max} {
		w, err := frontend.NewWitness(&goldReductionCircuit{Value: value, Expected: new(big.Int).Mod(value, p)}, ecc.BN254.ScalarField())
		if err != nil {
			t.Fatal(err)
		}
		if err := cs.IsSolved(w); err != nil {
			t.Fatal(err)
		}
	}
	w, err := frontend.NewWitness(&goldReductionCircuit{Value: new(big.Int).Mul(p, p), Expected: 0}, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	for _, attack := range []string{"quotient", "noncanonical-remainder", "high-limb", "low-limb"} {
		override := func(field *big.Int, in, out []*big.Int) error {
			if err := goldReduceHint(field, in, out); err != nil {
				return err
			}
			switch attack {
			case "quotient":
				out[0].Add(out[0], big.NewInt(1))
			case "noncanonical-remainder":
				out[0].Sub(out[0], big.NewInt(1))
				out[1].SetUint64(1)
				out[2].SetUint64(0xffffffff)
			case "high-limb":
				out[2].SetUint64(uint64(1) << 32)
			case "low-limb":
				out[1].SetUint64(uint64(1) << 32)
			}
			return nil
		}
		if err := cs.IsSolved(w, solver.OverrideHint(solver.GetHintID(goldReduceHint), override)); err == nil {
			t.Fatalf("accepted %s hint", attack)
		}
	}
}

type tip5ComparisonCircuit struct{ State [16]goldElement }

func (c *tip5ComparisonCircuit) Define(api frontend.API) error {
	h, err := newTip5(api)
	if err != nil {
		return err
	}
	var input [16]*goldElement
	for i := range input {
		h.f.AssertIsInRange(&c.State[i])
		input[i] = &c.State[i]
	}
	got, want := h.permutation(input), h.emulatedPermutation(input)
	for i := range got {
		h.f.AssertIsEqual(got[i], want[i])
	}
	return nil
}

func TestRecipientTip5NativeArithmeticMatchesEmulation(t *testing.T) {
	for _, start := range []uint64{0, 1, 0xffffffff, 0x100000000, goldPrime - 17} {
		assignment := new(tip5ComparisonCircuit)
		for i := range assignment.State {
			assignment.State[i] = emulated.ValueOf[gold](start + uint64(i))
		}
		if err := test.IsSolved(&tip5ComparisonCircuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecipientTip5RejectsByteHintSubstitution(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("native Tip5")
	}
	words := shielded3.Digest{1, 2, 3, 4, 5}
	expected, err := shielded3.HashPair(words, words)
	if err != nil {
		t.Fatal(err)
	}
	circuit := &tip5TestCircuit{Words: make([]goldElement, 10), Pair: true}
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		t.Fatal(err)
	}
	assignment := &tip5TestCircuit{Words: make([]goldElement, 10), Expected: digestAssignment(expected), Pair: true}
	for i := range assignment.Words {
		assignment.Words[i] = emulated.ValueOf[gold](words[i%5])
	}
	w, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.IsSolved(w); err != nil {
		t.Fatal(err)
	}
	override := func(field *big.Int, in, out []*big.Int) error {
		if err := goldBytesHint(field, in, out); err != nil {
			return err
		}
		out[0].Add(out[0], big.NewInt(256))
		out[1].Sub(out[1], big.NewInt(1))
		return nil
	}
	if err := cs.IsSolved(w, solver.OverrideHint(solver.GetHintID(goldBytesHint), override)); err == nil {
		t.Fatal("accepted byte hint outside fixed lookup table")
	}
}
