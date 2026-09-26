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
	"github.com/ethereum/go-ethereum/zk/shielded4"
)

// Shield4 is a versioned full-chain membership envelope. The proof uses the
// existing append-only Tip5 tree as its canonical anonymity set, while its
// native claim and linkability tag are distinct from Shield3.
const (
	ShieldedV4Magic     = "TKMSHIELD4"
	ShieldedV4MaxTxSize = ShieldedV3MaxTxSize
	ShieldedV4VerifyGas = ShieldedV3VerifyGas
)

type ShieldedV4Output struct {
	Commitment shielded4.Digest
	Incoming   []byte
	Outgoing   []byte
	Stamp      []byte
	OneTimeKey []byte `rlp:"optional"`
}

type ShieldedV4Transaction struct {
	Version              uint64
	Deposit              bool
	Anchor               shielded4.Digest
	Nullifier            shielded4.Digest
	StampRoot            shielded4.Digest
	LinkTag              shielded4.Digest
	Outputs              [shielded4.OutputSlots]ShieldedV4Output
	WithdrawalRecipient  common.Address
	WithdrawalValue      *big.Int
	GasSponsorValue      *big.Int
	Proof                []byte
	AdditionalNullifiers [shielded4.InputSlots - 1]shielded4.Digest `rlp:"optional"`
	InputCount           uint64                                     `rlp:"optional"`
	Relayed              bool                                       `rlp:"optional"`
	ValidUntil           uint64                                     `rlp:"optional"`
	// AssetID is optional for wire compatibility; zero means native TKM.
	AssetID uint64 `rlp:"optional"`
}

func shieldedV4AssetID(e *ShieldedV4Transaction) uint64 {
	if e == nil {
		return shielded3.AssetTKM
	}
	return shielded3.NormalizeAssetID(e.AssetID)
}

func HasShieldedV4Prefix(data []byte) bool { return bytes.HasPrefix(data, []byte(ShieldedV4Magic)) }

func EncodeShieldedV4Transaction(e *ShieldedV4Transaction) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	payload, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(ShieldedV4Magic), payload...), nil
}

func DecodeShieldedV4Transaction(data []byte) (*ShieldedV4Transaction, bool, error) {
	if !HasShieldedV4Prefix(data) {
		return nil, false, nil
	}
	if uint64(len(data)) > ShieldedV4MaxTxSize {
		return nil, true, ErrInvalidShieldedTx
	}
	var e ShieldedV4Transaction
	if err := rlp.DecodeBytes(data[len(ShieldedV4Magic):], &e); err != nil {
		return nil, true, fmt.Errorf("%w: malformed Shield4 envelope", ErrInvalidShieldedTx)
	}
	return &e, true, nil
}

func ShieldedV4Nullifiers(e *ShieldedV4Transaction) ([]shielded4.Digest, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	count := e.InputCount
	if !e.Deposit && count == 0 {
		count = 1
	}
	if count > shielded4.InputSlots || e.Deposit != (count == 0) {
		return nil, ErrInvalidShieldedTx
	}
	all := append([]shielded4.Digest{e.Nullifier}, e.AdditionalNullifiers[:]...)
	seen := make(map[shielded4.Digest]bool)
	for i, n := range all {
		if _, err := shielded4.DigestFromBytes(n.Bytes()); err != nil {
			return nil, err
		}
		if uint64(i) < count {
			if n == (shielded4.Digest{}) || seen[n] {
				return nil, ErrInvalidShieldedTx
			}
			seen[n] = true
		} else if n != (shielded4.Digest{}) {
			return nil, ErrInvalidShieldedTx
		}
	}
	return all[:count], nil
}

func ValidateShieldedV4Time(e *ShieldedV4Transaction, time uint64) error {
	if e.Relayed {
		if e.Deposit || e.WithdrawalValue == nil || e.WithdrawalValue.Sign() != 0 || e.ValidUntil <= time || e.ValidUntil-time > AntarticalStampSponsorshipLifetime {
			return fmt.Errorf("%w: invalid or expired relay authorization", ErrInvalidShieldedTx)
		}
	} else if e.ValidUntil != 0 {
		return fmt.Errorf("%w: expiry requires relay mode", ErrInvalidShieldedTx)
	}
	return nil
}

