package core

import (
	"bytes"
	"context"
	"crypto/sha512"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// Shield5 is a new, independently domain-separated envelope with up to
// sixteen private inputs. V3 and V4 wire formats are intentionally unchanged.
const (
	ShieldedV5Magic     = "TKMSHIELD5"
	ShieldedV5MaxTxSize = ShieldedV3MaxTxSize
	ShieldedV5VerifyGas = ShieldedV3VerifyGas
)

type ShieldedV5Transaction struct {
	Version                uint64
	Deposit                bool
	Anchor                 shielded3.Digest
	Nullifier              shielded3.Digest
	StampRoot              shielded3.Digest
	LinkTag                shielded3.Digest
	Outputs                [shielded3.OutputSlots]ShieldedV4Output
	WithdrawalRecipient    common.Address
	WithdrawalValue        *big.Int
	GasSponsorValue        *big.Int
	Proof                  []byte
	AdditionalNullifiers   [shielded3.InputSlots - 1]shielded3.Digest                           `rlp:"optional"`
	InputCount             uint64                                                               `rlp:"optional"`
	Relayed                bool                                                                 `rlp:"optional"`
	ValidUntil             uint64                                                               `rlp:"optional"`
	AssetID                uint64                                                               `rlp:"optional"`
	FeeMode                uint64                                                               `rlp:"optional"`
	AdditionalNullifiersV5 [shielded3.Shield5InputSlots - shielded3.InputSlots]shielded3.Digest `rlp:"optional"`
}

func HasShieldedV5Prefix(data []byte) bool { return bytes.HasPrefix(data, []byte(ShieldedV5Magic)) }

func EncodeShieldedV5Transaction(e *ShieldedV5Transaction) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	payload, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(ShieldedV5Magic), payload...), nil
}

func DecodeShieldedV5Transaction(data []byte) (*ShieldedV5Transaction, bool, error) {
	if !HasShieldedV5Prefix(data) {
		return nil, false, nil
	}
	if uint64(len(data)) > ShieldedV5MaxTxSize {
		return nil, true, ErrInvalidShieldedTx
	}
	var e ShieldedV5Transaction
	if err := rlp.DecodeBytes(data[len(ShieldedV5Magic):], &e); err != nil {
		return nil, true, fmt.Errorf("%w: malformed Shield5 envelope", ErrInvalidShieldedTx)
	}
	return &e, true, nil
}

func ShieldedV5Nullifiers(e *ShieldedV5Transaction) ([]shielded3.Digest, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	count := e.InputCount
	if !e.Deposit && count == 0 {
		count = 1
	}
	if count > shielded3.Shield5InputSlots || e.Deposit != (count == 0) {
		return nil, ErrInvalidShieldedTx
	}
	all := append([]shielded3.Digest{e.Nullifier}, e.AdditionalNullifiers[:]...)
	all = append(all, e.AdditionalNullifiersV5[:]...)
	seen := make(map[shielded3.Digest]bool, count)
	for i, n := range all {
		if _, err := shielded3.DigestFromBytes(n.Bytes()); err != nil {
			return nil, err
		}
		if uint64(i) < count {
			if n == (shielded3.Digest{}) || seen[n] {
				return nil, ErrInvalidShieldedTx
			}
			seen[n] = true
		} else if n != (shielded3.Digest{}) {
			return nil, ErrInvalidShieldedTx
		}
	}
	return all[:count], nil
}

func ShieldedV5Intent(tx *types.Transaction, e *ShieldedV5Transaction) ([64]byte, error) {
	var result [64]byte
	if tx == nil || e == nil {
		return result, ErrInvalidShieldedTx
	}
	clean := *e
	clean.Proof = nil
	data, err := EncodeShieldedV5Transaction(&clean)
	if err != nil {
		return result, err
	}
	algorithm, publicKey, _, _ := tx.PQTkmFields()
	payload := shieldedIntentPayload{Domain: []byte("TKM_SHIELD5_TRANSACTION_INTENT_V1"), TxType: tx.Type(), ChainID: tx.ChainId(), Nonce: tx.Nonce(), GasTipCap: tx.GasTipCap(), GasFeeCap: tx.GasFeeCap(), Gas: tx.Gas(), To: tx.To(), Value: tx.Value(), AccessList: tx.AccessList(), PQAlgorithm: algorithm, PQPublicKey: publicKey, Envelope: data}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return result, err
	}
	return sha512.Sum512(encoded), nil
}

func shieldedV5AssetID(e *ShieldedV5Transaction) uint64 {
	if e == nil {
		return shielded3.AssetTKM
	}
	return shielded3.NormalizeAssetID(e.AssetID)
}

