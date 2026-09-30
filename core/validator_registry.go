package core

// Antartical validator registration is deliberately implemented as a
// consensus envelope rather than an EVM contract.  This keeps the validator
// set in the state root, makes activation deterministic for every client, and
// prevents a contract implementation from being replaced by a different
// interpreter.

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

const (
	ValidatorRegistrationMagic  = "TKMVALREG1"
	ValidatorSlashMagic         = "TKMVSLASH1"
	ValidatorEnvelopeVersion    = uint64(1)
	ValidatorActivationDelay    = uint64(720)
	ValidatorUnbondingPeriod    = uint64(21600)
	ValidatorMaxSetSize         = uint64(4096)
	ValidatorBondTKM            = uint64(500_000)
	ValidatorRegistrationFeeTKM = uint64(100)
)

var (
	ErrValidatorNotActive         = errors.New("validator registration is not active until Antartical")
	ErrValidatorAlreadyRegistered = errors.New("validator is already registered")
	ErrValidatorSetFull           = errors.New("validator set is full")
	ErrValidatorNotFound          = errors.New("validator is not registered")
	ErrValidatorEvidenceInvalid   = errors.New("invalid validator slashing evidence")
	ErrValidatorRewardInvalid     = errors.New("invalid validator reward transaction")
)

func validatorAmount(tkm uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(tkm), big.NewInt(params.Ether))
}

func ValidatorBondWei() *big.Int            { return validatorAmount(ValidatorBondTKM) }
func ValidatorRegistrationFeeWei() *big.Int { return validatorAmount(ValidatorRegistrationFeeTKM) }

func ValidateValidatorRegistrationBalance(st *state.StateDB, from common.Address) error {
	if st == nil {
		return errors.New("validator registration state is unavailable")
	}
	required := new(big.Int).Add(ValidatorBondWei(), ValidatorRegistrationFeeWei())
	if st.GetBalance(from).ToBig().Cmp(required) < 0 {
		return errors.New("validator registration requires bond and fee balance")
	}
	return nil
}

// ValidatorRegistration is the RLP payload following ValidatorRegistrationMagic.
// The PQ transaction sender must equal the ML-DSA address derived from PublicKey.
type ValidatorRegistration struct {
	Version          uint64
	PublicKey        []byte
	RewardAddress    common.Address
	ActivationHeight uint64
}

func HasValidatorRegistrationPrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(ValidatorRegistrationMagic))
}

func EncodeValidatorRegistration(e *ValidatorRegistration) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	payload, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(ValidatorRegistrationMagic), payload...), nil
}

func DecodeValidatorRegistration(data []byte) (*ValidatorRegistration, error) {
	if !HasValidatorRegistrationPrefix(data) || uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, ErrInvalidShieldedTx
	}
	var e ValidatorRegistration
	if err := rlp.DecodeBytes(data[len(ValidatorRegistrationMagic):], &e); err != nil {
		return nil, fmt.Errorf("invalid validator registration: %w", err)
	}
	return &e, nil
}

// ValidatorSlashEvidence proves that one registered validator signed two
// different block hashes at the same height.  The evidence is independently
// verifiable and is replay-protected by its evidence digest in state.
type ValidatorSlashEvidence struct {
	Version    uint64
	Validator  common.Address
	Height     uint64
	BlockHashA common.Hash
	BlockHashB common.Hash
	SignatureA []byte
	SignatureB []byte
	PublicKey  []byte
}

func HasValidatorSlashPrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(ValidatorSlashMagic))
}

func EncodeValidatorSlash(e *ValidatorSlashEvidence) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	payload, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(ValidatorSlashMagic), payload...), nil
}

func DecodeValidatorSlash(data []byte) (*ValidatorSlashEvidence, error) {
	if !HasValidatorSlashPrefix(data) || uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, ErrInvalidShieldedTx
	}
	var e ValidatorSlashEvidence
	if err := rlp.DecodeBytes(data[len(ValidatorSlashMagic):], &e); err != nil {
		return nil, fmt.Errorf("invalid validator slash evidence: %w", err)
	}
	return &e, nil
}

// ValidatorAttestationDigest is the exact message validators sign when
// attesting to a block.  The domain and height prevent cross-protocol and
// cross-height replay.
func ValidatorAttestationDigest(height uint64, blockHash common.Hash) common.Hash {
	var encoded [8]byte
	for i := uint(0); i < 8; i++ {
		encoded[7-i] = byte(height >> (i * 8))
	}
	return crypto.Keccak256Hash([]byte("TKM_VALIDATOR_ATTEST_V1"), encoded[:], blockHash.Bytes())
}

type ValidatorRecord struct {
	Version          uint64
	Address          common.Address
	PublicKey        []byte
	RewardAddress    common.Address
	Stake            *big.Int
	ActivationHeight uint64
	ExitHeight       uint64
	JailedUntil      uint64
	RegisteredAt     uint64
	SlashCount       uint64
}

