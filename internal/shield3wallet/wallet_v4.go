package shield3wallet

import (
	"bytes"
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
	"github.com/ethereum/go-ethereum/zk/shielded4"
)

// BuildV4Consolidation combines the largest confirmed Shield4 notes into a
// self-owned output. It is used after an explicit send action discovers that
// the requested amount is spread across too many notes.
func BuildV4Consolidation(ctx context.Context, rpc RPC, seed []byte, identity *Identity, target *big.Int) (*types.Transaction, error) {
	if identity == nil || identity.Stamp == nil || target == nil || target.Sign() <= 0 {
		return nil, errors.New("invalid Shield4 consolidation request")
	}
	view := identity.ViewKey()
	defer view.Clear()
	scan, err := ScanAsset(ctx, rpc, view, shielded3.AssetTKM)
	if err != nil {
		return nil, err
	}
	tx, _, err := buildV4ConsolidationFromNotes(ctx, rpc, seed, identity, scan.Notes, target)
	return tx, err
}

// BuildV4ConsolidationFromNotes performs a private self-consolidation using a
// previously scanned note set and returns the newly created spendable outputs.
// Callers must only pass notes from a complete, canonical wallet scan; selected
// nullifiers are rechecked with the node before proof construction.
func BuildV4ConsolidationFromNotes(ctx context.Context, rpc RPC, seed []byte, identity *Identity, notes []OwnedNote, target *big.Int) (*types.Transaction, []OwnedNote, error) {
	return buildV4ConsolidationFromNotes(ctx, rpc, seed, identity, notes, target)
}

// BuildV4ConsolidationBatchFromNotes builds one merge from a disjoint group of
// up to four already-scanned notes. Multiple transactions built this way may
// share the same canonical anchor: consensus keeps historical Shield4 roots
// valid, while nullifiers prevent any input from being spent twice.
func BuildV4ConsolidationBatchFromNotes(ctx context.Context, rpc RPC, seed []byte, identity *Identity, notes []OwnedNote) (*types.Transaction, []OwnedNote, error) {
	if identity == nil || identity.Stamp == nil || len(notes) < 2 || len(notes) > shielded4.InputSlots {
		return nil, nil, errors.New("a Shield4 merge needs two to four distinct notes")
	}
	var active status
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
		return nil, nil, err
	}
	var gasPrice hexutil.Big
	if err := rpc.CallContext(ctx, &gasPrice, "eth_gasPrice"); err != nil {
		return nil, nil, err
	}
	maxGas := new(big.Int).Mul(new(big.Int).SetUint64(WalletGas), (*big.Int)(&gasPrice))
	var balance hexutil.Big
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
		return nil, nil, err
	}
	feeSponsor := new(big.Int)
	if active.ConfidentialLedger || active.ShieldedOnly {
		if (*big.Int)(&balance).Cmp(maxGas) < 0 {
			return nil, nil, errors.New("public balance must cover the network fee before Shield4 notes can be consolidated")
		}
	} else if (*big.Int)(&balance).Cmp(maxGas) < 0 {
		feeSponsor.Set(maxGas)
	}
	total := new(big.Int)
	for _, note := range notes {
		value, err := parseAmount(note.ValueWei)
		if err != nil {
			return nil, nil, err
		}
		if value.Sign() <= 0 {
			return nil, nil, errors.New("cannot consolidate an empty Shield4 note")
		}
		total.Add(total, value)
	}
	amount := new(big.Int).Sub(total, feeSponsor)
	if amount.Sign() <= 0 {
		return nil, nil, errors.New("selected Shield4 notes cannot cover the consolidation fee")
	}
	if cap := shielded4.MaxSendWei(); amount.Cmp(cap) > 0 {
		return nil, nil, errors.New("selected notes exceed one-output consolidation capacity; use smaller disjoint groups")
	}
	self, err := DecodePaymentCode(identity.Code, identity.ChainID)
	if err != nil {
		return nil, nil, err
	}
	outputs := make([]OwnedNote, 0, 1)
	known := &ScanResult{Notes: notes}
	tx, err := buildV4AssetWithWithdrawalFeeCached(ctx, rpc, seed, identity, []Payment{{Recipient: self, Amount: amount}}, false, nil, shielded3.AssetTKM, nil, nil, known, &outputs)
	return tx, outputs, err
}

