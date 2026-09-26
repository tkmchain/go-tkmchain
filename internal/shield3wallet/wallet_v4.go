package shield3wallet

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded4"
)

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
	assetID = shielded3.NormalizeAssetID(assetID)
	if !shielded3.IsSupportedAsset(assetID) {
		return nil, errors.New("unsupported shielded asset")
	}
	var amount *big.Int
	var err error
	if withdrawal != nil {
		if deposit || assetID != shielded3.AssetPTKM || withdrawal.Amount == nil || withdrawal.Amount.Sign() <= 0 || withdrawal.Amount.Cmp(shielded4.MaxSendWei()) > 0 || withdrawal.Recipient == (common.Address{}) {
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
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV4Status"); err != nil {
		return nil, err
	}
	if !active.Active || !active.NativeVerifier {
		return nil, errors.New("Shield4 requires an Antartical node with the embedded verifier")
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
	witness := shielded4.SpendWitness{SpendingSecret: identity.SpendingSecret}
	change := new(big.Int)
	envelope := &core.ShieldedV4Transaction{Version: 4, Deposit: deposit, AssetID: assetID, WithdrawalValue: new(big.Int), GasSponsorValue: sponsor}
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
		scan, err := ScanAsset(ctx, rpc, view, assetID)
		if err != nil {
			return nil, err
		}
		chosen, err := selectNotes(scan.Notes, required)
		if err != nil {
			return nil, err
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
