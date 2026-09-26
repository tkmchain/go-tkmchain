package antartical

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrInvalidCrossChainMessage = errors.New("invalid Antartical cross-chain message")

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
