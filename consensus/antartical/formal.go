package antartical

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// ExecutionClaim is the public statement bound to the zkEVM/Shield3 proof.
// The proof system supplies ProofDigest; consensus compares the complete claim
// digest so a proof cannot be reused for another block or state transition.
type ExecutionClaim struct {
	ParentStateRoot common.Hash
	StateRoot       common.Hash
	Transactions    common.Hash
	Receipts        common.Hash
	ProofDigest     common.Hash
}

func (c ExecutionClaim) Commitment() (common.Hash, error) {
	blob, err := rlp.EncodeToBytes([]interface{}{common.BytesToHash([]byte("TKM-ZKEVM-CLAIM-1")), c.ParentStateRoot, c.StateRoot, c.Transactions, c.Receipts, c.ProofDigest})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}
