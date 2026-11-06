// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package recipient

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/std/math/uints"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

// Statement contains only externally public facts. The verifier MUST derive
// these facts from the transaction and canonical state, not from the prover.
type Statement struct {
	ChainID, AssetID, OutputPosition uint64
	StampRoot, BindingRoot           shielded3.Digest
	StateAnchor, Intent              [32]byte
	Output                           shielded6.OutputRecord
}

// Witness is for a sender constructing a payment to a privately supplied
// recipient code. It contains no recipient spend/view SECRET, but the public
// keys, owner association, and membership paths must remain hidden on-chain.
type Witness struct {
	Owner           shielded3.Digest
	StampIndex      uint32
	StampPath       [MerkleDepth]shielded3.Digest
	BindingIndex    uint32
	BindingPath     [MerkleDepth]shielded3.Digest
	Recipient       shielded6.Address
	EphemeralSecret [32]byte
}

func (w *Witness) Clear() {
	if w == nil {
		return
	}
	*w = Witness{}
}
func validDigest(d shielded3.Digest) bool {
	for _, word := range d {
		if word >= 0xffffffff00000001 {
			return false
		}
	}
	return true
}
func (s Statement) Digest() ([32]byte, error) {
	if s.ChainID == 0 || s.AssetID != shielded6.AssetTKM || s.OutputPosition >= 16 || !validDigest(s.StampRoot) || !validDigest(s.BindingRoot) || len(s.Output.EncryptedNote) == 0 || len(s.Output.EncryptedNote) > 4096 {
		return [32]byte{}, errors.New("invalid recipient-binding statement")
	}
	data := append([]byte(nil), statementDomain...)
	for _, n := range []uint64{RelationVersion, s.ChainID, s.AssetID, s.OutputPosition} {
		data = binary.BigEndian.AppendUint64(data, n)
	}
	data = append(data, s.StampRoot.Bytes()...)
	data = append(data, s.BindingRoot.Bytes()...)
	ciphertextHash := sha256.Sum256(s.Output.EncryptedNote)
	for _, field := range [][]byte{s.StateAnchor[:], s.Intent[:], s.Output.OneTimeKey[:], s.Output.EphemeralKey[:], s.Output.Commitment[:], s.Output.AmountLimbs[0][:], s.Output.AmountLimbs[1][:], ciphertextHash[:]} {
		data = append(data, field...)
	}
	return sha256.Sum256(data), nil
}

// BindingLeaf is a NEW registry leaf format, never an interpretation of the
// existing immutable stamp leaf. Registration must authorize this commitment
// with the corresponding stamped identity; that state adapter is separate.
func BindingCommitment(chainID uint64, owner shielded3.Digest, address shielded6.Address) (shielded3.Digest, error) {
	if chainID == 0 || address.ChainID != chainID || !validDigest(owner) || owner == (shielded3.Digest{}) || address.Validate() != nil {
		return shielded3.Digest{}, errors.New("invalid recipient key binding")
	}
	words := []uint64{BindingLeafDomain, RelationVersion, uint64(uint32(chainID)), chainID >> 32}
	words = append(words, owner[:]...)
	for _, key := range [][32]byte{address.SpendPublic, address.ViewPublic} {
		for i := 0; i < 32; i += 4 {
			words = append(words, uint64(binary.LittleEndian.Uint32(key[i:i+4])))
		}
	}
	return shielded3.HashWords(words)
}

// AuthorizedBindingLeaf wraps an opaque commitment with the owner authenticated
// by the outer account signature and canonical stamp registry. Registration
// never publishes the recipient's static keys or assumes an unchecked opening.
func AuthorizedBindingLeaf(chainID uint64, owner, commitment shielded3.Digest) (shielded3.Digest, error) {
	if chainID == 0 || !validDigest(owner) || owner == (shielded3.Digest{}) || !validDigest(commitment) || commitment == (shielded3.Digest{}) {
		return shielded3.Digest{}, errors.New("invalid authorized recipient key binding")
	}
	words := []uint64{BindingRegistrationDomain, RelationVersion, uint64(uint32(chainID)), chainID >> 32}
	words = append(words, owner[:]...)
	words = append(words, commitment[:]...)
	return shielded3.HashWords(words)
}

func BindingLeaf(chainID uint64, owner shielded3.Digest, address shielded6.Address) (shielded3.Digest, error) {
	commitment, err := BindingCommitment(chainID, owner, address)
	if err != nil {
		return shielded3.Digest{}, err
	}
	return AuthorizedBindingLeaf(chainID, owner, commitment)
}

