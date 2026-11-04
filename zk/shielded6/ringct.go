// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// Shield6's classical RingCT cryptographic backend. Transaction policy and
// consensus state transitions live in core; this package owns strict wire
// decoding and the CLSAG/Bulletproofs+ primitives used by both consensus and
// the wallet.
package shielded6

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct/bulletproofs/plus"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct/clsag"
	"git.gammaspectra.live/P2Pool/consensus/v5/types"
	"git.gammaspectra.live/P2Pool/consensus/v5/utils"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	// RingSize is a consensus constant. The ring membership list is never
	// wallet-selectable in length, which bounds verifier work and wire size.
	RingSize                       = 16
	PointSize                      = curve25519.PublicKeySize
	ScalarSize                     = curve25519.PrivateKeySize
	AssetTKM                uint64 = 1
	CLSAGSize                      = RingSize*ScalarSize + ScalarSize + PointSize
	MaxRangeOutputs                = 16
	MaxRangeProofSize              = 8192 // maximum encoded size of one 64-bit limb proof
	MaxRangeProofBundleSize        = 2*MaxRangeProofSize + 16
)

var (
	ErrInvalidPoint      = errors.New("shield6: invalid point encoding")
	ErrInvalidRing       = errors.New("shield6: ring must contain exactly 16 valid members")
	ErrInvalidSignature  = errors.New("shield6: malformed CLSAG signature")
	ErrInvalidRangeProof = errors.New("shield6: malformed or invalid Bulletproofs+ range proof")
	ErrUnbalanced        = errors.New("shield6: commitment balance equation does not hold")
)

// RingMember is one eligible output in a CLSAG ring. Both points are encoded
// canonically as compressed Edwards25519 points.
type RingMember struct {
	OneTimeKey [PointSize]byte
	Commitment [PointSize]byte
}

// WideCommitment represents a 128-bit atomic amount using two 64-bit range
// proof limbs and one weighted aggregate Pedersen commitment.
type WideCommitment struct {
	Commitment [PointSize]byte
	Limbs      [2][PointSize]byte
}

// WideWitness is secret wallet data corresponding to a wide commitment.
// Mask is the total blinding scalar; LimbMasks produce the two range-proof
// commitments. Callers must protect and zeroize these values like spend keys.
type WideWitness struct {
	Mask      [ScalarSize]byte
	LimbMasks [2][ScalarSize]byte
}

// VerifyCLSAG verifies a canonical fixed-size signature against exactly one
// output ring and a pseudo-output commitment. message must be the TKM
// domain-separated transaction intent hash, not a hash supplied independently
// of the transaction's public fields.
func VerifyCLSAG(message [32]byte, ring []RingMember, keyImage, pseudoOutput [PointSize]byte, encoded []byte) error {
	if len(ring) != RingSize {
		return ErrInvalidRing
	}
	if len(encoded) != CLSAGSize {
		return ErrInvalidSignature
	}
	var members ringct.CommitmentRing[curve25519.VarTimeOperations]
	members = make(ringct.CommitmentRing[curve25519.VarTimeOperations], RingSize)
	seenKeys := make(map[[PointSize]byte]struct{}, RingSize)
	for i, member := range ring {
		if _, ok := seenKeys[member.OneTimeKey]; ok {
			return ErrInvalidRing
		}
		seenKeys[member.OneTimeKey] = struct{}{}
		key, err := decodePoint(member.OneTimeKey[:], false)
		if err != nil {
			return fmt.Errorf("ring member %d key: %w", i, err)
		}
		commitment, err := decodePoint(member.Commitment[:], true)
		if err != nil {
			return fmt.Errorf("ring member %d commitment: %w", i, err)
		}
		members[i] = [2]curve25519.PublicKey[curve25519.VarTimeOperations]{*key, *commitment}
	}
	image, err := decodePoint(keyImage[:], false)
	if err != nil {
		return fmt.Errorf("key image: %w", err)
	}
	pseudo, err := decodePoint(pseudoOutput[:], false)
	if err != nil {
		return fmt.Errorf("pseudo-output: %w", err)
	}
	var signature clsag.Signature[curve25519.VarTimeOperations]
	reader := bytes.NewReader(encoded)
	if err := signature.FromReader(reader, RingSize); err != nil || reader.Len() != 0 {
		return ErrInvalidSignature
	}
	if err := signature.Verify(types.Hash(message), members, image, pseudo); err != nil {
		return fmt.Errorf("shield6 CLSAG verification failed: %w", err)
	}
	return nil
}