func (r *ValidatorRecord) activeAt(height uint64) bool {
	return r != nil && r.Stake != nil && r.Stake.Sign() > 0 && r.ActivationHeight <= height &&
		(r.ExitHeight == 0 || height < r.ExitHeight) && height >= r.JailedUntil
}

func validatorRecordKey(addr common.Address) []byte { return addr.Bytes() }
func validatorRecordSlot(addr common.Address) common.Hash {
	return ShieldedV3StateSlot("validator/record", validatorRecordKey(addr))
}
func validatorRecordChunkCountSlot(addr common.Address) common.Hash {
	return ShieldedV3StateSlot("validator/record-chunk-count", validatorRecordKey(addr))
}
func validatorRecordByteLengthSlot(addr common.Address) common.Hash {
	return ShieldedV3StateSlot("validator/record-byte-length", validatorRecordKey(addr))
}
func validatorRecordChunkSlot(addr common.Address, index uint64) common.Hash {
	var b [28]byte
	copy(b[:20], addr.Bytes())
	for i := uint(0); i < 8; i++ {
		b[27-i] = byte(index >> (i * 8))
	}
	return ShieldedV3StateSlot("validator/record-chunk", b[:])
}
func validatorIndexSlot(index uint64) common.Hash {
	var b [8]byte
	for i := uint(0); i < 8; i++ {
		b[7-i] = byte(index >> (i * 8))
	}
	return ShieldedV3StateSlot("validator/index", b[:])
}
func validatorCountSlot() common.Hash { return ShieldedV3StateSlot("validator/count", nil) }
func validatorEvidenceSlot(hash common.Hash) common.Hash {
	return ShieldedV3StateSlot("validator/evidence", hash.Bytes())
}

func encodeValidatorRecord(r *ValidatorRecord) ([]byte, error) {
	if r == nil || r.Stake == nil {
		return nil, ErrValidatorNotFound
	}
	return rlp.EncodeToBytes(r)
}
func decodeValidatorRecord(raw []byte) (*ValidatorRecord, error) {
	var r ValidatorRecord
	if err := rlp.DecodeBytes(raw, &r); err != nil {
		return nil, err
	}
	if r.Stake == nil {
		r.Stake = new(big.Int)
	}
	return &r, nil
}

func readValidatorRecord(st shieldedStateReader, address common.Address) (*ValidatorRecord, error) {
	count := uint64FromHash(st.GetState(params.ShieldedPoolAddress, validatorRecordChunkCountSlot(address)))
	if count == 0 {
		return nil, ErrValidatorNotFound
	}
	raw := make([]byte, 0, count*32)
	for i := uint64(0); i < count; i++ {
		raw = append(raw, st.GetState(params.ShieldedPoolAddress, validatorRecordChunkSlot(address, i)).Bytes()...)
	}
	length := uint64FromHash(st.GetState(params.ShieldedPoolAddress, validatorRecordByteLengthSlot(address)))
	if length == 0 || length > uint64(len(raw)) {
		return nil, errors.New("validator record has invalid encoded length")
	}
	raw = raw[:length]
	return decodeValidatorRecord(raw)
}

func writeValidatorRecord(st shieldedStateWriter, record *ValidatorRecord) error {
	raw, err := encodeValidatorRecord(record)
	if err != nil {
		return err
	}
	count := (len(raw) + 31) / 32
	if count == 0 || count > 256 {
		return errors.New("validator record exceeds state encoding limit")
	}
	for i := 0; i < count; i++ {
		var word [32]byte
		start, end := i*32, (i+1)*32
		if end > len(raw) {
			end = len(raw)
		}
		copy(word[:], raw[start:end])
		st.SetState(params.ShieldedPoolAddress, validatorRecordChunkSlot(record.Address, uint64(i)), common.BytesToHash(word[:]))
	}
	st.SetState(params.ShieldedPoolAddress, validatorRecordChunkCountSlot(record.Address), uint64Hash(uint64(count)))
	st.SetState(params.ShieldedPoolAddress, validatorRecordByteLengthSlot(record.Address), uint64Hash(uint64(len(raw))))
	return nil
}

func ValidatorCount(st shieldedStateReader) uint64 {
	return uint64FromHash(st.GetState(params.ShieldedPoolAddress, validatorCountSlot()))
}

