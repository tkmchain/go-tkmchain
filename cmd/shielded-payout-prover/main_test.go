package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/frontend"
	"github.com/ethereum/go-ethereum/zk/shielded"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
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

func TestEncryptShieldedNoteRoundTrip(t *testing.T) {
	curve := ecdh.X25519()
	recipient, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	commitment := hashFromField(fieldElementFromUint64(42))
	opening := shieldedNoteOpening{
		Recipient:      "0x0000000000000000000000000000000000000001",
		OwnerSecret:    "1",
		AssetID:        "1",
		NoteValueWei:   "5",
		NoteRandomness: "9",
		Nullifier:      hashFromField(fieldElementFromUint64(10)).Hex(),
	}
	output, err := encryptShieldedNote(commitment, opening, recipient.PublicKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	ephemeral, err := curve.NewPublicKey(output.EphemeralPubKey)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := recipient.ECDH(ephemeral)
	if err != nil {
		t.Fatal(err)
	}
	material := make([]byte, chacha20poly1305.KeySize+1)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, commitment[:], []byte(shieldedNoteKDFInfo)), material); err != nil {
		t.Fatal(err)
	}
	if len(output.ViewTag) != 1 || output.ViewTag[0] != material[chacha20poly1305.KeySize] {
		t.Fatal("view tag does not match derived key material")
	}
	aead, err := chacha20poly1305.NewX(material[:chacha20poly1305.KeySize])
	if err != nil {
		t.Fatal(err)
	}
	aad := append([]byte(shieldedNoteKDFInfo), commitment[:]...)
	plain, err := aead.Open(nil, output.Nonce, output.EncryptedPayload, aad)
	if err != nil {
		t.Fatal(err)
	}
	var decoded shieldedNoteOpening
	if err := json.Unmarshal(plain, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Format != shieldedNotePayloadFormat || decoded.Commitment != commitment.Hex() || decoded.NoteValueWei != "5" {
		t.Fatalf("unexpected decrypted opening: %+v", decoded)
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
		RequestID:        "12345678",
		PoolWallet:       "0x0000000000000000000000000000000000000002",
		To:               "0x0000000000000000000000000000000000000001",
		AmountWei:        "0x5",
		RecipientViewKey: "09" + strings.Repeat("00", 31),
		ChangeViewKey:    "09" + strings.Repeat("00", 31),
	}
	_, assignment, err := (&Prover{}).buildDraftEnvelope(req, note, big.NewInt(5), big.NewInt(1), new(big.Int), 0, big.NewInt(1))
	if err != nil {
		t.Fatalf("build draft: %v", err)
	}
	if _, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("build witness: %v", err)
	}
}
