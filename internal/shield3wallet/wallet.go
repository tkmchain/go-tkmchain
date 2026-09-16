package shield3wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type RPC interface {
	CallContext(context.Context, any, string, ...any) error
}
type Note struct {
	Owner      shielded3.Digest `json:"owner"`
	Randomness shielded3.Digest `json:"randomness"`
	ValueWei   string           `json:"valueWei"`
	Recipient  common.Address   `json:"recipient"`
}
type OwnedNote struct {
	Note
	Commitment      shielded3.Digest `json:"commitment"`
	Nullifier       shielded3.Digest `json:"nullifier"`
	TransactionHash common.Hash      `json:"transactionHash"`
}
type HistoryEntry struct {
	Direction string `json:"direction"`
	Note
	Commitment      shielded3.Digest `json:"commitment"`
	TransactionHash common.Hash      `json:"transactionHash"`
	SpentBy         common.Hash      `json:"spentBy"`
	Pending         bool             `json:"pending"`
}
type ScanResult struct {
	History    []HistoryEntry `json:"history"`
	BalanceWei string         `json:"balanceWei"`
	Notes      []OwnedNote    `json:"notes"`
	HeadHash   common.Hash    `json:"headHash"`
	HeadNumber hexutil.Uint64 `json:"headNumber"`
}
type ViewKey struct {
	ChainID  uint64           `json:"chainId"`
	Owner    shielded3.Digest `json:"owner"`
	Incoming hexutil.Bytes    `json:"incoming"`
	Outgoing hexutil.Bytes    `json:"outgoing"`
}

func (i *Identity) ViewKey() ViewKey {
	return ViewKey{i.ChainID, i.Owner, common.CopyBytes(i.IncomingSeed), common.CopyBytes(i.OutgoingSeed)}
}

type scanOutput struct {
	Commitment      shielded3.Digest `json:"commitment"`
	Incoming        hexutil.Bytes    `json:"incoming"`
	Outgoing        hexutil.Bytes    `json:"outgoing"`
	TransactionHash common.Hash      `json:"transactionHash"`
}
type header struct {
	Number    hexutil.Uint64 `json:"number"`
	Timestamp hexutil.Uint64 `json:"timestamp"`
	Hash      common.Hash    `json:"hash"`
}
type status struct {
	Active         bool            `json:"active"`
	NativeVerifier bool            `json:"nativeVerifier"`
	ActivationTime *hexutil.Uint64 `json:"activationTime"`
}

func noteWords(chainID uint64, n Note, domain uint64) ([]uint64, error) {
	value, err := parseAmount(n.ValueWei)
	if err != nil {
		return nil, err
	}
	amount, err := shielded3.AmountFromBig(value)
	if err != nil {
		return nil, err
	}
	words := []uint64{domain, uint64(uint32(chainID)), chainID >> 32, shielded3.AssetTKM, 0}
	words = append(words, n.Owner[:]...)
	if domain == 3002 {
		for _, limb := range amount {
			words = append(words, uint64(limb))
		}
		words = append(words, n.Randomness[:]...)
	} else {
		words = append(words, n.Randomness[:]...)
		for _, limb := range amount {
			words = append(words, uint64(limb))
		}
	}
	return words, nil
}
func NoteCommitment(chainID uint64, n Note) (shielded3.Digest, error) {
	words, err := noteWords(chainID, n, 3002)
	if err != nil {
		return shielded3.Digest{}, err
	}
	return shielded3.HashWords(words)
}
func NoteNullifier(chainID uint64, n Note) (shielded3.Digest, error) {
	words, err := noteWords(chainID, n, 3003)
	if err != nil {
		return shielded3.Digest{}, err
	}
	return shielded3.HashWords(words)
}

