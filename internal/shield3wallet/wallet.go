package shield3wallet

import (
	"bytes"
	"context"
	"crypto/rand"
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
	// AssetID is authenticated by the commitment and defaults to native TKM
	// when omitted in legacy encrypted notes.
	AssetID    uint64           `json:"assetId,omitempty"`
	Owner      shielded3.Digest `json:"owner"`
	Randomness shielded3.Digest `json:"randomness"`
	ValueWei   string           `json:"valueWei"`
	Recipient  common.Address   `json:"recipient"`
	OneTimeKey []byte           `json:"oneTimeKey"`
	PaymentTag []byte           `json:"paymentTag"`
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
	Commitment       shielded3.Digest `json:"commitment"`
	TransactionHash  common.Hash      `json:"transactionHash"`
	SpentBy          common.Hash      `json:"spentBy"`
	Pending          bool             `json:"pending"`
	SpendStatusKnown bool             `json:"spendStatusKnown"`
}
type ScanResult struct {
	SpendStatusKnown bool           `json:"spendStatusKnown"`
	ReceivedWei      string         `json:"receivedWei"`
	History          []HistoryEntry `json:"history"`
	BalanceWei       string         `json:"balanceWei"`
	Notes            []OwnedNote    `json:"notes"`
	HeadHash         common.Hash    `json:"headHash"`
	HeadNumber       hexutil.Uint64 `json:"headNumber"`
}

// A full viewing key identifies spends without authorizing them. Incoming
// keys decrypt receipts only, and deliberately lack the independent nullifier key.
type ViewKey struct {
	Version      uint64            `json:"version"`
	Scope        string            `json:"scope"`
	ChainID      uint64            `json:"chainId"`
	Owner        shielded3.Digest  `json:"owner"`
	Incoming     hexutil.Bytes     `json:"incoming"`
	Outgoing     hexutil.Bytes     `json:"outgoing,omitempty"`
	NullifierKey *shielded3.Digest `json:"nullifierKey,omitempty"`
	AssetID      uint64            `json:"assetId,omitempty"`
}

