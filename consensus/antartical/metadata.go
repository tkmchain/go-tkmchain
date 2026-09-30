package antartical

// Versioned block metadata for the Antartical transition.  The historical
// Ethereum/TKM header and receipt RLP encodings remain unchanged; this record
// is carried in the versioned extra-data envelope after activation.

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidHeaderMetadata = errors.New("invalid Antartical header metadata")
)

var headerMetadataMagic = []byte("TKM_ANTARTICAL_META_V1")

// HeaderMetadata is the consensus metadata associated with one Antartical
// block.  Commitments are domain-separated hashes.  A domain with no entries
// uses EmptyCommitment(domain), never an all-zero hash, so omission and an
// empty set cannot be confused.
type HeaderMetadata struct {
	Version            uint64
	GasLimits          ProtocolGasVector
	StateWitness       common.Hash
	AssetRegistry      common.Hash
	ConflictTranscript common.Hash
	Randomness         common.Hash
	Oracle             common.Hash
	CrossChain         common.Hash
	PrecompileRegistry common.Hash
	Finality           common.Hash
}

type headerMetadataWire struct {
	Version     uint64
	GasLimits   []uint64
	Commitments []common.Hash
}

func EmptyCommitment(domain string) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_ANTARTICAL_EMPTY_V1"), []byte(domain))
}

// NewHeaderMetadata creates a complete metadata record for a block.  The
// caller supplies the independent resource ceilings; all protocol domains are
// initialized to explicit empty commitments and can then be replaced with
// their block-specific commitments.
func NewHeaderMetadata(limits ProtocolGasVector) HeaderMetadata {
	return HeaderMetadata{
		Version:            uint64(AntarticalProfileVersion),
		GasLimits:          limits,
		StateWitness:       EmptyCommitment("state-witness"),
		AssetRegistry:      EmptyCommitment("asset-registry"),
		ConflictTranscript: EmptyCommitment("conflict-transcript"),
		Randomness:         EmptyCommitment("randomness"),
		Oracle:             EmptyCommitment("oracle"),
		CrossChain:         EmptyCommitment("cross-chain"),
		PrecompileRegistry: EmptyCommitment("precompile-registry"),
		Finality:           EmptyCommitment("finality"),
	}
}

func (m HeaderMetadata) wire() headerMetadataWire {
	return headerMetadataWire{
		Version:     m.Version,
		GasLimits:   []uint64{m.GasLimits.EVM, m.GasLimits.TVM, m.GasLimits.Proof, m.GasLimits.Blob},
		Commitments: []common.Hash{m.StateWitness, m.AssetRegistry, m.ConflictTranscript, m.Randomness, m.Oracle, m.CrossChain, m.PrecompileRegistry, m.Finality},
	}
}

func (m HeaderMetadata) validate() error {
	if m.Version != uint64(AntarticalProfileVersion) {
		return ErrInvalidHeaderMetadata
	}
	// Every dimension is explicit.  Zero is a valid limit for a domain that a
	// block does not use, but the vector itself must always be present.
	if m.StateWitness == (common.Hash{}) || m.AssetRegistry == (common.Hash{}) || m.ConflictTranscript == (common.Hash{}) || m.Randomness == (common.Hash{}) || m.Oracle == (common.Hash{}) || m.CrossChain == (common.Hash{}) || m.PrecompileRegistry == (common.Hash{}) || m.Finality == (common.Hash{}) {
		return ErrInvalidHeaderMetadata
	}
	return nil
}

func (m HeaderMetadata) Validate() error { return m.validate() }

// Commitment is the value a future header commitment or light client uses.
func (m HeaderMetadata) Commitment() (common.Hash, error) {
	if err := m.validate(); err != nil {
		return common.Hash{}, err
	}
	b, err := rlp.EncodeToBytes(m.wire())
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash([]byte("TKM_ANTARTICAL_HEADER_META_V1"), b), nil
}

