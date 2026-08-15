package main

import (
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/frontend"
	"github.com/ethereum/go-ethereum/zk/shielded"
)

func TestParseFieldElementAcceptsDecimalAndHex(t *testing.T) {
	decimal, err := parseFieldElement("42")
	if err != nil {
		t.Fatalf("parse decimal: %v", err)
	}
	hexadecimal, err := parseFieldElement("0x2a")
	if err != nil {
		t.Fatalf("parse hex: %v", err)
	}
	if !decimal.Equal(&hexadecimal) {
		t.Fatalf("decimal and hex field elements differ")
	}
}

func TestComputeAnchorAcceptsHexWitness(t *testing.T) {
	decimalPath := make([]string, shielded.MerkleDepth)
	hexPath := make([]string, shielded.MerkleDepth)
	decimalIndex := make([]string, shielded.MerkleDepth)
	hexIndex := make([]string, shielded.MerkleDepth)
	for i := 0; i < shielded.MerkleDepth; i++ {
		decimalPath[i] = "0"
		hexPath[i] = "0x0"
		decimalIndex[i] = "0"
		hexIndex[i] = "0x0"
	}
	decimalPath[3], hexPath[3] = "42", "0x2a"
	decimalIndex[3], hexIndex[3] = "1", "0x1"

	commitment := fieldElementFromUint64(7)
	decimalAnchor, err := computeAnchor(commitment, decimalPath, decimalIndex)
	if err != nil {
		t.Fatalf("compute decimal anchor: %v", err)
	}
	hexAnchor, err := computeAnchor(commitment, hexPath, hexIndex)
	if err != nil {
		t.Fatalf("compute hex anchor: %v", err)
	}
	if !decimalAnchor.Equal(&hexAnchor) {
		t.Fatalf("decimal and hex witnesses produced different anchors")
	}
}

func TestComputeAnchorRejectsInvalidWitness(t *testing.T) {
	path := make([]string, shielded.MerkleDepth)
	index := make([]string, shielded.MerkleDepth)
	for i := 0; i < shielded.MerkleDepth; i++ {
		path[i] = "0"
		index[i] = "0"
	}

	path[5] = "not-a-field"
	if _, err := computeAnchor(fr.Element{}, path, index); err == nil {
		t.Fatalf("invalid field element was accepted")
	}
	path[5] = "0"
	index[5] = "2"
	if _, err := computeAnchor(fr.Element{}, path, index); err == nil {
		t.Fatalf("invalid path index was accepted")
	}
}

func TestSelectSpendableNoteSkipsInvalidWitness(t *testing.T) {
	validPath := make([]string, shielded.MerkleDepth)
	validIndex := make([]string, shielded.MerkleDepth)
	for i := 0; i < shielded.MerkleDepth; i++ {
		validPath[i] = "0x0"
		validIndex[i] = "0x0"
	}
	invalidPath := append([]string(nil), validPath...)
	invalidPath[0] = "invalid"
	store := NoteStore{Notes: []ShieldedNote{
		{ID: "invalid", NoteValueWei: "10", MerklePath: invalidPath, MerklePathIndex: validIndex, Status: "available"},
		{ID: "valid", NoteValueWei: "10", MerklePath: validPath, MerklePathIndex: validIndex, Status: "available"},
	}}
	if got := selectSpendableNote(store, big.NewInt(5)); got != 1 {
		t.Fatalf("selected note index %d, want 1", got)
	}
}

func TestBuildDraftEnvelopeAcceptsHexWitness(t *testing.T) {
	path := make([]string, shielded.MerkleDepth)
	index := make([]string, shielded.MerkleDepth)
	for i := 0; i < shielded.MerkleDepth; i++ {
		path[i] = "0x0"
		index[i] = "0x0"
	}
	note := ShieldedNote{
		ID:              "hex-witness",
		OwnerSecret:     "11",
		NoteRandomness:  "22",
		NoteValueWei:    "10",
		AssetID:         "1",
		MerklePath:      path,
		MerklePathIndex: index,
		Status:          "available",
	}
	req := PayoutRequest{
		RequestID: "12345678",
		To:        "0x0000000000000000000000000000000000000001",
		AmountWei: "0x5",
	}
	_, assignment, err := (&Prover{}).buildDraftEnvelope(req, note, big.NewInt(5), big.NewInt(1), new(big.Int), 0, big.NewInt(1))
	if err != nil {
		t.Fatalf("build draft: %v", err)
	}
	if _, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("build witness: %v", err)
	}
}
