package shielded3

import (
	"bytes"
	"context"
	"encoding/binary"
)

// Shield5 uses an independently versioned, bounded statement and witness.
// The original V3/V4 layouts and proof programs remain unchanged.
const (
	Shield5InputSlots  = 16
	shield5PublicWords = 173
	shield5SecretWords = 305
	shield5PathDigests = MerkleDepth * (Shield5InputSlots + OutputSlots)
	shield5Magic       = "TKMS5STK"
)

// StatementV5 keeps the first four input nullifiers in the frozen V4 layout
// and appends twelve more nullifiers after the V4 link tag.
type StatementV5 struct {
	Statement
	LinkTag                Digest
	AdditionalNullifiersV5 [Shield5InputSlots - InputSlots]Digest
}

func (s StatementV5) words() ([]uint64, error) {
	count := s.InputCount
	if !s.Deposit && count == 0 {
		count = 1
	}
	if count > Shield5InputSlots || s.Deposit != (count == 0) {
		return nil, ErrInvalidStatement
	}
	base := s.Statement
	base.InputCount = count
	if base.InputCount > InputSlots {
		base.InputCount = InputSlots
	}
	v4, err := (StatementV4{Statement: base, LinkTag: s.LinkTag}).words()
	if err != nil {
		return nil, err
	}
	v4[87] = uint64(count)
	words := append(v4, make([]uint64, 0, (Shield5InputSlots-InputSlots)*5)...)
	seen := make(map[Digest]bool, count)
	nullifiers := append([]Digest{s.Nullifier}, s.AdditionalNullifiers[:]...)
	nullifiers = append(nullifiers, s.AdditionalNullifiersV5[:]...)
	for i, n := range nullifiers {
		if uint32(i) < count {
			if n == (Digest{}) || seen[n] || !canonical(n[:]) {
				return nil, ErrInvalidStatement
			}
			seen[n] = true
		} else if n != (Digest{}) {
			return nil, ErrInvalidStatement
		}
		if i >= InputSlots {
			words = append(words, n[:]...)
		}
	}
	if len(words) != shield5PublicWords || !canonical(words) {
		return nil, ErrInvalidStatement
	}
	return words, nil
}

// SpendWitnessV5 adds twelve fixed input openings. Keeping a fixed maximum
// makes both the native ABI and consensus verifier resource bounds explicit.
type SpendWitnessV5 struct {
	SpendWitness
	InputCount         uint64
	AdditionalInputsV5 [Shield5InputSlots - InputSlots]InputOpening
}

func (w SpendWitnessV5) words() ([]uint64, error) {
	count := w.InputCount
	if count > Shield5InputSlots {
		return nil, ErrInvalidWitness
	}
	words, err := w.SpendWitness.words()
	if err != nil {
		return nil, err
	}
	for i, input := range w.AdditionalInputs {
		active := uint64(i+1) < count
		if active && input.Value == (Amount{}) {
			return nil, ErrInvalidWitness
		}
		if !active && input != (InputOpening{}) {
			return nil, ErrInvalidWitness
		}
	}
	for i, input := range w.AdditionalInputsV5 {
		active := uint64(InputSlots+i) < count
		if active {
			if input.Value == (Amount{}) || !canonical(input.Randomness[:]) {
				return nil, ErrInvalidWitness
			}
		} else if input != (InputOpening{}) {
			return nil, ErrInvalidWitness
		}
		words = append(words, input.Randomness[:]...)
		for _, limb := range input.Value {
			words = append(words, uint64(limb))
		}
		words = append(words, uint64(input.LeafIndex))
	}
	if len(words) != shield5SecretWords || !canonical(words) {
		return nil, ErrInvalidWitness
	}
	return words, nil
}

func (w SpendWitnessV5) allPaths() [][5]uint64 {
	paths := make([][5]uint64, 0, shield5PathDigests)
	for _, d := range w.MerklePath {
		paths = append(paths, d)
	}
	for _, input := range w.AdditionalInputs {
		for _, d := range input.MerklePath {
			paths = append(paths, d)
		}
	}
	for _, input := range w.AdditionalInputsV5 {
		for _, d := range input.MerklePath {
			paths = append(paths, d)
		}
	}
	for _, path := range w.StampPaths {
		for _, d := range path {
			paths = append(paths, d)
		}
	}
	return paths
}

// NativeBackendV5 verifies/proves the separate 16-input claim program.
type NativeBackendV5 struct{}

func shield5Request(s StatementV5, w SpendWitnessV5, describe bool) ([]byte, error) {
	if describe && !s.Deposit && s.LinkTag == (Digest{}) {
		s.LinkTag[0] = 1 // temporary canonical tag; omitted from describe public input below
	}
	public, err := s.words()
	if err != nil {
		return nil, err
	}
	if describe {
		public[108], public[109], public[110], public[111], public[112] = 0, 0, 0, 0, 0
	}
	secret, err := w.words()
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	request := append([]byte(shield5Magic), make([]byte, 0, (shield5PublicWords+shield5SecretWords+shield5PathDigests*5)*8)...)
	for _, word := range public {
		request = binary.LittleEndian.AppendUint64(request, word)
	}
	for _, word := range secret {
		request = binary.LittleEndian.AppendUint64(request, word)
	}
	for _, digest := range w.allPaths() {
		for _, word := range digest {
			request = binary.LittleEndian.AppendUint64(request, word)
		}
	}
	return request, nil
}

func (NativeBackendV5) Verify(ctx context.Context, s StatementV5, proof []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	public, err := s.words()
	if err != nil {
		return err
	}
	if !validProof(proof) {
		return ErrInvalidProof
	}
	request := appendWords([]byte(shield5Magic), public)
	request = append(request, proof...)
	out, err := nativeCall(13, request, 3)
	if err != nil {
		return err
	}
	if !bytes.Equal(out, []byte("OK\n")) {
		return ErrInvalidProof
	}
	return nil
}

func (NativeBackendV5) Prove(ctx context.Context, s StatementV5, w SpendWitnessV5) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := shield5Request(s, w, false)
	if err != nil {
		return nil, err
	}
	defer clear(request)
	proof, err := nativeCall(14, request, MaxProofSize)
	if err != nil {
		return nil, err
	}
	if !validProof(proof) {
		return nil, ErrInvalidProof
	}
	return proof, nil
}

func (NativeBackendV5) Describe(ctx context.Context, s StatementV5, w SpendWitnessV5) (DerivedSpend, error) {
	var result DerivedSpend
	if err := ctx.Err(); err != nil {
		return result, err
	}
	request, err := shield5Request(s, w, true)
	if err != nil {
		return result, err
	}
	defer clear(request)
	data, err := nativeCall(15, request, 520)
	if err != nil {
		return result, err
	}
	if len(data) != 520 {
		return result, ErrInvalidWitness
	}
	dest := []*Digest{&result.Owner, &result.InputCommitment, &result.Anchor, &result.Nullifier, &result.LinkTag}
	for i := range result.Outputs {
		dest = append(dest, &result.Outputs[i], &result.OneTimeKeys[i])
	}
	for i, digest := range dest {
		for j := range digest {
			digest[j] = binary.LittleEndian.Uint64(data[(i*40 + j*8):(i*40 + (j+1)*8)])
		}
		if !canonical(digest[:]) {
			return DerivedSpend{}, ErrInvalidWitness
		}
	}
	return result, nil
}