// VerifyRangeProof verifies one aggregate 64-bit Bulletproofs+ proof for the
// complete set of output commitments. The proof parser is bounded and rejects
// trailing bytes before any expensive curve operations are performed.
func VerifyRangeProof(commitments [][PointSize]byte, encoded []byte) error {
	if len(commitments) == 0 || len(commitments) > MaxRangeOutputs || len(encoded) == 0 || len(encoded) > MaxRangeProofSize {
		return ErrInvalidRangeProof
	}
	points := make([]curve25519.PublicKey[curve25519.VarTimeOperations], len(commitments))
	for i := range commitments {
		point, err := decodePoint(commitments[i][:], false)
		if err != nil {
			return fmt.Errorf("range commitment %d: %w", i, err)
		}
		points[i] = *point
	}
	var proof plus.AggregateRangeProof[curve25519.VarTimeOperations]
	reader := bytes.NewReader(encoded)
	if err := proof.FromReader(utils.ReaderAndByteReader(reader)); err != nil || reader.Len() != 0 {
		return ErrInvalidRangeProof
	}
	if !proof.Verify(points, newRangeProofVerifierReader(commitments, encoded)) {
		return ErrInvalidRangeProof
	}
	return nil
}

// rangeProofVerifierReader expands a transcript-derived seed for the
// Bulletproofs+ verifier's internal batch-check coefficient. The upstream
// verifier takes an io.Reader because it is also used for non-consensus
// batch-verification contexts. Consensus must not feed crypto/rand.Reader:
// independent nodes could then accept/reject the same proof differently.
// Deriving this coefficient stream from the full public statement and proof
// makes validation deterministic while retaining a 256-bit hash-derived
// coefficient for the internal equation batching.
type rangeProofVerifierReader struct {
	seed    types.Hash
	counter uint64
	block   [PointSize]byte
	used    int
}

func newRangeProofVerifierReader(commitments [][PointSize]byte, proof []byte) *rangeProofVerifierReader {
	parts := make([][]byte, 0, len(commitments)+3)
	parts = append(parts, []byte("TKM_SHIELD6_BULLETPROOF_PLUS_BATCH_V1"))
	var commitmentCount [8]byte
	binary.BigEndian.PutUint64(commitmentCount[:], uint64(len(commitments)))
	parts = append(parts, commitmentCount[:])
	for i := range commitments {
		parts = append(parts, commitments[i][:])
	}
	var proofLength [8]byte
	binary.BigEndian.PutUint64(proofLength[:], uint64(len(proof)))
	parts = append(parts, proofLength[:], proof)
	return &rangeProofVerifierReader{seed: crypto.Keccak256Var(parts...), used: PointSize}
}

func (r *rangeProofVerifierReader) Read(out []byte) (int, error) {
	written := 0
	for written < len(out) {
		if r.used == len(r.block) {
			var counter [8]byte
			binary.BigEndian.PutUint64(counter[:], r.counter)
			r.block = crypto.Keccak256Var(
				[]byte("TKM_SHIELD6_BULLETPROOF_PLUS_BATCH_SCALAR_V1"),
				r.seed[:], counter[:],
			)
			r.counter++
			r.used = 0
		}
		n := copy(out[written:], r.block[r.used:])
		written += n
		r.used += n
	}
	return written, nil
}

// VerifyCommitmentBalance checks sum(pseudoOutputs) = sum(outputs) + fee*H.
// Together with verified CLSAGs and range proofs, this is the commitment
// equation that prevents an ordinary private transfer from creating value.
func VerifyCommitmentBalance(pseudoOutputs, outputs [][PointSize]byte, fee uint64) error {
	return VerifyCommitmentBalance128(pseudoOutputs, outputs, new(big.Int).SetUint64(fee))
}

