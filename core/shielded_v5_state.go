package core

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

func validateShieldedV5State(st *state.StateDB, tx *types.Transaction, e *ShieldedV5Transaction) error {
	if e == nil {
		return ErrInvalidShieldedTx
	}
	shared := shieldedV5AsV4(e)
	// Reuse all common tree, stamp-root, reserve, commitment and withdrawal
	// checks. The helper checks the first four nullifiers; this loop checks the
	// appended V5 nullifiers in the same canonical asset namespace.
	if err := validateShieldedV4State(st, tx, shared); err != nil {
		return err
	}
	all, err := ShieldedV5Nullifiers(e)
	if err != nil {
		return err
	}
	asset := shieldedV5AssetID(e)
	if len(all) > 4 {
		for _, n := range all[4:] {
			if ShieldedV3NullifierTransactionForAsset(st, asset, n) != (common.Hash{}) {
				return fmt.Errorf("%w: Shield5 nullifier already spent", ErrInvalidShieldedTx)
			}
		}
	}
	return nil
}

func processShieldedV5(config *params.ChainConfig, number *big.Int, time uint64, st *state.StateDB, tx *types.Transaction, seen map[common.Hash]struct{}) error {
	e, err := shieldedV5Basics(config, number, time, tx)
	if err != nil {
		return err
	}
	from, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	if err := ValidateAntarticalStampState(st, from, tx.To(), tx.Value(), tx.Data()); err != nil {
		return err
	}
	if err = validateShieldedV5State(st, tx, e); err != nil {
		return err
	}
	nullifiers, err := ShieldedV5Nullifiers(e)
	if err != nil {
		return err
	}
	asset := shieldedV5AssetID(e)
	for _, n := range nullifiers {
		if _, ok := seen[ShieldedV3StateSlotForAsset(asset, "block-nullifier", n.Bytes())]; ok {
			return fmt.Errorf("%w: duplicate Shield5 nullifier in block", ErrInvalidShieldedTx)
		}
	}
	linkKey := ShieldedV4StateSlot("block-link-tag", e.LinkTag.Bytes())
	if asset != shielded3.AssetTKM {
		linkKey = ShieldedV3StateSlotForAsset(asset, "shield4/block-link-tag", e.LinkTag.Bytes())
	}
	if _, ok := seen[linkKey]; ok {
		return fmt.Errorf("%w: duplicate Shield5 linkability tag in block", ErrInvalidShieldedTx)
	}
	var ledger ConfidentialLedgerTotals
	if config.IsConfidentialLedger(number, time) {
		deposit := new(big.Int)
		if e.Deposit {
			deposit.Set(tx.Value())
		}
		ledger, err = ledgerTransition(st, asset, deposit, e.WithdrawalValue, e.GasSponsorValue)
		if err != nil {
			return err
		}
	}
	statement, err := ShieldedV5Statement(tx, e)
	if err != nil {
		return err
	}
	if err = (shielded3.NativeBackendV5{}).Verify(context.Background(), statement, e.Proof); err != nil {
		return fmt.Errorf("%w: Shield5 proof: %w", ErrInvalidShieldedTx, err)
	}
	sender, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	snapshot := st.Snapshot()
	for _, out := range e.Outputs {
		if err = appendShieldedV3Leaf(st, out.Commitment, tx.Hash()); err != nil {
			st.RevertToSnapshot(snapshot)
			return err
		}
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlotForAsset(asset, "commitment", out.Commitment.Bytes()), tx.Hash())
	}
	for _, n := range nullifiers {
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlotForAsset(asset, "nullifier", n.Bytes()), tx.Hash())
		seen[ShieldedV3StateSlotForAsset(asset, "block-nullifier", n.Bytes())] = struct{}{}
	}
	if asset == shielded3.AssetPTKM {
		supply := shieldedV3AssetSupply(st, asset)
		if e.Deposit {
			supply.Add(supply, tx.Value())
		} else {
			supply.Sub(supply, e.WithdrawalValue)
		}
		setShieldedV3AssetSupply(st, asset, supply)
	}
	if !e.Deposit {
		if asset == shielded3.AssetTKM {
			st.SetState(params.ShieldedPoolAddress, ShieldedV4StateSlot("link-tag", e.LinkTag.Bytes()), tx.Hash())
		} else {
			st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlotForAsset(asset, "shield4/link-tag", e.LinkTag.Bytes()), tx.Hash())
		}
		seen[linkKey] = struct{}{}
	}
	publicSponsor := e.GasSponsorValue
	if config.IsConfidentialLedger(number, time) {
		commitLedger(st, asset, ledger)
		if e.FeeMode == ShieldedFeePrepaid {
			burnConfidentialFee(st, tx, e.GasSponsorValue)
			publicSponsor = new(big.Int)
		}
	}
	for _, release := range []struct {
		to    common.Address
		value *big.Int
	}{{e.WithdrawalRecipient, e.WithdrawalValue}, {sender, publicSponsor}} {
		if release.value.Sign() > 0 {
			amount := uint256.MustFromBig(release.value)
			st.SubBalance(params.ShieldedPoolAddress, amount, tracing.BalanceChangeTransfer)
			st.AddBalance(release.to, amount, tracing.BalanceChangeTransfer)
		}
	}
	return nil
}
