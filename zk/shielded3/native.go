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
const protocolMagicV4 = "TKMS4STK"

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

// NativeBackendV4 calls the distinct Shield4 claim program. Its relation is
// full-chain membership in the shared Tip5 tree, with a protocol-specific
// frozen program digest so V3 proofs cannot be replayed as V4 proofs.
type NativeBackendV4 struct{}

func (NativeBackendV4) Verify(ctx context.Context, s StatementV4, proof []byte) error {
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
	request := appendWords([]byte(protocolMagicV4), words)
	request = append(request, proof...)
	out, err := nativeCall(8, request, 3)
	if err != nil {
		return err
	}
	if !bytes.Equal(out, []byte("OK\n")) {
		return ErrInvalidProof
	}
	return nil
}

func privateRequestV4(s StatementV4, w SpendWitness, describe bool) ([]byte, error) {
	var public []uint64
	var err error
	if describe {
		public, err = s.Statement.words()
		if err != nil {
			return nil, err
		}
		public = append(public, make([]uint64, 5)...)
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
	request := appendWords(append(make([]byte, 0, len(protocolMagicV4)+(publicWords+5+secretWords+MerkleDepth*5)*8), protocolMagicV4...), public)
	request = appendWords(request, secret)
	for _, d := range w.allPaths() {
		request = appendWords(request, d[:])
	}
	return request, nil
}

func privateRequestWithMagic(s Statement, w SpendWitness, describe bool, magic string) ([]byte, error) {
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
	request := appendWords(append(make([]byte, 0, len(magic)+(publicWords+secretWords+MerkleDepth*5)*8), magic...), public)
	request = appendWords(request, secret)
	for _, d := range w.allPaths() {
		request = appendWords(request, d[:])
	}
	return request, nil
}
func privateRequest(s Statement, w SpendWitness, describe bool) ([]byte, error) {
	return privateRequestWithMagic(s, w, describe, protocolMagic)
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
func (NativeBackendV4) Prove(ctx context.Context, s StatementV4, w SpendWitness) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := privateRequestV4(s, w, false)
	if err != nil {
		return nil, err
	}
	defer clear(request)
	proof, err := nativeCall(9, request, MaxProofSize)
	if err != nil {
		return nil, err
	}
	if !validProof(proof) {
		return nil, ErrInvalidProof
	}
	return proof, nil
}

func privateTVMWords(s PrivateTVMStatement) ([]uint64, error) {
	if s.ChainID == 0 || s.CodeHash == (Digest{}) || s.Intent == ([64]byte{}) || s.Operation > 1 {
		return nil, ErrInvalidStatement
	}
	words := []uint64{uint64(uint32(s.ChainID)), s.ChainID >> 32}
	words = append(words, s.CodeHash[:]...)
	words = append(words, s.OldRoot[:]...)
	words = append(words, s.NewRoot[:]...)
	for i := 0; i < len(s.Intent); i += 4 {
		words = append(words, uint64(binary.BigEndian.Uint32(s.Intent[i:i+4])))
	}
	words = append(words, uint64(s.Operation))
	if len(words) != privateTVMPublicWords || !canonical(words) {
		return nil, ErrInvalidStatement
	}
	return words, nil
}

// PrivateTVMExpectedWriteValue derives the deterministic hidden value required
// by a private write transition. Wallets use it when constructing the witness;
// validators recompute it inside the native relation.
func PrivateTVMExpectedWriteValue(codeHash, key, oldValue Digest, intent [64]byte) (Digest, error) {
	words := []uint64{privateTVMLeafDomain + 1}
	words = append(words, codeHash[:]...)
	words = append(words, key[:]...)
	words = append(words, oldValue[:]...)
	for i := 0; i < len(intent); i += 4 {
		words = append(words, uint64(binary.BigEndian.Uint32(intent[i:i+4])))
	}
	return HashWords(words)
}

func privateTVMWitnessWords(w PrivateTVMWitness) ([]uint64, [][5]uint64, error) {
	secret := make([]uint64, 0, privateTVMSecretWords)
	secret = append(secret, w.CodeHash[:]...)
	secret = append(secret, w.Key[:]...)
	secret = append(secret, w.OldValue[:]...)
	secret = append(secret, w.NewValue[:]...)
	secret = append(secret, uint64(w.LeafIndex))
	if len(secret) != privateTVMSecretWords || !canonical(secret) {
		return nil, nil, ErrInvalidWitness
	}
	path := make([][5]uint64, len(w.Path))
	for i, d := range w.Path {
		if !canonical(d[:]) {
			return nil, nil, ErrInvalidWitness
		}
		path[i] = d
	}
	return secret, path, nil
}

func privateTVMRequest(s PrivateTVMStatement, w PrivateTVMWitness) ([]byte, error) {
	public, err := privateTVMWords(s)
	if err != nil {
		return nil, err
	}
	secret, path, err := privateTVMWitnessWords(w)
	if err != nil {
		return nil, err
	}
	request := appendWords([]byte(privateTVMMagic), public)
	request = appendWords(request, secret)
	for _, d := range path {
		request = appendWords(request, d[:])
	}
	return request, nil
}

// ProvePrivateTVM creates a real native STARK proof for a private deterministic
// TVM storage transition. Witness bytes are cleared before returning.
func (NativeBackend) ProvePrivateTVM(ctx context.Context, s PrivateTVMStatement, w PrivateTVMWitness) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := privateTVMRequest(s, w)
	if err != nil {
		return nil, err
	}
	defer clear(request)
	proof, err := nativeCall(11, request, MaxProofSize)
	if err != nil {
		return nil, err
	}
	if !validProof(proof) {
		return nil, fmt.Errorf("%w: native prover returned malformed proof", ErrInvalidProof)
	}
	return proof, nil
}