// VerifyCommitmentBalance128 checks value conservation with a public amount
// exit/deposit component. For a withdrawal, publicValue is added on the output
// side. For a deposit, callers must use VerifyDepositCommitment because there
// are no pseudo-outputs and the public deposit mask is disclosed separately.
func VerifyCommitmentBalance128(pseudoOutputs, outputs [][PointSize]byte, publicValue *big.Int) error {
	if len(pseudoOutputs) == 0 || len(pseudoOutputs) > MaxRangeOutputs || len(outputs) > MaxRangeOutputs || (len(outputs) == 0 && (publicValue == nil || publicValue.Sign() == 0)) {
		return ErrUnbalanced
	}
	if publicValue == nil || publicValue.Sign() < 0 || publicValue.BitLen() > 128 {
		return ErrUnbalanced
	}
	var lhs, rhs curve25519.PublicKey[curve25519.VarTimeOperations]
	lhs.Identity()
	rhs.Identity()
	for i := range pseudoOutputs {
		point, err := decodePoint(pseudoOutputs[i][:], false)
		if err != nil {
			return fmt.Errorf("pseudo-output %d: %w", i, err)
		}
		lhs.Add(&lhs, point)
	}
	for i := range outputs {
		point, err := decodePoint(outputs[i][:], false)
		if err != nil {
			return fmt.Errorf("output commitment %d: %w", i, err)
		}
		rhs.Add(&rhs, point)
	}
	if publicValue.Sign() > 0 {
		var zeroMask [ScalarSize]byte
		publicCommitment, err := Commitment128(publicValue, zeroMask)
		if err != nil {
			return ErrUnbalanced
		}
		point, err := decodePoint(publicCommitment[:], false)
		if err != nil {
			return ErrUnbalanced
		}
		rhs.Add(&rhs, point)
	}
	if lhs.Equal(&rhs) != 1 {
		return ErrUnbalanced
	}
	return nil
}

// VerifyDepositCommitment verifies that a set of ranged outputs is backed by
// the public deposit amount and the disclosed aggregate blinding mask.
func VerifyDepositCommitment(outputs [][PointSize]byte, amount *big.Int, aggregateMask [ScalarSize]byte) error {
	if len(outputs) == 0 || len(outputs) > MaxRangeOutputs || amount == nil || amount.Sign() <= 0 || amount.BitLen() > 128 {
		return ErrUnbalanced
	}
	var sum curve25519.PublicKey[curve25519.VarTimeOperations]
	sum.Identity()
	for i, encoded := range outputs {
		point, err := decodePoint(encoded[:], false)
		if err != nil {
			return fmt.Errorf("output commitment %d: %w", i, err)
		}
		sum.Add(&sum, point)
	}
	deposit, err := Commitment128(amount, aggregateMask)
	if err != nil {
		return ErrUnbalanced
	}
	expected, err := decodePoint(deposit[:], false)
	if err != nil || sum.Equal(expected) != 1 {
		return ErrUnbalanced
	}
	return nil
}

// ProveRangeProof creates a real aggregate Bulletproofs+ range proof for the
// supplied value/mask witnesses. Wallet code supplies fresh, cryptographically
// random masks. This function never selects or reuses masks itself.
func ProveRangeProof(witness []ringct.LazyCommitment, random io.Reader) ([][PointSize]byte, []byte, error) {
	if len(witness) == 0 || len(witness) > MaxRangeOutputs || random == nil {
		return nil, nil, ErrInvalidRangeProof
	}
	points := make([]curve25519.PublicKey[curve25519.ConstantTimeOperations], len(witness))
	for i, value := range witness {
		points[i] = *ringct.CalculateCommitment(new(curve25519.PublicKey[curve25519.ConstantTimeOperations]), value)
	}
	statement := plus.AggregateRangeStatement[curve25519.ConstantTimeOperations]{V: points}
	proof, err := statement.Prove(plus.AggregateRangeWitness(witness), random)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := proof.AppendBinary(make([]byte, 0, proof.BufferLength(false)), false)
	if err != nil {
		return nil, nil, err
	}
	commitments := make([][PointSize]byte, len(points))
	for i := range points {
		copy(commitments[i][:], points[i].Bytes())
	}
	return commitments, encoded, nil
}

