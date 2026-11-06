// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package recipient

import (
	"math/big"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/math/cmp"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/std/math/uints"
)

// Edwards25519's base field differs from the native proving field.
type edwardsBase struct{}

func (edwardsBase) NbLimbs() uint     { return 4 }
func (edwardsBase) BitsPerLimb() uint { return 64 }
func (edwardsBase) IsPrime() bool     { return true }
func (edwardsBase) Modulus() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
}

type edElement = emulated.Element[edwardsBase]

// Point coordinates are private witness values. They are checked for canonical
// encoding, curve membership and prime-subgroup membership inside the circuit.
type Point struct{ X, Y edElement }
type edwardsCircuit struct {
	api frontend.API
	f   *emulated.Field[edwardsBase]
	d   *edElement
}

func newEdwards(api frontend.API) (*edwardsCircuit, error) {
	f, err := emulated.NewField[edwardsBase](api)
	if err != nil {
		return nil, err
	}
	modulus := new(edwardsBase).Modulus()
	d := new(big.Int).ModInverse(big.NewInt(121666), modulus)
	d.Mul(d, big.NewInt(-121665)).Mod(d, modulus)
	return &edwardsCircuit{api: api, f: f, d: f.NewElement(d)}, nil
}
func (e *edwardsCircuit) identity() Point { return Point{X: *e.f.Zero(), Y: *e.f.One()} }
func (e *edwardsCircuit) add(a, b Point) Point {
	xx, yy := e.f.Mul(&a.X, &b.X), e.f.Mul(&a.Y, &b.Y)
	product := e.f.Mul(e.d, e.f.Mul(xx, yy))
	xnum := e.f.Add(e.f.Mul(&a.X, &b.Y), e.f.Mul(&a.Y, &b.X))
	// a=-1: y numerator is y1*y2+x1*x2. Edwards25519 addition is complete.
	return Point{X: *e.f.Div(xnum, e.f.Add(e.f.One(), product)), Y: *e.f.Div(e.f.Add(yy, xx), e.f.Sub(e.f.One(), product))}
}
func (e *edwardsCircuit) selectPoint(bit frontend.Variable, a, b Point) Point {
	return Point{X: *e.f.Select(bit, &a.X, &b.X), Y: *e.f.Select(bit, &a.Y, &b.Y)}
}
func (e *edwardsCircuit) mul(p Point, bits []frontend.Variable) Point {
	if len(bits) != 256 {
		panic("recipient relation scalar width")
	}
	table := make([]Point, 16)
	table[0] = e.identity()
	table[1] = p
	for i := 2; i < 16; i++ {
		table[i] = e.add(table[i-1], p)
	}
	result := e.identity()
	for window := 63; window >= 0; window-- {
		for i := 0; i < 4; i++ {
			result = e.add(result, result)
		}
		choices := append([]Point(nil), table...)
		for digitBit := 0; digitBit < 4; digitBit++ {
			for i := 0; i < len(choices)/2; i++ {
				choices[i] = e.selectPoint(bits[window*4+digitBit], choices[2*i+1], choices[2*i])
			}
			choices = choices[:len(choices)/2]
		}
		result = e.add(result, choices[0])
	}
	return result
}
func scalarModulus() *big.Int {
	n, _ := new(big.Int).SetString("27742317777372353535851937790883648493", 10)
	return n.Add(n, new(big.Int).Lsh(big.NewInt(1), 252))
}
func constantBits(n *big.Int, size int) []frontend.Variable {
	bits := make([]frontend.Variable, size)
	for i := range bits {
		bits[i] = n.Bit(i)
	}
	return bits
}
func (e *edwardsCircuit) notIdentity(p Point) {
	e.api.AssertIsEqual(e.api.And(e.f.IsZero(&p.X), e.f.IsZero(e.f.Sub(&p.Y, e.f.One()))), 0)
}
func (e *edwardsCircuit) onCurve(p Point) {
	e.f.AssertIsInRange(&p.X)
	e.f.AssertIsInRange(&p.Y)
	xx, yy := e.f.Mul(&p.X, &p.X), e.f.Mul(&p.Y, &p.Y)
	e.f.AssertIsEqual(e.f.Sub(yy, xx), e.f.Add(e.f.One(), e.f.Mul(e.d, e.f.Mul(xx, yy))))
}
func (e *edwardsCircuit) validRecipient(p, preimage Point) {
	e.onCurve(p)
	e.onCurve(preimage)
	e.notIdentity(p)
	// E(Fq) has order 8*l. Requiring a curve point Q with [8]Q=P
	// proves [l]P=[8*l]Q=0, without a 253-bit multiplication by l.
	// Nonidentity P therefore has prime order l. Q is a private auxiliary
	// witness, computable from a valid public P as [8^-1 mod l]P.
	// See RFC 8032 section 5.1 for Edwards25519's order and cofactor.
	subgroup := preimage
	for i := 0; i < 3; i++ {
		subgroup = e.add(subgroup, subgroup)
	}
	e.f.AssertIsEqual(&subgroup.X, &p.X)
	e.f.AssertIsEqual(&subgroup.Y, &p.Y)
}
func (e *edwardsCircuit) compressed(p Point) []uints.U8 {
	bits := canonicalBits(e.api, e.f, &p.Y)
	xbits := canonicalBits(e.api, e.f, &p.X)
	bits = append(bits, xbits[0])
	out := make([]uints.U8, 32)
	for i := range out {
		out[i] = uints.U8{Val: e.api.FromBinary(bits[i*8 : (i+1)*8]...)}
	}
	return out
}
func bytesBits(api frontend.API, b []uints.U8) []frontend.Variable {
	result := make([]frontend.Variable, 0, len(b)*8)
	for _, v := range b {
		result = append(result, api.ToBinary(v.Val, 8)...)
	}
	return result
}
func canonicalScalar(api frontend.API, bits []frontend.Variable) frontend.Variable {
	less := cmp.IsLessBinary(api, bits, constantBits(scalarModulus(), len(bits)))
	var sum frontend.Variable = 0
	for _, bit := range bits {
		sum = api.Add(sum, bit)
	}
	return api.And(less, api.Sub(1, api.IsZero(sum)))
}