// Scanning needs viewing keys only; note ownership still requires the separate
// spending-secret preimage in the proof. Nullifiers use private note openings.
func Scan(ctx context.Context, rpc RPC, view ViewKey) (ScanResult, error) {
	result := ScanResult{BalanceWei: "0", Notes: make([]OwnedNote, 0)}
	if view.ChainID == 0 || len(view.Incoming) != pqcrypto.ShieldedV3ViewKeySize || (len(view.Outgoing) != 0 && len(view.Outgoing) != pqcrypto.ShieldedV3ViewKeySize) {
		return result, errors.New("invalid Shield3 viewing key")
	}
	var chain hexutil.Big
	if err := rpc.CallContext(ctx, &chain, "eth_chainId"); err != nil {
		return result, err
	}
	if (*big.Int)(&chain).Cmp(new(big.Int).SetUint64(view.ChainID)) != 0 {
		return result, errors.New("wallet chain ID mismatch")
	}
	var head header
	if err := rpc.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return result, err
	}
	if head.Hash == (common.Hash{}) {
		return result, errors.New("chain head unavailable")
	}
	result.HeadHash = head.Hash
	result.HeadNumber = head.Number
	var activation status
	if err := rpc.CallContext(ctx, &activation, "tkmprivacy_shieldedV3Status"); err != nil {
		return result, err
	}
	if activation.ActivationTime == nil || head.Timestamp < *activation.ActivationTime {
		return result, nil
	}
	// Locate the fork once, then request bounded batches of encrypted outputs.
	low, high := uint64(0), uint64(head.Number)
	for low < high {
		mid := low + (high-low)/2
		var h header
		if err := rpc.CallContext(ctx, &h, "eth_getBlockByNumber", hexutil.EncodeUint64(mid), false); err != nil {
			return result, err
		}
		if h.Hash == (common.Hash{}) {
			return result, fmt.Errorf("scan block %d unavailable", mid)
		}
		if h.Timestamp < *activation.ActivationTime {
			low = mid + 1
		} else {
			high = mid
		}
	}
	total := new(big.Int)
	seen := make(map[shielded3.Digest]bool)
	for from := low; from <= uint64(head.Number); {
		to := from + 31
		if to < from || to > uint64(head.Number) {
			to = uint64(head.Number)
		}
		var outputs []scanOutput
		for {
			err := rpc.CallContext(ctx, &outputs, "tkmprivacy_shieldedV3Outputs", hexutil.EncodeUint64(from), hexutil.EncodeUint64(to))
			if err == nil {
				break
			}
			if to == from {
				return result, err
			}
			// Busy ranges can exceed the RPC's bounded output response. Retry
			// progressively smaller ranges without advancing the scan cursor.
			to = from + (to-from)/2
		}
		for _, out := range outputs {
			if len(view.Outgoing) != 0 {
				plain, err := pqcrypto.OpenShieldedV3(view.Outgoing, out.Outgoing, core.ShieldedV3OutputContext(view.ChainID, pqcrypto.ShieldedV3Outgoing, out.Commitment))
				if err == nil {
					var note Note
					err = json.Unmarshal(plain, &note)
					clear(plain)
					commitment, commitmentErr := NoteCommitment(view.ChainID, note)
					value, valueErr := parseAmount(note.ValueWei)
					if err == nil && commitmentErr == nil && commitment == out.Commitment && valueErr == nil && value.Sign() > 0 && note.Owner != view.Owner {
						result.History = append(result.History, HistoryEntry{Direction: "outgoing", Note: note, Commitment: commitment, TransactionHash: out.TransactionHash})
					}
				}
			}
			plain, err := pqcrypto.OpenShieldedV3(view.Incoming, out.Incoming, core.ShieldedV3OutputContext(view.ChainID, pqcrypto.ShieldedV3Incoming, out.Commitment))
			if err != nil {
				continue
			}
			var note Note
			err = json.Unmarshal(plain, &note)
			clear(plain)
			if err != nil || note.Owner != view.Owner {
				continue
			}
			commitment, err := NoteCommitment(view.ChainID, note)
			if err != nil || commitment != out.Commitment || seen[commitment] {
				continue
			}
			seen[commitment] = true
			nullifier, err := NoteNullifier(view.ChainID, note)
			if err != nil {
				return result, err
			}
			var spent struct {
				TransactionHash common.Hash `json:"transactionHash"`
				Pending         bool        `json:"pending"`
			}
			if err = rpc.CallContext(ctx, &spent, "tkmprivacy_shieldedV3NullifierStatus", nullifier); err != nil {
				return result, err
			}
			value, err := parseAmount(note.ValueWei)
			if err != nil {
				return result, err
			}
			if value.Sign() == 0 {
				continue
			}
			result.History = append(result.History, HistoryEntry{Direction: "incoming", Note: note, Commitment: commitment, TransactionHash: out.TransactionHash, SpentBy: spent.TransactionHash, Pending: spent.Pending})
			if spent.Pending || spent.TransactionHash != (common.Hash{}) {
				continue
			}
			total.Add(total, value)
			result.Notes = append(result.Notes, OwnedNote{note, commitment, nullifier, out.TransactionHash})
		}
		if to == uint64(head.Number) {
			break
		}
		from = to + 1
	}
	var canonical header
	if err := rpc.CallContext(ctx, &canonical, "eth_getBlockByNumber", hexutil.EncodeUint64(uint64(head.Number)), false); err != nil {
		return result, err
	}
	if canonical.Hash != head.Hash {
		return ScanResult{}, errors.New("chain changed during scan; retry against the canonical chain")
	}
	result.BalanceWei = total.String()
	return result, nil
}

