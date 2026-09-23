// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package shielded3

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAmountExact256Bit(t *testing.T) {
	for _, v := range []*big.Int{new(big.Int), big.NewInt(5), new(big.Int).Exp(big.NewInt(10), big.NewInt(21), nil), new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))} {
		amount, err := AmountFromBig(v)
		if err != nil || amount.Big().Cmp(v) != 0 {
			t.Fatalf("exact amount round trip: %v", err)
		}
	}
	for _, v := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 256)} {
		if _, err := AmountFromBig(v); err != ErrInvalidStatement {
			t.Fatalf("amount overflow error: %v", err)
		}
	}
}

func TestSTARKFailClosed(t *testing.T) {
	if _, err := NewSTARKBackend(filepath.Join(t.TempDir(), "missing")); err != ErrBackendUnavailable {
		t.Fatalf("missing backend: %v", err)
	}
	s := Statement{ChainID: 8979, Anchor: Digest{1}, Nullifier: Digest{2}, StampRoot: Digest{3}}
	var backend *STARKBackend
	if err := backend.Verify(context.Background(), s, nil); err != ErrInvalidProof {
		t.Fatalf("empty proof: %v", err)
	}
	proof := binary.LittleEndian.AppendUint32(nil, 1)
	proof = binary.LittleEndian.AppendUint64(proof, 1)
	if err := backend.Verify(context.Background(), s, proof); err != ErrBackendUnavailable {
		t.Fatalf("unavailable verifier accepted proof: %v", err)
	}
	s.Nullifier = Digest{}
	if err := backend.Verify(context.Background(), s, proof); err != ErrInvalidStatement {
		t.Fatalf("zero nullifier: %v", err)
	}
	s.Nullifier = Digest{fieldModulus}
	if err := backend.Verify(context.Background(), s, proof); err != ErrInvalidStatement {
		t.Fatalf("noncanonical nullifier: %v", err)
	}
}

