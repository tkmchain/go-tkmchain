package shielded3

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
)

const AssetTKM uint64 = 1
const MaxSendTKM uint64 = 5_000_000

func MaxSendWei() *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(MaxSendTKM), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

func (d Digest) Bytes() []byte { return appendWords(make([]byte, 0, 40), d[:]) }
func DigestFromBytes(data []byte) (Digest, error) {
	var d Digest
	if len(data) != 40 {
		return d, ErrInvalidStatement
	}
	for i := range d {
		d[i] = binary.LittleEndian.Uint64(data[i*8:])
	}
	if !canonical(d[:]) {
		return Digest{}, ErrInvalidStatement
	}
	return d, nil
}
func (d Digest) MarshalJSON() ([]byte, error) {
	return json.Marshal("0x" + hex.EncodeToString(d.Bytes()))
}
func (d *Digest) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if len(value) != 82 || value[:2] != "0x" {
		return ErrInvalidStatement
	}
	raw, err := hex.DecodeString(value[2:])
	if err != nil {
		return err
	}
	decoded, err := DigestFromBytes(raw)
	if err == nil {
		*d = decoded
	}
	return err
}

// NativeBackend executes the pinned proof system inside the node process.
// Verification has deterministic proof/trace bounds and no external service or
// load-dependent timeout. Wallet context cancellation is checked before calls.
type NativeBackend struct{}

func (NativeBackend) Verify(ctx context.Context, s Statement, proof []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	words, err := s.words()
	if err != nil {
		return err
	}
	if !validProof(proof) {
		return ErrInvalidProof
	}
	request := appendWords([]byte(protocolMagic), words)
	request = append(request, proof...)
	out, err := nativeCall(1, request, 3)
	if err != nil {
		return err
	}
	if !bytes.Equal(out, []byte("OK\n")) {
		return ErrInvalidProof
	}
	return nil
}
func privateRequest(s Statement, w SpendWitness, describe bool) ([]byte, error) {
	var public []uint64
	var err error
	if describe {
		public = make([]uint64, publicWords)
		public[0] = uint64(uint32(s.ChainID))
		public[1] = s.ChainID >> 32
		public[2] = uint64(uint32(s.AssetID))
		public[3] = s.AssetID >> 32
		if s.ChainID == 0 {
			return nil, ErrInvalidStatement
		}
	} else {
		public, err = s.words()
		if err != nil {
			return nil, err
		}
	}
	secret, err := w.words()
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	request := appendWords(append(make([]byte, 0, len(protocolMagic)+(publicWords+secretWords+MerkleDepth*5)*8), protocolMagic...), public)
	request = appendWords(request, secret)
	for _, d := range w.MerklePath {
		request = appendWords(request, d[:])
	}
	return request, nil
}
func (NativeBackend) Prove(ctx context.Context, s Statement, w SpendWitness) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := privateRequest(s, w, false)
	if err != nil {
		return nil, err
	}
	defer clear(request)
	proof, err := nativeCall(2, request, MaxProofSize)
	if err != nil {
		return nil, err
	}
	if !validProof(proof) {
		return nil, ErrInvalidProof
	}
	return proof, nil
}
func (NativeBackend) Describe(ctx context.Context, chainID, assetID uint64, w SpendWitness) (DerivedSpend, error) {
	var result DerivedSpend
	if err := ctx.Err(); err != nil {
		return result, err
	}
	request, err := privateRequest(Statement{ChainID: chainID, AssetID: assetID}, w, true)
	if err != nil {
		return result, err
	}
	defer clear(request)
	data, err := nativeCall(3, request, 320)
	if err != nil {
		return result, err
	}
	if len(data) != 320 {
		return result, ErrInvalidWitness
	}
	dest := []*Digest{&result.Owner, &result.InputCommitment, &result.Anchor, &result.Nullifier}
	for i := range result.Outputs {
		dest = append(dest, &result.Outputs[i])
	}
	for i, d := range dest {
		*d, err = DigestFromBytes(data[i*40 : (i+1)*40])
		if err != nil {
			return DerivedSpend{}, err
		}
	}
	return result, nil
}
func HashPair(left, right Digest) (Digest, error) {
	if !canonical(left[:]) || !canonical(right[:]) {
		return Digest{}, ErrInvalidStatement
	}
	input := append(left.Bytes(), right.Bytes()...)
	out, err := nativeCall(4, input, 40)
	if err != nil {
		return Digest{}, err
	}
	return DigestFromBytes(out)
}
func HashWords(words []uint64) (Digest, error) {
	if len(words) == 0 || len(words) > 40 || !canonical(words) {
		return Digest{}, ErrInvalidStatement
	}
	out, err := nativeCall(5, appendWords(nil, words), 40)
	if err != nil {
		return Digest{}, fmt.Errorf("Shield3 hash: %w", err)
	}
	return DigestFromBytes(out)
}

// ValidProofEncoding checks canonical size and field words before native decoding.
func ValidProofEncoding(proof []byte) bool { return validProof(proof) }
