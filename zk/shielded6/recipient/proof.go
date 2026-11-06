// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package recipient

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend"
	"github.com/consensys/gnark/backend/groth16"
	groth16bn254 "github.com/consensys/gnark/backend/groth16/bn254"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/rs/zerolog"
)

const MaxProofBytes = 4096
const MaxVerifyingKeyBytes = 64 << 10

// Compile builds the full fixed-depth relation and hashes its serialized R1CS.
// It generates NO setup parameters. Groth16 requires a separately reviewed
// circuit-specific setup ceremony; test-generated keys must never be shipped.
func Compile() (constraint.ConstraintSystem, [32]byte, error) {
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &Circuit{})
	if err != nil {
		return nil, [32]byte{}, err
	}
	h := sha256.New()
	if _, err := cs.WriteTo(h); err != nil {
		return nil, [32]byte{}, err
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return cs, digest, nil
}

// Prove uses caller-supplied setup material. It never generates a setup or
// accepts a proof-supplied verification key. The caller owns private witnesses.
func Prove(cs constraint.ConstraintSystem, pk groth16.ProvingKey, s Statement, w Witness) ([]byte, error) {
	if cs == nil || pk == nil || pk.CurveID() != ecc.BN254 || cs.Field().Cmp(ecc.BN254.ScalarField()) != 0 {
		return nil, errors.New("recipient relation: invalid proving parameters")
	}
	defer w.Clear()
	assignment, err := NewAssignment(s, w)
	if err != nil {
		return nil, err
	}
	return proveAssignment(cs, pk, assignment)
}

func proveAssignment(cs constraint.ConstraintSystem, pk groth16.ProvingKey, assignment frontend.Circuit) ([]byte, error) {
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, errors.New("recipient relation: invalid witness encoding")
	}
	// Best-effort clearing of the serialized private witness after proving.
	defer func() {
		for i := range witness.Vector().(fr.Vector) {
			witness.Vector().(fr.Vector)[i].SetZero()
		}
	}()
	proof, err := groth16.Prove(cs, pk, witness, backend.WithSolverOptions(solver.WithLogger(zerolog.Nop())))
	if err != nil {
		return nil, errors.New("recipient relation: proving failed")
	}
	var output bytes.Buffer
	if _, err := proof.WriteTo(&output); err != nil {
		return nil, err
	}
	if output.Len() > MaxProofBytes {
		return nil, errors.New("recipient relation: proof exceeds bound")
	}
	return output.Bytes(), nil
}

// Verifier has a locally pinned key. The key hash must come from authenticated
// release/consensus configuration, never from the transaction or RPC peer.
// No key or activation value is configured by this package.
type Verifier struct {
	key         groth16.VerifyingKey
	commitments uint32
}

func NewVerifier(encodedKey []byte, expectedHash [32]byte) (*Verifier, error) {
	if len(encodedKey) == 0 || len(encodedKey) > MaxVerifyingKeyBytes || expectedHash == ([32]byte{}) || sha256.Sum256(encodedKey) != expectedHash {
		return nil, errors.New("recipient relation: verification key pin mismatch")
	}
	reader := bytes.NewReader(encodedKey)
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk.ReadFrom(reader); err != nil || reader.Len() != 0 {
		return nil, errors.New("recipient relation: invalid verification key")
	}
	native := vk.(*groth16bn254.VerifyingKey)
	count := len(native.PublicAndCommitmentCommitted)
	// gnark's NbPublicWitness includes internally derived commitment inputs;
	// only the remaining 32 values belong to the external digest witness.
	if vk.NbPublicWitness()-count != 32 || len(native.CommitmentKeys) != count || count > (MaxProofBytes-164)/32 {
		return nil, errors.New("recipient relation: invalid verification key shape")
	}
	for i, indices := range native.PublicAndCommitmentCommitted {
		for _, index := range indices {
			if index < 1 || index > 32+i {
				return nil, errors.New("recipient relation: invalid commitment metadata")
			}
		}
	}
	return &Verifier{key: vk, commitments: uint32(count)}, nil
}
func (v *Verifier) Verify(s Statement, encodedProof []byte) error {
	d, err := s.Digest()
	if err != nil {
		return err
	}
	return v.verifyDigest(d, encodedProof)
}
func (v *Verifier) verifyDigest(d [32]byte, encodedProof []byte) error {
	fail := errors.New("recipient relation: invalid proof")
	if v == nil || v.key == nil || !validProofEncoding(encodedProof) {
		return fail
	}
	if binary.BigEndian.Uint32(encodedProof[128:132]) != v.commitments {
		return fail
	}
	proof := groth16.NewProof(ecc.BN254)
	reader := bytes.NewReader(encodedProof)
	if _, err := proof.ReadFrom(reader); err != nil || reader.Len() != 0 {
		return fail
	}
	var canonical bytes.Buffer
	if _, err := proof.WriteTo(&canonical); err != nil || !bytes.Equal(canonical.Bytes(), encodedProof) {
		return fail
	}
	assignment := new(Circuit)
	set32(&assignment.StatementDigest, d)
	public, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return fail
	}
	if err := groth16.Verify(proof, v.key, public); err != nil {
		return fail
	}
	return nil
}

// The pinned gnark 0.14 BN254 compressed encoding is Ar(32), Bs(64),
// Krs(32), commitment-count(4 BE), commitments(32 each), PoK(32).
// Check shape BEFORE ReadFrom: a bounded byte slice alone does not prevent
// a forged slice-count prefix from requesting an enormous allocation.
func validProofEncoding(proof []byte) bool {
	const fixed = 32 + 64 + 32 + 4 + 32
	if len(proof) < fixed || len(proof) > MaxProofBytes {
		return false
	}
	count := uint64(binary.BigEndian.Uint32(proof[128:132]))
	if uint64(len(proof)) != uint64(fixed)+32*count {
		return false
	}
	for _, offset := range []int{0, 32, 96} {
		if proof[offset]&0xc0 == 0 {
			return false
		} // uncompressed point
	}
	for offset := 132; offset < len(proof); offset += 32 {
		if proof[offset]&0xc0 == 0 {
			return false
		}
	}
	return true
}