// Run against the actual compiled Rust verifier and a genuinely generated
// proof, never a mocked acceptance command. The README gives the commands.
func TestNativeSTARKInteroperability(t *testing.T) {
	executable, directory := os.Getenv("TKM_SHIELD3_STARK_BIN"), os.Getenv("TKM_SHIELD3_TESTDATA")
	if executable == "" || directory == "" {
		t.Skip("build and run native STARK tests to provide real proof vectors")
	}
	backend, err := NewSTARKBackend(executable)
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string, size int) []uint64 {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(directory, name+".bin"))
		if err != nil {
			t.Fatal(err)
		}
		if len(data)%8 != 0 || (size > 0 && len(data) != size*8) {
			t.Fatalf("invalid native %s fixture", name)
		}
		words := make([]uint64, len(data)/8)
		for i := range words {
			words[i] = binary.LittleEndian.Uint64(data[i*8 : i*8+8])
		}
		return words
	}
	public, secret, path, proofWords := read("public", publicWords), read("secret", secretWords), read("path", MerkleDepth*5*(InputSlots+OutputSlots)), read("proof", 0)
	s := Statement{ChainID: public[0] | public[1]<<32, AssetID: public[2] | public[3]<<32}
	for i := range s.PublicValue {
		s.PublicValue[i] = uint32(public[4+i])
	}
	for i := 0; i < 16; i++ {
		binary.BigEndian.PutUint32(s.Intent[i*4:i*4+4], uint32(public[12+i]))
	}
	copy(s.Anchor[:], public[28:33])
	copy(s.Nullifier[:], public[33:38])
	for i := range s.Outputs {
		copy(s.Outputs[i][:], public[38+5*i:43+5*i])
		copy(s.OneTimeKeys[i][:], public[88+5*i:93+5*i])
	}
	s.Deposit = public[58] == 1
	for i := range s.GasSponsor {
		s.GasSponsor[i] = uint32(public[59+i])
	}
	w := SpendWitness{LeafIndex: uint32(secret[18])}
	copy(w.SpendingSecret[:], secret[:5])
	copy(w.Randomness[:], secret[5:10])
	for i := range w.Value {
		w.Value[i] = uint32(secret[10+i])
	}
	for i := range w.Outputs {
		base := 19 + 18*i
		copy(w.Outputs[i].Owner[:], secret[base:base+5])
		copy(w.Outputs[i].Randomness[:], secret[base+5:base+10])
		for j := range w.Outputs[i].Value {
			w.Outputs[i].Value[j] = uint32(secret[base+10+j])
		}
	}
	for i := range w.MerklePath {
		copy(w.MerklePath[i][:], path[i*5:i*5+5])
	}
	copy(s.StampRoot[:], public[67:72])
	for i := range w.AdditionalInputs {
		base := 95 + 14*i
		copy(w.AdditionalInputs[i].Randomness[:], secret[base:base+5])
		for j := range w.AdditionalInputs[i].Value {
			w.AdditionalInputs[i].Value[j] = uint32(secret[base+5+j])
		}
		w.AdditionalInputs[i].LeafIndex = uint32(secret[base+13])
		for j := range w.AdditionalInputs[i].MerklePath {
			copy(w.AdditionalInputs[i].MerklePath[j][:], path[(MerkleDepth*(i+1)+j)*5:(MerkleDepth*(i+1)+j+1)*5])
		}
	}
	s.InputCount = uint32(public[87])
	for i := range s.AdditionalNullifiers {
		copy(s.AdditionalNullifiers[i][:], public[72+5*i:77+5*i])
	}

	for i := range w.StampIndices {
		w.StampIndices[i] = uint32(secret[91+i])
		for j := range w.StampPaths[i] {
			copy(w.StampPaths[i][j][:], path[(MerkleDepth*(i+InputSlots)+j)*5:(MerkleDepth*(i+InputSlots)+j+1)*5])
		}
	}
	serialized, serializeErr := w.words()
	if serializeErr != nil {
		t.Fatalf("wallet witness serialization: %v", serializeErr)
	}
	for i := range serialized {
		if serialized[i] != secret[i] {
			t.Fatalf("wallet witness word %d mismatch serialized=%d fixture=%d", i, serialized[i], secret[i])
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	derived, err := backend.Describe(ctx, s.ChainID, s.AssetID, w)
	if err != nil || derived.Anchor != s.Anchor || derived.Nullifier != s.Nullifier || derived.Outputs != s.Outputs || derived.OneTimeKeys != s.OneTimeKeys {
		t.Fatalf("wallet commitment interoperability: %v", err)
	}
	proof := appendWords(binary.LittleEndian.AppendUint32(nil, uint32(len(proofWords))), proofWords)
	if err := backend.Verify(ctx, s, proof); err != nil {
		t.Fatalf("actual native STARK proof: %v", err)
	}
	// Generate through the Go interface as well as verifying the Rust fixture.
	generated, err := backend.Prove(ctx, s, w)
	if err != nil {
		t.Fatalf("Go to native proving: %v", err)
	}
	if bytes.Equal(generated, proof) {
		t.Fatal("native prover reused deterministic proof randomness")
	}
	if err := backend.Verify(ctx, s, generated); err != nil {
		t.Fatalf("Go to native verification: %v", err)
	}
	changed := s
	changed.Intent[0] ^= 1
	if err := backend.Verify(ctx, changed, proof); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("transaction replay: %v", err)
	}
	changed = s
	changed.Outputs[0][0] ^= 1
	if err := backend.Verify(ctx, changed, proof); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("output substitution: %v", err)
	}
	malformed := bytes.Clone(proof)
	malformed[len(malformed)/2] ^= 1
	if err := backend.Verify(ctx, s, malformed); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("proof tampering: %v", err)
	}
	w.SpendingSecret[0] ^= 1
	if _, err := backend.Prove(ctx, s, w); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("wrong owner accepted: %v", err)
	}
	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if err := backend.Verify(cancelled, s, proof); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled verifier accepted: %v", err)
	}
	clear(secret)
	clear(path)
}

// io.Copy must not discover a promoted bytes.Buffer.ReadFrom that bypasses
// Write's output bound. A non-WriterTo source exercises this exact path.
func TestNativeOutputBoundCannotBeBypassed(t *testing.T) {
	output := &boundedOutput{remaining: 8}
	source := io.LimitReader(bytes.NewReader(make([]byte, 32)), 32)
	if _, err := io.Copy(output, source); !errors.Is(err, errOutputLimit) {
		t.Fatalf("output limit bypassed: %v", err)
	}
	if output.buffer.Len() != 0 {
		t.Fatal("oversized native output was buffered")
	}
}