// Shield4ConsolidationGroups partitions notes into disjoint four-input merges.
// It keeps each output within the per-transaction send limit and returns notes
// that cannot be safely merged in this wave unchanged.
func Shield4ConsolidationGroups(notes []OwnedNote) ([][]OwnedNote, []OwnedNote, error) {
	ordered := append([]OwnedNote(nil), notes...)
	values := make(map[shielded3.Digest]*big.Int, len(ordered))
	for _, note := range ordered {
		if _, exists := values[note.Commitment]; exists {
			return nil, nil, errors.New("duplicate spendable note")
		}
		value, err := parseAmount(note.ValueWei)
		if err != nil {
			return nil, nil, err
		}
		if value.Sign() <= 0 {
			return nil, nil, errors.New("cannot group an empty Shield4 note")
		}
		values[note.Commitment] = value
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := values[ordered[i].Commitment], values[ordered[j].Commitment]
		if cmp := a.Cmp(b); cmp != 0 {
			return cmp > 0
		}
		return bytes.Compare(ordered[i].Commitment.Bytes(), ordered[j].Commitment.Bytes()) < 0
	})
	var groups [][]OwnedNote
	var carry []OwnedNote
	for i := 0; i < len(ordered); {
		count := len(ordered) - i
		if count > shielded4.InputSlots {
			count = shielded4.InputSlots
		}
		for count >= 2 {
			total := new(big.Int)
			for _, note := range ordered[i : i+count] {
				total.Add(total, values[note.Commitment])
			}
			if total.Cmp(shielded4.MaxSendWei()) <= 0 {
				groups = append(groups, append([]OwnedNote(nil), ordered[i:i+count]...))
				i += count
				break
			}
			count--
		}
		if count < 2 {
			carry = append(carry, ordered[i])
			i++
		}
	}
	return groups, carry, nil
}

func buildV4ConsolidationFromNotes(ctx context.Context, rpc RPC, seed []byte, identity *Identity, notes []OwnedNote, target *big.Int) (*types.Transaction, []OwnedNote, error) {
	if identity == nil || identity.Stamp == nil || target == nil || target.Sign() <= 0 {
		return nil, nil, errors.New("invalid Shield4 consolidation request")
	}
	var active status
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
		return nil, nil, err
	}
	var gasPrice hexutil.Big
	if err := rpc.CallContext(ctx, &gasPrice, "eth_gasPrice"); err != nil {
		return nil, nil, err
	}
	maxGas := new(big.Int).Mul(new(big.Int).SetUint64(WalletGas), (*big.Int)(&gasPrice))
	var balance hexutil.Big
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
		return nil, nil, err
	}
	feeSponsor := new(big.Int)
	if active.ConfidentialLedger || active.ShieldedOnly {
		if (*big.Int)(&balance).Cmp(maxGas) < 0 {
			return nil, nil, errors.New("public balance must cover the network fee before Shield4 notes can be consolidated")
		}
	} else if (*big.Int)(&balance).Cmp(maxGas) < 0 {
		feeSponsor.Set(maxGas)
	}
	amount, err := shield4ConsolidationAmount(notes, target, feeSponsor)
	if err != nil {
		return nil, nil, err
	}
	self, err := DecodePaymentCode(identity.Code, identity.ChainID)
	if err != nil {
		return nil, nil, err
	}
	outputs := make([]OwnedNote, 0, 1)
	known := &ScanResult{Notes: notes}
	tx, err := buildV4AssetWithWithdrawalFeeCached(ctx, rpc, seed, identity, []Payment{{Recipient: self, Amount: amount}}, false, nil, shielded3.AssetTKM, nil, nil, known, &outputs)
	return tx, outputs, err
}

