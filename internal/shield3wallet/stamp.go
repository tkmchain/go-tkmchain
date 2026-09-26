package shield3wallet

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

const StampWalletGas uint64 = 4_000_000

func RequireRegisteredStamp(ctx context.Context, rpc RPC, recipient PaymentPayload) error {
	var stamp core.AntarticalStampStatus
	if err := rpc.CallContext(ctx, &stamp, "tkmprivacy_antarticalStamp", recipient.Address); err != nil {
		return err
	}
	if !stamp.Registered || stamp.Owner != recipient.Owner || stamp.Commitment != recipient.Stamp.Commitment {
		return errors.New("address stamp is not confirmed on this chain; register the original private stamp and wait for confirmation before sending")
	}
	return nil
}

func RequireRegisteredAddress(ctx context.Context, rpc RPC, address common.Address) error {
	var stamp core.AntarticalStampStatus
	if err := rpc.CallContext(ctx, &stamp, "tkmprivacy_antarticalStamp", address); err != nil {
		return err
	}
	if !stamp.Registered {
		return errors.New("withdrawal recipient address is not stamped on this chain")
	}
	return nil
}

// A registration contains no transferred value. Normal gas fees still apply;
// the private labels are encrypted, and the proof binds the owner and full intent.
func BuildStamp(ctx context.Context, rpc RPC, seed []byte, identity *Identity) (*types.Transaction, error) {
	if identity == nil || identity.Stamp == nil {
		return nil, errors.New("create the private stamp first")
	}
	var active status
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
		return nil, err
	}
	if !active.Active || !active.NativeVerifier {
		return nil, errors.New("consensus stamping activates at Antartical")
	}
	var current core.AntarticalStampStatus
	if err := rpc.CallContext(ctx, &current, "tkmprivacy_antarticalStamp", identity.Address); err != nil {
		return nil, err
	}
	if current.Registered {
		return nil, errors.New("this address already has an immutable confirmed stamp")
	}
	select {
	case buildSlot <- struct{}{}:
		defer func() { <-buildSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var nonce hexutil.Uint64
	var price, balance hexutil.Big
	if err := rpc.CallContext(ctx, &nonce, "eth_getTransactionCount", identity.Address, "pending"); err != nil {
		return nil, err
	}
	if err := rpc.CallContext(ctx, &price, "eth_gasPrice"); err != nil {
		return nil, err
	}
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", identity.Address, "latest"); err != nil {
		return nil, err
	}
	gasPrice := new(big.Int).Set((*big.Int)(&price))
	if gasPrice.Sign() <= 0 {
		return nil, errors.New("registration gas price unavailable")
	}
	if (*big.Int)(&balance).Cmp(new(big.Int).Mul(new(big.Int).SetUint64(StampWalletGas), gasPrice)) < 0 {
		return nil, errors.New("public TKM balance must cover stamp registration gas")
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	e := &core.AntarticalStampRegistration{Version: 1, Owner: identity.Owner, Stamp: *identity.Stamp}
	makeTx := func() (*types.Transaction, error) {
		data, err := core.EncodeAntarticalStamp(e)
		if err != nil {
			return nil, err
		}
		pool := params.ShieldedPoolAddress
		return types.NewTx(&types.PQTkmTx{ChainID: new(big.Int).SetUint64(identity.ChainID), Nonce: uint64(nonce), GasTipCap: gasPrice, GasFeeCap: gasPrice, Gas: StampWalletGas, To: &pool, Value: new(big.Int), Data: data, Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(key)}), nil
	}
	tx, err := makeTx()
	if err != nil {
		return nil, err
	}
	intent, err := core.AntarticalStampIntent(tx, e)
	if err != nil {
		return nil, err
	}
	e.Proof, err = (shielded3.NativeBackend{}).ProveOwner(ctx, identity.ChainID, identity.Owner, intent, identity.SpendingSecret)
	if err != nil {
		return nil, err
	}
	tx, err = makeTx()
	if err != nil {
		return nil, err
	}
	if err := core.ValidateAntarticalStampProof(tx); err != nil {
		return nil, err
	}
	gas, err := core.IntrinsicGasWithShield3(tx.Data(), nil, nil, false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > StampWalletGas {
		return nil, errors.New("stamp proof exceeds the registration gas budget")
	}
	return tx, nil
}