// ProveRangeProof128 proves each 128-bit amount with two 64-bit Bulletproofs+
// aggregates. The two proofs are RLP encoded as one canonical bundle. Keeping
// each aggregate at or below MaxRangeOutputs matches the pinned backend's
// 1024-generator limit and avoids an out-of-bounds panic.
func ProveRangeProof128(amounts []*big.Int, random io.Reader) ([]WideCommitment, []byte, error) {
	outputs, proof, witnesses, err := ProveRangeProof128WithWitness(amounts, random)
	clearWideWitnesses(witnesses)
	return outputs, proof, err
}

func clearWideWitnesses(witnesses []WideWitness) {
	for i := range witnesses {
		clear(witnesses[i].Mask[:])
		clear(witnesses[i].LimbMasks[0][:])
		clear(witnesses[i].LimbMasks[1][:])
	}
}

// ProveRangeProof128WithWitness returns the aggregate amount masks needed by
// the transaction signer and the individual masks needed to recover notes.
func ProveRangeProof128WithWitness(amounts []*big.Int, random io.Reader) ([]WideCommitment, []byte, []WideWitness, error) {
	if len(amounts) == 0 || len(amounts) > MaxRangeOutputs || random == nil {
		return nil, nil, nil, ErrInvalidRangeProof
	}
	lowWitness := make([]ringct.LazyCommitment, len(amounts))
	highWitness := make([]ringct.LazyCommitment, len(amounts))
	secrets := make([]WideWitness, len(amounts))
	retainSecrets := false
	defer func() {
		for i := range lowWitness {
			lowWitness[i].Mask.Set(new(curve25519.Scalar))
			highWitness[i].Mask.Set(new(curve25519.Scalar))
		}
		if !retainSecrets {
			clearWideWitnesses(secrets)
		}
	}()
	for _, amount := range amounts {
		if amount == nil || amount.Sign() < 0 || amount.BitLen() > 128 {
			return nil, nil, nil, ErrInvalidRangeProof
		}
	}
	for i, amount := range amounts {
		var amountBytes [16]byte
		amount.FillBytes(amountBytes[:])
		high := binary.BigEndian.Uint64(amountBytes[:8])
		low := binary.BigEndian.Uint64(amountBytes[8:])
		clear(amountBytes[:])
		var lowMask, highMask curve25519.Scalar
		defer lowMask.Set(new(curve25519.Scalar))
		defer highMask.Set(new(curve25519.Scalar))
		curve25519.RandomScalar(&lowMask, random)
		curve25519.RandomScalar(&highMask, random)
		copy(secrets[i].LimbMasks[0][:], lowMask.Bytes())
		copy(secrets[i].LimbMasks[1][:], highMask.Bytes())
		weightedHigh := new(curve25519.Scalar).Multiply(two64Scalar(), &highMask)
		totalMask := new(curve25519.Scalar).Add(&lowMask, weightedHigh)
		defer weightedHigh.Set(new(curve25519.Scalar))
		defer totalMask.Set(new(curve25519.Scalar))
		copy(secrets[i].Mask[:], totalMask.Bytes())
		lowWitness[i] = ringct.LazyCommitment{Mask: lowMask, Amount: low}
		highWitness[i] = ringct.LazyCommitment{Mask: highMask, Amount: high}
	}
	lowCommitments, lowProof, err := ProveRangeProof(lowWitness, random)
	if err != nil {
		return nil, nil, nil, err
	}
	highCommitments, highProof, err := ProveRangeProof(highWitness, random)
	if err != nil {
		return nil, nil, nil, err
	}
	proof, err := encodeRangeProofBundle(lowProof, highProof)
	if err != nil {
		return nil, nil, nil, err
	}
	outputs := make([]WideCommitment, len(amounts))
	for i := range outputs {
		copy(outputs[i].Limbs[0][:], lowCommitments[i][:])
		copy(outputs[i].Limbs[1][:], highCommitments[i][:])
		outputs[i].Commitment, err = CombineAmountLimbs(outputs[i].Limbs[0], outputs[i].Limbs[1])
		if err != nil {
			return nil, nil, nil, err
		}
	}
	retainSecrets = true
	return outputs, proof, secrets, nil
}