// shield4ConsolidationAmount is pure so the wallet batching policy can be
// tested without proof generation or a node. It selects up to four largest
// notes and subtracts the fee sponsor from the self-payment when the active
// transaction fee mode charges it from the private balance.
func shield4ConsolidationAmount(notes []OwnedNote, target, feeSponsor *big.Int) (*big.Int, error) {
	if target == nil || target.Sign() <= 0 || feeSponsor == nil || feeSponsor.Sign() < 0 {
		return nil, errors.New("invalid Shield4 consolidation target")
	}
	ordered := append([]OwnedNote(nil), notes...)
	sort.Slice(ordered, func(i, j int) bool {
		a, errA := parseAmount(ordered[i].ValueWei)
		b, errB := parseAmount(ordered[j].ValueWei)
		if errA != nil || errB != nil {
			return bytes.Compare(ordered[i].Commitment.Bytes(), ordered[j].Commitment.Bytes()) < 0
		}
		if cmp := a.Cmp(b); cmp != 0 {
			return cmp > 0
		}
		return bytes.Compare(ordered[i].Commitment.Bytes(), ordered[j].Commitment.Bytes()) < 0
	})
	all := new(big.Int)
	for _, note := range ordered {
		value, err := parseAmount(note.ValueWei)
		if err != nil {
			return nil, err
		}
		all.Add(all, value)
	}
	needed := new(big.Int).Add(new(big.Int).Set(target), feeSponsor)
	if all.Cmp(needed) < 0 {
		return nil, errors.New("confirmed Shield4 notes do not cover the payment and fee reserve; no consolidation was submitted")
	}
	if len(ordered) <= shielded4.InputSlots {
		return nil, errors.New("confirmed Shield4 notes fit the proof input limit; check the spendable balance and fee funding")
	}
	count := len(ordered)
	if count > shielded4.InputSlots {
		count = shielded4.InputSlots
	}
	chunk := new(big.Int)
	for _, note := range ordered[:count] {
		value, _ := parseAmount(note.ValueWei)
		chunk.Add(chunk, value)
	}
	amount := new(big.Int).Sub(chunk, feeSponsor)
	if cap := shielded4.MaxSendWei(); amount.Cmp(cap) > 0 {
		amount.Set(cap)
	}
	if amount.Sign() <= 0 {
		return nil, errors.New("selected Shield4 notes are too small to cover the consolidation fee reserve")
	}
	return amount, nil
}

// BuildV4 constructs a Shield4 full-chain membership transaction. It only
// returns an unsigned transaction; the caller still signs and broadcasts it.
func BuildV4(ctx context.Context, rpc RPC, seed []byte, identity *Identity, to PaymentPayload, amount *big.Int, deposit bool) (*types.Transaction, error) {
	return buildV4(ctx, rpc, seed, identity, []Payment{{to, amount}}, deposit, nil)
}

func BuildV4Asset(ctx context.Context, rpc RPC, seed []byte, identity *Identity, assetID uint64, to PaymentPayload, amount *big.Int, deposit bool) (*types.Transaction, error) {
	return buildV4Asset(ctx, rpc, seed, identity, []Payment{{to, amount}}, deposit, nil, assetID)
}

func BuildV4AssetWithdrawal(ctx context.Context, rpc RPC, seed []byte, identity *Identity, assetID uint64, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return buildV4AssetWithWithdrawal(ctx, rpc, seed, identity, nil, false, nil, assetID, &WithdrawalRequest{Recipient: recipient, Amount: amount})
}

// BuildV4Batch constructs one Shield4 spend with up to three payments. Inputs
// are selected from the shared Shield3/Shield4 note tree, so the resulting
// proof has one full-chain anchor and one linkability tag.
func BuildV4Batch(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment) (*types.Transaction, error) {
	return buildV4(ctx, rpc, seed, identity, payments, false, nil)
}

// BuildV4WithKnownNotes builds from an already scanned note set. This is
// intended for a wallet operation that has confirmed a prior transaction and
// updated its in-memory note set. It rechecks selected nullifiers and obtains
// fresh Merkle paths, avoiding another scan from the activation block.
func BuildV4WithKnownNotes(ctx context.Context, rpc RPC, seed []byte, identity *Identity, to PaymentPayload, amount *big.Int, notes []OwnedNote) (*types.Transaction, error) {
	known := &ScanResult{Notes: notes}
	return buildV4AssetWithWithdrawalFeeCached(ctx, rpc, seed, identity, []Payment{{Recipient: to, Amount: amount}}, false, nil, shielded3.AssetTKM, nil, nil, known, nil)
}

func BuildV4AssetBatch(ctx context.Context, rpc RPC, seed []byte, identity *Identity, assetID uint64, payments []Payment) (*types.Transaction, error) {
	return buildV4Asset(ctx, rpc, seed, identity, payments, false, nil, assetID)
}

// BuildV4Relayed constructs a Shield4 spend authorized for a relay sponsor.
func BuildV4Relayed(ctx context.Context, rpc RPC, seed []byte, identity *Identity, to PaymentPayload, amount *big.Int, offer *RelayOffer) (*types.Transaction, error) {
	return buildV4(ctx, rpc, seed, identity, []Payment{{to, amount}}, false, offer)
}

// BuildV4RelayedBatch combines multi-payment Shield4 construction with relay
// sponsorship while retaining the same bounded input/output limits.
func BuildV4RelayedBatch(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, offer *RelayOffer) (*types.Transaction, error) {
	return buildV4(ctx, rpc, seed, identity, payments, false, offer)
}