func (i *Identity) ScopedViewKey(scope string) (ViewKey, error) {
	v := ViewKey{Version: 2, Scope: scope, ChainID: i.ChainID, Owner: i.Owner}
	switch scope {
	case "incoming":
		v.Incoming = common.CopyBytes(i.IncomingSeed)
	case "outgoing":
		v.Outgoing = common.CopyBytes(i.OutgoingSeed)
	case "full":
		v.Incoming = common.CopyBytes(i.IncomingSeed)
		v.Outgoing = common.CopyBytes(i.OutgoingSeed)
		key := i.NullifierKey
		v.NullifierKey = &key
	default:
		return ViewKey{}, errors.New("choose incoming, outgoing, or full viewing scope")
	}
	return v, nil
}
func (i *Identity) ViewKey() ViewKey { v, _ := i.ScopedViewKey("full"); return v }
func (v *ViewKey) Clear() {
	clear(v.Incoming)
	clear(v.Outgoing)
	if v.NullifierKey != nil {
		clear(v.NullifierKey[:])
	}
}
func (v ViewKey) validate() error {
	if v.Version != 2 || v.ChainID == 0 || v.Owner == (shielded3.Digest{}) {
		return errors.New("invalid or obsolete Shield3 viewing key; re-export a scoped key")
	}
	if !shielded3.IsSupportedAsset(v.AssetID) {
		return errors.New("unsupported shielded asset viewing key")
	}
	switch v.Scope {
	case "incoming":
		if len(v.Incoming) != pqcrypto.ShieldedV3ViewKeySize {
			return errors.New("incoming viewing keys must contain an incoming key")
		}
		if len(v.Outgoing) != 0 || v.NullifierKey != nil {
			return errors.New("incoming viewing keys must not contain outgoing or nullifier keys")
		}
	case "outgoing":
		if len(v.Incoming) != 0 || len(v.Outgoing) != pqcrypto.ShieldedV3ViewKeySize || v.NullifierKey != nil {
			return errors.New("outgoing viewing keys must contain only the outgoing key")
		}
	case "full":
		if len(v.Incoming) != pqcrypto.ShieldedV3ViewKeySize || len(v.Outgoing) != pqcrypto.ShieldedV3ViewKeySize || v.NullifierKey == nil || *v.NullifierKey == (shielded3.Digest{}) {
			return errors.New("incomplete full viewing key")
		}
		if _, err := shielded3.DigestFromBytes(v.NullifierKey.Bytes()); err != nil {
			return err
		}
	default:
		return errors.New("unknown viewing key scope")
	}
	return nil
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
	assetID := shielded3.NormalizeAssetID(n.AssetID)
	if !shielded3.IsSupportedAsset(assetID) {
		return nil, errors.New("unsupported shielded asset")
	}
	words := []uint64{domain, uint64(uint32(chainID)), chainID >> 32, assetID, 0}
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
func NoteNullifier(chainID uint64, n Note, key shielded3.Digest) (shielded3.Digest, error) {
	if key == (shielded3.Digest{}) {
		return shielded3.Digest{}, errors.New("nullifier viewing key required")
	}
	words, err := noteWords(chainID, n, 3003)
	if err != nil {
		return shielded3.Digest{}, err
	}
	copy(words[5:10], key[:])
	return shielded3.HashWords(words)
}

// OneTimeOutputKey derives a unique output key from hidden note randomness.
// The key is safe to publish because the randomness remains inside the
// authenticated recipient ciphertext; reusing randomness would be rejected by
// the commitment/proof checks.
func OneTimeOutputKey(owner, randomness, commitment shielded3.Digest) []byte {
	// Tip5/Goldilocks digest output is 40 bytes and is proved by the
	// Shield3 circuit as part of the public statement.
	digest, err := shielded3.HashWords([]uint64{3006, owner[0], owner[1], owner[2], owner[3], owner[4], randomness[0], randomness[1], randomness[2], randomness[3], randomness[4], commitment[0], commitment[1], commitment[2], commitment[3], commitment[4]})
	if err != nil {
		return nil
	}
	return digest.Bytes()
}

func newPaymentTag() ([]byte, error) {
	tag := make([]byte, 32)
	if _, err := rand.Read(tag); err != nil {
		return nil, err
	}
	return tag, nil
}

// Scanning needs viewing keys only; note ownership still requires the separate
// spending-secret preimage in the proof. Incoming scans never query spends.
func Scan(ctx context.Context, rpc RPC, view ViewKey) (ScanResult, error) {
	return ScanAsset(ctx, rpc, view, shielded3.AssetTKM)
}

// ScanAsset scans only notes whose authenticated commitment belongs to the
// requested asset.  The encrypted note itself carries the asset ID, so an RPC
// cannot make a TKM note appear as pTKM (or vice versa).
func ScanAsset(ctx context.Context, rpc RPC, view ViewKey, assetID uint64) (ScanResult, error) {
	assetID = shielded3.NormalizeAssetID(assetID)
	if !shielded3.IsSupportedAsset(assetID) {
		return ScanResult{}, errors.New("unsupported shielded asset")
	}
	view.AssetID = assetID
	return scanAsset(ctx, rpc, view, assetID)
}

func scanAsset(ctx context.Context, rpc RPC, view ViewKey, assetID uint64) (ScanResult, error) {
	result := ScanResult{BalanceWei: "0", Notes: make([]OwnedNote, 0)}
	if err := view.validate(); err != nil {
		return result, err
	}
	result.SpendStatusKnown = view.Scope == "full"
	result.ReceivedWei = "0"
	if !result.SpendStatusKnown {
		result.BalanceWei = ""
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
	received := new(big.Int)
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
					if err == nil && commitmentErr == nil && commitment == out.Commitment && valueErr == nil && value.Sign() > 0 && note.Owner != view.Owner && shielded3.NormalizeAssetID(note.AssetID) == assetID {
						result.History = append(result.History, HistoryEntry{Direction: "outgoing", Note: note, Commitment: commitment, TransactionHash: out.TransactionHash})
					}
				}
			}
			if view.Scope == "outgoing" {
				continue
			}
			plain, err := pqcrypto.OpenShieldedV3(view.Incoming, out.Incoming, core.ShieldedV3OutputContext(view.ChainID, pqcrypto.ShieldedV3Incoming, out.Commitment))
			if err != nil {
				continue
			}
			var note Note
			err = json.Unmarshal(plain, &note)
			clear(plain)
			if err != nil || note.Owner != view.Owner || shielded3.NormalizeAssetID(note.AssetID) != assetID {
				continue
			}
			commitment, err := NoteCommitment(view.ChainID, note)
			expectedKey := OneTimeOutputKey(note.Owner, note.Randomness, commitment)
			newMetadataValid := len(note.OneTimeKey) == 0 && len(note.PaymentTag) == 0 || bytes.Equal(note.OneTimeKey, expectedKey) && len(note.PaymentTag) == 32
			if err != nil || commitment != out.Commitment || seen[commitment] || !newMetadataValid {
				continue
			}
			seen[commitment] = true
			value, err := parseAmount(note.ValueWei)
			if err != nil {
				return result, err
			}
			if value.Sign() == 0 {
				continue
			}
			received.Add(received, value)
			if !result.SpendStatusKnown {
				result.History = append(result.History, HistoryEntry{Direction: "incoming", Note: note, Commitment: commitment, TransactionHash: out.TransactionHash})
				continue
			}
			nullifier, err := NoteNullifier(view.ChainID, note, *view.NullifierKey)
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
			result.History = append(result.History, HistoryEntry{Direction: "incoming", Note: note, Commitment: commitment, TransactionHash: out.TransactionHash, SpentBy: spent.TransactionHash, Pending: spent.Pending, SpendStatusKnown: true})
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
	result.ReceivedWei = received.String()
	if result.SpendStatusKnown {
		result.BalanceWei = total.String()
	}
	return result, nil
}