// Encode serializes metadata as magic || uint32(big-endian length) || RLP.
// A length prefix makes malformed or concatenated extra-data unambiguous.
func (m HeaderMetadata) Encode() ([]byte, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	b, err := rlp.EncodeToBytes(m.wire())
	if err != nil || uint64(len(b)) > uint64(^uint32(0)) {
		return nil, ErrInvalidHeaderMetadata
	}
	out := make([]byte, 0, len(headerMetadataMagic)+4+len(b))
	out = append(out, headerMetadataMagic...)
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(b)))
	out = append(out, size[:]...)
	return append(out, b...), nil
}

func DecodeHeaderMetadata(encoded []byte) (HeaderMetadata, error) {
	if len(encoded) < len(headerMetadataMagic)+4 || !bytes.Equal(encoded[:len(headerMetadataMagic)], headerMetadataMagic) {
		return HeaderMetadata{}, ErrInvalidHeaderMetadata
	}
	start := len(headerMetadataMagic)
	size := int(binary.BigEndian.Uint32(encoded[start : start+4]))
	if size == 0 || start+4+size != len(encoded) {
		return HeaderMetadata{}, ErrInvalidHeaderMetadata
	}
	var wire headerMetadataWire
	if err := rlp.DecodeBytes(encoded[start+4:], &wire); err != nil || len(wire.GasLimits) != 4 || len(wire.Commitments) != 8 {
		return HeaderMetadata{}, ErrInvalidHeaderMetadata
	}
	m := HeaderMetadata{
		Version:      wire.Version,
		GasLimits:    ProtocolGasVector{EVM: wire.GasLimits[0], TVM: wire.GasLimits[1], Proof: wire.GasLimits[2], Blob: wire.GasLimits[3]},
		StateWitness: wire.Commitments[0], AssetRegistry: wire.Commitments[1], ConflictTranscript: wire.Commitments[2], Randomness: wire.Commitments[3],
		Oracle: wire.Commitments[4], CrossChain: wire.Commitments[5], PrecompileRegistry: wire.Commitments[6], Finality: wire.Commitments[7],
	}
	if err := m.validate(); err != nil {
		return HeaderMetadata{}, err
	}
	return m, nil
}

// AttachHeaderMetadata appends exactly one metadata record to extra.  Existing
// bytes are retained verbatim, which preserves all historical metadata.
func AttachHeaderMetadata(extra []byte, metadata HeaderMetadata) ([]byte, error) {
	encoded, err := metadata.Encode()
	if err != nil {
		return nil, err
	}
	// The block-hash anchor is deliberately the final suffix because existing
	// RandomX header validation requires that shape. Insert profile metadata
	// immediately before it when a caller starts from an anchored header.
	if i := bytes.Index(extra, []byte(blockHashAnchorMagic)); i >= 0 && i+BlockHashAnchorEncodedSize == len(extra) {
		prefix := extra[:i]
		if metadataIndex := bytes.LastIndex(prefix, headerMetadataMagic); metadataIndex >= 0 {
			prefix = prefix[:metadataIndex]
		}
		anchor := append([]byte(nil), extra[i:]...)
		result := append(append(append([]byte(nil), prefix...), encoded...), anchor...)
		return result, nil
	}
	if i := bytes.LastIndex(extra, headerMetadataMagic); i >= 0 {
		extra = extra[:i]
	}
	return append(append([]byte(nil), extra...), encoded...), nil
}

// HeaderMetadataFromExtra extracts the final record.  found=false is returned
// for legacy blocks; malformed records are rejected rather than ignored.
func HeaderMetadataFromExtra(extra []byte) (metadata HeaderMetadata, found bool, err error) {
	i := bytes.LastIndex(extra, headerMetadataMagic)
	if i < 0 {
		return HeaderMetadata{}, false, nil
	}
	if bytes.Index(extra, headerMetadataMagic) != i {
		return HeaderMetadata{}, true, ErrInvalidHeaderMetadata
	}
	start := i + len(headerMetadataMagic)
	if len(extra) < start+4 {
		return HeaderMetadata{}, true, ErrInvalidHeaderMetadata
	}
	size := int(binary.BigEndian.Uint32(extra[start : start+4]))
	end := start + 4 + size
	if size == 0 || end > len(extra) {
		return HeaderMetadata{}, true, ErrInvalidHeaderMetadata
	}
	metadata, err = DecodeHeaderMetadata(extra[i:end])
	return metadata, true, err
}
