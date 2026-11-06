// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package recipient

import (
	"math/big"

	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/math/emulated"
)

// gnark 0.14's ReduceStrict rejects compile-time constants (including zero).
// Resolve only genuine compiler constants locally; witness values ALWAYS go
// through the constrained canonical decomposition.
func canonicalBits[T emulated.FieldParams](api frontend.API, f *emulated.Field[T], a *emulated.Element[T]) []frontend.Variable {
	_, width := emulated.GetEffectiveFieldParams[T](api.Compiler().Field())
	value := new(big.Int)
	for i, limb := range a.Limbs {
		n, ok := api.Compiler().ConstantValue(limb)
		if !ok {
			return f.ToBitsCanonical(a)
		}
		part := new(big.Int).Lsh(new(big.Int).Set(n), uint(i)*width)
		value.Add(value, part)
	}
	var params T
	value.Mod(value, params.Modulus())
	return constantBits(value, params.Modulus().BitLen())
}