// VerifyRangeProof128 verifies each output's two-limb commitment relation and
// the paired low/high aggregate Bulletproofs+ proofs in the canonical bundle.
func VerifyRangeProof128(outputs []WideCommitment, encoded []byte) error {
	if len(outputs) == 0 || len(outputs) > MaxRangeOutputs {
		return ErrInvalidRangeProof
	}
	lowLimbs := make([][PointSize]byte, 0, len(outputs))
	highLimbs := make([][PointSize]byte, 0, len(outputs))
	for i, output := range outputs {
		combined, err := CombineAmountLimbs(output.Limbs[0], output.Limbs[1])
		if err != nil || !bytes.Equal(combined[:], output.Commitment[:]) {
			return fmt.Errorf("output %d: %w", i, ErrUnbalanced)
		}
		lowLimbs = append(lowLimbs, output.Limbs[0])
		highLimbs = append(highLimbs, output.Limbs[1])
	}
	lowProof, highProof, err := decodeRangeProofBundle(encoded)
	if err != nil {
		return err
	}
	if err := VerifyRangeProof(lowLimbs, lowProof); err != nil {
		return fmt.Errorf("low amount limbs: %w", err)
	}
	if err := VerifyRangeProof(highLimbs, highProof); err != nil {
		return fmt.Errorf("high amount limbs: %w", err)
	}
	return nil
}

func encodeRangeProofBundle(low, high []byte) ([]byte, error) {
	if len(low) == 0 || len(low) > MaxRangeProofSize || len(high) == 0 || len(high) > MaxRangeProofSize {
		return nil, ErrInvalidRangeProof
	}
	encoded, err := rlp.EncodeToBytes([][]byte{low, high})
	if err != nil || len(encoded) > MaxRangeProofBundleSize {
		return nil, ErrInvalidRangeProof
	}
	return encoded, nil
}

func decodeRangeProofBundle(encoded []byte) ([]byte, []byte, error) {
	if len(encoded) == 0 || len(encoded) > MaxRangeProofBundleSize {
		return nil, nil, ErrInvalidRangeProof
	}
	var proofs [][]byte
	if err := rlp.DecodeBytes(encoded, &proofs); err != nil || len(proofs) != 2 || len(proofs[0]) == 0 || len(proofs[0]) > MaxRangeProofSize || len(proofs[1]) == 0 || len(proofs[1]) > MaxRangeProofSize {
		return nil, nil, ErrInvalidRangeProof
	}
	canonical, err := rlp.EncodeToBytes(proofs)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return nil, nil, ErrInvalidRangeProof
	}
	return proofs[0], proofs[1], nil
}

// CombineAmountLimbs computes C_low + 2^64*C_high in the Edwards group.
func CombineAmountLimbs(low, high [PointSize]byte) ([PointSize]byte, error) {
	var result [PointSize]byte
	lowPoint, err := decodePoint(low[:], true)
	if err != nil {
		return result, err
	}
	highPoint, err := decodePoint(high[:], true)
	if err != nil {
		return result, err
	}
	two64 := two64Scalar()
	weightedHigh := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarMult(two64, highPoint)
	combined := new(curve25519.PublicKey[curve25519.VarTimeOperations]).Add(lowPoint, weightedHigh)
	copy(result[:], combined.Bytes())
	return result, nil
}

func two64Scalar() *curve25519.Scalar {
	var encoded [ScalarSize]byte
	encoded[8] = 1
	scalar, _ := new(curve25519.Scalar).SetCanonicalBytes(encoded[:])
	return scalar
}

// Commitment128 builds mask*G + amount*H for a nonnegative 128-bit amount.
func Commitment128(amount *big.Int, mask [ScalarSize]byte) ([PointSize]byte, error) {
	var result [PointSize]byte
	if amount == nil || amount.Sign() < 0 || amount.BitLen() > 128 {
		return result, ErrUnbalanced
	}
	maskScalar, err := decodeScalar(mask[:])
	if err != nil {
		return result, err
	}
	defer maskScalar.Set(new(curve25519.Scalar))
	var encoded [ScalarSize]byte
	defer clear(encoded[:])
	be := amount.FillBytes(make([]byte, ScalarSize))
	defer clear(be)
	for i := range encoded {
		encoded[i] = be[len(be)-1-i]
	}
	amountScalar, err := new(curve25519.Scalar).SetCanonicalBytes(encoded[:])
	if err != nil {
		return result, err
	}
	defer amountScalar.Set(new(curve25519.Scalar))
	maskPoint := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(maskScalar)
	amountPoint := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMultPrecomputed(amountScalar, crypto.GeneratorH)
	commitment := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).Add(maskPoint, amountPoint)
	copy(result[:], commitment.Bytes())
	return result, nil
}

