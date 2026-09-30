package antartical

import (
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrInvalidOracleObservation = errors.New("invalid Antartical oracle observation")
var ErrInvalidOracleQuorum = errors.New("invalid Antartical oracle quorum")

type OracleObservation struct {
	FeedID    common.Hash
	Round     uint64
	Value     []byte
	Timestamp uint64
	Signer    common.Address
	Signature []byte
}

func (o OracleObservation) Hash(chainID *big.Int) (common.Hash, error) {
	if chainID == nil || chainID.Sign() <= 0 || o.FeedID == (common.Hash{}) || o.Signer == (common.Address{}) || len(o.Value) == 0 {
		return common.Hash{}, ErrInvalidOracleObservation
	}
	blob, err := rlp.EncodeToBytes([]interface{}{common.BytesToHash([]byte("TKM-ORACLE-1")), chainID, o.FeedID, o.Round, o.Value, o.Timestamp, o.Signer})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

func (o OracleObservation) Verify(chainID *big.Int) error {
	hash, err := o.Hash(chainID)
	if err != nil || len(o.Signature) != crypto.SignatureLength {
		return ErrInvalidOracleObservation
	}
	pub, err := crypto.SigToPub(hash.Bytes(), o.Signature)
	if err != nil || crypto.PubkeyToAddress(*pub) != o.Signer {
		return ErrInvalidOracleObservation
	}
	return nil
}

// VerifyQuorum validates a complete feed round. Observations must all refer
// to the same feed, round and value, signers must be unique and sorted, and
// the caller's threshold is evaluated over the supplied committee size. This
// is the deterministic consensus rule; a node must not pick the first valid
// observation it receives from the network.
func VerifyOracleQuorum(chainID *big.Int, observations []OracleObservation, committeeSize, quorumNumerator, quorumDenominator uint64) error {
	if len(observations) == 0 || committeeSize == 0 || quorumDenominator == 0 || uint64(len(observations)) > committeeSize || uint64(len(observations))*quorumDenominator < committeeSize*quorumNumerator {
		return ErrInvalidOracleQuorum
	}
	first := observations[0]
	seen := make(map[common.Address]struct{}, len(observations))
	for i, observation := range observations {
		if i > 0 && (observation.Signer.Hex() < observations[i-1].Signer.Hex() || observation.FeedID != first.FeedID || observation.Round != first.Round || observation.Timestamp != first.Timestamp || string(observation.Value) != string(first.Value)) {
			return ErrInvalidOracleQuorum
		}
		if _, ok := seen[observation.Signer]; ok || observation.Verify(chainID) != nil {
			return ErrInvalidOracleQuorum
		}
		seen[observation.Signer] = struct{}{}
	}
	return nil
}

// SortOracleObservations returns the canonical signer order used by quorum
// encoding. It does not mutate the caller's slice.
func SortOracleObservations(observations []OracleObservation) []OracleObservation {
	out := append([]OracleObservation(nil), observations...)
	sort.Slice(out, func(i, j int) bool { return out[i].Signer.Hex() < out[j].Signer.Hex() })
	return out
}

// DeriveRandomness binds the block randomness to the parent mix, block hash,
// and slot. It is deterministic and cannot be selected independently by an
// execution client.
func DeriveRandomness(parentMix, blockHash common.Hash, slot uint64) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM-RANDOMNESS-1"), parentMix.Bytes(), blockHash.Bytes(), new(big.Int).SetUint64(slot).Bytes())
}
