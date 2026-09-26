package antartical

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrInvalidOracleObservation = errors.New("invalid Antartical oracle observation")

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

// DeriveRandomness binds the block randomness to the parent mix, block hash,
// and slot. It is deterministic and cannot be selected independently by an
// execution client.
func DeriveRandomness(parentMix, blockHash common.Hash, slot uint64) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM-RANDOMNESS-1"), parentMix.Bytes(), blockHash.Bytes(), new(big.Int).SetUint64(slot).Bytes())
}
