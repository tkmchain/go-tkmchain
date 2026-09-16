package core

import (
	"bytes"
	"context"
	"crypto/sha512"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

const ShieldedV3Magic = "TKMSHIELD3"

// Reserve space for headers and reward records within the encoded block cap.
const ShieldedV3MaxTxSize uint64 = params.MaxBlockSize - 64*1024
const ShieldedV3VerifyGas uint64 = 3_000_000

// V3 has a separate canonical encoding and 40-byte native Tip5 state. No V2
// roots, nullifiers, proofs, or field reductions are accepted by this verifier.
type ShieldedV3Output struct {
	Commitment shielded3.Digest
	Incoming   []byte
	Outgoing   []byte
	Stamp      []byte
}
type ShieldedV3Transaction struct {
	Version             uint64
	Deposit             bool
	Anchor              shielded3.Digest
	Nullifier           shielded3.Digest
	StampRoot           shielded3.Digest
	Outputs             [shielded3.OutputSlots]ShieldedV3Output
	WithdrawalRecipient common.Address
	WithdrawalValue     *big.Int
	GasSponsorValue     *big.Int
	Proof               []byte
}

func HasShieldedV3Prefix(data []byte) bool { return bytes.HasPrefix(data, []byte(ShieldedV3Magic)) }
func EncodeShieldedV3Transaction(e *ShieldedV3Transaction) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	payload, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(ShieldedV3Magic), payload...), nil
}
func DecodeShieldedV3Transaction(data []byte) (*ShieldedV3Transaction, bool, error) {
	if !HasShieldedV3Prefix(data) {
		return nil, false, nil
	}
	if uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, true, ErrInvalidShieldedTx
	}
	var e ShieldedV3Transaction
	if err := rlp.DecodeBytes(data[len(ShieldedV3Magic):], &e); err != nil {
		return nil, true, fmt.Errorf("%w: malformed Shield3 envelope", ErrInvalidShieldedTx)
	}
	return &e, true, nil
}
func ShieldedV3Intent(tx *types.Transaction, e *ShieldedV3Transaction) ([64]byte, error) {
	var result [64]byte
	if tx == nil || e == nil {
		return result, ErrInvalidShieldedTx
	}
	clean := *e
	clean.Proof = nil
	data, err := EncodeShieldedV3Transaction(&clean)
	if err != nil {
		return result, err
	}
	algorithm, publicKey, _, _ := tx.PQTkmFields()
	payload := shieldedIntentPayload{Domain: []byte("TKM_SHIELD3_TRANSACTION_INTENT_V1"), TxType: tx.Type(), ChainID: tx.ChainId(), Nonce: tx.Nonce(), GasTipCap: tx.GasTipCap(), GasFeeCap: tx.GasFeeCap(), Gas: tx.Gas(), To: tx.To(), Value: tx.Value(), AccessList: tx.AccessList(), PQAlgorithm: algorithm, PQPublicKey: publicKey, Envelope: data}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return result, err
	}
	return sha512.Sum512(encoded), nil
}

