package shield3wallet

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

// requirePublicFeePayer prevents a nominal fee reserve from becoming a public
// withdrawal/refund of private principal. Version 2 is the operator's signed
// consent to pay fees independently of the hidden notes.
func requirePublicFeePayer(ctx context.Context, rpc RPC, relay *RelayOffer, balance *hexutil.Big, required *big.Int) error {
	if relay != nil {
		if relay.Version != 2 {
			return errors.New("shielded-only payments require an explicitly fee-sponsored relay offer")
		}
		address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, relay.PublicKey)
		if err != nil {
			return err
		}
		if err := rpc.CallContext(ctx, balance, "eth_getBalance", address, "latest"); err != nil {
			return err
		}
	}
	if (*big.Int)(balance).Cmp(required) < 0 {
		return errors.New("shielded-only payments require public gas funds or an explicitly fee-sponsored relay; private notes cannot be withdrawn for gas")
	}
	return nil
}
