package recipient

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/hash/sha2"
	"github.com/consensys/gnark/std/math/uints"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

var registrationDomain = []byte("TKM_SHIELD6_KEY_CONTROL_RELATION_V2")

// RegistrationStatement is reconstructed from the signed registration and
// canonical stamped account. It publishes the already-public owner and opaque
// commitment, never either static key or secret scalar.
type RegistrationStatement struct {
	ChainID           uint64
	Owner, Commitment shielded3.Digest
	Intent            [32]byte
}

type RegistrationWitness struct{ SpendSecret, ViewSecret [32]byte }

func (w *RegistrationWitness) Clear() {
	if w != nil {
		*w = RegistrationWitness{}
	}
}

func (s RegistrationStatement) Digest() ([32]byte, error) {
	if s.ChainID == 0 || !validDigest(s.Owner) || s.Owner == (shielded3.Digest{}) || !validDigest(s.Commitment) || s.Commitment == (shielded3.Digest{}) {
		return [32]byte{}, errors.New("invalid recipient key-control statement")
	}
	data := append([]byte(nil), registrationDomain...)
	data = binary.BigEndian.AppendUint64(data, RelationVersion)
	data = binary.BigEndian.AppendUint64(data, s.ChainID)
	data = append(data, s.Owner.Bytes()...)
	data = append(data, s.Commitment.Bytes()...)
	data = append(data, s.Intent[:]...)
	return sha256.Sum256(data), nil
}

type RegistrationCircuit struct {
	StatementDigest         [32]uints.U8 `gnark:",public"`
	ChainID                 frontend.Variable
	Owner, Commitment       digest
	Intent                  [32]uints.U8
	SpendSecret, ViewSecret [32]uints.U8
}

func (c *RegistrationCircuit) Define(api frontend.API) error {
	tip, err := newTip5(api)
	if err != nil {
		return err
	}
	ed, err := newEdwards(api)
	if err != nil {
		return err
	}
	chainBits := api.ToBinary(c.ChainID, 64)
	api.AssertIsDifferent(c.ChainID, 0)
	var ownerZero frontend.Variable = 1
	for i := range c.Owner {
		tip.f.AssertIsInRange(&c.Owner[i])
		ownerZero = api.And(ownerZero, tip.f.IsZero(&c.Owner[i]))
	}
	api.AssertIsEqual(ownerZero, 0)
	spendBits, viewBits := bytesBits(api, c.SpendSecret[:]), bytesBits(api, c.ViewSecret[:])
	api.AssertIsEqual(canonicalScalar(api, spendBits), 1)
	api.AssertIsEqual(canonicalScalar(api, viewBits), 1)
	spend, view := ed.mulBase(spendBits), ed.mulBase(viewBits)
	ed.notIdentity(spend)
	ed.notIdentity(view)
	words := []*goldElement{tip.f.NewElement(BindingLeafDomain), tip.f.NewElement(RelationVersion), tip.f.NewElement(api.FromBinary(chainBits[:32]...)), tip.f.NewElement(api.FromBinary(chainBits[32:]...))}
	for i := range c.Owner {
		words = append(words, &c.Owner[i])
	}
	for _, key := range [][]uints.U8{ed.compressed(spend), ed.compressed(view)} {
		for i := 0; i < 32; i += 4 {
			words = append(words, tip.f.NewElement(api.FromBinary(bytesBits(api, key[i:i+4])...)))
		}
	}
	commitment := tip.hash(words...)
	for i := range commitment {
		tip.f.AssertIsInRange(&c.Commitment[i])
		tip.f.AssertIsEqual(&commitment[i], &c.Commitment[i])
	}
	encoded := uints.NewU8Array(registrationDomain)
	encoded = append(encoded, uints.NewU8Array([]byte{0, 0, 0, 0, 0, 0, 0, byte(RelationVersion)})...)
	encoded = append(encoded, wordBytes(api, c.ChainID)...)
	for _, field := range []digest{c.Owner, c.Commitment} {
		for i := range field {
			bits := tip.f.ToBitsCanonical(&field[i])
			for j := 0; j < 8; j++ {
				encoded = append(encoded, uints.U8{Val: api.FromBinary(bits[j*8 : (j+1)*8]...)})
			}
		}
	}
	for _, value := range c.Intent {
		api.ToBinary(value.Val, 8)
	}
	encoded = append(encoded, c.Intent[:]...)
	hash, err := sha2.New(api)
	if err != nil {
		return err
	}
	hash.Write(encoded)
	assertBytes(api, hash.Sum(), c.StatementDigest[:])
	return nil
}

func NewRegistrationAssignment(s RegistrationStatement, w RegistrationWitness) (*RegistrationCircuit, error) {
	d, err := s.Digest()
	if err != nil {
		return nil, err
	}
	if shielded6.ValidateScalarEncoding(w.SpendSecret[:]) != nil || shielded6.ValidateScalarEncoding(w.ViewSecret[:]) != nil || w.SpendSecret == ([32]byte{}) || w.ViewSecret == ([32]byte{}) {
		return nil, errors.New("invalid recipient key-control witness")
	}
	c := &RegistrationCircuit{ChainID: s.ChainID, Owner: digestAssignment(s.Owner), Commitment: digestAssignment(s.Commitment)}
	set32(&c.StatementDigest, d)
	set32(&c.Intent, s.Intent)
	set32(&c.SpendSecret, w.SpendSecret)
	set32(&c.ViewSecret, w.ViewSecret)
	return c, nil
}

func CompileRegistration() (constraint.ConstraintSystem, [32]byte, error) {
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &RegistrationCircuit{})
	if err != nil {
		return nil, [32]byte{}, err
	}
	h := sha256.New()
	if _, err := cs.WriteTo(h); err != nil {
		return nil, [32]byte{}, err
	}
	var hash [32]byte
	copy(hash[:], h.Sum(nil))
	return cs, hash, nil
}

func ProveRegistration(cs constraint.ConstraintSystem, pk groth16.ProvingKey, s RegistrationStatement, w RegistrationWitness) ([]byte, error) {
	defer w.Clear()
	if cs == nil || pk == nil || pk.CurveID() != ecc.BN254 || cs.Field().Cmp(ecc.BN254.ScalarField()) != 0 {
		return nil, errors.New("invalid recipient registration proving parameters")
	}
	assignment, err := NewRegistrationAssignment(s, w)
	if err != nil {
		return nil, err
	}
	return proveAssignment(cs, pk, assignment)
}

func (v *Verifier) VerifyRegistration(s RegistrationStatement, proof []byte) error {
	d, err := s.Digest()
	if err != nil {
		return err
	}
	return v.verifyDigest(d, proof)
}
