// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

// Package recipient implements the versioned Shield6 recipient-binding
// constraint relation. It does not activate a consensus fork or supply trusted
// setup parameters. Public inputs are a hash of the externally authenticated
// statement; stamp ownership, paths, recipient keys and sender entropy remain
// private witnesses.
package recipient

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/sha2"
	"github.com/consensys/gnark/std/math/uints"
)

const (
	RelationVersion           = uint64(2)
	BindingLeafDomain         = uint64(6006)
	BindingRegistrationDomain = uint64(6007)
	MerkleDepth               = 32
	// Restrict accepted ephemeral choices to ones whose V1 rejection sampling
	// succeeds in the first candidate. The sender can resample r; receivers
	// continue to use the unchanged V1 tweak, with no public counter or key link.
	TweakCandidates = 1
)

var statementDomain = []byte("TKM_SHIELD6_RECIPIENT_RELATION_V2")

type claim struct {
	ChainID, AssetID, Position                                           frontend.Variable
	StampRoot, BindingRoot                                               digest
	Anchor, Intent, OneTimeKey, EphemeralKey, Commitment, CiphertextHash [32]uints.U8
	AmountLimbs                                                          [2][32]uints.U8
}

// Circuit binds one output, not an entire transaction. A future consensus
// adapter must require one proof for EVERY output, authenticate both registry
// roots and the state anchor, and compute StatementDigest from the actual
// transaction fields. A prover-supplied digest or root is never sufficient.
type Circuit struct {
	StatementDigest             [32]uints.U8 `gnark:",public"`
	Claim                       claim
	Owner                       digest
	StampIndex                  frontend.Variable
	StampPath                   [MerkleDepth]digest
	BindingIndex                frontend.Variable
	BindingPath                 [MerkleDepth]digest
	Spend, View                 Point
	SpendPreimage, ViewPreimage Point
	EphemeralSecret             [32]uints.U8
}