// VerifyPrivateTVM verifies the native STARK without receiving any private
// witness. Invalid or malformed proofs fail closed.
func (NativeBackend) VerifyPrivateTVM(ctx context.Context, s PrivateTVMStatement, proof []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	public, err := privateTVMWords(s)
	if err != nil {
		return err
	}
	if !validProof(proof) {
		return ErrInvalidProof
	}
	request := appendWords([]byte(privateTVMMagic), public)
	request = append(request, proof...)
	out, err := nativeCall(12, request, 3)
	if err != nil {
		return err
	}
	if !bytes.Equal(out, []byte("OK\n")) {
		return fmt.Errorf("%w: native verifier returned unexpected response", ErrInvalidProof)
	}
	return nil
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
	data, err := nativeCall(3, request, 480)
	if err != nil {
		return result, err
	}
	if len(data) != 480 {
		return result, ErrInvalidWitness
	}
	dest := []*Digest{&result.Owner, &result.InputCommitment, &result.Anchor, &result.Nullifier}
	for i := range result.Outputs {
		// Rust describe_spend returns each commitment followed by its
		// one-time key. Preserve that wire order while decoding.
		dest = append(dest, &result.Outputs[i], &result.OneTimeKeys[i])
	}
	for i, d := range dest {
		*d, err = DigestFromBytes(data[i*40 : (i+1)*40])
		if err != nil {
			return DerivedSpend{}, err
		}
	}
	return result, nil
}
func (NativeBackendV4) Describe(ctx context.Context, s StatementV4, w SpendWitness) (DerivedSpend, error) {
	var result DerivedSpend
	if err := ctx.Err(); err != nil {
		return result, err
	}
	request, err := privateRequestV4(s, w, true)
	if err != nil {
		return result, err
	}
	defer clear(request)
	data, err := nativeCall(10, request, 520)
	if err != nil {
		return result, err
	}
	if len(data) != 520 {
		return result, ErrInvalidWitness
	}
	dest := []*Digest{&result.Owner, &result.InputCommitment, &result.Anchor, &result.Nullifier, &result.LinkTag}
	for i := range result.Outputs {
		dest = append(dest, &result.Outputs[i], &result.OneTimeKeys[i])
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

func ownerRequest(chainID uint64, owner Digest, intent [64]byte) ([]byte, error) {
	if chainID == 0 || owner == (Digest{}) || !canonical(owner[:]) {
		return nil, ErrInvalidStatement
	}
	words := []uint64{uint64(uint32(chainID)), chainID >> 32}
	words = append(words, owner[:]...)
	for i := 0; i < len(intent); i += 4 {
		words = append(words, uint64(binary.BigEndian.Uint32(intent[i:i+4])))
	}
	return appendWords([]byte("TKMS3OWN"), words), nil
}
func (NativeBackend) ProveOwner(ctx context.Context, chainID uint64, owner Digest, intent [64]byte, secret Digest) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := ownerRequest(chainID, owner, intent)
	if err != nil {
		return nil, err
	}
	if !canonical(secret[:]) {
		return nil, ErrInvalidWitness
	}
	request = appendWords(request, secret[:])
	defer clear(request)
	proof, err := nativeCall(6, request, MaxProofSize)
	if err != nil {
		return nil, err
	}
	if !validProof(proof) {
		return nil, ErrInvalidProof
	}
	return proof, nil
}
func (NativeBackend) VerifyOwner(ctx context.Context, chainID uint64, owner Digest, intent [64]byte, proof []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	request, err := ownerRequest(chainID, owner, intent)
	if err != nil {
		return err
	}
	if !validProof(proof) {
		return ErrInvalidProof
	}
	out, err := nativeCall(7, append(request, proof...), 3)
	if err != nil {
		return err
	}
	if !bytes.Equal(out, []byte("OK\n")) {
		return ErrInvalidProof
	}
	return nil
}
