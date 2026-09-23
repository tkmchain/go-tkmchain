// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

// Package shielded3 provides the independent V3 private-spend relation backed
// by Triton VM's native zk-STARK verifier. It is not the V1/V2 Groth16 verifier.
package shielded3

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	MerkleDepth   = 32
	OutputSlots   = 4
	InputSlots    = 4
	fieldModulus  = uint64(0xffffffff00000001)
	publicWords   = 108
	secretWords   = 137
	maxProofWords = 1 << 20
	MaxProofSize  = 4 + maxProofWords*8
	protocolMagic = "TKMS3STK"
)

var (
	ErrInvalidStatement   = errors.New("invalid Shield3 proof statement")
	ErrInvalidWitness     = errors.New("invalid Shield3 spending witness")
	ErrInvalidProof       = errors.New("invalid Shield3 STARK proof")
	ErrBackendUnavailable = errors.New("Shield3 STARK backend unavailable")
	errOutputLimit        = errors.New("STARK backend output exceeds fixed limit")
)

// Digest is the canonical five-word Tip5 digest, without truncation or reduction
// into a legacy BN254 scalar. It has a 40-byte encoding; every word is < p.
// Roots and nullifiers require a separate V3 state tree and storage format.
type Digest [5]uint64

// Amount is an unsigned 256-bit value in eight little-endian u32 limbs.
// Values use the chain's smallest unit; no floating-point conversion is used.
type Amount [8]uint32

func AmountFromBig(value *big.Int) (Amount, error) {
	var result Amount
	if value == nil || value.Sign() < 0 || value.BitLen() > 256 {
		return result, ErrInvalidStatement
	}
	var data [32]byte
	value.FillBytes(data[:])
	for i := range result {
		result[i] = binary.BigEndian.Uint32(data[28-4*i : 32-4*i])
	}
	return result, nil
}

func (a Amount) Big() *big.Int {
	var data [32]byte
	for i, limb := range a {
		binary.BigEndian.PutUint32(data[28-4*i:32-4*i], limb)
	}
	return new(big.Int).SetBytes(data[:])
}

// Statement contains consensus public inputs. Intent must commit to the
// unsigned transaction's complete V3 envelope, including encrypted payloads,
// fees, withdrawals and sponsorship, but excluding the spend proofs. Consensus
// must supply the canonical anchor and reject previously recorded nullifiers.
// Neither sender identity nor private note values are public inputs here.
type Statement struct {
	Deposit              bool
	ChainID              uint64
	AssetID              uint64
	PublicValue          Amount
	GasSponsor           Amount
	Intent               [64]byte
	Anchor               Digest
	Nullifier            Digest
	Outputs              [OutputSlots]Digest
	OneTimeKeys          [OutputSlots]Digest
	StampRoot            Digest
	AdditionalNullifiers [InputSlots - 1]Digest
	InputCount           uint32
}

// OutputOpening is private witness data for one fixed output slot. The owner
// digest is Tip5(DOMAIN_OWNER || spendingSecret); fresh secret randomness hides
// the owner and value in the note commitment, including zero-valued decoys.
type OutputOpening struct {
	Owner      Digest
	Randomness Digest
	Value      Amount
}

type InputOpening struct {
	Randomness Digest
	Value      Amount
	LeafIndex  uint32
	MerklePath [MerkleDepth]Digest
}

type SpendWitness struct {
	AdditionalInputs [InputSlots - 1]InputOpening
	SpendingSecret   Digest
	Randomness       Digest
	Value            Amount
	LeafIndex        uint32
	Outputs          [OutputSlots]OutputOpening
	MerklePath       [MerkleDepth]Digest
	StampIndices     [OutputSlots]uint32
	StampPaths       [OutputSlots][MerkleDepth]Digest
}

// STARKBackend calls the pinned native verifier through a bounded binary
// protocol, passing secret witness data only through stdin. Build it from
// zk/shielded3/stark. This process adapter is for wallet/prover integration;
// production consensus needs an embedded, identically built verifier on every
// supported platform before enabling V3. Missing binaries never accept proofs.
type STARKBackend struct{ executable string }

func NewSTARKBackend(executable string) (*STARKBackend, error) {
	path, err := filepath.Abs(executable)
	if err != nil || executable == "" {
		return nil, ErrBackendUnavailable
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return nil, ErrBackendUnavailable
	}
	return &STARKBackend{executable: path}, nil
}