// Convert only the fields shared with V4 so the already-established fee,
// ciphertext, deposit/withdrawal, address and size rules remain identical.
func shieldedV5AsV4(e *ShieldedV5Transaction) *ShieldedV4Transaction {
	count := e.InputCount
	if count > shielded3.InputSlots {
		count = shielded3.InputSlots
	}
	return &ShieldedV4Transaction{Version: 4, Deposit: e.Deposit, Anchor: e.Anchor, Nullifier: e.Nullifier, StampRoot: e.StampRoot, LinkTag: e.LinkTag, Outputs: e.Outputs, WithdrawalRecipient: e.WithdrawalRecipient, WithdrawalValue: e.WithdrawalValue, GasSponsorValue: e.GasSponsorValue, Proof: e.Proof, AdditionalNullifiers: e.AdditionalNullifiers, InputCount: count, Relayed: e.Relayed, ValidUntil: e.ValidUntil, AssetID: e.AssetID, FeeMode: e.FeeMode}
}

func shieldedV5Basics(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction) (*ShieldedV5Transaction, error) {
	if config == nil || !config.IsShield5(number, time) {
		return nil, fmt.Errorf("%w: Shield5 is not active", ErrInvalidShieldedTx)
	}
	if tx == nil || !HasShieldedV5Prefix(tx.Data()) {
		return nil, ErrInvalidShieldedTx
	}
	e, _, err := DecodeShieldedV5Transaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if e == nil || e.Version != 5 {
		return nil, fmt.Errorf("%w: expected envelope version 5", ErrInvalidShieldedTx)
	}
	if _, err := ShieldedV5Nullifiers(e); err != nil {
		return nil, fmt.Errorf("%w: invalid Shield5 input set", ErrInvalidShieldedTx)
	}
	// Check all of the shared public transaction shape with a synthetic V4
	// envelope whose count is capped to four. V5-specific nullifiers/count and
	// the V5 proof are checked independently above and below.
	shared := shieldedV5AsV4(e)
	if err := ValidateShieldedOnlyTransaction(config, number, time, tx); err != nil {
		return nil, err
	}
	if tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || config.ChainID == nil || tx.ChainId().Cmp(config.ChainID) != 0 {
		return nil, fmt.Errorf("%w: invalid Shield5 transaction domain", ErrInvalidShieldedTx)
	}
	if !shielded3.IsSupportedAsset(e.AssetID) || shared.WithdrawalValue == nil || shared.GasSponsorValue == nil || shared.WithdrawalValue.Sign() < 0 || shared.GasSponsorValue.Sign() < 0 {
		return nil, ErrInvalidShieldedTx
	}
	if err := validateConfidentialLedgerShape(config, number, time, tx, e.Deposit, e.GasSponsorValue, e.FeeMode, shieldedV5AssetID(e)); err != nil {
		return nil, err
	}
	if err := ValidateShieldedV4Time(shared, time); err != nil {
		return nil, err
	}
	maxGasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())
	if e.GasSponsorValue.Cmp(maxGasCost) > 0 {
		return nil, fmt.Errorf("%w: gas sponsorship exceeds transaction gas cost", ErrInvalidShieldedTx)
	}
	if e.Relayed && !config.IsShieldedOnly(number, time) && !config.IsConfidentialLedger(number, time) && e.GasSponsorValue.Cmp(maxGasCost) != 0 {
		return nil, fmt.Errorf("%w: relay requires the exact authorized gas reserve", ErrInvalidShieldedTx)
	}
	if e.WithdrawalValue.Cmp(shielded3.MaxSendWei()) > 0 {
		return nil, fmt.Errorf("%w: send exceeds 5000000 TKM", ErrInvalidShieldedTx)
	}
	if (e.WithdrawalValue.Sign() > 0) != (e.WithdrawalRecipient != (common.Address{})) {
		return nil, ErrInvalidShieldedTx
	}
	if e.Deposit {
		if tx.Value().Sign() <= 0 || tx.Value().Cmp(shielded3.MaxSendWei()) > 0 || e.Anchor != (shielded3.Digest{}) || e.Nullifier != (shielded3.Digest{}) || e.LinkTag != (shielded3.Digest{}) || e.GasSponsorValue.Sign() != 0 || e.WithdrawalValue.Sign() != 0 {
			return nil, ErrInvalidShieldedTx
		}
	} else if tx.Value().Sign() != 0 || e.Anchor == (shielded3.Digest{}) || e.Nullifier == (shielded3.Digest{}) || e.LinkTag == (shielded3.Digest{}) {
		return nil, ErrInvalidShieldedTx
	}
	if e.GasSponsorValue.Cmp(new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())) > 0 {
		return nil, ErrInvalidShieldedTx
	}
	if shieldedV5AssetID(e) == shielded3.AssetPTKM && e.GasSponsorValue.Sign() != 0 {
		return nil, ErrInvalidShieldedTx
	}
	seen := make(map[shielded3.Digest]bool)
	keys := make(map[string]bool)
	for _, out := range e.Outputs {
		if out.Commitment == (shielded3.Digest{}) || seen[out.Commitment] || len(out.OneTimeKey) != 40 || keys[string(out.OneTimeKey)] {
			return nil, ErrInvalidShieldedTx
		}
		if _, err := shielded3.DigestFromBytes(out.Commitment.Bytes()); err != nil {
			return nil, fmt.Errorf("%w: noncanonical output commitment", ErrInvalidShieldedTx)
		}
		if bytes.Equal(out.OneTimeKey, make([]byte, 40)) {
			return nil, fmt.Errorf("%w: zero one-time output key", ErrInvalidShieldedTx)
		}
		seen[out.Commitment] = true
		keys[string(out.OneTimeKey)] = true
		if _, err := shielded3.DigestFromBytes(out.OneTimeKey); err != nil {
			return nil, err
		}
		for _, item := range []struct {
			data []byte
			role pqcrypto.ShieldedV3Purpose
		}{{out.Incoming, pqcrypto.ShieldedV3Incoming}, {out.Outgoing, pqcrypto.ShieldedV3Outgoing}, {out.Stamp, pqcrypto.ShieldedV3Stamp}} {
			if !validateV3Ciphertext(item.data, ShieldedV3OutputContext(config.ChainID.Uint64(), item.role, out.Commitment)) {
				return nil, ErrInvalidShieldedTx
			}
		}
	}
	if !shielded3.ValidProofEncoding(e.Proof) {
		return nil, ErrInvalidShieldedTx
	}
	return e, nil
}

