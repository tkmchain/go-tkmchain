package recipient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
)

// ArtifactPins must come from an authenticated release manifest installed
// locally. A transaction or RPC server must never select these hashes/paths.
type ArtifactPins struct {
	Version      uint64   `json:"version"`
	R1CS         [32]byte `json:"r1csSHA256"`
	ProvingKey   [32]byte `json:"provingKeySHA256"`
	VerifyingKey [32]byte `json:"verifyingKeySHA256"`
}

// PreparedProver retains immutable, authenticated circuit/setup material across
// sends. It never compiles or sets up a circuit on the transaction path. A gate
// bounds memory by allowing one proof at a time per loaded prover.
type PreparedProver struct {
	cs           constraint.ConstraintSystem
	pk           groth16.ProvingKey
	verifier     *Verifier
	gate         chan struct{}
	pins         ArtifactPins
	registration bool
}

// CircuitSHA256 pins this release's versioned relation, separately from the
// externally reviewed ceremony keys. Updating the relation requires a new pin.
const CircuitSHA256 = "a3426a12b2e63f581f4d792662d7dd2d1e3d6d1fa8b5b6dbca174d2110d2a50b"

const RegistrationCircuitSHA256 = "0040720811ae44399e005fe617b7f6673fea63c458fdaf55eacf5b410eaa53c3"

func validArtifactPins(pins ArtifactPins, registration bool) bool {
	fingerprint := CircuitSHA256
	if registration {
		fingerprint = RegistrationCircuitSHA256
	}
	expected, _ := hex.DecodeString(fingerprint)
	return pins.Version == RelationVersion && string(pins.R1CS[:]) == string(expected) && pins.ProvingKey != ([32]byte{}) && pins.VerifyingKey != ([32]byte{})
}

// NewPreparedProver authenticates already loaded parameters against explicit
// release pins. This also supports ceremony validation without temporary key
// files. It does not create or bless any setup material.
func NewPreparedProver(cs constraint.ConstraintSystem, pk groth16.ProvingKey, encodedVK []byte, pins ArtifactPins) (*PreparedProver, error) {
	return newPreparedProver(cs, pk, encodedVK, pins, false)
}
func NewRegistrationProver(cs constraint.ConstraintSystem, pk groth16.ProvingKey, encodedVK []byte, pins ArtifactPins) (*PreparedProver, error) {
	return newPreparedProver(cs, pk, encodedVK, pins, true)
}
func newPreparedProver(cs constraint.ConstraintSystem, pk groth16.ProvingKey, encodedVK []byte, pins ArtifactPins, registration bool) (*PreparedProver, error) {
	if !validArtifactPins(pins, registration) || cs == nil || pk == nil || pk.CurveID() != ecc.BN254 || cs.Field().Cmp(ecc.BN254.ScalarField()) != 0 {
		return nil, errors.New("recipient relation: invalid prepared parameters")
	}
	for _, item := range []struct {
		object io.WriterTo
		pin    [32]byte
	}{{cs, pins.R1CS}, {pk, pins.ProvingKey}} {
		h := sha256.New()
		if _, err := item.object.WriteTo(h); err != nil {
			return nil, err
		}
		if string(h.Sum(nil)) != string(item.pin[:]) {
			return nil, errors.New("recipient relation: prepared artifact pin mismatch")
		}
	}
	v, err := NewVerifier(encodedVK, pins.VerifyingKey)
	if err != nil {
		return nil, err
	}
	return &PreparedProver{cs: cs, pk: pk, verifier: v, gate: make(chan struct{}, 1), pins: pins, registration: registration}, nil
}