func ValidatorRecords(st shieldedStateReader) ([]*ValidatorRecord, error) {
	count := ValidatorCount(st)
	if count > ValidatorMaxSetSize {
		return nil, ErrValidatorSetFull
	}
	result := make([]*ValidatorRecord, 0, count)
	for i := uint64(0); i < count; i++ {
		hash := st.GetState(params.ShieldedPoolAddress, validatorIndexSlot(i))
		if hash == (common.Hash{}) {
			return nil, errors.New("validator activation queue is corrupt")
		}
		r, err := readValidatorRecord(st, common.BytesToAddress(hash.Bytes()))
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

func ActiveValidatorRecords(st shieldedStateReader, height uint64) ([]*ValidatorRecord, error) {
	records, err := ValidatorRecords(st)
	if err != nil {
		return nil, err
	}
	active := records[:0]
	for _, r := range records {
		if r.activeAt(height) {
			active = append(active, r)
		}
	}
	sort.Slice(active, func(i, j int) bool { return bytes.Compare(active[i].Address.Bytes(), active[j].Address.Bytes()) < 0 })
	return active, nil
}

// SelectValidator uses a parent-hash-seeded stake-weighted choice over the
// sorted active set. With the fixed bond this is a deterministic rotating
// committee while retaining a safe extension point for variable stake.
func SelectValidator(st shieldedStateReader, height uint64, seed common.Hash) (*ValidatorRecord, error) {
	active, err := ActiveValidatorRecords(st, height)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, ErrValidatorNotFound
	}
	total := new(big.Int)
	for _, r := range active {
		total.Add(total, r.Stake)
	}
	material := append([]byte("TKM_VALIDATOR_SELECT_V1"), seed.Bytes()...)
	var h [8]byte
	for i := uint(0); i < 8; i++ {
		h[7-i] = byte(height >> (i * 8))
	}
	material = append(material, h[:]...)
	digest := sha256.Sum256(material)
	choice := new(big.Int).Mod(new(big.Int).SetBytes(digest[:]), total)
	for _, r := range active {
		if choice.Cmp(r.Stake) < 0 {
			return r, nil
		}
		choice.Sub(choice, r.Stake)
	}
	return active[len(active)-1], nil
}

func ValidateValidatorRegistrationBasics(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction) error {
	if config == nil || !config.IsAntartical(number, time) {
		return ErrValidatorNotActive
	}
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Cmp(ValidatorBondWei()) != 0 || tx.ChainId().Cmp(config.ChainID) != 0 {
		return errors.New("validator registration requires a bond-sized PQ transaction to the reserved pool")
	}
	e, err := DecodeValidatorRegistration(tx.Data())
	if err != nil {
		return err
	}
	if e.Version != ValidatorEnvelopeVersion || e.RewardAddress == (common.Address{}) || e.ActivationHeight < number.Uint64()+ValidatorActivationDelay {
		return errors.New("invalid validator registration activation or reward address")
	}
	algorithm, publicKey, _, ok := tx.PQTkmFields()
	if !ok || algorithm != pqcrypto.AlgorithmMLDSA87 || len(e.PublicKey) != pqcrypto.MLDSA87PublicKeySize || !bytes.Equal(publicKey, e.PublicKey) {
		return errors.New("validator registration requires an ML-DSA-87 public key bound to the transaction")
	}
	address, err := pqcrypto.Address(algorithm, publicKey)
	if err != nil {
		return err
	}
	from, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	if from != address || e.RewardAddress != from {
		return errors.New("validator public key, sender, and reward address do not match")
	}
	return nil
}

func ValidateValidatorRegistrationState(st shieldedStateReader, from common.Address, data []byte, height uint64) error {
	e, err := DecodeValidatorRegistration(data)
	if err != nil {
		return err
	}
	if _, err := readValidatorRecord(st, from); err == nil {
		return ErrValidatorAlreadyRegistered
	}
	if ValidatorCount(st) >= ValidatorMaxSetSize {
		return ErrValidatorSetFull
	}
	if e.ActivationHeight < height+ValidatorActivationDelay {
		return errors.New("validator activation is too soon")
	}
	return nil
}

func ProcessValidatorRegistration(config *params.ChainConfig, number *big.Int, time uint64, st *state.StateDB, tx *types.Transaction) error {
	if err := ValidateValidatorRegistrationBasics(config, number, time, tx); err != nil {
		return err
	}
	from, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	if err := ValidateValidatorRegistrationState(st, from, tx.Data(), number.Uint64()); err != nil {
		return err
	}
	e, _ := DecodeValidatorRegistration(tx.Data())
	if st.GetBalance(from).ToBig().Cmp(ValidatorRegistrationFeeWei()) < 0 {
		return errors.New("validator registration fee cannot be burned")
	}
	st.SubBalance(from, uint256.MustFromBig(ValidatorRegistrationFeeWei()), tracing.BalanceDecreaseAddressVoteBurn)
	record := &ValidatorRecord{Version: e.Version, Address: from, PublicKey: append([]byte(nil), e.PublicKey...), RewardAddress: e.RewardAddress, Stake: ValidatorBondWei(), ActivationHeight: e.ActivationHeight, RegisteredAt: number.Uint64()}
	if err := writeValidatorRecord(st, record); err != nil {
		return err
	}
	index := ValidatorCount(st)
	st.SetState(params.ShieldedPoolAddress, validatorIndexSlot(index), common.BytesToHash(from.Bytes()))
	st.SetState(params.ShieldedPoolAddress, validatorCountSlot(), uint64Hash(index+1))
	return nil
}

func ValidateValidatorSlashBasics(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction) error {
	if config == nil || !config.IsAntartical(number, time) {
		return ErrValidatorNotActive
	}
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 || tx.ChainId().Cmp(config.ChainID) != 0 {
		return errors.New("validator slash requires a zero-value PQ transaction to the reserved pool")
	}
	e, err := DecodeValidatorSlash(tx.Data())
	if err != nil {
		return err
	}
	if e.Version != ValidatorEnvelopeVersion || e.Validator == (common.Address{}) || e.Height == 0 || e.BlockHashA == e.BlockHashB {
		return ErrValidatorEvidenceInvalid
	}
	if len(e.PublicKey) != pqcrypto.MLDSA87PublicKeySize || len(e.SignatureA) == 0 || len(e.SignatureB) == 0 {
		return ErrValidatorEvidenceInvalid
	}
	digestA, digestB := ValidatorAttestationDigest(e.Height, e.BlockHashA), ValidatorAttestationDigest(e.Height, e.BlockHashB)
	if !pqcrypto.VerifyMLDSA87(e.PublicKey, digestA.Bytes(), e.SignatureA) || !pqcrypto.VerifyMLDSA87(e.PublicKey, digestB.Bytes(), e.SignatureB) {
		return ErrValidatorEvidenceInvalid
	}
	derived, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, e.PublicKey)
	if err != nil || derived != e.Validator {
		return ErrValidatorEvidenceInvalid
	}
	return nil
}

