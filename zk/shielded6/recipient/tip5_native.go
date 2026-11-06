package recipient

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark/constraint/solver"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/rangecheck"
)

const goldPrime uint64 = 0xffffffff00000001

func init() { solver.RegisterHint(goldReduceHint, goldBytesHint) }

func goldReduceHint(_ *big.Int, inputs, outputs []*big.Int) error {
	if len(inputs) != 1 || len(outputs) != 3 {
		return fmt.Errorf("invalid Goldilocks reduction hint shape")
	}
	p := new(big.Int).SetUint64(goldPrime)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(inputs[0], p, r)
	outputs[0].Set(q)
	outputs[1].SetUint64(uint64(uint32(r.Uint64())))
	outputs[2].SetUint64(r.Uint64() >> 32)
	return nil
}

func goldBytesHint(_ *big.Int, inputs, outputs []*big.Int) error {
	if len(inputs) != 1 || len(outputs) != 8 {
		return fmt.Errorf("invalid Goldilocks byte hint shape")
	}
	value := inputs[0].Uint64()
	for i := range outputs {
		outputs[i].SetUint64((value >> uint(8*i)) & 255)
	}
	return nil
}

// reduceGold proves an INTEGER quotient/remainder equation, not just an
// unchecked hint. Every caller supplies an expression below 2^192. q has at
// most 128 bits, so q*p+r < 2^193 < the 254-bit native field modulus: equality
// cannot wrap. Two bounded 32-bit limbs encode r. For p=2^64-2^32+1, r<p iff
// high!=2^32-1 OR low=0. This exact bound avoids a general 64-bit comparator.
func reduceGold(api frontend.API, value frontend.Variable, quotientBits int) frontend.Variable {
	if quotientBits < 1 || quotientBits > 128 {
		panic("invalid Goldilocks quotient bound")
	}
	if constant, ok := api.Compiler().ConstantValue(value); ok {
		return new(big.Int).Mod(constant, new(big.Int).SetUint64(goldPrime))
	}
	parts, err := api.Compiler().NewHint(goldReduceHint, 3, value)
	if err != nil {
		panic(err)
	}
	check := rangecheck.New(api)
	check.Check(parts[0], quotientBits)
	check.Check(parts[1], 32)
	check.Check(parts[2], 32)
	api.AssertIsEqual(api.Mul(api.IsZero(api.Sub(parts[2], uint64(0xffffffff))), parts[1]), 0)
	remainder := api.Add(parts[1], api.Mul(parts[2], uint64(1)<<32))
	api.AssertIsEqual(value, api.Add(api.Mul(parts[0], goldPrime), remainder))
	return remainder
}

func (h *tip5Circuit) permutation(state [16]*goldElement) [16]*goldElement {
	var values [16]frontend.Variable
	for i := range state {
		values[i] = h.api.FromBinary(canonicalBits(h.api, h.f, state[i])...)
	}
	// R=2^64 mod p=2^32-1. Lookup bytes are Montgomery residues exactly as
	// in the native Tip5 reference. Mapped bytes need not themselves be <p.
	inverse := new(big.Int).ModInverse(new(big.Int).SetUint64(0xffffffff), new(big.Int).SetUint64(goldPrime))
	for round := 0; round < 5; round++ {
		for i := 0; i < 4; i++ {
			mont := reduceGold(h.api, h.api.Mul(values[i], uint64(0xffffffff)), 32)
			var inputBytes []frontend.Variable
			if constant, ok := h.api.Compiler().ConstantValue(mont); ok {
				inputBytes = make([]frontend.Variable, 8)
				for j := range inputBytes {
					inputBytes[j] = (constant.Uint64() >> uint(8*j)) & 255
				}
			} else {
				var err error
				inputBytes, err = h.api.Compiler().NewHint(goldBytesHint, 8, mont)
				if err != nil {
					panic(err)
				}
			}
			var reconstructed, mapped frontend.Variable = 0, 0
			for j, index := range inputBytes {
				// Membership in this fixed 256-row table bounds index AND value
				// to bytes. The exact reconstruction then constrains every hint.
				value := h.lookup.Lookup(index)[0]
				weight := new(big.Int).Lsh(big.NewInt(1), uint(8*j))
				reconstructed = h.api.Add(reconstructed, h.api.Mul(index, weight))
				mapped = h.api.Add(mapped, h.api.Mul(value, weight))
			}
			h.api.AssertIsEqual(mont, reconstructed)
			values[i] = reduceGold(h.api, h.api.Mul(mapped, inverse), 64)
		}
		for i := 4; i < 16; i++ {
			// Each cubic integer fits in 192 bits; two reductions suffice for
			// x^7: c=x^3 mod p, then c^2*x mod p. No product wraps BN254.
			cube := reduceGold(h.api, h.api.Mul(values[i], values[i], values[i]), 128)
			values[i] = reduceGold(h.api, h.api.Mul(cube, cube, values[i]), 128)
		}
		var next [16]frontend.Variable
		for row := 0; row < 16; row++ {
			var sum frontend.Variable = tip5RoundConstants[round*16+row]
			for col := 0; col < 16; col++ {
				sum = h.api.Add(sum, h.api.Mul(values[col], tip5MDS[(16+row-col)%16]))
			}
			// Sum of MDS coefficients is below 2^20. The round constant
			// contributes less than p, so q fits in 20 bits.
			next[row] = reduceGold(h.api, sum, 20)
		}
		values = next
	}
	for i := range state {
		state[i] = h.f.NewElement(values[i])
	}
	return state
}