func ShieldedV5Statement(tx *types.Transaction, e *ShieldedV5Transaction) (shielded3.StatementV5, error) {
	if tx == nil || e == nil || e.WithdrawalValue == nil || e.GasSponsorValue == nil || !tx.ChainId().IsUint64() {
		return shielded3.StatementV5{}, ErrInvalidShieldedTx
	}
	count := e.InputCount
	if !e.Deposit && count == 0 {
		count = 1
	}
	if count > shielded3.Shield5InputSlots {
		return shielded3.StatementV5{}, ErrInvalidShieldedTx
	}
	value := new(big.Int).Add(e.WithdrawalValue, e.GasSponsorValue)
	if e.Deposit {
		value = tx.Value()
	}
	amount, err := shielded3.AmountFromBig(value)
	if err != nil {
		return shielded3.StatementV5{}, err
	}
	intent, err := ShieldedV5Intent(tx, e)
	if err != nil {
		return shielded3.StatementV5{}, err
	}
	sponsor, err := shielded3.AmountFromBig(e.GasSponsorValue)
	if err != nil {
		return shielded3.StatementV5{}, err
	}
	base := shielded3.Statement{ChainID: tx.ChainId().Uint64(), AssetID: shieldedV5AssetID(e), PublicValue: amount, GasSponsor: sponsor, Intent: intent, Anchor: e.Anchor, Nullifier: e.Nullifier, StampRoot: e.StampRoot, Deposit: e.Deposit, InputCount: uint32(count), AdditionalNullifiers: e.AdditionalNullifiers}
	for i := range e.Outputs {
		base.Outputs[i] = e.Outputs[i].Commitment
		key, err := shielded3.DigestFromBytes(e.Outputs[i].OneTimeKey)
		if err != nil {
			return shielded3.StatementV5{}, err
		}
		base.OneTimeKeys[i] = key
	}
	return shielded3.StatementV5{Statement: base, LinkTag: e.LinkTag, AdditionalNullifiersV5: e.AdditionalNullifiersV5}, nil
}

func ValidateShieldedV5Proof(tx *types.Transaction) error {
	if tx == nil {
		return ErrInvalidShieldedTx
	}
	e, ok, err := DecodeShieldedV5Transaction(tx.Data())
	if err != nil || !ok {
		return err
	}
	statement, err := ShieldedV5Statement(tx, e)
	if err != nil {
		return err
	}
	if err = (shielded3.NativeBackendV5{}).Verify(context.Background(), statement, e.Proof); err != nil {
		return fmt.Errorf("%w: Shield5 proof: %w", ErrInvalidShieldedTx, err)
	}
	return nil
}

func ShieldedV5GasData(data []byte) ([]byte, uint64, error) {
	e, ok, err := DecodeShieldedV5Transaction(data)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return data, 0, nil
	}
	if len(e.Proof) > shielded3.MaxProofSize {
		return nil, 0, ErrInvalidShieldedTx
	}
	clean := *e
	clean.Proof = nil
	encoded, err := EncodeShieldedV5Transaction(&clean)
	return encoded, ShieldedV5VerifyGas + uint64(len(e.Proof)), err
}

func IntrinsicGasWithShield5(data []byte, access types.AccessList, auth []types.SetCodeAuthorization, creation, homestead, eip2028, eip3860, amsterdam, active bool) (vm.GasCosts, error) {
	var proofGas uint64
	var err error
	if active {
		data, proofGas, err = ShieldedV5GasData(data)
		if err != nil {
			return vm.GasCosts{}, err
		}
	}
	gas, err := IntrinsicGas(data, access, auth, creation, homestead, eip2028, eip3860, amsterdam)
	if err != nil {
		return gas, err
	}
	if ^uint64(0)-gas.RegularGas < proofGas {
		return vm.GasCosts{}, ErrGasUintOverflow
	}
	gas.RegularGas += proofGas
	return gas, nil
}