// NewAssignment encodes a native statement/witness into circuit variables.
// Acceptance is decided by constrained computation, not these host checks.
func NewAssignment(s Statement, w Witness) (*Circuit, error) {
	d, err := s.Digest()
	if err != nil {
		return nil, err
	}
	if w.Recipient.ChainID != s.ChainID || w.Recipient.Validate() != nil || !validDigest(w.Owner) || w.Owner == (shielded3.Digest{}) || shielded6.ValidateScalarEncoding(w.EphemeralSecret[:]) != nil || w.EphemeralSecret == ([32]byte{}) {
		return nil, errors.New("invalid recipient-binding witness")
	}
	spend, err := pointAssignment(w.Recipient.SpendPublic)
	if err != nil {
		return nil, err
	}
	view, err := pointAssignment(w.Recipient.ViewPublic)
	if err != nil {
		return nil, err
	}
	spendPreimage, err := subgroupPreimage(w.Recipient.SpendPublic)
	if err != nil {
		return nil, err
	}
	viewPreimage, err := subgroupPreimage(w.Recipient.ViewPublic)
	if err != nil {
		return nil, err
	}
	c := &Circuit{Owner: digestAssignment(w.Owner), StampIndex: w.StampIndex, BindingIndex: w.BindingIndex, Spend: spend, View: view, SpendPreimage: spendPreimage, ViewPreimage: viewPreimage}
	set32(&c.StatementDigest, d)
	c.Claim.ChainID = s.ChainID
	c.Claim.AssetID = s.AssetID
	c.Claim.Position = s.OutputPosition
	c.Claim.StampRoot = digestAssignment(s.StampRoot)
	c.Claim.BindingRoot = digestAssignment(s.BindingRoot)
	for i := 0; i < MerkleDepth; i++ {
		if !validDigest(w.StampPath[i]) || !validDigest(w.BindingPath[i]) {
			return nil, errors.New("noncanonical recipient membership path")
		}
		c.StampPath[i] = digestAssignment(w.StampPath[i])
		c.BindingPath[i] = digestAssignment(w.BindingPath[i])
	}
	for _, field := range []struct {
		dest *[32]uints.U8
		src  [32]byte
	}{
		{&c.Claim.Anchor, s.StateAnchor}, {&c.Claim.Intent, s.Intent}, {&c.Claim.OneTimeKey, s.Output.OneTimeKey}, {&c.Claim.EphemeralKey, s.Output.EphemeralKey}, {&c.Claim.Commitment, s.Output.Commitment}, {&c.Claim.AmountLimbs[0], s.Output.AmountLimbs[0]}, {&c.Claim.AmountLimbs[1], s.Output.AmountLimbs[1]}, {&c.Claim.CiphertextHash, sha256.Sum256(s.Output.EncryptedNote)}, {&c.EphemeralSecret, w.EphemeralSecret},
	} {
		set32(field.dest, field.src)
	}
	return c, nil
}

// PublicAssignment exposes exactly the statement digest to the proof verifier.
func PublicAssignment(s Statement) (*Circuit, error) {
	d, err := s.Digest()
	if err != nil {
		return nil, err
	}
	c := new(Circuit)
	set32(&c.StatementDigest, d)
	return c, nil
}
func set32(dest *[32]uints.U8, src [32]byte) {
	for i := range src {
		dest[i] = uints.NewU8(src[i])
	}
}
func digestAssignment(d shielded3.Digest) digest {
	var result digest
	for i := range result {
		result[i] = emulated.ValueOf[gold](d[i])
	}
	return result
}
func littleInt(b []byte) *big.Int {
	reversed := append([]byte(nil), b...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return new(big.Int).SetBytes(reversed)
}
func pointAssignment(encoded [32]byte) (Point, error) {
	x, y, err := pointCoordinates(encoded)
	if err != nil {
		return Point{}, err
	}
	return Point{X: emulated.ValueOf[edwardsBase](x), Y: emulated.ValueOf[edwardsBase](y)}, nil
}
func pointCoordinates(encoded [32]byte) (*big.Int, *big.Int, error) {
	if shielded6.ValidatePublicKey(encoded[:]) != nil {
		return nil, nil, errors.New("invalid recipient point")
	}
	sign := encoded[31] >> 7
	encoded[31] &= 127
	p := new(edwardsBase).Modulus()
	y := littleInt(encoded[:])
	yy := new(big.Int).Mul(y, y)
	yy.Mod(yy, p)
	d := new(big.Int).ModInverse(big.NewInt(121666), p)
	d.Mul(d, big.NewInt(-121665)).Mod(d, p)
	denominator := new(big.Int).Mul(d, yy)
	denominator.Add(denominator, big.NewInt(1)).Mod(denominator, p)
	inverse := new(big.Int).ModInverse(denominator, p)
	if inverse == nil {
		return nil, nil, errors.New("invalid Edwards denominator")
	}
	square := new(big.Int).Sub(yy, big.NewInt(1))
	square.Mul(square, inverse).Mod(square, p)
	x := new(big.Int).ModSqrt(square, p)
	if x == nil {
		return nil, nil, errors.New("invalid Edwards square root")
	}
	if x.Bit(0) != uint(sign) {
		x.Sub(p, x)
	}
	return x, y, nil
}

func subgroupPreimage(encoded [32]byte) (Point, error) {
	p, err := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).SetBytes(encoded[:])
	if err != nil {
		return Point{}, err
	}
	inverse := new(big.Int).ModInverse(big.NewInt(8), scalarModulus())
	var scalarBytes [32]byte
	inverse.FillBytes(scalarBytes[:])
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		scalarBytes[i], scalarBytes[j] = scalarBytes[j], scalarBytes[i]
	}
	scalar, err := new(curve25519.Scalar).SetCanonicalBytes(scalarBytes[:])
	if err != nil {
		return Point{}, err
	}
	q := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(scalar, p)
	var qbytes [32]byte
	copy(qbytes[:], q.Bytes())
	return pointAssignment(qbytes)
}