func (c *Circuit) Define(api frontend.API) error {
	tip, err := newTip5(api)
	if err != nil {
		return err
	}
	ed, err := newEdwards(api)
	if err != nil {
		return err
	}
	u, err := uints.NewBinaryField[uints.U64](api)
	if err != nil {
		return err
	}
	chainBits := api.ToBinary(c.Claim.ChainID, 64)
	api.AssertIsDifferent(c.Claim.ChainID, 0)
	api.AssertIsEqual(c.Claim.AssetID, 1) // native TKM only
	api.ToBinary(c.Claim.Position, 4)     // transaction-local output position, 0..15
	chainWords := []*goldElement{tip.f.NewElement(api.FromBinary(chainBits[:32]...)), tip.f.NewElement(api.FromBinary(chainBits[32:]...))}
	ownerZero := frontend.Variable(1)
	for i := range c.Owner {
		tip.f.AssertIsInRange(&c.Owner[i])
		ownerZero = api.And(ownerZero, tip.f.IsZero(&c.Owner[i]))
	}
	api.AssertIsEqual(ownerZero, 0)
	stampWords := []*goldElement{tip.f.NewElement(3004)}
	stampWords = append(stampWords, chainWords...)
	for i := range c.Owner {
		stampWords = append(stampWords, &c.Owner[i])
	}
	stamp := tip.path(tip.hash(stampWords...), c.StampIndex, c.StampPath)
	for i := range stamp {
		tip.f.AssertIsInRange(&c.Claim.StampRoot[i])
		tip.f.AssertIsEqual(&stamp[i], &c.Claim.StampRoot[i])
	}
	ed.validRecipient(c.Spend, c.SpendPreimage)
	ed.validRecipient(c.View, c.ViewPreimage)
	spendBytes, viewBytes := ed.compressed(c.Spend), ed.compressed(c.View)
	bindingWords := []*goldElement{tip.f.NewElement(BindingLeafDomain), tip.f.NewElement(RelationVersion)}
	bindingWords = append(bindingWords, chainWords...)
	for i := range c.Owner {
		bindingWords = append(bindingWords, &c.Owner[i])
	}
	for _, key := range [][]uints.U8{spendBytes, viewBytes} {
		for i := 0; i < 32; i += 4 {
			bits := bytesBits(api, key[i:i+4])
			bindingWords = append(bindingWords, tip.f.NewElement(api.FromBinary(bits...)))
		}
	}
	commitment := tip.hash(bindingWords...)
	// Registration supplies an opaque key commitment. Consensus wraps it with
	// the authenticated account's stamp owner; the prover must open THAT owner.
	// Otherwise an account could register a commitment containing another owner.
	registration := []*goldElement{tip.f.NewElement(BindingRegistrationDomain), tip.f.NewElement(RelationVersion)}
	registration = append(registration, chainWords...)
	for i := range c.Owner {
		registration = append(registration, &c.Owner[i])
	}
	for i := range commitment {
		registration = append(registration, &commitment[i])
	}
	binding := tip.path(tip.hash(registration...), c.BindingIndex, c.BindingPath)
	for i := range binding {
		tip.f.AssertIsInRange(&c.Claim.BindingRoot[i])
		tip.f.AssertIsEqual(&binding[i], &c.Claim.BindingRoot[i])
	}
	rbits := bytesBits(api, c.EphemeralSecret[:])
	api.AssertIsEqual(canonicalScalar(api, rbits), 1)
	ephemeral := ed.mulBase(rbits)
	assertBytes(api, ed.compressed(ephemeral), c.Claim.EphemeralKey[:])
	shared := ed.compressed(ed.mul(c.View, rbits))
	salt := append(wordBytes(api, c.Claim.ChainID), wordBytes(api, c.Claim.Position)...)
	prk := hmac512Circuit(u, salt, shared)
	candidates := [TweakCandidates][]frontend.Variable{}
	valid := [TweakCandidates]frontend.Variable{}
	for i := 0; i < TweakCandidates; i++ {
		info := uints.NewU8Array([]byte("TKM_SHIELD6_OUTPUT_TWEAK_V1/"))
		info = append(info, uints.NewU8(0), uints.NewU8(0), uints.NewU8(0), uints.NewU8(uint8(i)), uints.NewU8(1))
		// One HKDF-Expand block suffices for the 32-byte output.
		material := hmac512Circuit(u, prk, info)
		candidates[i] = bytesBits(api, material[:32])
		valid[i] = canonicalScalar(api, candidates[i])
	}
	api.AssertIsEqual(valid[0], 1)
	tweak := candidates[0]
	output := ed.add(c.Spend, ed.mulBase(tweak))
	ed.notIdentity(output)
	assertBytes(api, ed.compressed(output), c.Claim.OneTimeKey[:])
	// Every context field participates in a constrained public statement hash.
	// This binds even fields which do not enter the stealth-key derivation.
	encoded := uints.NewU8Array(statementDomain)
	encoded = append(encoded, uints.NewU8Array([]byte{0, 0, 0, 0, 0, 0, 0, byte(RelationVersion)})...)
	encoded = append(encoded, wordBytes(api, c.Claim.ChainID)...)
	encoded = append(encoded, wordBytes(api, c.Claim.AssetID)...)
	encoded = append(encoded, wordBytes(api, c.Claim.Position)...)
	for _, root := range []digest{c.Claim.StampRoot, c.Claim.BindingRoot} {
		for i := range root {
			bits := tip.f.ToBitsCanonical(&root[i])
			for j := 0; j < 8; j++ {
				encoded = append(encoded, uints.U8{Val: api.FromBinary(bits[j*8 : (j+1)*8]...)})
			}
		}
	}
	for _, field := range [][]uints.U8{c.Claim.Anchor[:], c.Claim.Intent[:], c.Claim.OneTimeKey[:], c.Claim.EphemeralKey[:], c.Claim.Commitment[:], c.Claim.AmountLimbs[0][:], c.Claim.AmountLimbs[1][:], c.Claim.CiphertextHash[:]} {
		// U8 witness values require explicit bounds, including context-only fields.
		for _, b := range field {
			api.ToBinary(b.Val, 8)
		}
		encoded = append(encoded, field...)
	}
	hasher, err := sha2.New(api)
	if err != nil {
		return err
	}
	hasher.Write(encoded)
	assertBytes(api, hasher.Sum(), c.StatementDigest[:])
	return nil
}
func wordBytes(api frontend.API, word frontend.Variable) []uints.U8 {
	bits := api.ToBinary(word, 64)
	result := make([]uints.U8, 8)
	for i := range result {
		result[7-i] = uints.U8{Val: api.FromBinary(bits[i*8 : (i+1)*8]...)}
	}
	return result
}
func assertBytes(api frontend.API, a, b []uints.U8) {
	if len(a) != len(b) {
		panic("recipient relation byte length mismatch")
	}
	for i := range a {
		api.ToBinary(a[i].Val, 8)
		api.ToBinary(b[i].Val, 8)
		api.AssertIsEqual(a[i].Val, b[i].Val)
	}
}
