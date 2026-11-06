package recipient

import (
	"math/big"

	"github.com/consensys/gnark/frontend"
)

func (h *tip5Circuit) emulatedPermutation(state [16]*goldElement) [16]*goldElement {
	// Tip5's lookup S-box operates on the Montgomery representation x*2^64.
	// Emulated field operations use ordinary residues, so convert both ways.
	montgomery := new(big.Int).Lsh(big.NewInt(1), 64)
	inverse := new(big.Int).ModInverse(montgomery, new(gold).Modulus())
	for round := 0; round < 5; round++ {
		for i := 0; i < 4; i++ {
			raw := canonicalBits(h.api, h.f, h.f.MulConst(state[i], montgomery))
			mapped := make([]frontend.Variable, 64)
			for j := 0; j < 8; j++ {
				index := h.api.FromBinary(raw[j*8 : (j+1)*8]...)
				value := h.lookup.Lookup(index)[0]
				copy(mapped[j*8:(j+1)*8], h.api.ToBinary(value, 8))
			}
			state[i] = h.f.MulConst(h.f.FromBits(mapped...), inverse)
		}
		for i := 4; i < 16; i++ {
			square := h.f.Mul(state[i], state[i])
			fourth := h.f.Mul(square, square)
			state[i] = h.f.Mul(h.f.Mul(square, fourth), state[i])
		}
		var next [16]*goldElement
		for row := 0; row < 16; row++ {
			next[row] = h.f.NewElement(tip5RoundConstants[round*16+row])
			for col := 0; col < 16; col++ {
				next[row] = h.f.Add(next[row], h.f.MulConst(state[col], new(big.Int).SetUint64(tip5MDS[(16+row-col)%16])))
			}
			next[row] = h.f.Reduce(next[row])
		}
		state = next
	}
	return state
}
