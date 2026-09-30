package antartical

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidFinalityCertificate = errors.New("invalid Antartical finality certificate")
	ErrInvalidFinalityEnvelope    = errors.New("invalid Antartical finality envelope")
)

const finalityEnvelopeMagic = "TKM_ANTARTICAL_FINALITY_V1"

type FinalityCertificate struct {
	Slot          uint64
	BlockHash     common.Hash
	CommitteeSize uint64
	Signers       []common.Address
	PublicKeys    [][]byte
	Signatures    [][]byte
}

func (c FinalityCertificate) Verify(quorumNumerator, quorumDenominator uint64) error {
	committeeSize := c.CommitteeSize
	if committeeSize == 0 {
		committeeSize = uint64(len(c.Signers))
	}
	if c.Slot == 0 || c.BlockHash == (common.Hash{}) || len(c.Signers) == 0 || committeeSize < uint64(len(c.Signers)) || len(c.Signers) != len(c.PublicKeys) || len(c.Signers) != len(c.Signatures) || quorumDenominator == 0 || quorumNumerator > quorumDenominator {
		return ErrInvalidFinalityCertificate
	}
	seen := make(map[common.Address]bool)
	valid := uint64(0)
	for i, signer := range c.Signers {
		if signer == (common.Address{}) || seen[signer] || i > 0 && bytes.Compare(signer.Bytes(), c.Signers[i-1].Bytes()) <= 0 || len(c.PublicKeys[i]) == 0 {
			return ErrInvalidFinalityCertificate
		}
		seen[signer] = true
		if len(c.PublicKeys[i]) == pqcrypto.MLDSA87PublicKeySize {
			address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, c.PublicKeys[i])
			if err != nil || address != signer || !pqcrypto.VerifyMLDSA87(c.PublicKeys[i], c.BlockHash.Bytes(), c.Signatures[i]) {
				return ErrInvalidFinalityCertificate
			}
		} else {
			if len(c.Signatures[i]) != crypto.SignatureLength {
				return ErrInvalidFinalityCertificate
			}
			pub, err := crypto.UnmarshalPubkey(c.PublicKeys[i])
			if err != nil || crypto.PubkeyToAddress(*pub) != signer || !crypto.VerifySignature(c.PublicKeys[i], c.BlockHash.Bytes(), c.Signatures[i][:64]) {
				return ErrInvalidFinalityCertificate
			}
		}
		valid++
	}
	left := new(big.Int).Mul(new(big.Int).SetUint64(valid), new(big.Int).SetUint64(quorumDenominator))
	right := new(big.Int).Mul(new(big.Int).SetUint64(committeeSize), new(big.Int).SetUint64(quorumNumerator))
	if left.Cmp(right) < 0 {
		return ErrInvalidFinalityCertificate
	}
	return nil
}

// EncodeFinalityCertificate serializes a certificate into the versioned
// header-extra envelope. The envelope is deliberately outside historical
// receipt/block-body RLP, so replay of old blocks is unaffected.
func EncodeFinalityCertificate(c FinalityCertificate) ([]byte, error) {
	if err := c.Verify(0, 1); err != nil {
		return nil, err
	}
	payload, err := rlp.EncodeToBytes(c)
	if err != nil || len(payload) == 0 || uint64(len(payload)) > uint64(^uint32(0)) {
		return nil, ErrInvalidFinalityEnvelope
	}
	result := make([]byte, 0, len(finalityEnvelopeMagic)+4+len(payload))
	result = append(result, []byte(finalityEnvelopeMagic)...)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	result = append(result, length[:]...)
	return append(result, payload...), nil
}

func decodeFinalityCertificate(encoded []byte) (FinalityCertificate, error) {
	if len(encoded) < len(finalityEnvelopeMagic)+4 || !bytes.Equal(encoded[:len(finalityEnvelopeMagic)], []byte(finalityEnvelopeMagic)) {
		return FinalityCertificate{}, ErrInvalidFinalityEnvelope
	}
	start := len(finalityEnvelopeMagic)
	size := int(binary.BigEndian.Uint32(encoded[start : start+4]))
	if size == 0 || start+4+size != len(encoded) {
		return FinalityCertificate{}, ErrInvalidFinalityEnvelope
	}
	var c FinalityCertificate
	if err := rlp.DecodeBytes(encoded[start+4:], &c); err != nil || c.Slot == 0 {
		return FinalityCertificate{}, ErrInvalidFinalityEnvelope
	}
	if err := c.Verify(0, 1); err != nil {
		return FinalityCertificate{}, err
	}
	return c, nil
}

