//go:build shield3 && cgo

package shielded4

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func readShield4Words(t *testing.T, path string, count int) []uint64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != count*8 {
		t.Fatalf("%s: got %d bytes, want %d", path, len(data), count*8)
	}
	words := make([]uint64, count)
	for i := range words {
		words[i] = binary.LittleEndian.Uint64(data[i*8:])
	}
	return words
}

func shield4Digest(words []uint64) Digest {
	var digest Digest
	copy(digest[:], words)
	return digest
}

func TestNativeShield4Interoperability(t *testing.T) {
	directory := os.Getenv("TKM_SHIELD4_TESTDATA")
	if directory == "" {
		t.Skip("set TKM_SHIELD4_TESTDATA to a native Shield4 vector directory")
	}
	public := readShield4Words(t, filepath.Join(directory, "public.bin"), 113)
	proofRaw, err := os.ReadFile(filepath.Join(directory, "proof.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(proofRaw)%8 != 0 {
		t.Fatalf("proof has non-word-aligned length %d", len(proofRaw))
	}
	proofWords := readShield4Words(t, filepath.Join(directory, "proof.bin"), len(proofRaw)/8)
	proof := make([]byte, 4+len(proofRaw))
	binary.LittleEndian.PutUint32(proof, uint32(len(proofWords)))
	copy(proof[4:], proofRaw)
	var intent [64]byte
	for i, word := range public[12:28] {
		binary.BigEndian.PutUint32(intent[i*4:], uint32(word))
	}
	var value, sponsor [8]uint32
	for i := range value {
		value[i] = uint32(public[4+i])
		sponsor[i] = uint32(public[59+i])
	}
	statement := Statement{Statement: shielded3.Statement{
		Deposit:     public[58] == 1,
		ChainID:     public[0] | public[1]<<32,
		AssetID:     public[2] | public[3]<<32,
		PublicValue: value,
		GasSponsor:  sponsor,
		Intent:      intent,
		Anchor:      shield4Digest(public[28:33]),
		Nullifier:   shield4Digest(public[33:38]),
		StampRoot:   shield4Digest(public[67:72]),
		InputCount:  uint32(public[87]),
	}, LinkTag: shield4Digest(public[108:113])}
	for i := range statement.Outputs {
		statement.Outputs[i] = shield4Digest(public[38+i*5 : 43+i*5])
		statement.OneTimeKeys[i] = shield4Digest(public[88+i*5 : 93+i*5])
	}
	for i := range statement.AdditionalNullifiers {
		statement.AdditionalNullifiers[i] = shield4Digest(public[72+i*5 : 77+i*5])
	}
	if err := (NativeBackend{}).Verify(context.Background(), statement, proof); err != nil {
		t.Fatalf("native Shield4 proof did not verify: %v", err)
	}
	statement.LinkTag[0] ^= 1
	if err := (NativeBackend{}).Verify(context.Background(), statement, proof); !errors.Is(err, shielded3.ErrInvalidProof) {
		t.Fatalf("tampered Shield4 link tag was accepted: %v", err)
	}
}
