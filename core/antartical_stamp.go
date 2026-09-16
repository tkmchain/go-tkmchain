package core

import (
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

const AntarticalStampMagic = "TKMSTAMP1"
const AntarticalStampVerifyGas uint64 = 1_000_000

var ErrUnstampedAddress = errors.New("illegal transaction: address has no confirmed Antartical stamp")

type AntarticalStampRegistration struct {
	Version uint64
	Owner   shielded3.Digest
	Stamp   pqcrypto.ShieldedV3StampRecord
	Proof   []byte
}
type AntarticalStampStatus struct {
	Registered      bool             `json:"registered"`
	Owner           shielded3.Digest `json:"owner"`
	Commitment      [64]byte         `json:"commitment"`
	TransactionHash common.Hash      `json:"transactionHash"`
}

func HasAntarticalStampPrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(AntarticalStampMagic))
}
func EncodeAntarticalStamp(e *AntarticalStampRegistration) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	encoded, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(AntarticalStampMagic), encoded...), nil
}
func DecodeAntarticalStamp(data []byte) (*AntarticalStampRegistration, error) {
	if !HasAntarticalStampPrefix(data) || uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, ErrInvalidShieldedTx
	}
	var e AntarticalStampRegistration
	if err := rlp.DecodeBytes(data[len(AntarticalStampMagic):], &e); err != nil {
		return nil, fmt.Errorf("invalid stamp registration: %w", err)
	}
	return &e, nil
}
func AntarticalStampIntent(tx *types.Transaction, e *AntarticalStampRegistration) ([64]byte, error) {
	if tx == nil || e == nil {
		return [64]byte{}, ErrInvalidShieldedTx
	}
	clean := *e
	clean.Proof = nil
	data, err := EncodeAntarticalStamp(&clean)
	if err != nil {
		return [64]byte{}, err
	}
	algorithm, publicKey, _, _ := tx.PQTkmFields()
	payload := shieldedIntentPayload{Domain: []byte("TKM_ANTARTICAL_STAMP_REGISTRATION_V1"), TxType: tx.Type(), ChainID: tx.ChainId(), Nonce: tx.Nonce(), GasTipCap: tx.GasTipCap(), GasFeeCap: tx.GasFeeCap(), Gas: tx.Gas(), To: tx.To(), Value: tx.Value(), AccessList: tx.AccessList(), PQAlgorithm: algorithm, PQPublicKey: publicKey, Envelope: data}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return [64]byte{}, err
	}
	return sha512.Sum512(encoded), nil
}
func ValidateAntarticalStampBasics(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction) error {
	if config == nil || config.ChainID == nil || !config.IsAntartical(number, time) {
		return errors.New("stamp registration is not active until Antartical")
	}
	if tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 || tx.ChainId().Cmp(config.ChainID) != 0 {
		return errors.New("stamp registration requires a zero-value PQ transaction to the reserved pool")
	}
	e, err := DecodeAntarticalStamp(tx.Data())
	if err != nil {
		return err
	}
	algorithm, publicKey, _, _ := tx.PQTkmFields()
	if e.Version != 1 || !config.ChainID.IsUint64() || e.Stamp.ChainID != config.ChainID.Uint64() || algorithm != pqcrypto.AlgorithmMLDSA87 || !pqcrypto.VerifyShieldedV3Stamp(publicKey, &e.Stamp) || e.Owner == (shielded3.Digest{}) || !shielded3.ValidProofEncoding(e.Proof) {
		return errors.New("invalid signed stamp or owner proof")
	}
	if _, err := shielded3.DigestFromBytes(e.Owner.Bytes()); err != nil {
		return err
	}
	return nil
}
func ValidateAntarticalStampProof(tx *types.Transaction) error {
	e, err := DecodeAntarticalStamp(tx.Data())
	if err != nil {
		return err
	}
	intent, err := AntarticalStampIntent(tx, e)
	if err != nil {
		return err
	}
	return (shielded3.NativeBackend{}).VerifyOwner(context.Background(), e.Stamp.ChainID, e.Owner, intent, e.Proof)
}
func IsAntarticalStamped(st shieldedStateReader, address common.Address) bool {
	return st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", address.Bytes())) != (common.Hash{})
}
func AntarticalStampForAddress(st shieldedStateReader, address common.Address) (AntarticalStampStatus, error) {
	s := AntarticalStampStatus{TransactionHash: st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", address.Bytes()))}
	s.Registered = s.TransactionHash != (common.Hash{})
	if !s.Registered {
		return s, nil
	}
	var err error
	s.Owner, err = v3ReadDigest(st, "stamp/address-owner", address.Bytes())
	a := st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address-commitment/0", address.Bytes()))
	b := st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address-commitment/1", address.Bytes()))
	copy(s.Commitment[:32], a[:])
	copy(s.Commitment[32:], b[:])
	return s, err
}

// Registration is the only transaction an unstamped address may submit. It
// carries no transferred value. A wallet-file stamp alone never grants access.
func ValidateAntarticalStampState(st shieldedStateReader, from common.Address, to *common.Address, value *big.Int, data []byte) error {
	if HasAntarticalStampPrefix(data) {
		if to == nil || *to != params.ShieldedPoolAddress || value.Sign() != 0 {
			return errors.New("illegal value transfer disguised as stamp registration")
		}
		return nil
	}
	if !IsAntarticalStamped(st, from) {
		return fmt.Errorf("%w: sender %s", ErrUnstampedAddress, from)
	}
	if value.Sign() > 0 && (to == nil || (*to != params.ShieldedPoolAddress && !IsAntarticalStamped(st, *to))) {
		return fmt.Errorf("%w: recipient", ErrUnstampedAddress)
	}
	if HasShieldedV3Prefix(data) {
		e, _, err := DecodeShieldedV3Transaction(data)
		if err != nil {
			return err
		}
		if e.WithdrawalValue != nil && e.WithdrawalValue.Sign() > 0 && !IsAntarticalStamped(st, e.WithdrawalRecipient) {
			return fmt.Errorf("%w: withdrawal recipient", ErrUnstampedAddress)
		}
	} else if e, ok, err := DecodeShieldedTransaction(data); err != nil {
		return err
	} else if ok && shieldedWithdrawalValue(e).Sign() > 0 && !IsAntarticalStamped(st, e.WithdrawalRecipient) {
		return fmt.Errorf("%w: migration recipient", ErrUnstampedAddress)
	}
	return nil
}

func ProcessAntarticalStamp(config *params.ChainConfig, number *big.Int, time uint64, st *state.StateDB, tx *types.Transaction) error {
	if err := ValidateAntarticalStampBasics(config, number, time, tx); err != nil {
		return err
	}
	from, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	existing := st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", from.Bytes()))
	if existing == tx.Hash() {
		return nil
	} // same transaction's processing stage
	if existing != (common.Hash{}) {
		return errors.New("illegal transaction: address stamp is immutable and already registered")
	}
	if st.GetCodeSize(params.ShieldedPoolAddress) != 0 {
		return errors.New("stamp registry must not contain executable code")
	}
	e, err := DecodeAntarticalStamp(tx.Data())
	if err != nil {
		return err
	}
	if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/owner-index", e.Owner.Bytes())) != (common.Hash{}) {
		return errors.New("stamp owner is already registered")
	}
	if err := ValidateAntarticalStampProof(tx); err != nil {
		return fmt.Errorf("illegal stamp ownership proof: %w", err)
	}
	snapshot := st.Snapshot()
	if err := appendAntarticalStamp(st, e.Stamp.ChainID, e.Owner, tx.Hash()); err != nil {
		st.RevertToSnapshot(snapshot)
		return err
	}
	// Storage does not make an EIP-161 account nonempty. Keep the reserved
	// code-free registry alive even when its monetary reserve is zero.
	if st.GetNonce(params.ShieldedPoolAddress) == 0 {
		st.SetNonce(params.ShieldedPoolAddress, 1, tracing.NonceChangeUnspecified)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", from.Bytes()), tx.Hash())
	v3WriteDigest(st, "stamp/address-owner", from.Bytes(), e.Owner)
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address-commitment/0", from.Bytes()), common.BytesToHash(e.Stamp.Commitment[:32]))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address-commitment/1", from.Bytes()), common.BytesToHash(e.Stamp.Commitment[32:]))
	return nil
}

