package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type ShieldedV3Status struct {
	Active         bool            `json:"active"`
	NativeVerifier bool            `json:"nativeVerifier"`
	ActivationTime *hexutil.Uint64 `json:"activationTime"`
	MaxSendWei     string          `json:"maxSendWei"`
	MaxRecipients  uint64          `json:"maxRecipients"`
	ViewKeyVersion uint64          `json:"viewKeyVersion"`
	MaxInputs      uint64          `json:"maxInputs"`
	MaxSendTKM     uint64          `json:"maxSendTKM"`
}

func (api *PrivacyAPI) ShieldedV3Status() ShieldedV3Status {
	status := ShieldedV3Status{MaxRecipients: shielded3.OutputSlots - 1, ViewKeyVersion: 2, MaxInputs: shielded3.InputSlots, NativeVerifier: shielded3.NativeAvailable(), MaxSendWei: shielded3.MaxSendWei().String(), MaxSendTKM: shielded3.MaxSendTKM}
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return status
	}
	cfg := api.e.blockchain.Config()
	if cfg.AntarticalTime != nil {
		t := hexutil.Uint64(*cfg.AntarticalTime)
		status.ActivationTime = &t
	}
	head := api.e.blockchain.CurrentBlock()
	status.Active = head != nil && cfg.IsAntartical(head.Number, head.Time) && cfg.IsPrivacyCommitments(head.Number, head.Time)
	return status
}
func (api *PrivacyAPI) ShieldedV3Path(commitment shielded3.Digest) (core.ShieldedV3Path, error) {
	if _, err := shielded3.DigestFromBytes(commitment.Bytes()); err != nil {
		return core.ShieldedV3Path{}, err
	}
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return core.ShieldedV3Path{}, err
	}
	return core.ShieldedV3CommitmentPath(st, commitment)
}
func (api *PrivacyAPI) ShieldedV3Nullifier(nullifier shielded3.Digest) (common.Hash, error) {
	if _, err := shielded3.DigestFromBytes(nullifier.Bytes()); err != nil {
		return common.Hash{}, err
	}
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return common.Hash{}, err
	}
	return core.ShieldedV3NullifierTransaction(st, nullifier), nil
}

type ShieldedV3OutputStatus struct {
	BlockNumber     hexutil.Uint64   `json:"blockNumber"`
	BlockHash       common.Hash      `json:"blockHash"`
	TransactionHash common.Hash      `json:"transactionHash"`
	OutputIndex     hexutil.Uint64   `json:"outputIndex"`
	Commitment      shielded3.Digest `json:"commitment"`
	Incoming        hexutil.Bytes    `json:"incoming"`
	Outgoing        hexutil.Bytes    `json:"outgoing"`
	Stamp           hexutil.Bytes    `json:"stamp"`
}

// Scan data comes exclusively from canonical blocks; no plaintext amount,
// recipient viewing public key, or spending witness is served by this RPC.
func (api *PrivacyAPI) ShieldedV3Outputs(fromBlock, toBlock hexutil.Uint64) ([]ShieldedV3OutputStatus, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return nil, fmt.Errorf("blockchain unavailable")
	}
	from, to := uint64(fromBlock), uint64(toBlock)
	if to < from || to-from >= 64 {
		return nil, fmt.Errorf("Shield3 scan requires an inclusive range of at most 64 blocks")
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return nil, fmt.Errorf("chain head unavailable")
	}
	result := make([]ShieldedV3OutputStatus, 0)
	if from > head.Number.Uint64() {
		return result, nil
	}
	if to > head.Number.Uint64() {
		to = head.Number.Uint64()
	}
	for number := from; number <= to; number++ {
		block := api.e.blockchain.GetBlockByNumber(number)
		if block == nil {
			return nil, fmt.Errorf("canonical scan block %d unavailable", number)
		}
		for _, tx := range block.Transactions() {
			e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			for i, out := range e.Outputs {
				if len(result) >= 512 {
					return nil, fmt.Errorf("Shield3 scan exceeds 512 outputs; reduce the block range")
				}
				result = append(result, ShieldedV3OutputStatus{hexutil.Uint64(number), block.Hash(), tx.Hash(), hexutil.Uint64(i), out.Commitment, common.CopyBytes(out.Incoming), common.CopyBytes(out.Outgoing), common.CopyBytes(out.Stamp)})
			}
		}
		if number == ^uint64(0) {
			break
		}
	}
	return result, nil
}

type ShieldedV3NullifierStatus struct {
	TransactionHash common.Hash `json:"transactionHash"`
	Pending         bool        `json:"pending"`
}

func (api *PrivacyAPI) ShieldedV3NullifierStatus(nullifier shielded3.Digest) (ShieldedV3NullifierStatus, error) {
	hash, err := api.ShieldedV3Nullifier(nullifier)
	if err != nil {
		return ShieldedV3NullifierStatus{}, err
	}
	if hash != (common.Hash{}) {
		return ShieldedV3NullifierStatus{TransactionHash: hash}, nil
	}
	if api.e.txPool != nil {
		pending, queued := api.e.txPool.Content()
		for _, group := range []map[common.Address][]*types.Transaction{pending, queued} {
			for _, transactions := range group {
				for _, tx := range transactions {
					if !core.HasShieldedV3Prefix(tx.Data()) {
						continue
					}
					e, _, err := core.DecodeShieldedV3Transaction(tx.Data())
					if err == nil {
						nullifiers, err := core.ShieldedV3Nullifiers(e)
						if err == nil {
							for _, n := range nullifiers {
								if n == nullifier {
									return ShieldedV3NullifierStatus{TransactionHash: tx.Hash(), Pending: true}, nil
								}
							}
						}
					}
				}
			}
		}
	}
	return ShieldedV3NullifierStatus{}, nil
}

func (api *PrivacyAPI) AntarticalStamp(address common.Address) (core.AntarticalStampStatus, error) {
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return core.AntarticalStampStatus{}, err
	}
	return core.AntarticalStampForAddress(st, address)
}
func (api *PrivacyAPI) AntarticalStampPath(owner shielded3.Digest) (core.ShieldedV3Path, error) {
	if _, err := shielded3.DigestFromBytes(owner.Bytes()); err != nil {
		return core.ShieldedV3Path{}, err
	}
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return core.ShieldedV3Path{}, err
	}
	return core.AntarticalStampPath(st, owner)
}

// ShieldedV3Paths constructs all selected note paths from one state snapshot.
func (api *PrivacyAPI) ShieldedV3Paths(commitments []shielded3.Digest) ([]core.ShieldedV3Path, error) {
	if len(commitments) == 0 || len(commitments) > shielded3.InputSlots {
		return nil, fmt.Errorf("requires one to four note commitments")
	}
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return nil, err
	}
	result := make([]core.ShieldedV3Path, len(commitments))
	seen := map[shielded3.Digest]bool{}
	for i, c := range commitments {
		if seen[c] {
			return nil, fmt.Errorf("duplicate note commitment")
		}
		seen[c] = true
		if _, err := shielded3.DigestFromBytes(c.Bytes()); err != nil {
			return nil, err
		}
		result[i], err = core.ShieldedV3CommitmentPath(st, c)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (api *PrivacyAPI) ShieldedV3RootsKnown(anchor, stampRoot shielded3.Digest) (bool, error) {
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return false, err
	}
	return st.GetState(params.ShieldedPoolAddress, core.ShieldedV3StateSlot("root", anchor.Bytes())) != (common.Hash{}) && st.GetState(params.ShieldedPoolAddress, core.ShieldedV3StateSlot("stamp/root", stampRoot.Bytes())) != (common.Hash{}), nil
}
