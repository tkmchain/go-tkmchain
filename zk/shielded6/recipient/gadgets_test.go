package recipient

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"math/big"
	"testing"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/std/math/uints"
	"github.com/consensys/gnark/test"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type sha512TestCircuit struct {
	Data     []uints.U8
	Key      []uints.U8
	Expected [64]uints.U8
	HMAC     bool `gnark:"-"`
}

func (c *sha512TestCircuit) Define(api frontend.API) error {
	u, err := uints.NewBinaryField[uints.U64](api)
	if err != nil {
		return err
	}
	for _, b := range append(append([]uints.U8(nil), c.Data...), c.Key...) {
		api.ToBinary(b.Val, 8)
	}
	var actual []uints.U8
	if c.HMAC {
		actual = hmac512Circuit(u, c.Key, c.Data)
	} else {
		actual = sha512Circuit(u, c.Data)
	}
	assertBytes(api, actual, c.Expected[:])
	return nil
}
func TestRecipientSHA512AndHMAC(t *testing.T) {
	for _, size := range []int{0, 3, 111, 112, 127, 128, 241} {
		data := bytes.Repeat([]byte{byte(size)}, size)
		expected := sha512.Sum512(data)
		assignment := &sha512TestCircuit{Data: uints.NewU8Array(data)}
		for i, v := range expected {
			assignment.Expected[i] = uints.NewU8(v)
		}
		if err := test.IsSolved(&sha512TestCircuit{Data: make([]uints.U8, size)}, assignment, ecc.BN254.ScalarField()); err != nil {
			t.Fatalf("SHA512 length %d: %v", size, err)
		}
	}
	for _, keySize := range []int{16, 64, 129} {
		key, data := bytes.Repeat([]byte{0xa3}, keySize), []byte("recipient binding HMAC vector")
		mac := hmac.New(sha512.New, key)
		mac.Write(data)
		assignment := &sha512TestCircuit{Data: uints.NewU8Array(data), Key: uints.NewU8Array(key), HMAC: true}
		for i, v := range mac.Sum(nil) {
			assignment.Expected[i] = uints.NewU8(v)
		}
		circuit := &sha512TestCircuit{Data: make([]uints.U8, len(data)), Key: make([]uints.U8, keySize), HMAC: true}
		if err := test.IsSolved(circuit, assignment, ecc.BN254.ScalarField()); err != nil {
			t.Fatalf("HMAC key length %d: %v", keySize, err)
		}
		assignment.Expected[0] = uints.NewU8(mac.Sum(nil)[0] ^ 1)
		if err := test.IsSolved(circuit, assignment, ecc.BN254.ScalarField()); err == nil {
			t.Fatal("accepted altered HMAC")
		}
	}
}

type tip5TestCircuit struct {
	Words    []goldElement
	Expected digest
	Pair     bool `gnark:"-"`
}

func (c *tip5TestCircuit) Define(api frontend.API) error {
	h, err := newTip5(api)
	if err != nil {
		return err
	}
	words := make([]*goldElement, len(c.Words))
	for i := range words {
		h.f.AssertIsInRange(&c.Words[i])
		words[i] = &c.Words[i]
	}
	var got digest
	if c.Pair {
		var a, b digest
		copy(a[:], c.Words[:5])
		copy(b[:], c.Words[5:])
		got = h.pair(a, b)
	} else {
		got = h.hash(words...)
	}
	for i := range got {
		h.f.AssertIsEqual(&got[i], &c.Expected[i])
	}
	return nil
}
func TestRecipientTip5MatchesNative(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Tip5 reference")
	}
	for _, size := range []int{1, 8, 10, 25} {
		words := make([]uint64, size)
		assignment := &tip5TestCircuit{Words: make([]goldElement, size)}
		for i := range words {
			words[i] = 0xffffffff00000000 - uint64(i)*0x1234567
			assignment.Words[i] = emulated.ValueOf[gold](words[i])
		}
		expected, err := shielded3.HashWords(words)
		if err != nil {
			t.Fatal(err)
		}
		assignment.Expected = digestAssignment(expected)
		if err := test.IsSolved(&tip5TestCircuit{Words: make([]goldElement, size)}, assignment, ecc.BN254.ScalarField()); err != nil {
			t.Fatalf("Tip5 size %d: %v", size, err)
		}
		if size == 10 {
			var a, b shielded3.Digest
			copy(a[:], words[:5])
			copy(b[:], words[5:])
			expected, err = shielded3.HashPair(a, b)
			if err != nil {
				t.Fatal(err)
			}
			assignment.Pair = true
			assignment.Expected = digestAssignment(expected)
			if err := test.IsSolved(&tip5TestCircuit{Words: make([]goldElement, size), Pair: true}, assignment, ecc.BN254.ScalarField()); err != nil {
				t.Fatalf("Tip5 pair: %v", err)
			}
		}
	}
}

type edwardsTestCircuit struct {
	Point    Point
	Preimage Point
	Expected [32]uints.U8
}

