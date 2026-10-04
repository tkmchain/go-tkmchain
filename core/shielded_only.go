package core

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// ErrPublicPaymentDisabled means a transaction would expose payment principal
// after the shielded-only fork. Fees and protocol-generated rewards remain
// public; this rule is not a claim that the transaction itself is invisible.
var ErrPublicPaymentDisabled = errors.New("shielded-only fork requires a private Shield3/Shield4 spend; public deposits, withdrawals and other public payment paths are disabled")

// ValidateShieldedOnlyTransaction is shared by admission, block processing and
// direct shielded validation. Run it before proofs or state mutations. A note
// spend must still pass all existing ownership, membership and conservation
// checks: this function only narrows the permitted transaction shapes.
func ValidateShieldedOnlyTransaction(config *params.ChainConfig, number *big.Int, blockTime uint64, tx *types.Transaction) error {
	if !config.IsShieldedOnly(number, blockTime) {
		return nil
	}
	if tx == nil {
		return ErrPublicPaymentDisabled
	}
	_, err := shieldedOnlyFields(tx.Type(), tx.To(), tx.Value(), tx.Data())
	return err
}

func shieldedOnlyFields(txType uint8, to *common.Address, value *big.Int, data []byte) (*big.Int, error) {
	fail := func(reason string) (*big.Int, error) {
		return nil, fmt.Errorf("%w: %s", ErrPublicPaymentDisabled, reason)
	}
	if txType != types.PQTkmTxType || to == nil || *to != params.ShieldedPoolAddress || value == nil || value.Sign() != 0 {
		return fail("requires a zero-value PQ envelope to the shielded pool")
	}
	// Registration is required to receive private notes. Its existing verifier
	// checks ownership and permits no transfer of principal. Other protocol
	// envelopes can debit/release public balances and are not exempted here.
	if HasAntarticalStampPrefix(data) {
		if _, err := DecodeAntarticalStamp(data); err != nil {
			return fail("malformed stamp registration")
		}
		return nil, nil
	}
	var deposit bool
	var recipient common.Address
	var withdrawal, sponsor *big.Int
	switch {
	case HasShieldedV3Prefix(data):
		e, ok, err := DecodeShieldedV3Transaction(data)
		if err != nil || !ok || e == nil || e.Version != 3 {
			return fail("malformed Shield3 envelope")
		}
		deposit, recipient, withdrawal, sponsor = e.Deposit, e.WithdrawalRecipient, e.WithdrawalValue, e.GasSponsorValue
	case HasShieldedV4Prefix(data):
		e, ok, err := DecodeShieldedV4Transaction(data)
		if err != nil || !ok || e == nil || e.Version != 4 {
			return fail("malformed Shield4 envelope")
		}
		deposit, recipient, withdrawal, sponsor = e.Deposit, e.WithdrawalRecipient, e.WithdrawalValue, e.GasSponsorValue
	default:
		return fail("unsupported public or legacy envelope")
	}
	if deposit || withdrawal == nil || withdrawal.Sign() != 0 || recipient != (common.Address{}) {
		return fail("deposits and public withdrawals reveal their amount")
	}
	if sponsor == nil || sponsor.Sign() != 0 {
		return fail("gas must be paid outside the private notes; public reserve refunds would expose value")
	}
	return sponsor, nil
}
