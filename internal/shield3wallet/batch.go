package shield3wallet

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// Payment occupies one of three payment slots. Slot four is always change.
type PaymentRequest struct {
	Recipient string `json:"recipient"`
	AmountWei string `json:"amountWei"`
}

// DecodePayments authenticates payment codes and parses exact integer amounts.
func DecodePayments(chainID uint64, requests []PaymentRequest) ([]Payment, error) {
	if len(requests) < 1 || len(requests) > 3 {
		return nil, errors.New("choose one to three Shield3 recipients")
	}
	payments := make([]Payment, len(requests))
	for index, request := range requests {
		recipient, err := DecodePaymentCode(request.Recipient, chainID)
		if err != nil {
			return nil, err
		}
		amount, err := parseAmount(request.AmountWei)
		if err != nil || amount.Sign() <= 0 {
			return nil, errors.New("invalid batch amount in smallest units")
		}
		payments[index] = Payment{recipient, amount}
	}
	return payments, nil
}

type Payment struct {
	Recipient PaymentPayload
	Amount    *big.Int
}

func validatePayments(identity *Identity, payments []Payment) (*big.Int, error) {
	if identity == nil || identity.Stamp == nil || len(payments) < 1 || len(payments) > 3 {
		return nil, errors.New("Shield3 requires one to three recipients and a stamped identity")
	}
	total := new(big.Int)
	for _, p := range payments {
		if p.Recipient.ChainID != identity.ChainID {
			return nil, errors.New("Shield3 recipient chain mismatch")
		}
		if p.Amount == nil || p.Amount.Sign() <= 0 || p.Amount.BitLen() > 256 {
			return nil, errors.New("invalid Shield3 payment amount")
		}
		total.Add(total, p.Amount)
		if total.Cmp(shielded3.MaxSendWei()) > 0 {
			return nil, errors.New("Shield3 maximum aggregate send is 5000000 TKM")
		}
	}
	return total, nil
}
func BuildBatch(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment) (*types.Transaction, error) {
	return build(ctx, rpc, seed, identity, payments, false, nil)
}

func BuildAssetBatch(ctx context.Context, rpc RPC, seed []byte, identity *Identity, assetID uint64, payments []Payment) (*types.Transaction, error) {
	return buildAsset(ctx, rpc, seed, identity, payments, false, nil, assetID)
}
func BuildRelayedBatch(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, offer *RelayOffer) (*types.Transaction, error) {
	return build(ctx, rpc, seed, identity, payments, false, offer)
}

// BuildAndSignBatch is the browser/WASM entry point. It keeps proof
// construction and ML-DSA signing inside the same canonical Go code used by
// the node wallet; the caller only receives the final typed transaction bytes.
func BuildAndSignBatch(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment) (*types.Transaction, error) {
	tx, err := BuildBatch(ctx, rpc, seed, identity, payments)
	if err != nil {
		return nil, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	return types.SignPQTkmTx(tx, types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), key)
}

// BuildAndSignDeposit builds the canonical Shield3 public-to-private funding
// transaction for the identity itself.  Keeping this entry point beside the
// batch builder lets browser wallets use the exact same deposit relation and
// signer as desktop wallets without exposing a prover endpoint.
func BuildAndSignDeposit(ctx context.Context, rpc RPC, seed []byte, identity *Identity, amount *big.Int) (*types.Transaction, error) {
	if identity == nil || identity.Stamp == nil {
		return nil, errors.New("missing Shield3 identity")
	}
	if amount == nil || amount.Sign() <= 0 {
		return nil, errors.New("invalid Shield3 deposit amount")
	}
	to := PaymentPayload{
		ChainID: identity.ChainID,
		Address: identity.Address,
		Owner:   identity.Owner,
		Stamp:   *identity.Stamp,
	}
	tx, err := Build(ctx, rpc, seed, identity, to, amount, true)
	if err != nil {
		return nil, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	return types.SignPQTkmTx(tx, types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), key)
}

// BuildAndSignStamp builds and signs the immutable Antartical stamp
// registration using the same owner-proof relation as the node wallet.
func BuildAndSignStamp(ctx context.Context, rpc RPC, seed []byte, identity *Identity) (*types.Transaction, error) {
	if identity == nil || identity.Stamp == nil {
		return nil, errors.New("missing Shield3 identity stamp")
	}
	tx, err := BuildStamp(ctx, rpc, seed, identity)
	if err != nil {
		return nil, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	return types.SignPQTkmTx(tx, types.NewQuantumSigner(new(big.Int).SetUint64(identity.ChainID)), key)
}