// SignCLSAGs signs a transaction's inputs as one batch so the pseudo-output
// masks balance the aggregate output mask. Inputs must already be ordered by
// the canonical transaction encoding. The caller is responsible for using the
// returned pseudo-outputs and key images in the transaction and for proving
// output ranges with ProveRangeProof.
func SignCLSAGs(message [32]byte, inputs []SigningInput, sumOutputMasks [ScalarSize]byte, random io.Reader) ([]SignedInput, error) {
	wide := make([]SigningInput128, len(inputs))
	defer func() {
		for i := range wide {
			wide[i].Clear()
		}
	}()
	for i, input := range inputs {
		wide[i] = SigningInput128{Ring: input.Ring, SignerIndex: input.SignerIndex, SpendSecret: input.SpendSecret, CommitmentMask: input.CommitmentMask, Amount: new(big.Int).SetUint64(input.Amount)}
	}
	return SignCLSAGs128(message, wide, sumOutputMasks, random)
}

// SigningInput contains the wallet witness for one input. SpendSecret and
// CommitmentMask are canonical little-endian scalars. Amount is never put on
// chain; it is used only to validate the selected ring commitment locally.
type SigningInput struct {
	Ring           [RingSize]RingMember
	SignerIndex    uint64
	SpendSecret    [ScalarSize]byte
	CommitmentMask [ScalarSize]byte
	Amount         uint64
}

type SignedInput struct {
	Signature    [CLSAGSize]byte
	PseudoOutput [PointSize]byte
	KeyImage     [PointSize]byte
}

func decodePoint(encoded []byte, allowIdentity bool) (*curve25519.PublicKey[curve25519.VarTimeOperations], error) {
	if len(encoded) != PointSize {
		return nil, ErrInvalidPoint
	}
	point := new(curve25519.PublicKey[curve25519.VarTimeOperations])
	if _, err := point.SetBytes(encoded); err != nil || !point.IsTorsionFree() || (!allowIdentity && point.IsIdentity() == 1) {
		return nil, ErrInvalidPoint
	}
	return point, nil
}

// decodePointCT validates an encoded public point into the constant-time
// operation wrapper. Wallet and signing paths use this type so secret scalar
// operations cannot accidentally dispatch to the variable-time backend.
func decodePointCT(encoded []byte, allowIdentity bool) (*curve25519.PublicKey[curve25519.ConstantTimeOperations], error) {
	if len(encoded) != PointSize {
		return nil, ErrInvalidPoint
	}
	point := new(curve25519.PublicKey[curve25519.ConstantTimeOperations])
	if _, err := point.SetBytes(encoded); err != nil || !point.IsTorsionFree() || (!allowIdentity && point.IsIdentity() == 1) {
		return nil, ErrInvalidPoint
	}
	return point, nil
}

func decodeScalar(encoded []byte) (*curve25519.Scalar, error) {
	if len(encoded) != ScalarSize {
		return nil, errors.New("invalid scalar length")
	}
	scalar, err := new(curve25519.Scalar).SetCanonicalBytes(encoded)
	if err != nil {
		return nil, err
	}
	return scalar, nil
}

// ValidatePublicKey checks the canonical, non-identity Edwards encoding used
// for a Shield6 one-time output key. Consensus uses this before indexing a
// new output so malformed points cannot poison future decoy rings.
func ValidatePublicKey(encoded []byte) error {
	_, err := decodePoint(encoded, false)
	return err
}

// ValidateScalarEncoding checks canonical 32-byte scalar encoding.
func ValidateScalarEncoding(encoded []byte) error {
	_, err := decodeScalar(encoded)
	return err
}

// ValidateCommitmentEncoding checks that a point can be used as a nonzero
// Pedersen commitment or pseudo-output under the canonical Edwards encoding.
func ValidateCommitmentEncoding(encoded []byte) error {
	_, err := decodePoint(encoded, false)
	return err
}