func (s Statement) words() ([]uint64, error) {
	if s.ChainID == 0 || (!s.Deposit && (s.Anchor == (Digest{}) || s.Nullifier == (Digest{}))) || (s.Deposit && (s.Anchor != (Digest{}) || s.Nullifier != (Digest{}))) {
		return nil, ErrInvalidStatement
	}
	words := []uint64{uint64(uint32(s.ChainID)), s.ChainID >> 32, uint64(uint32(s.AssetID)), s.AssetID >> 32}
	for _, limb := range s.PublicValue {
		words = append(words, uint64(limb))
	}
	for i := 0; i < len(s.Intent); i += 4 {
		words = append(words, uint64(binary.BigEndian.Uint32(s.Intent[i:i+4])))
	}
	words = append(words, s.Anchor[:]...)
	words = append(words, s.Nullifier[:]...)
	for _, output := range s.Outputs {
		words = append(words, output[:]...)
	}
	// Consensus serialization order must match the Rust circuit exactly.
	if s.Deposit {
		words = append(words, 1)
	} else {
		words = append(words, 0)
	}
	for _, limb := range s.GasSponsor {
		words = append(words, uint64(limb))
	}
	words = append(words, s.StampRoot[:]...)
	if s.StampRoot == (Digest{}) || !canonical(words) {
		return nil, ErrInvalidStatement
	}
	count := s.InputCount
	if !s.Deposit && count == 0 {
		count = 1
	}
	if count > InputSlots || s.Deposit != (count == 0) {
		return nil, ErrInvalidStatement
	}
	seen := map[Digest]bool{}
	for i, n := range append([]Digest{s.Nullifier}, s.AdditionalNullifiers[:]...) {
		if uint32(i) < count {
			if n == (Digest{}) || seen[n] {
				return nil, ErrInvalidStatement
			}
			seen[n] = true
		} else if n != (Digest{}) {
			return nil, ErrInvalidStatement
		}
		if i > 0 {
			words = append(words, n[:]...)
		}
	}
	words = append(words, uint64(count))
	for _, key := range s.OneTimeKeys {
		words = append(words, key[:]...)
	}
	if len(words) != publicWords || !canonical(words) {
		return nil, ErrInvalidStatement
	}
	return words, nil
}

func (w SpendWitness) words() ([]uint64, error) {
	if w.Value == (Amount{}) {
		return nil, ErrInvalidWitness
	}
	words := append(make([]uint64, 0, secretWords), w.SpendingSecret[:]...)
	words = append(words, w.Randomness[:]...)
	for _, limb := range w.Value {
		words = append(words, uint64(limb))
	}
	words = append(words, uint64(w.LeafIndex))
	for _, out := range w.Outputs {
		words = append(words, out.Owner[:]...)
		words = append(words, out.Randomness[:]...)
		for _, limb := range out.Value {
			words = append(words, uint64(limb))
		}
	}
	for _, index := range w.StampIndices {
		words = append(words, uint64(index))
	}
	for _, input := range w.AdditionalInputs {
		words = append(words, input.Randomness[:]...)
		for _, limb := range input.Value {
			words = append(words, uint64(limb))
		}
		words = append(words, uint64(input.LeafIndex))
	}
	if len(words) != secretWords || !canonical(words) {
		return nil, ErrInvalidWitness
	}
	for _, digest := range w.allPaths() {
		if !canonical(digest[:]) {
			return nil, ErrInvalidWitness
		}
	}
	return words, nil
}

func canonical(words []uint64) bool {
	for _, word := range words {
		if word >= fieldModulus {
			return false
		}
	}
	return true
}

func appendWords(data []byte, words []uint64) []byte {
	for _, word := range words {
		data = binary.LittleEndian.AppendUint64(data, word)
	}
	return data
}

func validProof(proof []byte) bool {
	if len(proof) < 12 || len(proof) > MaxProofSize {
		return false
	}
	count := binary.LittleEndian.Uint32(proof[:4])
	if count == 0 || count > maxProofWords || uint64(len(proof)) != 4+uint64(count)*8 {
		return false
	}
	for i := 4; i < len(proof); i += 8 {
		if binary.LittleEndian.Uint64(proof[i:i+8]) >= fieldModulus {
			return false
		}
	}
	return true
}

// GenerateSecret samples five uniform canonical field words with crypto/rand.
// Use independent fresh values for spending secrets and each note's randomness.
func GenerateSecret() (Digest, error) {
	var result Digest
	var word [8]byte
	defer clear(word[:])
	for i := range result {
		for {
			if _, err := rand.Read(word[:]); err != nil {
				return Digest{}, err
			}
			value := binary.LittleEndian.Uint64(word[:])
			if value < fieldModulus {
				result[i] = value
				break
			}
		}
	}
	return result, nil
}

type DerivedSpend struct {
	Owner           Digest
	InputCommitment Digest
	Anchor          Digest
	Nullifier       Digest
	Outputs         [OutputSlots]Digest
	OneTimeKeys     [OutputSlots]Digest
}