// mulBase uses fixed, locally generated 4-bit windows. No private value or
// prover-supplied point can enter this table. Native arithmetic is used ONLY
// to compute circuit constants for the standard basepoint; digit selection
// and every addition of the selected constants are constrained.
func (e *edwardsCircuit) mulBase(bits []frontend.Variable) Point {
	if len(bits) != 256 {
		panic("recipient relation scalar width")
	}
	var baseEncoding [32]byte
	baseEncoding[0] = 0x58
	for i := 1; i < 32; i++ {
		baseEncoding[i] = 0x66
	}
	base, err := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).SetBytes(baseEncoding[:])
	if err != nil {
		panic(err)
	}
	result := e.identity()
	for window := 0; window < 64; window++ {
		choices := make([]Point, 16)
		choices[0] = e.identity()
		point := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).Identity()
		for digit := 1; digit < 16; digit++ {
			point.Add(point, base)
			var encoded [32]byte
			copy(encoded[:], point.Bytes())
			x, y, err := pointCoordinates(encoded)
			if err != nil {
				panic(err)
			}
			choices[digit] = Point{X: *e.f.NewElement(x), Y: *e.f.NewElement(y)}
		}
		for digitBit := 0; digitBit < 4; digitBit++ {
			for i := 0; i < len(choices)/2; i++ {
				choices[i] = e.selectPoint(bits[window*4+digitBit], choices[2*i+1], choices[2*i])
			}
			choices = choices[:len(choices)/2]
		}
		result = e.add(result, choices[0])
		for i := 0; i < 4; i++ {
			base.Double(base)
		}
	}
	return result
}
