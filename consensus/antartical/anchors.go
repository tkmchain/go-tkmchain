package antartical

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

const blockHashAnchorMagic = "TKM_BLOCK_HASH_ANCHOR_V1"

// BlockHashAnchorEncodedSize is the fixed size of the suffix carried by an
// Antartical block header: a domain marker, the anchored height, the parent
// hash, and the rolling commitment.
const BlockHashAnchorEncodedSize = len(blockHashAnchorMagic) + 8 + common.HashLength + common.HashLength

var (
	ErrInvalidBlockHashAnchor = errors.New("invalid TKM block-hash anchor")
	ErrMissingBlockHashAnchor = errors.New("missing TKM block-hash anchor")
	ErrAnchorExtraOverflow    = errors.New("TKM block-hash anchor does not fit header extra data")
)

// BlockHashAnchor is the consensus metadata for one block. Height and Hash
// identify the block being anchored (the current header's parent); Rolling is
// a domain-separated chain commitment that makes the complete anchor history
// tamper evident.
type BlockHashAnchor struct {
	Height  uint64
	Hash    common.Hash
	Rolling common.Hash
}

// BlockHashAnchorCommitment computes the next rolling commitment. The height
// is included so the same parent hash cannot be moved to another position.
func BlockHashAnchorCommitment(previous common.Hash, height uint64, blockHash common.Hash) common.Hash {
	var heightBytes [8]byte
	binary.BigEndian.PutUint64(heightBytes[:], height)
	return crypto.Keccak256Hash([]byte(blockHashAnchorMagic), previous[:], heightBytes[:], blockHash[:])
}

// ValidateBlockHashAnchor checks the deterministic relation between a header,
// its parent, and the previous rolling commitment.
func ValidateBlockHashAnchor(anchor BlockHashAnchor, blockNumber uint64, parentHash, previousRolling common.Hash) error {
	if blockNumber == 0 || anchor.Height != blockNumber-1 || anchor.Hash != parentHash {
		return ErrInvalidBlockHashAnchor
	}
	want := BlockHashAnchorCommitment(previousRolling, anchor.Height, parentHash)
	if anchor.Rolling != want {
		return ErrInvalidBlockHashAnchor
	}
	return nil
}

// BlockHashAnchorFromHeaderExtra parses the mandatory suffix. The marker must
// be the final field so unrelated header metadata cannot be mistaken for an
// anchor. A marker anywhere else is malformed and rejected.
func BlockHashAnchorFromHeaderExtra(extra []byte) (BlockHashAnchor, bool, error) {
	marker := []byte(blockHashAnchorMagic)
	index := bytes.Index(extra, marker)
	if index < 0 {
		return BlockHashAnchor{}, false, nil
	}
	if bytes.Index(extra[index+len(marker):], marker) >= 0 || index+BlockHashAnchorEncodedSize != len(extra) {
		return BlockHashAnchor{}, false, ErrInvalidBlockHashAnchor
	}
	start := index + len(marker)
	anchor := BlockHashAnchor{
		Height:  binary.BigEndian.Uint64(extra[start : start+8]),
		Hash:    common.BytesToHash(extra[start+8 : start+8+common.HashLength]),
		Rolling: common.BytesToHash(extra[start+8+common.HashLength : start+8+common.HashLength+common.HashLength]),
	}
	return anchor, true, nil
}

// AttachBlockHashAnchor replaces a previous anchor suffix or appends one to
// the existing header metadata. At Antartical the envelope is fixed at 128
// bytes; the legacy prefix is never silently truncated.
func AttachBlockHashAnchor(extra []byte, anchor BlockHashAnchor) ([]byte, error) {
	encoded := make([]byte, BlockHashAnchorEncodedSize)
	copy(encoded, blockHashAnchorMagic)
	start := len(blockHashAnchorMagic)
	binary.BigEndian.PutUint64(encoded[start:start+8], anchor.Height)
	copy(encoded[start+8:start+8+common.HashLength], anchor.Hash[:])
	copy(encoded[start+8+common.HashLength:], anchor.Rolling[:])

	base := append([]byte(nil), extra...)
	if index := bytes.Index(base, []byte(blockHashAnchorMagic)); index >= 0 {
		if bytes.Index(base[index+len(blockHashAnchorMagic):], []byte(blockHashAnchorMagic)) >= 0 || index+BlockHashAnchorEncodedSize != len(base) {
			return nil, ErrInvalidBlockHashAnchor
		}
		base = base[:index]
	}
	if uint64(len(base))+uint64(BlockHashAnchorEncodedSize) > params.AntarticalMaximumExtraDataSize {
		return nil, ErrAnchorExtraOverflow
	}
	result := append(base, encoded...)
	return result, nil
}