func (c *edwardsTestCircuit) Define(api frontend.API) error {
	e, err := newEdwards(api)
	if err != nil {
		return err
	}
	e.validRecipient(c.Point, c.Preimage)
	assertBytes(api, e.compressed(c.Point), c.Expected[:])
	return nil
}
func TestRecipientEdwardsSubgroupAndEncoding(t *testing.T) {
	var encoded [32]byte
	encoded[0] = 0x58
	for i := 1; i < 32; i++ {
		encoded[i] = 0x66
	}
	point, err := pointAssignment(encoded)
	if err != nil {
		t.Fatal(err)
	}
	preimage, err := subgroupPreimage(encoded)
	if err != nil {
		t.Fatal(err)
	}
	assignment := &edwardsTestCircuit{Point: point, Preimage: preimage}
	set32(&assignment.Expected, encoded)
	if err := test.IsSolved(&edwardsTestCircuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatal(err)
	}
	identity := *assignment
	identity.Point = Point{X: emulated.ValueOf[edwardsBase](0), Y: emulated.ValueOf[edwardsBase](1)}
	if err := test.IsSolved(&edwardsTestCircuit{}, &identity, ecc.BN254.ScalarField()); err == nil {
		t.Fatal("accepted identity")
	}
	torsion := *assignment
	torsion.Point = Point{X: emulated.ValueOf[edwardsBase](0), Y: emulated.ValueOf[edwardsBase](new(big.Int).Sub(new(edwardsBase).Modulus(), big.NewInt(1)))}
	if err := test.IsSolved(&edwardsTestCircuit{}, &torsion, ecc.BN254.ScalarField()); err == nil {
		t.Fatal("accepted small-order point")
	}
	// Adding the order-two point maps (x,y) to (-x,-y), producing a
	// nontrivial mixed-torsion point that is still on the Edwards curve.
	x, _ := new(big.Int).SetString("15112221349535400772501151409588531511454012693041857206046113283949847762202", 10)
	y, _ := new(big.Int).SetString("46316835694926478169428394003475163141307993866256225615783033603165251855960", 10)
	p := new(edwardsBase).Modulus()
	mixed := *assignment
	mixed.Point = Point{X: emulated.ValueOf[edwardsBase](new(big.Int).Sub(p, x)), Y: emulated.ValueOf[edwardsBase](new(big.Int).Sub(p, y))}
	if err := test.IsSolved(&edwardsTestCircuit{}, &mixed, ecc.BN254.ScalarField()); err == nil {
		t.Fatal("accepted mixed-torsion point")
	}
}

type scalarTestCircuit struct {
	Bytes [32]uints.U8
	Valid frontend.Variable
}

func (c *scalarTestCircuit) Define(api frontend.API) error {
	api.AssertIsEqual(canonicalScalar(api, bytesBits(api, c.Bytes[:])), c.Valid)
	return nil
}
func TestRecipientScalarCanonicalBounds(t *testing.T) {
	for _, tc := range []struct {
		value *big.Int
		valid int
	}{
		{new(big.Int), 0}, {big.NewInt(1), 1}, {new(big.Int).Sub(scalarModulus(), big.NewInt(1)), 1}, {scalarModulus(), 0}, {new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)), 0},
	} {
		var bigEndian [32]byte
		tc.value.FillBytes(bigEndian[:])
		c := &scalarTestCircuit{Valid: tc.valid}
		for i := range bigEndian {
			c.Bytes[i] = uints.NewU8(bigEndian[31-i])
		}
		if err := test.IsSolved(&scalarTestCircuit{}, c, ecc.BN254.ScalarField()); err != nil {
			t.Fatalf("scalar %x: %v", tc.value, err)
		}
	}
}

type multiplicationTestCircuit struct {
	Point    Point
	Scalar   [32]uints.U8
	Expected [32]uints.U8
	Fixed    bool `gnark:"-"`
}

func (c *multiplicationTestCircuit) Define(api frontend.API) error {
	e, err := newEdwards(api)
	if err != nil {
		return err
	}
	scalar := bytesBits(api, c.Scalar[:])
	var got Point
	if c.Fixed {
		got = e.mulBase(scalar)
	} else {
		e.onCurve(c.Point)
		got = e.mul(c.Point, scalar)
	}
	assertBytes(api, e.compressed(got), c.Expected[:])
	return nil
}
func TestRecipientWindowedMultiplication(t *testing.T) {
	var encoded [32]byte
	encoded[0] = 0x58
	for i := 1; i < 32; i++ {
		encoded[i] = 0x66
	}
	point, err := pointAssignment(encoded)
	if err != nil {
		t.Fatal(err)
	}
	native, err := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).SetBytes(encoded[:])
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []*big.Int{new(big.Int), big.NewInt(1), big.NewInt(15), big.NewInt(16), big.NewInt(256), new(big.Int).Sub(scalarModulus(), big.NewInt(1))} {
		var scalarBytes [32]byte
		n.FillBytes(scalarBytes[:])
		for i, j := 0, 31; i < j; i, j = i+1, j-1 {
			scalarBytes[i], scalarBytes[j] = scalarBytes[j], scalarBytes[i]
		}
		scalar, err := new(curve25519.Scalar).SetCanonicalBytes(scalarBytes[:])
		if err != nil {
			t.Fatal(err)
		}
		actual := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(scalar, native).Bytes()
		for _, fixed := range []bool{false, true} {
			c := &multiplicationTestCircuit{Point: point, Fixed: fixed}
			set32(&c.Scalar, scalarBytes)
			for i := range actual {
				c.Expected[i] = uints.NewU8(actual[i])
			}
			if err := test.IsSolved(&multiplicationTestCircuit{Fixed: fixed}, c, ecc.BN254.ScalarField()); err != nil {
				t.Fatalf("scalar=%s fixed=%v: %v", n, fixed, err)
			}
		}
	}
}