// AttachFinalityCertificate inserts one certificate before the block-hash
// anchor (when present), preserving the anchor's required final position.
func AttachFinalityCertificate(extra []byte, c FinalityCertificate) ([]byte, error) {
	encoded, err := EncodeFinalityCertificate(c)
	if err != nil {
		return nil, err
	}
	base, found, err := FinalityCertificateFromHeaderExtra(extra)
	if err != nil {
		return nil, err
	}
	if found {
		extra = base
	}
	if i := bytes.Index(extra, []byte(blockHashAnchorMagic)); i >= 0 && i+BlockHashAnchorEncodedSize == len(extra) {
		return append(append(append([]byte(nil), extra[:i]...), encoded...), extra[i:]...), nil
	}
	return append(append([]byte(nil), extra...), encoded...), nil
}

// FinalityCertificateFromHeaderExtra returns the certificate and the extra
// bytes with the certificate removed. It rejects duplicate or truncated
// envelopes instead of treating them as opaque miner data.
func FinalityCertificateFromHeaderExtra(extra []byte) ([]byte, bool, error) {
	marker := []byte(finalityEnvelopeMagic)
	i := bytes.Index(extra, marker)
	if i < 0 {
		return append([]byte(nil), extra...), false, nil
	}
	if bytes.Index(extra[i+len(marker):], marker) >= 0 {
		return nil, true, ErrInvalidFinalityEnvelope
	}
	start := i + len(marker)
	if len(extra) < start+4 {
		return nil, true, ErrInvalidFinalityEnvelope
	}
	size := int(binary.BigEndian.Uint32(extra[start : start+4]))
	end := start + 4 + size
	if size == 0 || end > len(extra) {
		return nil, true, ErrInvalidFinalityEnvelope
	}
	if _, err := decodeFinalityCertificate(extra[i:end]); err != nil {
		return nil, true, err
	}
	base := append([]byte(nil), extra[:i]...)
	base = append(base, extra[end:]...)
	return base, true, nil
}

func DecodeFinalityCertificateFromHeaderExtra(extra []byte) (FinalityCertificate, bool, error) {
	marker := []byte(finalityEnvelopeMagic)
	i := bytes.Index(extra, marker)
	if i < 0 {
		return FinalityCertificate{}, false, nil
	}
	if bytes.Index(extra[i+len(marker):], marker) >= 0 {
		return FinalityCertificate{}, true, ErrInvalidFinalityEnvelope
	}
	start := i + len(marker)
	if len(extra) < start+4 {
		return FinalityCertificate{}, true, ErrInvalidFinalityEnvelope
	}
	size := int(binary.BigEndian.Uint32(extra[start : start+4]))
	end := start + 4 + size
	if size == 0 || end > len(extra) {
		return FinalityCertificate{}, true, ErrInvalidFinalityEnvelope
	}
	c, err := decodeFinalityCertificate(extra[i:end])
	return c, true, err
}

// HeaderFinalityDigest is signed before the certificate is inserted into the
// header. This avoids a circular dependency between a signature and the final
// block hash while still binding the certificate to every consensus header
// field and the network chain ID.
func HeaderFinalityDigest(chainID *big.Int, header *types.Header) (common.Hash, error) {
	if header == nil {
		return common.Hash{}, ErrInvalidFinalityCertificate
	}
	base, _, err := FinalityCertificateFromHeaderExtra(header.Extra)
	if err != nil {
		return common.Hash{}, err
	}
	copyHeader := *header
	copyHeader.Extra = base
	return LightClientFinalityDigest(chainID, copyHeader.Hash())
}

func FinalityCertificateCommitment(c FinalityCertificate) (common.Hash, error) {
	b, err := rlp.EncodeToBytes(c)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash([]byte(finalityEnvelopeMagic), b), nil
}

func SortedSigners(signers []common.Address) []common.Address {
	out := append([]common.Address(nil), signers...)
	sort.Slice(out, func(i, j int) bool { return out[i].Hex() < out[j].Hex() })
	return out
}
