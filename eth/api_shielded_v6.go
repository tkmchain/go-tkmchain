package eth

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

const (
	maxShieldedV6OutputPage       = uint64(256)
	maxShieldedV6WalletOutputPage = uint64(64)
	maxShieldedV6KeyImagePage     = 256
	maxShieldedV6PendingImages    = 16_384
)

type ShieldedV6Status struct {
	Active                     bool            `json:"active"`
	ChainID                    *hexutil.Big    `json:"chainId,omitempty"`
	ActivationTime             *hexutil.Uint64 `json:"activationTime,omitempty"`
	Accumulator                common.Hash     `json:"accumulatorRoot"`
	OutputCount                hexutil.Uint64  `json:"outputCount"`
	SpentKeyImageCount         hexutil.Uint64  `json:"spentKeyImageCount"`
	RingSize                   uint64          `json:"ringSize"`
	MaxInputs                  uint64          `json:"maxInputs"`
	MaxOutputs                 uint64          `json:"maxOutputs"`
	RecipientProofsRequired    bool            `json:"recipientProofsRequired"`
	RecipientKeyHash           common.Hash     `json:"recipientKeyHash"`
	RegistrationKeyHash        common.Hash     `json:"registrationKeyHash"`
	BindingCount               hexutil.Uint64  `json:"bindingCount"`
	HeadTime                   hexutil.Uint64  `json:"headTime"`
	RecipientProvingKeyHash    common.Hash     `json:"recipientProvingKeyHash"`
	RegistrationProvingKeyHash common.Hash     `json:"registrationProvingKeyHash"`
}

type ShieldedV6OutputView struct {
	Index      hexutil.Uint64 `json:"index"`
	OneTimeKey hexutil.Bytes  `json:"oneTimeKey"`
	Commitment hexutil.Bytes  `json:"commitment"`
}

type ShieldedV6WalletOutputView struct {
	AccumulatorIndex hexutil.Uint64   `json:"accumulatorIndex"`
	OutputPosition   hexutil.Uint64   `json:"outputPosition"`
	BlockNumber      hexutil.Uint64   `json:"blockNumber"`
	BlockHash        common.Hash      `json:"blockHash"`
	TransactionHash  common.Hash      `json:"transactionHash"`
	TransactionIndex hexutil.Uint64   `json:"transactionIndex"`
	OneTimeKey       hexutil.Bytes    `json:"oneTimeKey"`
	EphemeralKey     hexutil.Bytes    `json:"ephemeralKey"`
	Commitment       hexutil.Bytes    `json:"commitment"`
	AmountLimbs      [2]hexutil.Bytes `json:"amountLimbs"`
	EncryptedNote    hexutil.Bytes    `json:"encryptedNote"`
}

// ShieldedV6Status reports the activation gate and public accumulator state.
// It contains no output owner, amount, or decrypted note information.
func (api *PrivacyAPI) ShieldedV6Status() (ShieldedV6Status, error) {
	status := ShieldedV6Status{RingSize: shielded6.RingSize, MaxInputs: core.ShieldedV6MaxInputs, MaxOutputs: core.ShieldedV6MaxOutputs}
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return status, fmt.Errorf("blockchain unavailable")
	}
	cfg := api.e.blockchain.Config()
	if cfg.ChainID != nil {
		chainID := hexutil.Big(*new(big.Int).Set(cfg.ChainID))
		status.ChainID = &chainID
	}
	if cfg.Shield6Time != nil {
		activation := hexutil.Uint64(*cfg.Shield6Time)
		status.ActivationTime = &activation
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return status, fmt.Errorf("chain head unavailable")
	}
	status.Active = cfg.IsShield6(head.Number, head.Time)
	status.HeadTime = hexutil.Uint64(head.Time)
	status.RecipientProofsRequired = cfg.IsShield6Recipients(head.Number, head.Time)
	status.RecipientKeyHash = cfg.Shield6RecipientKeyHash
	status.RegistrationKeyHash = cfg.Shield6RegistrationKeyHash
	status.RecipientProvingKeyHash = cfg.Shield6RecipientProvingKeyHash
	status.RegistrationProvingKeyHash = cfg.Shield6RegistrationProvingKeyHash
	state, err := api.e.currentPrivacyState()
	if err != nil {
		return status, err
	}
	status.OutputCount = hexutil.Uint64(core.ShieldedV6OutputCount(state))
	status.SpentKeyImageCount = hexutil.Uint64(core.ShieldedV6KeyImageCount(state))
	status.Accumulator = core.ShieldedV6OutputRoot(state)
	status.BindingCount = hexutil.Uint64(core.ShieldedV6BindingCount(state))
	return status, nil
}

