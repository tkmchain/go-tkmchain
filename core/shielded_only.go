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

// ErrValidatorTransactionsNotActive is returned when a validator registry
// transaction is submitted before its separately scheduled consensus fork.
var ErrValidatorTransactionsNotActive = errors.New("validator registry transactions are not active yet")

// ValidateShieldedOnlyTransaction is shared by admission, block processing and
// direct shielded validation. Run it before proofs or state mutations. A note
// spend must still pass all existing ownership, membership and conservation
// checks: this function only narrows the permitted transaction shapes.
func ValidateShieldedOnlyTransaction(config *params.ChainConfig, number *big.Int, blockTime uint64, tx *types.Transaction) error {
	if config != nil && tx != nil && config.IsAntartical(number, blockTime) && isValidatorProtocolEnvelope(tx.Data()) {
		if !config.IsValidatorTransactionsActive(number, blockTime) {
			return ErrValidatorTransactionsNotActive
		}
		return validateValidatorProtocolBasics(config, number, blockTime, tx)
	}
	if !config.IsShieldedOnly(number, blockTime) {
		return nil
	}
	if tx == nil {
		return ErrPublicPaymentDisabled
	}
	// Validator registry operations are narrowly scoped consensus-control
	// envelopes. Registration moves the fixed protocol bond to the reserved
	// pool; the other operations carry no value. They are not EVM payments and
	// remain subject to the validator-specific PQ, stamp, balance and state
	// checks below the shared transaction policy.
	_, err := shieldedOnlyFields(tx.Type(), tx.To(), tx.Value(), tx.Data())
	return err
}

func shieldedOnlyFields(txType uint8, to *common.Address, value *big.Int, data []byte) (*big.Int, error) {
	fail := func(reason string) (*big.Int, error) {
		return nil, fmt.Errorf("%w: %s", ErrPublicPaymentDisabled, reason)
	}
	if txType != types.PQTkmTxType || to == nil || *to != params.ShieldedPoolAddress || value == nil || value.Sign() != 0 {
		// Registration is the only protocol operation with an outer value. It
		// is fixed by consensus and is transferred only to the reserved protocol
		// pool, never to an arbitrary recipient or EVM contract.
		if txType != types.PQTkmTxType || to == nil || *to != params.ShieldedPoolAddress || value == nil || !HasValidatorRegistrationPrefix(data) || value.Cmp(ValidatorBondWei()) != 0 {
			return fail("requires a zero-value PQ envelope to the shielded pool")
		}
	}
	if isValidatorProtocolEnvelope(data) {
		if txType != types.PQTkmTxType || to == nil || *to != params.ShieldedPoolAddress || value == nil {
			return fail("validator protocol envelope requires a PQ transaction to the reserved pool")
		}
		if HasValidatorRegistrationPrefix(data) {
			if value.Cmp(ValidatorBondWei()) != 0 {
				return fail("validator registration must transfer exactly the consensus bond")
			}
			if _, err := DecodeValidatorRegistration(data); err != nil {
				return fail("malformed validator registration")
			}
			return nil, nil
		}
		if value.Sign() != 0 {
			return fail("validator control operations cannot transfer value")
		}
		if err := validateValidatorEnvelopeEncoding(data); err != nil {
			return fail("malformed validator control envelope")
		}
		return nil, nil
	}
	// Stamp registration is required before a sender can access private notes.
	// Its verifier checks address ownership and transfers no principal. Other
	// value-bearing operations need an explicit consensus envelope above.
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