func OpenPreparedProver(csPath, pkPath, vkPath string, pins ArtifactPins) (*PreparedProver, error) {
	return openPreparedProver(csPath, pkPath, vkPath, pins, false)
}
func OpenRegistrationProver(csPath, pkPath, vkPath string, pins ArtifactPins) (*PreparedProver, error) {
	return openPreparedProver(csPath, pkPath, vkPath, pins, true)
}
func openPreparedProver(csPath, pkPath, vkPath string, pins ArtifactPins, registration bool) (*PreparedProver, error) {
	if !validArtifactPins(pins, registration) {
		return nil, errors.New("recipient relation: artifact version mismatch")
	}
	cs := groth16.NewCS(ecc.BN254)
	if err := readPinnedObject(csPath, pins.R1CS, 512<<20, cs); err != nil {
		return nil, err
	}
	pk := groth16.NewProvingKey(ecc.BN254)
	// ArtifactPins are authenticated release metadata, not caller-selected
	// transaction data. readPinnedObject hashes the complete file BEFORE this
	// decoder and again while decoding. For this already authenticated SRS,
	// use gnark's trusted-source API instead of spending minutes repeating
	// subgroup checks over millions of setup points. Proof and verification-key
	// decoders retain their full validation; no unpinned file reaches this path.
	if err := readPinnedObject(pkPath, pins.ProvingKey, 2<<30, trustedProvingKeyReader{pk}); err != nil {
		return nil, err
	}
	vkFile, err := os.Open(vkPath)
	if err != nil {
		return nil, err
	}
	defer vkFile.Close()
	encoded, err := io.ReadAll(io.LimitReader(vkFile, MaxVerifyingKeyBytes+1))
	if err != nil {
		return nil, err
	}
	verifier, err := NewVerifier(encoded, pins.VerifyingKey)
	if err != nil {
		return nil, err
	}
	return &PreparedProver{cs: cs, pk: pk, verifier: verifier, gate: make(chan struct{}, 1), pins: pins, registration: registration}, nil
}

type trustedProvingKeyReader struct{ groth16.ProvingKey }

func (r trustedProvingKeyReader) ReadFrom(input io.Reader) (int64, error) {
	return r.UnsafeReadFrom(input)
}

func readPinnedObject(path string, pin [32]byte, limit int64, object io.ReaderFrom) error {
	if pin == ([32]byte{}) {
		return errors.New("recipient relation: missing artifact pin")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > limit {
		return errors.New("recipient relation: artifact size limit")
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(file, limit+1)); err != nil {
		return err
	}
	if string(h.Sum(nil)) != string(pin[:]) {
		return errors.New("recipient relation: artifact pin mismatch")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h.Reset()
	reader := io.TeeReader(io.LimitReader(file, stat.Size()), h)
	n, err := object.ReadFrom(reader)
	if err != nil || n != stat.Size() || string(h.Sum(nil)) != string(pin[:]) {
		return errors.New("recipient relation: invalid or changed artifact")
	}
	return nil
}

func (p *PreparedProver) Pins() ArtifactPins {
	if p == nil {
		return ArtifactPins{}
	}
	return p.pins
}
func (p *PreparedProver) Verifier() *Verifier {
	if p == nil {
		return nil
	}
	return p.verifier
}

func (p *PreparedProver) Prove(ctx context.Context, s Statement, w Witness) ([]byte, error) {
	defer w.Clear()
	if p == nil || ctx == nil || p.registration {
		return nil, errors.New("recipient relation: prover unavailable")
	}
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	proof, err := Prove(p.cs, p.pk, s, w)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		clear(proof)
		return nil, err
	}
	if err := p.verifier.Verify(s, proof); err != nil {
		clear(proof)
		return nil, err
	}
	return proof, nil
}

func (p *PreparedProver) ProveRegistration(ctx context.Context, s RegistrationStatement, w RegistrationWitness) ([]byte, error) {
	defer w.Clear()
	if p == nil || ctx == nil || !p.registration {
		return nil, errors.New("registration prover unavailable")
	}
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	proof, err := ProveRegistration(p.cs, p.pk, s, w)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		clear(proof)
		return nil, err
	}
	if err := p.verifier.VerifyRegistration(s, proof); err != nil {
		clear(proof)
		return nil, err
	}
	return proof, nil
}