// ShieldedV6BindingPath is used by receivers to create their private payment
// code. Payers carry that path in the code and never query a receiver owner.
func (api *PrivacyAPI) ShieldedV6BindingPath(owner shielded3.Digest) (core.ShieldedV3Path, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return core.ShieldedV3Path{}, fmt.Errorf("blockchain unavailable")
	}
	if _, err := shielded3.DigestFromBytes(owner.Bytes()); err != nil {
		return core.ShieldedV3Path{}, err
	}
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return core.ShieldedV3Path{}, err
	}
	return core.ShieldedV6BindingPath(st, owner)
}

func (api *PrivacyAPI) ShieldedV6RecipientRootsKnown(roots []core.ShieldedV6RecipientRoots) (bool, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return false, fmt.Errorf("blockchain unavailable")
	}
	if len(roots) == 0 || len(roots) > core.ShieldedV6MaxOutputs {
		return false, fmt.Errorf("invalid recipient root count")
	}
	st, err := api.e.currentPrivacyState()
	if err != nil {
		return false, err
	}
	cfg := api.e.blockchain.Config()
	if cfg.ChainID == nil || !cfg.ChainID.IsUint64() {
		return false, fmt.Errorf("invalid chain ID")
	}
	for _, r := range roots {
		if _, err := shielded3.DigestFromBytes(r.StampRoot.Bytes()); err != nil {
			return false, err
		}
		if _, err := shielded3.DigestFromBytes(r.BindingRoot.Bytes()); err != nil {
			return false, err
		}
		if r.Anchor != core.ShieldedV6RecipientAnchor(cfg.ChainID.Uint64(), r.StampRoot, r.BindingRoot) ||
			st.GetState(params.ShieldedPoolAddress, core.ShieldedV3StateSlot("stamp/root", r.StampRoot.Bytes())) == (common.Hash{}) ||
			st.GetState(params.ShieldedPoolAddress, core.ShieldedV3StateSlot("shield6-binding/root", r.BindingRoot.Bytes())) == (common.Hash{}) {
			return false, nil
		}
	}
	return true, nil
}

// ShieldedV6SpentKeyImages returns a bounded page of canonical key images.
// Key images are already public in Shield6 spends; exposing them as a public
// set lets wallets match their own notes locally without telling this RPC
// which outputs they control.
func (api *PrivacyAPI) ShieldedV6SpentKeyImages(start, limit hexutil.Uint64) ([]hexutil.Bytes, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return nil, fmt.Errorf("blockchain unavailable")
	}
	if limit == 0 || uint64(limit) > maxShieldedV6KeyImagePage {
		return nil, fmt.Errorf("Shield6 spent-key-image page limit must be between 1 and %d", maxShieldedV6KeyImagePage)
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return nil, fmt.Errorf("chain head unavailable")
	}
	if !api.e.blockchain.Config().IsShield6(head.Number, head.Time) {
		return nil, fmt.Errorf("Shield6 is not active")
	}
	state, err := api.e.currentPrivacyState()
	if err != nil {
		return nil, err
	}
	count := core.ShieldedV6KeyImageCount(state)
	from := uint64(start)
	if from > count {
		return nil, fmt.Errorf("Shield6 spent-key-image start %d exceeds count %d", from, count)
	}
	end := from + uint64(limit)
	if end < from || end > count {
		end = count
	}
	images := make([]hexutil.Bytes, 0, end-from)
	for index := from; index < end; index++ {
		image, err := core.ShieldedV6KeyImageAt(state, index)
		if err != nil {
			return nil, fmt.Errorf("read Shield6 key image %d: %w", index, err)
		}
		images = append(images, common.CopyBytes(image[:]))
	}
	return images, nil
}

