package shield3wallet

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// BuildPrepaidPayment authorizes a fixed, non-refundable fee burned from notes.
// It never tops up or refunds a public account. The caller must confirm the
// budget before proving; nil is not authorization.
func BuildPrepaidPayment(ctx context.Context, rpc RPC, seed []byte, identity *Identity, payments []Payment, version uint64, maxFeeWei *big.Int) (*types.Transaction, error) {
	if maxFeeWei == nil || maxFeeWei.Sign() <= 0 || maxFeeWei.BitLen() > 256 {
		return nil, errors.New("explicit prepaid fee budget required")
	}
	switch version {
	case 3:
		return buildAssetWithWithdrawalFee(ctx, rpc, seed, identity, payments, false, nil, shielded3.AssetTKM, nil, maxFeeWei)
	case 4:
		return buildV4AssetWithWithdrawalFee(ctx, rpc, seed, identity, payments, false, nil, shielded3.AssetTKM, nil, maxFeeWei)
	default:
		return nil, errors.New("unsupported shielded version")
	}
}

// BuildPublicWithdrawal is an explicit privacy exit: its amount and recipient
// are public. A nil fee budget selects public gas; a positive budget authorizes
// the fixed prepaid fee. Remaining note balances stay encrypted.
func BuildPublicWithdrawal(ctx context.Context, rpc RPC, seed []byte, identity *Identity, recipient common.Address, amount *big.Int, version uint64, maxFeeWei *big.Int) (*types.Transaction, error) {
	withdrawal := &WithdrawalRequest{Recipient: recipient, Amount: amount}
	switch version {
	case 3:
		return buildAssetWithWithdrawalFee(ctx, rpc, seed, identity, nil, false, nil, shielded3.AssetTKM, withdrawal, maxFeeWei)
	case 4:
		return buildV4AssetWithWithdrawalFee(ctx, rpc, seed, identity, nil, false, nil, shielded3.AssetTKM, withdrawal, maxFeeWei)
	default:
		return nil, errors.New("unsupported shielded version")
	}
}