const WalletGas uint64 = 7_000_000

var buildSlot = make(chan struct{}, 1)

// Build constructs and locally verifies a bounded four-input spend/deposit. It
// never submits a transaction; the caller signs and broadcasts once explicitly.
func Build(ctx context.Context, rpc RPC, seed []byte, identity *Identity, to PaymentPayload, amount *big.Int, deposit bool) (*types.Transaction, error) {
	return build(ctx, rpc, seed, identity, []Payment{{to, amount}}, deposit, nil)
}

// BuildAsset constructs a Shield3 transaction for a registered shielded
// asset. AssetPTKM is the wrapped private TKM unit; its public deposit is the
// mint operation and its private withdrawal is the burn operation.
func BuildAsset(ctx context.Context, rpc RPC, seed []byte, identity *Identity, assetID uint64, to PaymentPayload, amount *big.Int, deposit bool) (*types.Transaction, error) {
	return buildAsset(ctx, rpc, seed, identity, []Payment{{to, amount}}, deposit, nil, assetID)
}

// WithdrawalRequest burns private units and releases the same amount of
// public TKM to a stamped address. It is intentionally limited to pTKM.
type WithdrawalRequest struct {
	Recipient common.Address
	Amount    *big.Int
}

func BuildAssetWithdrawal(ctx context.Context, rpc RPC, seed []byte, identity *Identity, assetID uint64, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return buildAssetWithWithdrawal(ctx, rpc, seed, identity, nil, false, nil, assetID, &WithdrawalRequest{Recipient: recipient, Amount: amount})
}

func build(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer) (*types.Transaction, error) {
	return buildAssetWithWithdrawal(ctx, rpc, seed, identity, payments, deposit, relay, shielded3.AssetTKM, nil)
}

func buildAsset(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer, assetID uint64) (*types.Transaction, error) {
	return buildAssetWithWithdrawal(ctx, rpc, seed, identity, payments, deposit, relay, assetID, nil)
}