// ShieldedV6PendingKeyImages returns the public key images currently present
// in this node's transaction pool. Wallets can exclude pending spends locally
// without submitting wallet-derived key images to the node.
func (api *PrivacyAPI) ShieldedV6PendingKeyImages() ([]hexutil.Bytes, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return nil, fmt.Errorf("blockchain unavailable")
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil || !api.e.blockchain.Config().IsShield6(head.Number, head.Time) {
		return nil, fmt.Errorf("Shield6 is not active")
	}
	if api.e.txPool == nil {
		return []hexutil.Bytes{}, nil
	}
	pending, queued := api.e.txPool.Content()
	images := make(map[[shielded6.PointSize]byte]struct{})
	for _, group := range []map[common.Address][]*types.Transaction{pending, queued} {
		for _, transactions := range group {
			for _, tx := range transactions {
				e, recognized, err := core.DecodeShieldedV6Transaction(tx.Data())
				if err != nil || !recognized || e == nil {
					continue
				}
				for _, input := range e.Inputs {
					if shielded6.ValidatePublicKey(input.KeyImage[:]) != nil {
						continue
					}
					images[input.KeyImage] = struct{}{}
					if len(images) > maxShieldedV6PendingImages {
						return nil, fmt.Errorf("Shield6 pending-key-image set exceeds the RPC limit of %d", maxShieldedV6PendingImages)
					}
				}
			}
		}
	}
	ordered := make([][shielded6.PointSize]byte, 0, len(images))
	for image := range images {
		ordered = append(ordered, image)
	}
	sort.Slice(ordered, func(i, j int) bool { return bytes.Compare(ordered[i][:], ordered[j][:]) < 0 })
	result := make([]hexutil.Bytes, len(ordered))
	for i := range ordered {
		result[i] = common.CopyBytes(ordered[i][:])
	}
	return result, nil
}

// ShieldedV6Outputs returns a bounded page of public decoy candidates from
// the canonical output accumulator. Callers should fetch candidate ranges and
// select the real-member ring locally; never send the real index to an RPC
// service for server-side selection.
func (api *PrivacyAPI) ShieldedV6Outputs(start, limit hexutil.Uint64) ([]ShieldedV6OutputView, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return nil, fmt.Errorf("blockchain unavailable")
	}
	if limit == 0 || uint64(limit) > maxShieldedV6OutputPage {
		return nil, fmt.Errorf("Shield6 output page limit must be between 1 and %d", maxShieldedV6OutputPage)
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return nil, fmt.Errorf("chain head unavailable")
	}
	cfg := api.e.blockchain.Config()
	if !cfg.IsShield6(head.Number, head.Time) {
		return nil, fmt.Errorf("Shield6 is not active")
	}
	state, err := api.e.currentPrivacyState()
	if err != nil {
		return nil, err
	}
	count := core.ShieldedV6OutputCount(state)
	from := uint64(start)
	if from > count {
		return nil, fmt.Errorf("Shield6 output start %d exceeds accumulator count %d", from, count)
	}
	end := from + uint64(limit)
	if end < from || end > count {
		end = count
	}
	outputs := make([]ShieldedV6OutputView, 0, end-from)
	for index := from; index < end; index++ {
		member, err := core.ShieldedV6OutputAt(state, index)
		if err != nil {
			return nil, fmt.Errorf("read Shield6 accumulator output %d: %w", index, err)
		}
		outputs = append(outputs, ShieldedV6OutputView{
			Index: hexutil.Uint64(index), OneTimeKey: common.CopyBytes(member.OneTimeKey[:]),
			Commitment: common.CopyBytes(member.Commitment[:]),
		})
	}
	return outputs, nil
}

// ShieldedV6OutputsByIndex resolves a bounded set of decoy candidates in the
// caller's randomized request order. It does not choose rings server-side and
// rejects duplicate indices so malformed clients cannot disguise a short ring
// as a full batch.
func (api *PrivacyAPI) ShieldedV6OutputsByIndex(indices []hexutil.Uint64) ([]ShieldedV6OutputView, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return nil, fmt.Errorf("blockchain unavailable")
	}
	if len(indices) == 0 || uint64(len(indices)) > maxShieldedV6OutputPage {
		return nil, fmt.Errorf("Shield6 indexed output batch must contain between 1 and %d indices", maxShieldedV6OutputPage)
	}
	seen := make(map[uint64]struct{}, len(indices))
	for i, encoded := range indices {
		index := uint64(encoded)
		if _, duplicate := seen[index]; duplicate {
			return nil, fmt.Errorf("duplicate Shield6 output index at position %d", i)
		}
		seen[index] = struct{}{}
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return nil, fmt.Errorf("chain head unavailable")
	}
	if !api.e.blockchain.Config().IsShield6(head.Number, head.Time) {
		return nil, fmt.Errorf("Shield6 is not active")
	}
	state, err := api.e.currentPrivacyState()
	if err != nil {
		return nil, err
	}
	count := core.ShieldedV6OutputCount(state)
	outputs := make([]ShieldedV6OutputView, len(indices))
	for position, encoded := range indices {
		index := uint64(encoded)
		if index >= count {
			return nil, fmt.Errorf("Shield6 output index %d exceeds accumulator count %d", index, count)
		}
		member, err := core.ShieldedV6OutputAt(state, index)
		if err != nil {
			return nil, fmt.Errorf("read Shield6 accumulator output %d: %w", index, err)
		}
		outputs[position] = ShieldedV6OutputView{
			Index: hexutil.Uint64(index), OneTimeKey: common.CopyBytes(member.OneTimeKey[:]),
			Commitment: common.CopyBytes(member.Commitment[:]),
		}
	}
	return outputs, nil
}