func ProcessValidatorSlash(config *params.ChainConfig, number *big.Int, time uint64, st *state.StateDB, tx *types.Transaction) error {
	if err := ValidateValidatorSlashBasics(config, number, time, tx); err != nil {
		return err
	}
	e, _ := DecodeValidatorSlash(tx.Data())
	record, err := readValidatorRecord(st, e.Validator)
	if err != nil {
		return err
	}
	if !bytes.Equal(record.PublicKey, e.PublicKey) {
		return ErrValidatorEvidenceInvalid
	}
	evidenceHash := crypto.Keccak256Hash(tx.Data())
	if st.GetState(params.ShieldedPoolAddress, validatorEvidenceSlot(evidenceHash)) != (common.Hash{}) {
		return errors.New("validator evidence was already processed")
	}
	// Equivocation burns the complete bond and immediately removes the
	// validator from selection for the unbonding period.
	slashed := new(big.Int).Set(record.Stake)
	if slashed.Sign() > 0 {
		if st.GetBalance(params.ShieldedPoolAddress).ToBig().Cmp(slashed) < 0 {
			return errors.New("validator bond reserve is unavailable for slashing")
		}
		st.SubBalance(params.ShieldedPoolAddress, uint256.MustFromBig(slashed), tracing.BalanceDecreaseSelfdestructBurn)
	}
	record.Stake.SetUint64(0)
	record.ExitHeight = number.Uint64()
	record.JailedUntil = number.Uint64() + ValidatorUnbondingPeriod
	record.SlashCount++
	if err := writeValidatorRecord(st, record); err != nil {
		return err
	}
	st.SetState(params.ShieldedPoolAddress, validatorEvidenceSlot(evidenceHash), tx.Hash())
	return nil
}

func BuildValidatorRewardTx(st shieldedStateReader, header *types.Header) (*types.Transaction, error) {
	if header == nil {
		return nil, ErrValidatorRewardInvalid
	}
	record, err := SelectValidator(st, header.Number.Uint64(), header.ParentHash)
	if err != nil {
		return nil, err
	}
	shares := antartical.RewardSharesAt(header.Number.Uint64(), antartical.DefaultBlocksPerHalving)
	return types.NewValidatorRewardTx(header.Number.Uint64(), record.RewardAddress, shares.Validator), nil
}

func ValidateValidatorRewardTransaction(st shieldedStateReader, header *types.Header, tx *types.Transaction) error {
	if header == nil || tx == nil || !types.IsValidatorRewardTx(tx) {
		return ErrValidatorRewardInvalid
	}
	record, err := SelectValidator(st, header.Number.Uint64(), header.ParentHash)
	if err != nil {
		return err
	}
	shares := antartical.RewardSharesAt(header.Number.Uint64(), antartical.DefaultBlocksPerHalving)
	if tx.To() == nil || *tx.To() != record.RewardAddress || tx.Value().Cmp(shares.Validator) != 0 {
		return ErrValidatorRewardInvalid
	}
	return nil
}