func buildAssetWithWithdrawal(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer, assetID uint64, withdrawal *WithdrawalRequest) (*types.Transaction, error) {
	assetID = shielded3.NormalizeAssetID(assetID)
	if !shielded3.IsSupportedAsset(assetID) {
		return nil, errors.New("unsupported shielded asset")
	}
	var amount *big.Int
	var err error
	if withdrawal != nil {
		if deposit || assetID != shielded3.AssetPTKM || withdrawal.Amount == nil || withdrawal.Amount.Sign() <= 0 || withdrawal.Amount.Cmp(shielded3.MaxSendWei()) > 0 || withdrawal.Recipient == (common.Address{}) {
			return nil, errors.New("invalid wrapped private TKM withdrawal")
		}
		if err := RequireRegisteredAddress(ctx, rpc, withdrawal.Recipient); err != nil {
			return nil, err
		}
		if identity == nil || identity.Stamp == nil {
			return nil, errors.New("create the private stamp first")
		}
		amount = new(big.Int)
		payments = nil
	} else {
		amount, err = validatePayments(identity, payments)
		if err != nil {
			return nil, err
		}
	}
	if deposit && len(payments) != 1 {
		return nil, errors.New("shielding requires exactly one destination")
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
	self := PaymentPayload{ChainID: identity.ChainID, Address: identity.Address, Owner: identity.Owner, Stamp: *identity.Stamp}
	if err := RequireRegisteredStamp(ctx, rpc, self); err != nil {
		return nil, err
	}
	for _, payment := range payments {
		if err := RequireRegisteredStamp(ctx, rpc, payment.Recipient); err != nil {
			return nil, err
		}
	}
	var nonce hexutil.Uint64
	var gasPrice *big.Int
	var signingPublicKey []byte
	if relay != nil {
		if deposit {
			return nil, errors.New("public deposits cannot hide their funding account through a relay")
		}
		if err := validateRelayOffer(ctx, rpc, relay, identity.ChainID); err != nil {
			return nil, err
		}
		nonce = hexutil.Uint64(relay.Nonce)
		gasPrice = new(big.Int).Set((*big.Int)(relay.GasFeeCap))
		signingPublicKey = common.CopyBytes(relay.PublicKey)
	} else {
		if err := rpc.CallContext(ctx, &nonce, "eth_getTransactionCount", identity.Address, "pending"); err != nil {
			return nil, err
		}
		var price hexutil.Big
		if err := rpc.CallContext(ctx, &price, "eth_gasPrice"); err != nil {
			return nil, err
		}
		gasPrice = new(big.Int).Set((*big.Int)(&price))
		key, err := pqcrypto.NewMLDSA87FromSeed(seed)
		if err != nil {
			return nil, err
		}
		signingPublicKey = pqcrypto.PublicKeyBytes(key)
	}
	if gasPrice.Sign() <= 0 {
		return nil, errors.New("gas price unavailable")
	}
	var balance hexutil.Big
	if relay == nil {
		if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
			return nil, err
		}
	}
	maxGas := new(big.Int).Mul(new(big.Int).SetUint64(WalletGas), gasPrice)
	sponsor := new(big.Int)
	if assetID == shielded3.AssetPTKM && relay != nil {
		return nil, errors.New("wrapped private TKM requires the sender to pay public gas")
	}
	if !deposit && assetID == shielded3.AssetPTKM && (*big.Int)(&balance).Cmp(maxGas) < 0 {
		return nil, errors.New("public balance cannot cover gas for wrapped private TKM")
	}
	if !deposit && assetID != shielded3.AssetPTKM && (relay != nil || (*big.Int)(&balance).Cmp(maxGas) < 0) {
		sponsor.Set(maxGas)
	}
	required := new(big.Int).Add(amount, sponsor)
	if withdrawal != nil {
		required.Add(required, withdrawal.Amount)
	}
	witness := shielded3.SpendWitness{SpendingSecret: identity.SpendingSecret}
	change := new(big.Int)
	envelope := &core.ShieldedV3Transaction{Version: 3, Deposit: deposit, AssetID: assetID, WithdrawalValue: new(big.Int), GasSponsorValue: sponsor}
	if withdrawal != nil {
		envelope.WithdrawalRecipient = withdrawal.Recipient
		envelope.WithdrawalValue.Set(withdrawal.Amount)
	}
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
		defer view.Clear()
		scan, err := ScanAsset(ctx, rpc, view, assetID)
		if err != nil {
			return nil, err
		}
		chosen, err := selectNotes(scan.Notes, required)
		if err != nil {
			return nil, err
		}
		commitments := make([]shielded3.Digest, len(chosen))
		for i, n := range chosen {
			commitments[i] = n.Commitment
		}
		var paths []core.ShieldedV3Path
		if err := rpc.CallContext(ctx, &paths, "tkmprivacy_shieldedV3Paths", commitments); err != nil {
			return nil, err
		}
		if len(paths) != len(chosen) {
			return nil, errors.New("daemon returned incomplete input paths")
		}
		total := new(big.Int)
		for i, n := range chosen {
			path := paths[i]
			if !path.Found || path.Index >= uint64(1)<<32 || path.Commitment != n.Commitment || path.Root != paths[0].Root {
				return nil, errors.New("input notes no longer share a canonical root; rescan")
			}
			value, err := parseAmount(n.ValueWei)
			if err != nil {
				return nil, err
			}
			limbs, err := shielded3.AmountFromBig(value)
			if err != nil {
				return nil, err
			}
			total.Add(total, value)
			if i == 0 {
				witness.Value = limbs
				witness.Randomness = n.Randomness
				witness.LeafIndex = uint32(path.Index)
				witness.MerklePath = path.Path
				envelope.Nullifier = n.Nullifier
			} else {
				witness.AdditionalInputs[i-1] = shielded3.InputOpening{Randomness: n.Randomness, Value: limbs, LeafIndex: uint32(path.Index), MerklePath: path.Path}
				envelope.AdditionalNullifiers[i-1] = n.Nullifier
			}
		}
		envelope.InputCount = uint64(len(chosen))
		envelope.Anchor = paths[0].Root
		change.Sub(total, required)
	}
	recipients := [4]PaymentPayload{}
	values := [4]*big.Int{}
	self.IncomingPublicKey, self.StampPublicKey = identity.IncomingPublicKey, identity.StampPublicKey
	for slot := range recipients {
		recipients[slot], values[slot] = self, new(big.Int)
		if slot < len(payments) {
			recipients[slot], values[slot] = payments[slot].Recipient, payments[slot].Amount
		}
		if slot == 3 {
			values[slot] = change
		}
	}
	for slot := range recipients {
		recipient, value := recipients[slot], values[slot]
		var path core.ShieldedV3Path
		if err := rpc.CallContext(ctx, &path, "tkmprivacy_antarticalStampPath", recipient.Owner); err != nil {
			return nil, err
		}
		if !path.Found || path.Index >= uint64(1)<<32 || (slot > 0 && path.Root != envelope.StampRoot) {
			return nil, errors.New("stamp registry changed or recipient is unstamped; retry against the canonical chain")
		}
		envelope.StampRoot = path.Root
		witness.StampIndices[slot], witness.StampPaths[slot] = uint32(path.Index), path.Path
		random, err := shielded3.GenerateSecret()
		if err != nil {
			return nil, err
		}
		limbs, err := shielded3.AmountFromBig(value)
		if err != nil {
			return nil, err
		}
		witness.Outputs[slot] = shielded3.OutputOpening{Owner: recipient.Owner, Randomness: random, Value: limbs}
		commitment, err := NoteCommitment(identity.ChainID, Note{AssetID: assetID, Owner: recipient.Owner, Randomness: random, ValueWei: value.String(), Recipient: recipient.Address})
		if err != nil {
			return nil, err
		}
		oneTimeKey := OneTimeOutputKey(recipient.Owner, random, commitment)
		paymentTag, err := newPaymentTag()
		if err != nil {
			return nil, err
		}
		note := Note{AssetID: assetID, Owner: recipient.Owner, Randomness: random, ValueWei: value.String(), Recipient: recipient.Address, OneTimeKey: oneTimeKey, PaymentTag: paymentTag}
		envelope.Outputs[slot].Commitment = commitment
		envelope.Outputs[slot].OneTimeKey = common.CopyBytes(oneTimeKey)
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

	if relay != nil {
		envelope.Relayed = true
		envelope.ValidUntil = relay.ValidUntil
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
		tip := gasPrice
		if relay != nil {
			tip = (*big.Int)(relay.GasTipCap)
		}
		return types.NewTx(&types.PQTkmTx{ChainID: new(big.Int).SetUint64(identity.ChainID), Nonce: uint64(nonce), GasTipCap: tip, GasFeeCap: gasPrice, Gas: WalletGas, To: &pool, Value: value, Data: data, Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: signingPublicKey}), nil
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

// Prefer one covering note; otherwise the largest available notes minimize
// input count. Selection never exceeds the fixed four-input consensus budget.
func selectNotes(notes []OwnedNote, required *big.Int) ([]OwnedNote, error) {
	if required == nil || required.Sign() <= 0 {
		return nil, errors.New("invalid required amount")
	}
	eligible := make([]OwnedNote, 0, len(notes))
	seen := map[shielded3.Digest]bool{}
	for _, n := range notes {
		value, err := parseAmount(n.ValueWei)
		if err != nil {
			return nil, err
		}
		if value.Sign() == 0 {
			continue
		}
		if seen[n.Commitment] {
			return nil, errors.New("duplicate spendable note")
		}
		seen[n.Commitment] = true
		eligible = append(eligible, n)
	}
	sort.Slice(eligible, func(i, j int) bool {
		a, _ := parseAmount(eligible[i].ValueWei)
		b, _ := parseAmount(eligible[j].ValueWei)
		if a.Cmp(b) == 0 {
			return bytes.Compare(eligible[i].Commitment.Bytes(), eligible[j].Commitment.Bytes()) < 0
		}
		return a.Cmp(b) < 0
	})
	for _, n := range eligible {
		v, _ := parseAmount(n.ValueWei)
		if v.Cmp(required) >= 0 {
			return []OwnedNote{n}, nil
		}
	}
	chosen := make([]OwnedNote, 0, shielded3.InputSlots)
	total := new(big.Int)
	for i := len(eligible) - 1; i >= 0 && len(chosen) < shielded3.InputSlots; i-- {
		chosen = append(chosen, eligible[i])
		v, _ := parseAmount(eligible[i].ValueWei)
		total.Add(total, v)
		if total.Cmp(required) >= 0 {
			if total.BitLen() > 256 {
				return nil, errors.New("selected note total overflows uint256")
			}
			return chosen, nil
		}
	}
	return nil, errors.New("up to four confirmed Shield3 notes cannot cover this amount and fees; consolidate smaller notes or wait for pending payments")
}