// Ciphertext context contains the full hiding digest, never a bare hash of an
// amount or identity. Roles and chain ID are authenticated by the KEM envelope.
func ShieldedV3OutputContext(chainID uint64, purpose pqcrypto.ShieldedV3Purpose, d shielded3.Digest) pqcrypto.ShieldedV3Context {
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD3_NOTE_ENCRYPTION_CONTEXT_V1"))
	h.Write(d.Bytes())
	var commitment [64]byte
	copy(commitment[:], h.Sum(nil))
	return pqcrypto.ShieldedV3Context{ChainID: chainID, Purpose: purpose, Commitment: commitment}
}
func validateV3Ciphertext(data []byte, ctx pqcrypto.ShieldedV3Context) bool {
	return pqcrypto.ValidateShieldedV3CiphertextContext(data, ctx) == nil
}
func shieldedV3Basics(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction) (*ShieldedV3Transaction, error) {
	fail := func(message string) (*ShieldedV3Transaction, error) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidShieldedTx, message)
	}
	if config == nil || !config.IsAntartical(number, time) || !config.IsPrivacyCommitments(number, time) {
		return fail("Shield3 is not active until Antartical")
	}
	if tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress {
		return fail("Shield3 requires a PQ transaction to the shielded pool")
	}
	if config.ChainID == nil || !config.ChainID.IsUint64() || tx.ChainId().Cmp(config.ChainID) != 0 {
		return fail("Shield3 chain ID mismatch")
	}
	e, _, err := DecodeShieldedV3Transaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if e == nil || e.Version != 3 {
		return fail("expected Shield3 envelope version 3")
	}
	if e.WithdrawalValue == nil || e.GasSponsorValue == nil || e.WithdrawalValue.Sign() < 0 || e.GasSponsorValue.Sign() < 0 {
		return fail("invalid Shield3 public values")
	}
	release := new(big.Int).Add(e.WithdrawalValue, e.GasSponsorValue)
	if e.WithdrawalValue.Cmp(shielded3.MaxSendWei()) > 0 {
		return fail("Shield3 send exceeds 5000000 TKM")
	}
	if (e.WithdrawalValue.Sign() > 0) != (e.WithdrawalRecipient != (common.Address{})) {
		return fail("Shield3 withdrawal recipient/value mismatch")
	}
	if e.Deposit {
		if tx.Value().Sign() <= 0 || tx.Value().Cmp(shielded3.MaxSendWei()) > 0 || release.Sign() != 0 || e.Anchor != (shielded3.Digest{}) || e.Nullifier != (shielded3.Digest{}) {
			return fail("invalid Shield3 deposit or amount exceeds 5000000 TKM")
		}
	} else {
		if tx.Value().Sign() != 0 || e.Anchor == (shielded3.Digest{}) || e.Nullifier == (shielded3.Digest{}) {
			return fail("invalid Shield3 private spend references")
		}
	}
	if _, err := shielded3.DigestFromBytes(e.Anchor.Bytes()); err != nil {
		return fail("noncanonical Shield3 anchor")
	}
	if _, err := shielded3.DigestFromBytes(e.Nullifier.Bytes()); err != nil {
		return fail("noncanonical Shield3 nullifier")
	}
	if e.GasSponsorValue.Cmp(new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())) > 0 {
		return fail("Shield3 gas sponsorship exceeds transaction gas cost")
	}
	seen := make(map[shielded3.Digest]bool)
	for _, out := range e.Outputs {
		if out.Commitment == (shielded3.Digest{}) || seen[out.Commitment] {
			return fail("zero or duplicate Shield3 output commitment")
		}
		seen[out.Commitment] = true
		if _, err := shielded3.DigestFromBytes(out.Commitment.Bytes()); err != nil {
			return fail("noncanonical Shield3 output commitment")
		}
		for _, record := range []struct {
			data []byte
			role pqcrypto.ShieldedV3Purpose
		}{{out.Incoming, pqcrypto.ShieldedV3Incoming}, {out.Outgoing, pqcrypto.ShieldedV3Outgoing}, {out.Stamp, pqcrypto.ShieldedV3Stamp}} {
			if !validateV3Ciphertext(record.data, ShieldedV3OutputContext(config.ChainID.Uint64(), record.role, out.Commitment)) {
				return fail("invalid Shield3 encrypted output context")
			}
		}
	}
	if !shielded3.ValidProofEncoding(e.Proof) {
		return fail("invalid Shield3 proof encoding")
	}
	return e, nil
}
func ShieldedV3Statement(tx *types.Transaction, e *ShieldedV3Transaction) (shielded3.Statement, error) {
	if tx == nil || e == nil || e.WithdrawalValue == nil || e.GasSponsorValue == nil || !tx.ChainId().IsUint64() {
		return shielded3.Statement{}, ErrInvalidShieldedTx
	}
	value := new(big.Int).Add(e.WithdrawalValue, e.GasSponsorValue)
	if e.Deposit {
		value = tx.Value()
	}
	amount, err := shielded3.AmountFromBig(value)
	if err != nil {
		return shielded3.Statement{}, err
	}
	intent, err := ShieldedV3Intent(tx, e)
	if err != nil {
		return shielded3.Statement{}, err
	}
	sponsor, err := shielded3.AmountFromBig(e.GasSponsorValue)
	if err != nil {
		return shielded3.Statement{}, err
	}
	s := shielded3.Statement{ChainID: tx.ChainId().Uint64(), AssetID: shielded3.AssetTKM, PublicValue: amount, GasSponsor: sponsor, Intent: intent, Anchor: e.Anchor, Nullifier: e.Nullifier, StampRoot: e.StampRoot, Deposit: e.Deposit}
	for i := range e.Outputs {
		s.Outputs[i] = e.Outputs[i].Commitment
	}
	return s, nil
}