func stampNode(st shieldedStateReader, level int, index uint64, zeroes [shielded3.MerkleDepth + 1]shielded3.Digest) (shielded3.Digest, error) {
	key := v3NodeKey(level, index)
	if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/node-present", key)) == (common.Hash{}) {
		return zeroes[level], nil
	}
	return v3ReadDigest(st, "stamp/node", key)
}
func appendAntarticalStamp(st shieldedStateWriter, chainID uint64, owner shielded3.Digest, txHash common.Hash) error {
	index := uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/next-index", nil)))
	if index >= uint64(1)<<shielded3.MerkleDepth {
		return errors.New("stamp registry is full")
	}
	zeroes, err := v3Zeroes()
	if err != nil {
		return err
	}
	current, err := shielded3.StampLeaf(chainID, owner)
	if err != nil {
		return err
	}
	cursor := index
	for level := 0; level <= shielded3.MerkleDepth; level++ {
		key := v3NodeKey(level, cursor)
		v3WriteDigest(st, "stamp/node", key, current)
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/node-present", key), uint64Hash(1))
		if level == shielded3.MerkleDepth {
			break
		}
		sibling, err := stampNode(st, level, cursor^1, zeroes)
		if err != nil {
			return err
		}
		if cursor&1 == 0 {
			current, err = shielded3.HashPair(current, sibling)
		} else {
			current, err = shielded3.HashPair(sibling, current)
		}
		if err != nil {
			return err
		}
		cursor >>= 1
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/owner-index", owner.Bytes()), uint64Hash(index+1))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/next-index", nil), uint64Hash(index+1))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/root", current.Bytes()), txHash)
	v3WriteDigest(st, "stamp/current-root", nil, current)
	return nil
}
func AntarticalStampPath(st shieldedStateReader, owner shielded3.Digest) (ShieldedV3Path, error) {
	result := ShieldedV3Path{Commitment: owner}
	index := uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/owner-index", owner.Bytes())))
	if index == 0 {
		return result, nil
	}
	result.Found = true
	result.Index = index - 1
	zeroes, err := v3Zeroes()
	if err != nil {
		return result, err
	}
	cursor := result.Index
	for level := range result.Path {
		result.Path[level], err = stampNode(st, level, cursor^1, zeroes)
		if err != nil {
			return result, err
		}
		cursor >>= 1
	}
	result.Root, err = v3ReadDigest(st, "stamp/current-root", nil)
	return result, err
}
