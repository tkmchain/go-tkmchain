package antartical

import (
	"bytes"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// LightClientFinalityDigest is the chain-bound message signed by a finality
// certificate used in StatelessLightClientProof. A certificate from another
// TKM network cannot be replayed by changing only the advertised chain ID.
func LightClientFinalityDigest(chainID *big.Int, blockHash common.Hash) (common.Hash, error) {
	if chainID == nil || chainID.Sign() <= 0 || chainID.BitLen() > 256 || blockHash == (common.Hash{}) {
		return common.Hash{}, ErrInvalidLightClientProof
	}
	blob, err := rlp.EncodeToBytes([]interface{}{[]byte("TKM_LIGHT_CLIENT_V1"), chainID, blockHash})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

// StatelessLightClientProof binds a canonical witness to a finalized block.
// The finality certificate signs a chain-bound digest of the block hash; the witness root and
// commitment are checked locally, so a light client does not need the full
// state database.
type StatelessLightClientProof struct {
	ChainID           *big.Int
	BlockNumber       uint64
	BlockHash         common.Hash
	StateRoot         common.Hash
	WitnessCommitment common.Hash
	Witness           StateWitness
	Certificate       FinalityCertificate
}

func (p StatelessLightClientProof) Verify(quorumNumerator, quorumDenominator uint64) error {
	finalityDigest, err := LightClientFinalityDigest(p.ChainID, p.BlockHash)
	if err != nil || p.StateRoot == (common.Hash{}) || p.WitnessCommitment == (common.Hash{}) || p.Witness.Root != p.StateRoot || p.Certificate.Slot != p.BlockNumber || p.Certificate.BlockHash != finalityDigest {
		return ErrInvalidLightClientProof
	}
	if !p.Witness.VerifyCanonical(p.WitnessCommitment) {
		return ErrInvalidLightClientProof
	}
	if err := p.Certificate.Verify(quorumNumerator, quorumDenominator); err != nil {
		return ErrInvalidLightClientProof
	}
	return nil
}

func (p StatelessLightClientProof) VerifyAtVersion(version uint8, quorumNumerator, quorumDenominator uint64) error {
	if !VersionedMetadataActive(version) {
		return ErrProfileInactive
	}
	return p.Verify(quorumNumerator, quorumDenominator)
}

// StateWitness is the transport-independent witness used by stateless
// execution. CanonicalCommitment sorts nodes so peers produce the same root
// regardless of receive order; Commitment preserves the historical encoding.
type StateWitness struct {
	Root  common.Hash
	Nodes [][]byte
}

func (w StateWitness) Commitment() (common.Hash, error) {
	blob, err := rlp.EncodeToBytes([]interface{}{w.Root, w.Nodes})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash([]byte("TKM-STATE-WITNESS-1"), blob), nil
}

func (w StateWitness) Verify(expected common.Hash) bool {
	commitment, err := w.Commitment()
	return err == nil && commitment == expected
}

// CommitmentAtVersion selects the historical witness encoding before
// Antartical and the canonical sorted encoding after activation.
func (w StateWitness) CommitmentAtVersion(version uint8) (common.Hash, error) {
	if VersionedMetadataActive(version) {
		return w.CanonicalCommitment()
	}
	return w.Commitment()
}

// CanonicalCommitment is the versioned witness format for the new
// stateless/light-client path. Commitment keeps its historical ordering for
// replay compatibility; new headers and light-client proofs should use this
// method so equivalent node sets cannot produce different commitments.
func (w StateWitness) CanonicalCommitment() (common.Hash, error) {
	nodes := make([][]byte, len(w.Nodes))
	for i, node := range w.Nodes {
		nodes[i] = append([]byte(nil), node...)
	}
	sort.Slice(nodes, func(i, j int) bool { return bytes.Compare(nodes[i], nodes[j]) < 0 })
	blob, err := rlp.EncodeToBytes([]interface{}{common.BytesToHash([]byte("TKM-STATE-WITNESS-V2")), w.Root, nodes})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

func (w StateWitness) VerifyCanonical(expected common.Hash) bool {
	commitment, err := w.CanonicalCommitment()
	return err == nil && commitment == expected
}
