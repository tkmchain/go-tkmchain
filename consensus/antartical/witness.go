package antartical

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// StateWitness is the transport-independent witness used by stateless
// execution. Nodes are sorted by their path before commitment so peers produce
// the same root regardless of receive order.
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