func buildV4(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer) (*types.Transaction, error) {
	return buildV4AssetWithWithdrawal(ctx, rpc, seed, identity, payments, deposit, relay, shielded3.AssetTKM, nil)
}

func buildV4Asset(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer, assetID uint64) (*types.Transaction, error) {
	return buildV4AssetWithWithdrawal(ctx, rpc, seed, identity, payments, deposit, relay, assetID, nil)
}

func buildV4AssetWithWithdrawal(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer, assetID uint64, withdrawal *WithdrawalRequest) (*types.Transaction, error) {
	return buildV4AssetWithWithdrawalFee(ctx, rpc, seed, identity, payments, deposit, relay, assetID, withdrawal, nil)
}

func buildV4AssetWithWithdrawalFee(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer, assetID uint64, withdrawal *WithdrawalRequest, prepaidLimit *big.Int) (*types.Transaction, error) {
	return buildV4AssetWithWithdrawalFeeCached(ctx, rpc, seed, identity, payments, deposit, relay, assetID, withdrawal, prepaidLimit, nil, nil)
}

func buildV4AssetWithWithdrawalFeeCached(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, deposit bool, relay *RelayOffer, assetID uint64, withdrawal *WithdrawalRequest, prepaidLimit *big.Int, knownScan *ScanResult, generatedNotes *[]OwnedNote) (*types.Transaction, error) {
	assetID = shielded3.NormalizeAssetID(assetID)
	if !shielded3.IsSupportedAsset(assetID) {
		return nil, errors.New("unsupported shielded asset")
	}
	var amount *big.Int
	var err error
	if withdrawal != nil {
		if deposit || withdrawal.Amount == nil || withdrawal.Amount.Sign() <= 0 || withdrawal.Amount.Cmp(shielded4.MaxSendWei()) > 0 || withdrawal.Recipient == (common.Address{}) {
			return nil, errors.New("invalid wrapped private TKM withdrawal")
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

	var active status
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV4Status"); err != nil {
		return nil, err
	}
	if active.ShieldedOnly && (deposit || withdrawal != nil) {
		return nil, core.ErrPublicPaymentDisabled
	}
	prepaid := prepaidLimit != nil
	if prepaid && (!active.ConfidentialLedger || deposit || assetID != shielded3.AssetTKM || prepaidLimit.Sign() <= 0 || prepaidLimit.BitLen() > 256 || relay != nil) {
		return nil, errors.New("prepaid note fees require an active confidential ledger, a native TKM spend, and an explicit fee budget")
	}
	if withdrawal != nil && assetID == shielded3.AssetTKM && !active.ConfidentialLedger {
		return nil, errors.New("native TKM public withdrawal requires the confidential ledger fork")
	}
	if !active.Active || !active.NativeVerifier {
		return nil, errors.New("Shield4 requires an Antartical node with the embedded verifier")
	}
	select {
	case buildSlot <- struct{}{}:
		defer func() { <-buildSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if withdrawal != nil {
		if err := RequireRegisteredAddress(ctx, rpc, withdrawal.Recipient); err != nil {
			return nil, err
		}
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
	if prepaid {
		if maxGas.Cmp(prepaidLimit) > 0 {
			return nil, errors.New("prepaid note fee exceeds the authorized budget")
		}
		sponsor.Set(maxGas)
	} else if active.ConfidentialLedger && !deposit {
		if err := requirePublicFeePayer(ctx, rpc, relay, &balance, maxGas); err != nil {
			return nil, errors.New("authorize prepaid note fees, fund public gas, or use a fee-sponsored relay")
		}
	}
	if active.ShieldedOnly {
		if err := requirePublicFeePayer(ctx, rpc, relay, &balance, maxGas); err != nil {
			return nil, err
		}
	}
	if assetID == shielded3.AssetPTKM && relay != nil && !active.ShieldedOnly && !active.ConfidentialLedger {
		return nil, errors.New("wrapped private TKM requires the sender to pay public gas")
	}
	if !deposit && assetID == shielded3.AssetPTKM && (*big.Int)(&balance).Cmp(maxGas) < 0 {
		return nil, errors.New("public balance cannot cover gas for wrapped private TKM")
	}
	if !active.ShieldedOnly && !active.ConfidentialLedger && !deposit && assetID != shielded3.AssetPTKM && (relay != nil || (*big.Int)(&balance).Cmp(maxGas) < 0) {
		sponsor.Set(maxGas)
	}
	required := new(big.Int).Add(amount, sponsor)
	if withdrawal != nil {
		required.Add(required, withdrawal.Amount)
	}
	witness := shielded4.SpendWitness{SpendingSecret: identity.SpendingSecret}
	change := new(big.Int)
	envelope := &core.ShieldedV4Transaction{Version: 4, Deposit: deposit, AssetID: assetID, WithdrawalValue: new(big.Int), GasSponsorValue: sponsor}
	if prepaid {
		envelope.FeeMode = core.ShieldedFeePrepaid
	}
	if withdrawal != nil {
		envelope.WithdrawalRecipient = withdrawal.Recipient
		envelope.WithdrawalValue.Set(withdrawal.Amount)
	}
	if deposit {
		if (*big.Int)(&balance).Cmp(new(big.Int).Add(amount, maxGas)) < 0 {
			return nil, errors.New("public balance cannot cover shielding and gas")
		}
		witness.Value, _ = shielded4.AmountFromBig(amount)
		var err error
		witness.Randomness, err = shielded4.GenerateSecret()
		if err != nil {
			return nil, err
		}
	} else {
		view := identity.ViewKey()
		defer view.Clear()
		var scan ScanResult
		if knownScan != nil {
			scan = *knownScan
		} else {
			scan, err = ScanAsset(ctx, rpc, view, assetID)
			if err != nil {
				return nil, err
			}
		}
		chosen, err := selectNotes(scan.Notes, required)
		if err != nil {
			return nil, err
		}
		if knownScan != nil {
			for _, note := range chosen {
				var spent struct {
					TransactionHash common.Hash `json:"transactionHash"`
					Pending         bool        `json:"pending"`
				}
				if err := rpc.CallContext(ctx, &spent, "tkmprivacy_shieldedV3NullifierStatus", note.Nullifier); err != nil {
					return nil, fmt.Errorf("recheck cached note %x: %w", note.Commitment, err)
				}
				if spent.Pending || spent.TransactionHash != (common.Hash{}) {
					return nil, errors.New("cached Shield4 note was spent or is pending; rescan the wallet")
				}
			}
		}
		commitments := make([]shielded4.Digest, len(chosen))
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
			limbs, err := shielded4.AmountFromBig(value)
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
				witness.AdditionalInputs[i-1] = shielded4.InputOpening{Randomness: n.Randomness, Value: limbs, LeafIndex: uint32(path.Index), MerklePath: path.Path}
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
		random, err := shielded4.GenerateSecret()
		if err != nil {
			return nil, err
		}
		limbs, err := shielded4.AmountFromBig(value)
		if err != nil {
			return nil, err
		}
		witness.Outputs[slot] = shielded4.OutputOpening{Owner: recipient.Owner, Randomness: random, Value: limbs}
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
		if generatedNotes != nil && value.Sign() > 0 {
			nullifier, err := NoteNullifier(identity.ChainID, note, identity.NullifierKey)
			if err != nil {
				return nil, err
			}
			*generatedNotes = append(*generatedNotes, OwnedNote{Note: note, Commitment: commitment, Nullifier: nullifier})
		}
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
		data, err := core.EncodeShieldedV4Transaction(envelope)
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
	statement, err := core.ShieldedV4Statement(unsigned, envelope)
	if err != nil {
		return nil, err
	}
	if !deposit {
		derived, err := (shielded4.NativeBackend{}).Describe(ctx, statement, witness)
		if err != nil {
			return nil, err
		}
		if derived.Anchor != envelope.Anchor || derived.Nullifier != envelope.Nullifier {
			return nil, errors.New("Shield4 witness does not match the canonical note root")
		}
		envelope.LinkTag = derived.LinkTag
		statement, err = core.ShieldedV4Statement(unsigned, envelope)
		if err != nil {
			return nil, err
		}
	}
	envelope.Proof, err = (shielded4.NativeBackend{}).Prove(ctx, statement, witness)
	if err != nil {
		return nil, err
	}
	if err = (shielded4.NativeBackend{}).Verify(ctx, statement, envelope.Proof); err != nil {
		return nil, err
	}
	unsigned, err = makeTx()
	if err != nil {
		return nil, err
	}
	gas, err := core.IntrinsicGasWithShield4(unsigned.Data(), nil, nil, false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > WalletGas {
		return nil, errors.New("Shield4 proof exceeds the wallet gas budget")
	}
	return unsigned, nil
}