// ValidateShieldedV3Proof rejects invalid native claims at transaction admission.
// Consensus also verifies independently before applying any state updates.
func ValidateShieldedV3Proof(tx *types.Transaction) error {
	e, ok, err := DecodeShieldedV3Transaction(tx.Data())
	if err != nil || !ok {
		return err
	}
	statement, err := ShieldedV3Statement(tx, e)
	if err != nil {
		return err
	}
	if err := (shielded3.NativeBackend{}).Verify(context.Background(), statement, e.Proof); err != nil {
		return fmt.Errorf("%w: Shield3 proof: %w", ErrInvalidShieldedTx, err)
	}
	return nil
}
func validateShieldedV3State(st *state.StateDB, tx *types.Transaction, e *ShieldedV3Transaction) error {
	// Processing precedes normal value transfer. The reserved pool must remain
	// code-free so execution cannot revert a funded deposit after note creation.
	if st.GetCodeSize(params.ShieldedPoolAddress) != 0 {
		return fmt.Errorf("%w: Shield3 pool must not contain executable code", ErrInvalidShieldedTx)
	}
	if e.StampRoot == (shielded3.Digest{}) || st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/root", e.StampRoot.Bytes())) == (common.Hash{}) {
		return fmt.Errorf("%w: unknown stamp registry root", ErrInvalidShieldedTx)
	}
	if e.WithdrawalValue.Sign() > 0 && !IsAntarticalStamped(st, e.WithdrawalRecipient) {
		return ErrUnstampedAddress
	}
	if !e.Deposit {
		if ShieldedV3NullifierTransaction(st, e.Nullifier) != (common.Hash{}) {
			return fmt.Errorf("%w: Shield3 nullifier already spent", ErrInvalidShieldedTx)
		}
		if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("root", e.Anchor.Bytes())) == (common.Hash{}) {
			return fmt.Errorf("%w: unknown Shield3 note root", ErrInvalidShieldedTx)
		}
	}
	for _, out := range e.Outputs {
		if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("commitment", out.Commitment.Bytes())) != (common.Hash{}) {
			return fmt.Errorf("%w: Shield3 commitment already exists", ErrInvalidShieldedTx)
		}
	}
	release := new(big.Int).Add(e.WithdrawalValue, e.GasSponsorValue)
	if release.BitLen() > 256 || st.GetBalance(params.ShieldedPoolAddress).Cmp(uint256.MustFromBig(release)) < 0 {
		return fmt.Errorf("%w: insufficient shielded pool reserve", ErrInvalidShieldedTx)
	}
	if ShieldedV3NextIndex(st) >= (uint64(1)<<shielded3.MerkleDepth)-3 {
		return fmt.Errorf("%w: Shield3 tree is full", ErrInvalidShieldedTx)
	}
	return nil
}
func processShieldedV3(config *params.ChainConfig, number *big.Int, time uint64, st *state.StateDB, tx *types.Transaction, seen map[common.Hash]struct{}) error {
	e, err := shieldedV3Basics(config, number, time, tx)
	if err != nil {
		return err
	}
	if err = validateShieldedV3State(st, tx, e); err != nil {
		return err
	}
	blockKey := ShieldedV3StateSlot("block-nullifier", e.Nullifier.Bytes())
	if !e.Deposit {
		if _, ok := seen[blockKey]; ok {
			return fmt.Errorf("%w: duplicate Shield3 nullifier in block", ErrInvalidShieldedTx)
		}
	}
	statement, err := ShieldedV3Statement(tx, e)
	if err != nil {
		return err
	}
	if err = (shielded3.NativeBackend{}).Verify(context.Background(), statement, e.Proof); err != nil {
		return fmt.Errorf("%w: Shield3 proof: %w", ErrInvalidShieldedTx, err)
	}
	sender, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	// A failure restores all V3 storage/balance updates and leaves seen untouched.
	snapshot := st.Snapshot()
	for _, out := range e.Outputs {
		if err = appendShieldedV3Leaf(st, out.Commitment, tx.Hash()); err != nil {
			st.RevertToSnapshot(snapshot)
			return err
		}
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("commitment", out.Commitment.Bytes()), tx.Hash())
	}
	if !e.Deposit {
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("nullifier", e.Nullifier.Bytes()), tx.Hash())
		seen[blockKey] = struct{}{}
	}
	for _, release := range []struct {
		to    common.Address
		value *big.Int
	}{{e.WithdrawalRecipient, e.WithdrawalValue}, {sender, e.GasSponsorValue}} {
		if release.value.Sign() > 0 {
			amount := uint256.MustFromBig(release.value)
			st.SubBalance(params.ShieldedPoolAddress, amount, tracing.BalanceChangeTransfer)
			st.AddBalance(release.to, amount, tracing.BalanceChangeTransfer)
		}
	}
	return nil
}

// ShieldedV3GasData prices the bounded proof at one gas per byte plus a fixed
// verifier charge. Other envelope bytes retain ordinary calldata pricing.
func ShieldedV3GasData(data []byte) ([]byte, uint64, error) {
	if HasAntarticalStampPrefix(data) {
		e, err := DecodeAntarticalStamp(data)
		if err != nil {
			return nil, 0, err
		}
		if len(e.Proof) > shielded3.MaxProofSize {
			return nil, 0, ErrInvalidShieldedTx
		}
		gas := AntarticalStampVerifyGas + uint64(len(e.Proof))
		e.Proof = nil
		encoded, err := EncodeAntarticalStamp(e)
		return encoded, gas, err
	}
	e, ok, err := DecodeShieldedV3Transaction(data)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return data, 0, nil
	}
	if len(e.Proof) > shielded3.MaxProofSize {
		return nil, 0, ErrInvalidShieldedTx
	}
	proofGas := ShieldedV3VerifyGas + uint64(len(e.Proof))
	clean := *e
	clean.Proof = nil
	encoded, err := EncodeShieldedV3Transaction(&clean)
	return encoded, proofGas, err
}

// IntrinsicGasWithShield3 preserves historical calldata pricing before the
// fork. Only activated V3 transactions receive bounded native-proof pricing.
func IntrinsicGasWithShield3(data []byte, access types.AccessList, auth []types.SetCodeAuthorization, creation, homestead, eip2028, eip3860, amsterdam, active bool) (vm.GasCosts, error) {
	var proofGas uint64
	if active {
		var err error
		data, proofGas, err = ShieldedV3GasData(data)
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