// ShieldedV6WalletOutputs returns encrypted note records in accumulator order.
// Values and owners remain encrypted; wallet holders scan locally using their
// view keys. The endpoint resolves each record through a canonical transaction
// lookup rather than copying ciphertext into consensus state.
func (api *PrivacyAPI) ShieldedV6WalletOutputs(start, limit hexutil.Uint64) ([]ShieldedV6WalletOutputView, error) {
	if api == nil || api.e == nil || api.e.blockchain == nil {
		return nil, fmt.Errorf("blockchain unavailable")
	}
	if limit == 0 || uint64(limit) > maxShieldedV6WalletOutputPage {
		return nil, fmt.Errorf("Shield6 wallet output page limit must be between 1 and %d", maxShieldedV6WalletOutputPage)
	}
	head := api.e.blockchain.CurrentBlock()
	if head == nil {
		return nil, fmt.Errorf("chain head unavailable")
	}
	cfg := api.e.blockchain.Config()
	if !cfg.IsShield6(head.Number, head.Time) {
		return nil, fmt.Errorf("Shield6 is not active")
	}
	state, err := api.e.currentPrivacyState()
	if err != nil {
		return nil, err
	}
	count := core.ShieldedV6OutputCount(state)
	from := uint64(start)
	if from > count {
		return nil, fmt.Errorf("Shield6 output start %d exceeds accumulator count %d", from, count)
	}
	end := from + uint64(limit)
	if end < from || end > count {
		end = count
	}
	type transactionRecord struct {
		blockNumber uint64
		blockHash   common.Hash
		txIndex     uint64
		outputs     []core.ShieldedV6Output
	}
	transactions := make(map[common.Hash]transactionRecord)
	result := make([]ShieldedV6WalletOutputView, 0, end-from)
	for index := from; index < end; index++ {
		txHash, err := core.ShieldedV6OutputTransactionAt(state, index)
		if err != nil {
			return nil, fmt.Errorf("resolve Shield6 output %d: %w", index, err)
		}
		record, ok := transactions[txHash]
		if !ok {
			lookup, tx := api.e.blockchain.GetCanonicalTransaction(txHash)
			if lookup == nil || tx == nil {
				return nil, fmt.Errorf("canonical transaction for Shield6 output %d is unavailable", index)
			}
			block := api.e.blockchain.GetBlockByHash(lookup.BlockHash)
			if block == nil || block.NumberU64() != lookup.BlockIndex {
				return nil, fmt.Errorf("canonical block for Shield6 output %d is unavailable", index)
			}
			envelope, recognized, err := core.DecodeShieldedV6Transaction(tx.Data())
			if err != nil || !recognized || envelope == nil {
				return nil, fmt.Errorf("malformed canonical Shield6 transaction for output %d", index)
			}
			record = transactionRecord{blockNumber: lookup.BlockIndex, blockHash: lookup.BlockHash, txIndex: lookup.Index, outputs: envelope.Outputs}
			transactions[txHash] = record
		}
		member, err := core.ShieldedV6OutputAt(state, index)
		if err != nil {
			return nil, err
		}
		localIndex := -1
		for i, output := range record.outputs {
			if output.OneTimeKey == member.OneTimeKey && output.Commitment == member.Commitment {
				localIndex = i
				break
			}
		}
		if localIndex < 0 {
			return nil, fmt.Errorf("Shield6 accumulator output %d is missing from its canonical transaction", index)
		}
		output := record.outputs[localIndex]
		result = append(result, ShieldedV6WalletOutputView{
			AccumulatorIndex: hexutil.Uint64(index), BlockNumber: hexutil.Uint64(record.blockNumber),
			OutputPosition: hexutil.Uint64(localIndex),
			BlockHash:      record.blockHash, TransactionHash: txHash, TransactionIndex: hexutil.Uint64(record.txIndex),
			OneTimeKey: common.CopyBytes(output.OneTimeKey[:]), EphemeralKey: common.CopyBytes(output.EphemeralKey[:]),
			Commitment:    common.CopyBytes(output.Commitment[:]),
			AmountLimbs:   [2]hexutil.Bytes{common.CopyBytes(output.AmountLimbs[0][:]), common.CopyBytes(output.AmountLimbs[1][:])},
			EncryptedNote: common.CopyBytes(output.EncryptedNote),
		})
	}
	return result, nil
}
