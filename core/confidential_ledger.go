package core

import (
	"errors"
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

const (
	ShieldedFeePublic uint64 = 0
	// Prepaid fees burn the exact authorized reserve from the input notes.
	// They never create a public gas refund or credit the outer signing account.
	ShieldedFeePrepaid     uint64 = 1
	ConfidentialPrepaidGas uint64 = 7_000_000
)

type ShieldedPrivacyBoundary string

const (
	PrivatePayment    ShieldedPrivacyBoundary = "private-payment"
	PublicShielding   ShieldedPrivacyBoundary = "public-deposit"
	PublicUnshielding ShieldedPrivacyBoundary = "public-withdrawal"
)

// Issuance here means backed issuance of private notes, not additional mining
// subsidy. The existing proof proves outputs = deposited backing, or inputs =
// outputs + withdrawals + prepaid fees. Mining emission is unchanged.
type ConfidentialLedgerTotals struct {
	OpeningBacking *big.Int `json:"openingBackingWei"`
	Deposited      *big.Int `json:"publicDepositsWei"`
	Withdrawn      *big.Int `json:"publicWithdrawalsWei"`
	FeesBurned     *big.Int `json:"prepaidFeesBurnedWei"`
	Backing        *big.Int `json:"backingWei"`
	Initialized    bool     `json:"initialized"`
}

func ledgerSlot(asset uint64, role string) common.Hash {
	return ShieldedV3StateSlotForAsset(asset, "confidential-ledger/v1/"+role, nil)
}

func ConfidentialLedgerState(st shieldedStateReader, asset uint64) ConfidentialLedgerTotals {
	read := func(role string) *big.Int {
		return new(big.Int).SetBytes(st.GetState(params.ShieldedPoolAddress, ledgerSlot(asset, role)).Bytes())
	}
	return ConfidentialLedgerTotals{read("opening"), read("deposited"), read("withdrawn"), read("fees"), read("backing"), read("initialized").Sign() != 0}
}

func validateConfidentialLedgerShape(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction, deposit bool, sponsor *big.Int, mode, asset uint64) error {
	active := config.IsConfidentialLedger(number, time)
	if mode > ShieldedFeePrepaid || (mode != ShieldedFeePublic && !active) {
		return fmt.Errorf("%w: inactive or unknown confidential fee mode", ErrInvalidShieldedTx)
	}
	if !active {
		return nil
	}
	if mode == ShieldedFeePublic {
		if sponsor.Sign() != 0 {
			return fmt.Errorf("%w: note-funded fees require explicit prepaid mode; public gas refunds are disabled", ErrInvalidShieldedTx)
		}
		return nil
	}
	if deposit || asset != shielded3.AssetTKM || tx.Value().Sign() != 0 || tx.Gas() != ConfidentialPrepaidGas || tx.GasFeeCap().Sign() <= 0 || tx.GasFeeCap().Cmp(tx.GasTipCap()) != 0 {
		return fmt.Errorf("%w: invalid prepaid fee authorization", ErrInvalidShieldedTx)
	}
	expected := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())
	if expected.BitLen() > 256 || sponsor.Cmp(expected) != 0 {
		return fmt.Errorf("%w: prepaid fee must equal the authorized fixed gas charge", ErrInvalidShieldedTx)
	}
	return nil
}

// ledgerTransition is computed before mutation, then committed only after the
// native proof has verified. Existing backing is an upper bound on historical
// note liabilities: unsolicited old transfers may have left excess reserves.
func ledgerTransition(st *state.StateDB, asset uint64, deposit, withdrawal, fee *big.Int) (ConfidentialLedgerTotals, error) {
	l := ConfidentialLedgerState(st, asset)
	reserve, err := confidentialNativeReserve(st)
	if err != nil {
		return l, err
	}
	native := ConfidentialLedgerState(st, shielded3.AssetTKM)
	if native.Initialized && reserve.Cmp(native.Backing) < 0 {
		return l, errors.New("confidential native ledger backing shortfall")
	}
	if asset == shielded3.AssetPTKM {
		reserve = shieldedV3AssetSupply(st, asset)
	}
	if !l.Initialized {
		l.OpeningBacking.Set(reserve)
		l.Backing.Set(reserve)
		l.Initialized = true
	}
	if reserve.Sign() < 0 || reserve.Cmp(l.Backing) < 0 {
		return l, errors.New("confidential ledger backing shortfall")
	}
	for _, n := range []*big.Int{deposit, withdrawal, fee} {
		if n == nil || n.Sign() < 0 || n.BitLen() > 256 {
			return l, errors.New("invalid confidential ledger value")
		}
	}
	l.Deposited.Add(l.Deposited, deposit)
	l.Withdrawn.Add(l.Withdrawn, withdrawal)
	l.FeesBurned.Add(l.FeesBurned, fee)
	l.Backing.Add(l.Backing, deposit)
	l.Backing.Sub(l.Backing, withdrawal)
	l.Backing.Sub(l.Backing, fee)
	for _, n := range []*big.Int{l.Deposited, l.Withdrawn, l.FeesBurned, l.Backing} {
		if n.Sign() < 0 || n.BitLen() > 256 {
			return l, errors.New("confidential ledger overdraw or overflow")
		}
	}
	return l, nil
}