const WalletGas uint64 = 7_000_000

var buildSlot = make(chan struct{}, 1)

// Build constructs and locally verifies a single bounded spend/deposit. It
// never submits a transaction; the caller signs and broadcasts once explicitly.
func Build(ctx context.Context, rpc RPC, seed []byte, identity *Identity, to PaymentPayload, amount *big.Int, deposit bool) (*types.Transaction, error) {
	if amount == nil || amount.Sign() <= 0 || amount.Cmp(shielded3.MaxSendWei()) > 0 {
		return nil, errors.New("Shield3 maximum send is 5000000 TKM")
	}
	if identity == nil || to.ChainID != identity.ChainID {
		return nil, errors.New("Shield3 recipient chain mismatch")
	}
	select {
	case buildSlot <- struct{}{}:
		defer func() { <-buildSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var active status
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
		return nil, err
	}
	if !active.Active || !active.NativeVerifier {
		return nil, errors.New("Shield3 requires an Antartical node with the embedded verifier")
	}
	var nonce hexutil.Uint64
	if err := rpc.CallContext(ctx, &nonce, "eth_getTransactionCount", identity.Address, "pending"); err != nil {
		return nil, err
	}
	var price hexutil.Big
	if err := rpc.CallContext(ctx, &price, "eth_gasPrice"); err != nil {
		return nil, err
	}
	gasPrice := new(big.Int).Set((*big.Int)(&price))
	if gasPrice.Sign() <= 0 {
		return nil, errors.New("gas price unavailable")
	}
	var balance hexutil.Big
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
		return nil, err
	}
	maxGas := new(big.Int).Mul(new(big.Int).SetUint64(WalletGas), gasPrice)
	sponsor := new(big.Int)
	if !deposit && (*big.Int)(&balance).Cmp(maxGas) < 0 {
		sponsor.Set(maxGas)
	}
	required := new(big.Int).Add(amount, sponsor)
	witness := shielded3.SpendWitness{SpendingSecret: identity.SpendingSecret}
	change := new(big.Int)
	envelope := &core.ShieldedV3Transaction{Version: 3, Deposit: deposit, WithdrawalValue: new(big.Int), GasSponsorValue: sponsor}
	if deposit {
		if (*big.Int)(&balance).Cmp(new(big.Int).Add(amount, maxGas)) < 0 {
			return nil, errors.New("public balance cannot cover shielding and gas")
		}
		witness.Value, _ = shielded3.AmountFromBig(amount)
		var err error
		witness.Randomness, err = shielded3.GenerateSecret()
		if err != nil {
			return nil, err
		}
	} else {
		view := identity.ViewKey()
		defer clear(view.Incoming)
		defer clear(view.Outgoing)
		scan, err := Scan(ctx, rpc, view)
		if err != nil {
			return nil, err
		}
		sort.Slice(scan.Notes, func(i, j int) bool {
			a, _ := parseAmount(scan.Notes[i].ValueWei)
			b, _ := parseAmount(scan.Notes[j].ValueWei)
			return a.Cmp(b) < 0
		})
		var chosen *OwnedNote
		for i := range scan.Notes {
			value, _ := parseAmount(scan.Notes[i].ValueWei)
			if value.Cmp(required) >= 0 {
				chosen = &scan.Notes[i]
				break
			}
		}
		if chosen == nil {
			return nil, errors.New("no single spendable Shield3 note covers this amount and fees; shield funds first or consolidate smaller payments")
		}
		var path core.ShieldedV3Path
		if err = rpc.CallContext(ctx, &path, "tkmprivacy_shieldedV3Path", chosen.Commitment); err != nil {
			return nil, err
		}
		if !path.Found || path.Index >= uint64(1)<<32 {
			return nil, errors.New("note is no longer canonical; rescan")
		}
		value, _ := parseAmount(chosen.ValueWei)
		witness.Value, _ = shielded3.AmountFromBig(value)
		witness.Randomness = chosen.Randomness
		witness.LeafIndex = uint32(path.Index)
		witness.MerklePath = path.Path
		envelope.Anchor = path.Root
		envelope.Nullifier = chosen.Nullifier
		change.Sub(value, required)
	}
	for slot := 0; slot < 4; slot++ {
		recipient := to
		value := new(big.Int)
		if slot == 0 {
			value.Set(amount)
		}
		if slot == 3 {
			recipient = PaymentPayload{ChainID: identity.ChainID, Address: identity.Address, Owner: identity.Owner, IncomingPublicKey: identity.IncomingPublicKey, StampPublicKey: identity.StampPublicKey, Stamp: *identity.Stamp}
			value.Set(change)
		}
		random, err := shielded3.GenerateSecret()
		if err != nil {
			return nil, err
		}
		limbs, _ := shielded3.AmountFromBig(value)
		witness.Outputs[slot] = shielded3.OutputOpening{Owner: recipient.Owner, Randomness: random, Value: limbs}
		note := Note{recipient.Owner, random, value.String(), recipient.Address}
		commitment, err := NoteCommitment(identity.ChainID, note)
		if err != nil {
			return nil, err
		}
		envelope.Outputs[slot].Commitment = commitment
		plain, err := json.Marshal(note)
		if err != nil {
			return nil, err
		}
		envelope.Outputs[slot].Incoming, err = pqcrypto.SealShieldedV3(recipient.IncomingPublicKey, plain, core.ShieldedV3OutputContext(identity.ChainID, pqcrypto.ShieldedV3Incoming, commitment))
		if err != nil {
			clear(plain)
			return nil, err
		}
		envelope.Outputs[slot].Outgoing, err = pqcrypto.SealShieldedV3(identity.OutgoingPublicKey, plain, core.ShieldedV3OutputContext(identity.ChainID, pqcrypto.ShieldedV3Outgoing, commitment))
		clear(plain)
		if err != nil {
			return nil, err
		}
		envelope.Outputs[slot].Stamp, err = pqcrypto.SealShieldedV3(recipient.StampPublicKey, recipient.Stamp.Commitment[:], core.ShieldedV3OutputContext(identity.ChainID, pqcrypto.ShieldedV3Stamp, commitment))
		if err != nil {
			return nil, err
		}
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	pool := params.ShieldedPoolAddress
	value := new(big.Int)
	if deposit {
		value.Set(amount)
	}
	makeTx := func() (*types.Transaction, error) {
		data, err := core.EncodeShieldedV3Transaction(envelope)
		if err != nil {
			return nil, err
		}
		return types.NewTx(&types.PQTkmTx{ChainID: new(big.Int).SetUint64(identity.ChainID), Nonce: uint64(nonce), GasTipCap: gasPrice, GasFeeCap: gasPrice, Gas: WalletGas, To: &pool, Value: value, Data: data, Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(key)}), nil
	}
	unsigned, err := makeTx()
	if err != nil {
		return nil, err
	}
	statement, err := core.ShieldedV3Statement(unsigned, envelope)
	if err != nil {
		return nil, err
	}
	envelope.Proof, err = (shielded3.NativeBackend{}).Prove(ctx, statement, witness)
	if err != nil {
		return nil, err
	}
	if err = (shielded3.NativeBackend{}).Verify(ctx, statement, envelope.Proof); err != nil {
		return nil, err
	}
	unsigned, err = makeTx()
	if err != nil {
		return nil, err
	}
	gas, err := core.IntrinsicGasWithShield3(unsigned.Data(), nil, nil, false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > WalletGas {
		return nil, errors.New("Shield3 proof exceeds the wallet gas budget")
	}
	return unsigned, nil
}