// Describe computes note commitments and a candidate witness root for wallets.
// It does not attest validity, check the chain's root or mark anything spent.
// A Statement must use the root authenticated by the daemon, not blindly trust
// this helper's candidate root. Verify performs the actual STARK proof check.
func (b *STARKBackend) Describe(ctx context.Context, chainID, assetID uint64, witness SpendWitness) (DerivedSpend, error) {
	var result DerivedSpend
	if chainID == 0 {
		return result, ErrInvalidStatement
	}
	secret, err := witness.words()
	if err != nil {
		return result, err
	}
	defer clear(secret)
	public := make([]uint64, publicWords)
	public[0], public[1] = uint64(uint32(chainID)), chainID>>32
	public[2], public[3] = uint64(uint32(assetID)), assetID>>32
	request := appendWords(append(make([]byte, 0, len(protocolMagic)+(publicWords+secretWords+MerkleDepth*5)*8), protocolMagic...), public)
	request = appendWords(request, secret)
	for _, sibling := range witness.allPaths() {
		request = appendWords(request, sibling[:])
	}
	defer clear(request)
	data, err := b.run(ctx, "describe", request, 40*12)
	if err != nil {
		return result, err
	}
	if len(data) != 40*12 {
		return result, ErrInvalidWitness
	}
	destinations := []*Digest{&result.Owner, &result.InputCommitment, &result.Anchor, &result.Nullifier}
	for i := range result.Outputs {
		// Rust describe_spend returns each commitment followed by its
		// one-time key. Preserve that wire order while decoding.
		destinations = append(destinations, &result.Outputs[i], &result.OneTimeKeys[i])
	}
	for i, digest := range destinations {
		for j := range digest {
			digest[j] = binary.LittleEndian.Uint64(data[(i*5+j)*8 : (i*5+j+1)*8])
		}
		if !canonical(digest[:]) {
			return DerivedSpend{}, ErrInvalidWitness
		}
	}
	return result, nil
}

func (b *STARKBackend) Verify(ctx context.Context, statement Statement, proof []byte) error {
	words, err := statement.words()
	if err != nil {
		return err
	}
	if !validProof(proof) {
		return ErrInvalidProof
	}
	request := appendWords([]byte(protocolMagic), words)
	request = append(request, proof...)
	result, err := b.run(ctx, "verify", request, 3)
	if err != nil {
		return err
	}
	if !bytes.Equal(result, []byte("OK\n")) {
		return ErrInvalidProof
	}
	return nil
}

func (b *STARKBackend) Prove(ctx context.Context, statement Statement, witness SpendWitness) ([]byte, error) {
	public, err := statement.words()
	if err != nil {
		return nil, err
	}
	secret, err := witness.words()
	if err != nil {
		return nil, err
	}
	request := appendWords(append(make([]byte, 0, len(protocolMagic)+(publicWords+secretWords+MerkleDepth*5)*8), protocolMagic...), public)
	request = appendWords(request, secret)
	for _, sibling := range witness.allPaths() {
		request = appendWords(request, sibling[:])
	}
	defer clear(request)
	defer clear(secret)
	result, err := b.run(ctx, "prove", request, MaxProofSize)
	if err != nil {
		return nil, err
	}
	if !validProof(result) {
		return nil, ErrInvalidProof
	}
	return result, nil
}

type boundedOutput struct {
	buffer    bytes.Buffer
	remaining int
}

func (w *boundedOutput) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		return 0, errOutputLimit
	}
	w.remaining -= len(data)
	return w.buffer.Write(data)
}

func (b *STARKBackend) run(ctx context.Context, operation string, input []byte, limit int) ([]byte, error) {
	if b == nil || b.executable == "" {
		return nil, ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, b.executable, operation)
	command.Stdin = bytes.NewReader(input)
	output := &boundedOutput{remaining: limit}
	diagnostic := &boundedOutput{remaining: 4096}
	command.Stdout = output
	command.Stderr = diagnostic
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var rejected *exec.ExitError
		if errors.As(err, &rejected) {
			return nil, ErrInvalidProof
		}
		return nil, fmt.Errorf("%w: %v", ErrBackendUnavailable, err)
	}
	// Backend diagnostics and private witness data are never propagated to
	// callers or logs. Only explicitly verified output constitutes success.
	return output.buffer.Bytes(), nil
}

func (w SpendWitness) allPaths() []Digest {
	paths := append(make([]Digest, 0, MerkleDepth*(InputSlots+OutputSlots)), w.MerklePath[:]...)
	for _, input := range w.AdditionalInputs {
		paths = append(paths, input.MerklePath[:]...)
	}
	for _, path := range w.StampPaths {
		paths = append(paths, path[:]...)
	}
	return paths
}
func StampLeaf(chainID uint64, owner Digest) (Digest, error) {
	words := []uint64{3004, uint64(uint32(chainID)), chainID >> 32}
	return HashWords(append(words, owner[:]...))
}