// The shared account also holds validator bonds and wrapped-token backing.
// Neither is native note issuance. Include exited-but-unwithdrawn bonds;
// slashed/withdrawn records have zero stake. Registry traversal is bounded.
func confidentialNativeReserve(st *state.StateDB) (*big.Int, error) {
	reserve := st.GetBalance(params.ShieldedPoolAddress).ToBig()
	records, err := ValidatorRecords(st)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.Stake == nil || record.Stake.Sign() < 0 || record.Stake.BitLen() > 256 {
			return nil, errors.New("invalid validator reserve")
		}
		reserve.Sub(reserve, record.Stake)
	}
	reserve.Sub(reserve, shieldedV3AssetSupply(st, shielded3.AssetPTKM))
	if reserve.Sign() < 0 {
		return nil, errors.New("shared pool liabilities exceed backing")
	}
	return reserve, nil
}

func commitLedger(st *state.StateDB, asset uint64, l ConfidentialLedgerTotals) {
	for _, entry := range []struct {
		role  string
		value *big.Int
	}{{"opening", l.OpeningBacking}, {"deposited", l.Deposited}, {"withdrawn", l.Withdrawn}, {"fees", l.FeesBurned}, {"backing", l.Backing}, {"initialized", big.NewInt(1)}} {
		st.SetState(params.ShieldedPoolAddress, ledgerSlot(asset, entry.role), common.BigToHash(entry.value))
	}
}

func confidentialFeeTicket(hash common.Hash) common.Hash {
	return ShieldedV3StateSlot("confidential-ledger/v1/fee-ticket", hash.Bytes())
}

func burnConfidentialFee(st *state.StateDB, tx *types.Transaction, fee *big.Int) {
	st.SubBalance(params.ShieldedPoolAddress, uint256.MustFromBig(fee), tracing.BalanceDecreaseGasBuy)
	st.SetState(params.ShieldedPoolAddress, confidentialFeeTicket(tx.Hash()), common.BigToHash(fee))
}

// ShieldedPaymentMetadata makes public boundaries machine-readable without
// revealing an output value. Prepaid fee amounts are already public gas data.
func ShieldedPaymentMetadata(tx *types.Transaction) (ShieldedPrivacyBoundary, uint64, error) {
	if tx == nil {
		return "", 0, ErrInvalidShieldedTx
	}
	var deposit bool
	var withdrawal *big.Int
	var mode uint64
	if HasShieldedV4Prefix(tx.Data()) {
		e, ok, err := DecodeShieldedV4Transaction(tx.Data())
		if err != nil || !ok || e == nil {
			return "", 0, ErrInvalidShieldedTx
		}
		deposit, withdrawal, mode = e.Deposit, e.WithdrawalValue, e.FeeMode
	} else {
		e, ok, err := DecodeShieldedV3Transaction(tx.Data())
		if err != nil || !ok || e == nil {
			return "", 0, ErrInvalidShieldedTx
		}
		deposit, withdrawal, mode = e.Deposit, e.WithdrawalValue, e.FeeMode
	}
	if withdrawal == nil {
		return "", 0, ErrInvalidShieldedTx
	}
	if deposit {
		return PublicShielding, mode, nil
	}
	if withdrawal.Sign() > 0 {
		return PublicUnshielding, mode, nil
	}
	return PrivatePayment, mode, nil
}
