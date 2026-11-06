// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package recipient

import (
	"encoding/binary"

	"github.com/consensys/gnark/std/math/uints"
)

// sha512Circuit hashes fixed-length bytes with every compression operation
// constrained. No host hash hint is accepted as a substitute for these gates.
func sha512Circuit(u *uints.BinaryField[uints.U64], message []uints.U8) []uints.U8 {
	padded := append(append([]uints.U8(nil), message...), uints.NewU8(0x80))
	for len(padded)%128 != 112 {
		padded = append(padded, uints.NewU8(0))
	}
	length := make([]byte, 16)
	binary.BigEndian.PutUint64(length[8:], uint64(len(message))*8)
	padded = append(padded, uints.NewU8Array(length)...)
	h := [8]uints.U64{uints.NewU64(0x6a09e667f3bcc908), uints.NewU64(0xbb67ae8584caa73b), uints.NewU64(0x3c6ef372fe94f82b), uints.NewU64(0xa54ff53a5f1d36f1), uints.NewU64(0x510e527fade682d1), uints.NewU64(0x9b05688c2b3e6c1f), uints.NewU64(0x1f83d9abfb41bd6b), uints.NewU64(0x5be0cd19137e2179)}
	for offset := 0; offset < len(padded); offset += 128 {
		var w [80]uints.U64
		for i := 0; i < 16; i++ {
			w[i] = u.PackMSB(padded[offset+i*8 : offset+(i+1)*8]...)
		}
		for i := 16; i < 80; i++ {
			a, b := w[i-15], w[i-2]
			s0 := u.Xor(u.Lrot(a, -1), u.Lrot(a, -8), u.Rshift(a, 7))
			s1 := u.Xor(u.Lrot(b, -19), u.Lrot(b, -61), u.Rshift(b, 6))
			w[i] = u.Add(w[i-16], s0, w[i-7], s1)
		}
		a, b, c, d, e, f, g, v := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]
		for i := 0; i < 80; i++ {
			s1 := u.Xor(u.Lrot(e, -14), u.Lrot(e, -18), u.Lrot(e, -41))
			ch := u.Xor(u.And(e, f), u.And(u.Not(e), g))
			t1 := u.Add(v, s1, ch, uints.NewU64(sha512K[i]), w[i])
			s0 := u.Xor(u.Lrot(a, -28), u.Lrot(a, -34), u.Lrot(a, -39))
			maj := u.Xor(u.And(a, b), u.And(a, c), u.And(b, c))
			t2 := u.Add(s0, maj)
			v, g, f, e, d, c, b, a = g, f, e, u.Add(d, t1), c, b, a, u.Add(t1, t2)
		}
		for i, v := range [8]uints.U64{a, b, c, d, e, f, g, v} {
			h[i] = u.Add(h[i], v)
		}
	}
	out := make([]uints.U8, 0, 64)
	for _, v := range h {
		out = append(out, u.UnpackMSB(v)...)
	}
	return out
}
func hmac512Circuit(u *uints.BinaryField[uints.U64], key, message []uints.U8) []uints.U8 {
	if len(key) > 128 {
		key = sha512Circuit(u, key)
	}
	inner, outer := make([]uints.U8, 128), make([]uints.U8, 128)
	for i := 0; i < 128; i++ {
		v := uints.NewU8(0)
		if i < len(key) {
			v = key[i]
		}
		inner[i] = u.Bytes.Xor(v, uints.NewU8(0x36))
		outer[i] = u.Bytes.Xor(v, uints.NewU8(0x5c))
	}
	hashed := sha512Circuit(u, append(inner, message...))
	return sha512Circuit(u, append(outer, hashed...))
}