func ShieldedV4Intent(tx *types.Transaction, e *ShieldedV4Transaction) ([64]byte, error) {
	var result [64]byte
	if tx == nil || e == nil {
		return result, ErrInvalidShieldedTx
	}
	clean := *e
	clean.Proof = nil
	data, err := EncodeShieldedV4Transaction(&clean)
	if err != nil {
		return result, err
	}
	algorithm, publicKey, _, _ := tx.PQTkmFields()
	payload := shieldedIntentPayload{Domain: []byte("TKM_SHIELD4_TRANSACTION_INTENT_V1"), TxType: tx.Type(), ChainID: tx.ChainId(), Nonce: tx.Nonce(), GasTipCap: tx.GasTipCap(), GasFeeCap: tx.GasFeeCap(), Gas: tx.Gas(), To: tx.To(), Value: tx.Value(), AccessList: tx.AccessList(), PQAlgorithm: algorithm, PQPublicKey: publicKey, Envelope: data}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return result, err
	}
	return sha512.Sum512(encoded), nil
}

func shieldedV4Basics(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction) (*ShieldedV4Transaction, error) {
	fail := func(message string) (*ShieldedV4Transaction, error) {
		return nil, fmt.Errorf("%w: Shield4 %s", ErrInvalidShieldedTx, message)
	}
	if config == nil || !config.IsAntartical(number, time) || !config.IsPrivacyCommitments(number, time) {
		return fail("is not active until Antartical")
	}
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress {
		return fail("requires a PQ transaction to the shielded pool")
	}
	if config.ChainID == nil || !config.ChainID.IsUint64() || tx.ChainId().Cmp(config.ChainID) != 0 {
		return fail("chain ID mismatch")
	}
	e, _, err := DecodeShieldedV4Transaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if e == nil || e.Version != 4 {
		return fail("expected envelope version 4")
	}
	if !shielded3.IsSupportedAsset(e.AssetID) {
		return fail("unsupported Shield4 asset ID")
	}
	if e.WithdrawalValue == nil || e.GasSponsorValue == nil || e.WithdrawalValue.Sign() < 0 || e.GasSponsorValue.Sign() < 0 {
		return fail("invalid public values")
	}
	if err := ValidateShieldedV4Time(e, time); err != nil {
		return nil, err
	}
	if _, err := ShieldedV4Nullifiers(e); err != nil {
		return fail("invalid input count or nullifiers")
	}
	if e.Relayed && e.GasSponsorValue.Cmp(new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())) != 0 {
		return fail("relay requires the exact authorized gas reserve")
	}
	release := new(big.Int).Add(e.WithdrawalValue, e.GasSponsorValue)
	if e.WithdrawalValue.Cmp(shielded4.MaxSendWei()) > 0 {
		return fail("send exceeds 5000000 TKM")
	}
	if (e.WithdrawalValue.Sign() > 0) != (e.WithdrawalRecipient != (common.Address{})) {
		return fail("withdrawal recipient/value mismatch")
	}
	if e.Deposit {
		if tx.Value().Sign() <= 0 || tx.Value().Cmp(shielded4.MaxSendWei()) > 0 || release.Sign() != 0 || e.Anchor != (shielded4.Digest{}) || e.Nullifier != (shielded4.Digest{}) || e.LinkTag != (shielded4.Digest{}) {
			return fail("invalid deposit or amount exceeds 5000000 TKM")
		}
	} else if tx.Value().Sign() != 0 || e.Anchor == (shielded4.Digest{}) || e.Nullifier == (shielded4.Digest{}) || e.LinkTag == (shielded4.Digest{}) {
		return fail("invalid private spend references")
	}
	for _, d := range []shielded4.Digest{e.Anchor, e.Nullifier, e.StampRoot, e.LinkTag} {
		if _, err := shielded4.DigestFromBytes(d.Bytes()); err != nil {
			return fail("noncanonical digest")
		}
	}
	if e.GasSponsorValue.Cmp(new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())) > 0 {
		return fail("gas sponsorship exceeds transaction gas cost")
	}
	if shieldedV4AssetID(e) == shielded3.AssetPTKM && e.GasSponsorValue.Sign() != 0 {
		return fail("wrapped private TKM cannot sponsor public gas")
	}
	seen := make(map[shielded4.Digest]bool)
	seenOneTime := make(map[string]bool)
	for _, out := range e.Outputs {
		if out.Commitment == (shielded4.Digest{}) || seen[out.Commitment] {
			return fail("zero or duplicate output commitment")
		}
		seen[out.Commitment] = true
		if len(out.OneTimeKey) != 40 || bytes.Equal(out.OneTimeKey, make([]byte, 40)) || seenOneTime[string(out.OneTimeKey)] {
			return fail("invalid or duplicate one-time output key")
		}
		seenOneTime[string(out.OneTimeKey)] = true
		if _, err := shielded4.DigestFromBytes(out.Commitment.Bytes()); err != nil {
			return fail("noncanonical output commitment")
		}
		if _, err := shielded4.DigestFromBytes(out.OneTimeKey); err != nil {
			return fail("noncanonical one-time output key")
		}
		for _, record := range []struct {
			data []byte
			role pqcrypto.ShieldedV3Purpose
		}{{out.Incoming, pqcrypto.ShieldedV3Incoming}, {out.Outgoing, pqcrypto.ShieldedV3Outgoing}, {out.Stamp, pqcrypto.ShieldedV3Stamp}} {
			if !validateV3Ciphertext(record.data, ShieldedV3OutputContext(config.ChainID.Uint64(), record.role, out.Commitment)) {
				return fail("invalid encrypted output context")
			}
		}
	}
	if !shielded4.ValidProofEncoding(e.Proof) {
		return fail("invalid STARK proof encoding")
	}
	return e, nil
}

