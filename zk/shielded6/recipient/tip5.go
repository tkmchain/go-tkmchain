// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package recipient

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/lookup/logderivlookup"
	"github.com/consensys/gnark/std/math/emulated"
)

type gold = emulated.Goldilocks
type goldElement = emulated.Element[gold]
type digest [5]goldElement

type tip5Circuit struct {
	api    frontend.API
	f      *emulated.Field[gold]
	lookup logderivlookup.Table
}

func newTip5(api frontend.API) (*tip5Circuit, error) {
	f, err := emulated.NewField[gold](api)
	if err != nil {
		return nil, err
	}
	lookup := logderivlookup.New(api)
	for _, v := range tip5Lookup {
		lookup.Insert(v)
	}
	return &tip5Circuit{api: api, f: f, lookup: lookup}, nil
}

func (h *tip5Circuit) hash(words ...*goldElement) digest {
	words = append(append([]*goldElement(nil), words...), h.f.One())
	for len(words)%10 != 0 {
		words = append(words, h.f.Zero())
	}
	var state [16]*goldElement
	for i := range state {
		state[i] = h.f.Zero()
	}
	for i := 0; i < len(words); i += 10 {
		// Triton sponge_absorb overwrites the rate portion; it does not add it.
		copy(state[:10], words[i:i+10])
		state = h.permutation(state)
	}
	var result digest
	for i := range result {
		result[i] = *state[i]
	}
	return result
}

func (h *tip5Circuit) pair(left, right digest) digest {
	var state [16]*goldElement
	for i := 0; i < 5; i++ {
		state[i] = &left[i]
		state[i+5] = &right[i]
	}
	for i := 10; i < 16; i++ {
		state[i] = h.f.One()
	} // FixedLength domain.
	state = h.permutation(state)
	var result digest
	for i := range result {
		result[i] = *state[i]
	}
	return result
}

func (h *tip5Circuit) path(leaf digest, index frontend.Variable, path [32]digest) digest {
	bits := h.api.ToBinary(index, 32)
	current := leaf
	for level := range path {
		var left, right digest
		for j := 0; j < 5; j++ {
			h.f.AssertIsInRange(&path[level][j])
			left[j] = *h.f.Select(bits[level], &path[level][j], &current[j])
			right[j] = *h.f.Select(bits[level], &current[j], &path[level][j])
		}
		current = h.pair(left, right)
	}
	return current
}
