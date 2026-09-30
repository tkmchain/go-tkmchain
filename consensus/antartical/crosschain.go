package antartical

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrInvalidCrossChainMessage = errors.New("invalid Antartical cross-chain message")
var ErrInvalidCrossChainQuorum = errors.New("invalid Antartical cross-chain quorum")

type CrossChainMessage struct {
	SourceChainID      *big.Int
	DestinationChainID *big.Int
	Nonce              uint64
	Sender             common.Address
	Target             common.Address
	Payload            []byte
}

func (m CrossChainMessage) Hash() (common.Hash, error) {
	if m.SourceChainID == nil || m.DestinationChainID == nil || m.SourceChainID.Sign() <= 0 || m.DestinationChainID.Sign() <= 0 || m.Sender == (common.Address{}) || m.Target == (common.Address{}) {
		return common.Hash{}, ErrInvalidCrossChainMessage
	}
	blob, err := rlp.EncodeToBytes([]interface{}{common.BytesToHash([]byte("TKM-XCHAIN-1")), m.SourceChainID, m.DestinationChainID, m.Nonce, m.Sender, m.Target, crypto.Keccak256Hash(m.Payload)})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

func (m CrossChainMessage) ReplayKey() (common.Hash, error) { return m.Hash() }

// SignedCrossChainMessage is the consensus envelope used by a bridge or
// light-client relay. The unsigned message remains useful for deriving the
// replay key, while this wrapper binds authorization to both chain IDs.
type SignedCrossChainMessage struct {
	Message   CrossChainMessage
	Signer    common.Address
	Signature []byte
}

type CrossChainAttestation struct {
	Signer    common.Address
	Signature []byte
}

func (a CrossChainAttestation) Verify(message CrossChainMessage) error {
	if a.Signer == (common.Address{}) || len(a.Signature) != crypto.SignatureLength {
		return ErrInvalidCrossChainQuorum
	}
	digest, err := message.Hash()
	if err != nil {
		return err
	}
	pub, err := crypto.SigToPub(digest.Bytes(), a.Signature)
	if err != nil || crypto.PubkeyToAddress(*pub) != a.Signer {
		return ErrInvalidCrossChainQuorum
	}
	return nil
}

// VerifyCrossChainQuorum enforces canonical signer ordering, uniqueness, and
// the same two-thirds threshold used by validator finality. Committee
// membership is supplied by the state transition, so a relay cannot choose a
// larger committee in its message.
func VerifyCrossChainQuorum(message CrossChainMessage, attestations []CrossChainAttestation, committee map[common.Address]struct{}, numerator, denominator uint64) error {
	if len(attestations) == 0 || len(committee) == 0 || denominator == 0 || numerator > denominator {
		return ErrInvalidCrossChainQuorum
	}
	left := new(big.Int).Mul(new(big.Int).SetUint64(uint64(len(attestations))), new(big.Int).SetUint64(denominator))
	right := new(big.Int).Mul(new(big.Int).SetUint64(uint64(len(committee))), new(big.Int).SetUint64(numerator))
	if left.Cmp(right) < 0 {
		return ErrInvalidCrossChainQuorum
	}
	seen := make(map[common.Address]struct{}, len(attestations))
	for i, attestation := range attestations {
		if i > 0 && string(attestation.Signer.Bytes()) <= string(attestations[i-1].Signer.Bytes()) {
			return ErrInvalidCrossChainQuorum
		}
		if _, ok := committee[attestation.Signer]; !ok {
			return ErrInvalidCrossChainQuorum
		}
		if _, ok := seen[attestation.Signer]; ok || attestation.Verify(message) != nil {
			return ErrInvalidCrossChainQuorum
		}
		seen[attestation.Signer] = struct{}{}
	}
	return nil
}

func (m SignedCrossChainMessage) Verify() error {
	if m.Signer == (common.Address{}) || len(m.Signature) != crypto.SignatureLength {
		return ErrInvalidCrossChainMessage
	}
	digest, err := m.Message.Hash()
	if err != nil {
		return err
	}
	pub, err := crypto.SigToPub(digest.Bytes(), m.Signature)
	if err != nil || crypto.PubkeyToAddress(*pub) != m.Signer {
		return ErrInvalidCrossChainMessage
	}
	return nil
}
