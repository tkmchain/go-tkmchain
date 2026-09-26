package shield3wallet

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
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