func ShieldedV4Statement(tx *types.Transaction, e *ShieldedV4Transaction) (shielded4.Statement, error) {
	if tx == nil || e == nil || e.WithdrawalValue == nil || e.GasSponsorValue == nil || !tx.ChainId().IsUint64() {
		return shielded4.Statement{}, ErrInvalidShieldedTx
	}
	if !shielded3.IsSupportedAsset(e.AssetID) {
		return shielded4.Statement{}, fmt.Errorf("%w: unsupported Shield4 asset ID", ErrInvalidShieldedTx)
	}
	inputCount := e.InputCount
	if !e.Deposit && inputCount == 0 {
		inputCount = 1
	}
	if inputCount > shielded4.InputSlots {
		return shielded4.Statement{}, ErrInvalidShieldedTx
	}
	value := new(big.Int).Add(e.WithdrawalValue, e.GasSponsorValue)
	if e.Deposit {
		value = tx.Value()
	}
	amount, err := shielded4.AmountFromBig(value)
	if err != nil {
		return shielded4.Statement{}, err
	}
	intent, err := ShieldedV4Intent(tx, e)
	if err != nil {
		return shielded4.Statement{}, err
	}
	sponsor, err := shielded4.AmountFromBig(e.GasSponsorValue)
	if err != nil {
		return shielded4.Statement{}, err
	}
	base := shielded3.Statement{ChainID: tx.ChainId().Uint64(), AssetID: shieldedV4AssetID(e), PublicValue: amount, GasSponsor: sponsor, Intent: intent, Anchor: e.Anchor, Nullifier: e.Nullifier, StampRoot: e.StampRoot, Deposit: e.Deposit, InputCount: uint32(inputCount), AdditionalNullifiers: e.AdditionalNullifiers}
	for i := range e.Outputs {
		base.Outputs[i] = e.Outputs[i].Commitment
		key, err := shielded4.DigestFromBytes(e.Outputs[i].OneTimeKey)
		if err != nil {
			return shielded4.Statement{}, err
		}
		base.OneTimeKeys[i] = key
	}
	return shielded4.Statement{Statement: base, LinkTag: e.LinkTag}, nil
}

func ValidateShieldedV4Proof(tx *types.Transaction) error {
	e, ok, err := DecodeShieldedV4Transaction(tx.Data())
	if err != nil || !ok {
		return err
	}
	statement, err := ShieldedV4Statement(tx, e)
	if err != nil {
		return err
	}
	if err := (shielded4.NativeBackend{}).Verify(context.Background(), statement, e.Proof); err != nil {
		return fmt.Errorf("%w: Shield4 proof: %w", ErrInvalidShieldedTx, err)
	}
	return nil
}

func ShieldedV4GasData(data []byte) ([]byte, uint64, error) {
	e, ok, err := DecodeShieldedV4Transaction(data)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return data, 0, nil
	}
	if len(e.Proof) > shielded4.MaxProofSize {
		return nil, 0, ErrInvalidShieldedTx
	}
	clean := *e
	clean.Proof = nil
	encoded, err := EncodeShieldedV4Transaction(&clean)
	return encoded, ShieldedV4VerifyGas + uint64(len(e.Proof)), err
}

func IntrinsicGasWithShield4(data []byte, access types.AccessList, auth []types.SetCodeAuthorization, creation, homestead, eip2028, eip3860, amsterdam, active bool) (vm.GasCosts, error) {
	var proofGas uint64
	if active {
		var err error
		data, proofGas, err = ShieldedV4GasData(data)
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
