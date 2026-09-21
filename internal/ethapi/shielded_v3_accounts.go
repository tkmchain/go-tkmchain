package ethapi

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

func (api *TKMPaymentAPI) antarticalAccountGate(ctx context.Context) (bool, error) {
	head, err := api.b.HeaderByNumber(ctx, rpc.LatestBlockNumber)
	if err != nil {
		return false, err
	}
	if head == nil {
		return false, errors.New("chain head unavailable")
	}
	return api.b.ChainConfig().IsAntartical(head.Number, head.Time), nil
}
func (api *TKMPaymentAPI) ImportStampedPQSeedWithPassphrase(ctx context.Context, seed hexutil.Bytes, passphrase, name, country string) (common.Address, error) {
	active, err := api.antarticalAccountGate(ctx)
	if err != nil {
		return common.Address{}, err
	}
	if !active {
		return common.Address{}, errors.New("stamped account creation activates at Antartical")
	}
	chainID := api.b.ChainConfig().ChainID
	if chainID == nil || !chainID.IsUint64() {
		return common.Address{}, errors.New("invalid Shield3 chain ID")
	}
	ks, err := api.keystore()
	if err != nil {
		return common.Address{}, err
	}
	a, err := ks.ImportStampedPQSeed(seed, passphrase, chainID.Uint64(), name, country)
	return a.Address, err
}
func (api *TKMPaymentAPI) StampPQAccountWithPassphrase(ctx context.Context, addr common.Address, passphrase, name, country string) error {
	active, err := api.antarticalAccountGate(ctx)
	if err != nil {
		return err
	}
	if !active {
		return errors.New("Shield3 stamps activate at Antartical")
	}
	chainID := api.b.ChainConfig().ChainID
	if chainID == nil || !chainID.IsUint64() {
		return errors.New("invalid Shield3 chain ID")
	}
	ks, err := api.keystore()
	if err != nil {
		return err
	}
	return ks.StampPQAccount(accounts.Account{Address: addr}, passphrase, chainID.Uint64(), name, country)
}

// ImportPQBackupWithPassphrase restores an encrypted PQ keyfile. Antartical
// imports preserve the original stamp and require it to match this chain.
func (api *TKMPaymentAPI) ImportPQBackupWithPassphrase(ctx context.Context, blob hexutil.Bytes, passphrase, newPassphrase string) (common.Address, error) {
	if len(blob) == 0 || len(blob) > 512<<10 {
		return common.Address{}, errors.New("PQ backup must be between 1 byte and 512 KiB")
	}
	active, err := api.antarticalAccountGate(ctx)
	if err != nil {
		return common.Address{}, err
	}
	ks, err := api.keystore()
	if err != nil {
		return common.Address{}, err
	}
	if active {
		chain := api.b.ChainConfig().ChainID
		if chain == nil || !chain.IsUint64() {
			return common.Address{}, errors.New("invalid Shield3 chain ID")
		}
		a, err := ks.ImportStampedPQBackup(blob, passphrase, newPassphrase, chain.Uint64())
		return a.Address, err
	}
	a, err := ks.ImportPQ(blob, passphrase, newPassphrase)
	return a.Address, err
}
