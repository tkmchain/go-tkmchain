package shielded

import (
	"bytes"
	"os"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/test"
)

func TestSpendCircuitVectors(t *testing.T) {
	assert := test.NewAssert(t)
	var circuit SpendCircuit
	valid := assignmentFromVector(t, DeterministicTestVectors().Valid)
	assert.ProverSucceeded(&circuit, valid, test.WithCurves(ecc.BN254))
	for _, bad := range DeterministicTestVectors().Invalid {
		t.Run(bad.Name, func(t *testing.T) {
			assert.ProverFailed(&circuit, assignmentFromVector(t, bad.Case), test.WithCurves(ecc.BN254))
		})
	}
}

func TestSpendCircuitCompiles(t *testing.T) {
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &SpendCircuit{})
	if err != nil {
		t.Fatal(err)
	}
	if ccs.GetNbConstraints() == 0 {
		t.Fatal("shielded spend circuit compiled with no constraints")
	}
}

func TestDeterministicTestVectorsJSON(t *testing.T) {
	out, err := DeterministicTestVectorsJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty test vector JSON")
	}
}

func TestTestdataVectorsUpToDate(t *testing.T) {
	want, err := DeterministicTestVectorsJSON()
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	got, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("testdata/vectors.json is not up to date; run go run ./cmd/shielded-vectors -out zk/shielded/testdata/vectors.json")
	}
}

func assignmentFromVector(t *testing.T, vector VectorCase) *SpendCircuit {
	t.Helper()
	c := &SpendCircuit{
		ChainID:           vector.Public.ChainID,
		BlockNumber:       vector.Public.BlockNumber,
		TxHashHi:          vector.Public.TxHashHi,
		TxHashLo:          vector.Public.TxHashLo,
		SpendIndex:        vector.Public.SpendIndex,
		Nullifier:         vector.Public.Nullifier,
		Anchor:            vector.Public.Anchor,
		BalanceCommitment: vector.Public.BalanceCommitment,
		PublicValue:       vector.Public.PublicValue,
		OutputRoot:        vector.Public.OutputRoot,
		BindingSigHash:    vector.Public.BindingSigHash,
		OwnerSecret:       vector.Private.OwnerSecret,
		NoteRandomness:    vector.Private.NoteRandomness,
		NoteValue:         vector.Private.NoteValue,
		AssetID:           vector.Private.AssetID,
	}
	fillArray(t, c.MerklePath[:], vector.Private.MerklePath)
	fillArray(t, c.MerklePathIndex[:], vector.Private.MerklePathIndex)
	fillArray(t, c.OutputRecipient[:], vector.Private.OutputRecipient)
	fillArray(t, c.OutputValue[:], vector.Private.OutputValue)
	fillArray(t, c.OutputRandomness[:], vector.Private.OutputRandomness)
	fillArray(t, c.OutputCommitment[:], vector.Private.OutputCommitment)
	return c
}

func fillArray(t *testing.T, dst []frontend.Variable, src []string) {
	t.Helper()
	if len(dst) != len(src) {
		t.Fatalf("vector length mismatch: got %d, want %d", len(src), len(dst))
	}
	for i := range dst {
		dst[i] = src[i]
	}
}
