package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	shieldedTxMagic                 = "TKMSHIELD1"
	shieldedTxVersion               = uint64(1)
	shieldedTxIntentDomain          = "TKM_SHIELDED_INTENT_V1"
	shieldedMinEncryptedOutputBytes = 32
	shieldedMinNonceBytes           = 12
	shieldedMaxEncryptedBytes       = 16 * 1024
	shieldedMaxProofBytes           = 192 * 1024
	shieldedMaxSpendsPerTx          = 16
	shieldedOutputSlots             = 4
)

var (
	ErrShieldedVerifierUnavailable = errors.New("shielded proof verifier unavailable")
	ErrInvalidShieldedTx           = errors.New("invalid shielded transaction")
)

// ShieldedOutput is one encrypted output note created by a shielded transaction.
type ShieldedOutput struct {
	Commitment       common.Hash
	PayloadHash      common.Hash
	EphemeralPubKey  []byte
	ViewTag          []byte
	EncryptedPayload []byte
	Nonce            []byte
}

// ShieldedSpend is one private note spend. The nullifier is public; ownership,
// note opening and balance constraints must be proven by Proof.
type ShieldedSpend struct {
	Nullifier          common.Hash
	Anchor             common.Hash
	Proof              []byte
	EncryptedSpendData []byte
}

// ShieldedTransaction is the consensus envelope carried in tx.Data().
type ShieldedTransaction struct {
	Version           uint64
	Spends            []ShieldedSpend
	Outputs           []ShieldedOutput
	BalanceCommitment common.Hash
	BindingSig        []byte
}

// ShieldedProofContext is the exact public input passed into the ZK verifier.
type ShieldedProofContext struct {
	ChainID           *big.Int
	BlockNumber       *big.Int
	TxHash            common.Hash
	SpendIndex        int
	Nullifier         common.Hash
	Anchor            common.Hash
	BalanceCommitment common.Hash
	PublicValue       *big.Int
	OutputCommitments []common.Hash
	BindingSig        []byte
}

type shieldedIntentPayload struct {
	Domain                []byte
	TxType                uint8
	ChainID               *big.Int
	Nonce                 uint64
	GasTipCap             *big.Int
	GasFeeCap             *big.Int
	Gas                   uint64
	To                    *common.Address `rlp:"nil"`
	Value                 *big.Int
	AccessList            types.AccessList
	BlobGas               uint64
	BlobFeeCap            *big.Int
	BlobHashes            []common.Hash
	SetCodeAuthorizations []types.SetCodeAuthorization
	PQAlgorithm           string
	PQPublicKey           []byte
	Envelope              []byte
}

// ShieldedProofVerifier verifies a shielded spend proof against consensus public inputs.
type ShieldedProofVerifier interface {
	VerifyShieldedSpend(ctx ShieldedProofContext, proof []byte) error
}

type unavailableShieldedVerifier struct{}

func (unavailableShieldedVerifier) VerifyShieldedSpend(ShieldedProofContext, []byte) error {
	return ErrShieldedVerifierUnavailable
}

var (
	shieldedVerifierMu         sync.RWMutex
	shieldedVerifier           ShieldedProofVerifier = unavailableShieldedVerifier{}
	shieldedVerifierConfigured bool
)

// SetShieldedProofVerifier installs the consensus shielded proof verifier.
// Passing nil restores the rejecting verifier.
func SetShieldedProofVerifier(verifier ShieldedProofVerifier) {
	shieldedVerifierMu.Lock()
	defer shieldedVerifierMu.Unlock()
	if verifier == nil {
		shieldedVerifier = unavailableShieldedVerifier{}
		shieldedVerifierConfigured = false
		return
	}
	shieldedVerifier = verifier
	shieldedVerifierConfigured = true
}

// EncodeShieldedTransaction encodes a shielded transaction into tx.Data().
func EncodeShieldedTransaction(tx *ShieldedTransaction) ([]byte, error) {
	payload, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 0, len(shieldedTxMagic)+len(payload))
	data = append(data, []byte(shieldedTxMagic)...)
	data = append(data, payload...)
	return data, nil
}

// DecodeShieldedTransaction decodes tx.Data() if it carries a shielded envelope.
func DecodeShieldedTransaction(data []byte) (*ShieldedTransaction, bool, error) {
	if !bytes.HasPrefix(data, []byte(shieldedTxMagic)) {
		return nil, false, nil
	}
	var tx ShieldedTransaction
	if err := rlp.DecodeBytes(data[len(shieldedTxMagic):], &tx); err != nil {
		return nil, true, err
	}
	return &tx, true, nil
}

// ShieldedTransactionIntentHash returns the transaction binding hash used as
// the shielded circuit's TxHash public input. Proof bytes are excluded so the
// proof can be generated before the final envelope is assembled.
func ShieldedTransactionIntentHash(tx *types.Transaction, envelope *ShieldedTransaction) (common.Hash, error) {
	if tx == nil {
		return common.Hash{}, fmt.Errorf("%w: nil transaction", ErrInvalidShieldedTx)
	}
	if envelope == nil {
		return common.Hash{}, fmt.Errorf("%w: nil shielded envelope", ErrInvalidShieldedTx)
	}
	cleanData, err := EncodeShieldedTransaction(stripShieldedProofs(envelope))
	if err != nil {
		return common.Hash{}, err
	}
	blobFeeCap := tx.BlobGasFeeCap()
	if blobFeeCap == nil {
		blobFeeCap = new(big.Int)
	}
	pqAlgorithm, pqPublicKey, _, _ := tx.PQTkmFields()
	payload := shieldedIntentPayload{
		Domain:                []byte(shieldedTxIntentDomain),
		TxType:                tx.Type(),
		ChainID:               tx.ChainId(),
		Nonce:                 tx.Nonce(),
		GasTipCap:             tx.GasTipCap(),
		GasFeeCap:             tx.GasFeeCap(),
		Gas:                   tx.Gas(),
		To:                    tx.To(),
		Value:                 tx.Value(),
		AccessList:            tx.AccessList(),
		BlobGas:               tx.BlobGas(),
		BlobFeeCap:            blobFeeCap,
		BlobHashes:            tx.BlobHashes(),
		SetCodeAuthorizations: tx.SetCodeAuthorizations(),
		PQAlgorithm:           pqAlgorithm,
		PQPublicKey:           pqPublicKey,
		Envelope:              cleanData,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

func stripShieldedProofs(tx *ShieldedTransaction) *ShieldedTransaction {
	cpy := &ShieldedTransaction{
		Version:           tx.Version,
		Spends:            make([]ShieldedSpend, len(tx.Spends)),
		Outputs:           make([]ShieldedOutput, len(tx.Outputs)),
		BalanceCommitment: tx.BalanceCommitment,
		BindingSig:        common.CopyBytes(tx.BindingSig),
	}
	for i, spend := range tx.Spends {
		cpy.Spends[i] = ShieldedSpend{
			Nullifier:          spend.Nullifier,
			Anchor:             spend.Anchor,
			EncryptedSpendData: common.CopyBytes(spend.EncryptedSpendData),
		}
	}
	for i, output := range tx.Outputs {
		cpy.Outputs[i] = ShieldedOutput{
			Commitment:       output.Commitment,
			PayloadHash:      output.PayloadHash,
			EphemeralPubKey:  common.CopyBytes(output.EphemeralPubKey),
			ViewTag:          common.CopyBytes(output.ViewTag),
			EncryptedPayload: common.CopyBytes(output.EncryptedPayload),
			Nonce:            common.CopyBytes(output.Nonce),
		}
	}
	return cpy
}

func processShieldedTransaction(config *params.ChainConfig, blockNumber *big.Int, blockTime uint64, statedb *state.StateDB, tx *types.Transaction, seen map[common.Hash]struct{}) error {
	envelope, ok, err := DecodeShieldedTransaction(tx.Data())
	if err != nil {
		return fmt.Errorf("%w: malformed envelope: %v", ErrInvalidShieldedTx, err)
	}
	if !ok {
		if config != nil && config.IsPrivacyCommitments(blockNumber, blockTime) {
			return fmt.Errorf("%w: transparent transactions are disabled after privacy activation", ErrInvalidShieldedTx)
		}
		return nil
	}
	if config == nil || !config.IsPrivacyCommitments(blockNumber, blockTime) {
		return fmt.Errorf("%w: privacy commitments are not active", ErrInvalidShieldedTx)
	}
	if to := tx.To(); to == nil || *to != params.ShieldedPoolAddress {
		return fmt.Errorf("%w: shielded tx must target %s", ErrInvalidShieldedTx, params.ShieldedPoolAddress.Hex())
	}
	if err := validateShieldedEnvelope(envelope); err != nil {
		return err
	}
	if tx.Value().Sign() != 0 {
		return fmt.Errorf("%w: shielded tx must not expose transparent value", ErrInvalidShieldedTx)
	}
	if len(envelope.Spends) == 0 {
		return fmt.Errorf("%w: shielded tx must spend at least one private note", ErrInvalidShieldedTx)
	}
	txHash := tx.Hash()
	intentHash, err := ShieldedTransactionIntentHash(tx, envelope)
	if err != nil {
		return err
	}
	outputCommitments := make([]common.Hash, len(envelope.Outputs))
	for i, output := range envelope.Outputs {
		outputCommitments[i] = output.Commitment
	}
	for i, spend := range envelope.Spends {
		if _, ok := seen[spend.Nullifier]; ok {
			return fmt.Errorf("%w: duplicate nullifier in block %s", ErrInvalidShieldedTx, spend.Nullifier.Hex())
		}
		if stored := statedb.GetState(params.ShieldedPoolAddress, shieldedNullifierSlot(spend.Nullifier)); stored != (common.Hash{}) {
			return fmt.Errorf("%w: nullifier already spent %s", ErrInvalidShieldedTx, spend.Nullifier.Hex())
		}
		ctx := ShieldedProofContext{
			ChainID:           config.ChainID,
			BlockNumber:       blockNumber,
			TxHash:            intentHash,
			SpendIndex:        i,
			Nullifier:         spend.Nullifier,
			Anchor:            spend.Anchor,
			BalanceCommitment: envelope.BalanceCommitment,
			PublicValue:       new(big.Int).Set(tx.Value()),
			OutputCommitments: outputCommitments,
			BindingSig:        append([]byte(nil), envelope.BindingSig...),
		}
		if err := activeShieldedProofVerifier(config).VerifyShieldedSpend(ctx, spend.Proof); err != nil {
			return fmt.Errorf("%w: spend proof %d: %w", ErrInvalidShieldedTx, i, err)
		}
		seen[spend.Nullifier] = struct{}{}
		statedb.SetState(params.ShieldedPoolAddress, shieldedNullifierSlot(spend.Nullifier), txHash)
	}
	for _, output := range envelope.Outputs {
		slot := shieldedCommitmentSlot(output.Commitment)
		if stored := statedb.GetState(params.ShieldedPoolAddress, slot); stored != (common.Hash{}) {
			return fmt.Errorf("%w: commitment already exists %s", ErrInvalidShieldedTx, output.Commitment.Hex())
		}
		statedb.SetState(params.ShieldedPoolAddress, slot, output.PayloadHash)
	}
	return nil
}

func activeShieldedProofVerifier(config *params.ChainConfig) ShieldedProofVerifier {
	shieldedVerifierMu.RLock()
	verifier := shieldedVerifier
	configured := shieldedVerifierConfigured
	shieldedVerifierMu.RUnlock()
	if configured {
		return verifier
	}
	if configVerifier := shieldedGroth16VerifierFromChainConfig(config); configVerifier != nil {
		return configVerifier
	}
	return verifier
}

func validateShieldedEnvelope(tx *ShieldedTransaction) error {
	if tx.Version != shieldedTxVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidShieldedTx, tx.Version)
	}
	if len(tx.Spends) == 0 && len(tx.Outputs) == 0 {
		return fmt.Errorf("%w: empty shielded envelope", ErrInvalidShieldedTx)
	}
	if len(tx.Spends) > shieldedMaxSpendsPerTx {
		return fmt.Errorf("%w: too many spends %d", ErrInvalidShieldedTx, len(tx.Spends))
	}
	if len(tx.Outputs) != shieldedOutputSlots {
		return fmt.Errorf("%w: shielded tx must carry exactly %d padded outputs", ErrInvalidShieldedTx, shieldedOutputSlots)
	}
	if len(tx.Spends) > 0 && len(tx.BindingSig) == 0 {
		return fmt.Errorf("%w: binding signature required for spends", ErrInvalidShieldedTx)
	}
	if len(tx.BindingSig) != common.HashLength {
		return fmt.Errorf("%w: binding hash must be %d bytes", ErrInvalidShieldedTx, common.HashLength)
	}
	if !isCanonicalShieldedFieldHash(common.BytesToHash(tx.BindingSig)) {
		return fmt.Errorf("%w: binding hash is not a canonical BN254 field element", ErrInvalidShieldedTx)
	}
	seenNullifiers := make(map[common.Hash]struct{}, len(tx.Spends))
	for i, spend := range tx.Spends {
		if spend.Nullifier == (common.Hash{}) {
			return fmt.Errorf("%w: spend %d zero nullifier", ErrInvalidShieldedTx, i)
		}
		if spend.Anchor == (common.Hash{}) {
			return fmt.Errorf("%w: spend %d zero anchor", ErrInvalidShieldedTx, i)
		}
		if !isCanonicalShieldedFieldHash(spend.Nullifier) {
			return fmt.Errorf("%w: spend %d nullifier is not a canonical BN254 field element", ErrInvalidShieldedTx, i)
		}
		if !isCanonicalShieldedFieldHash(spend.Anchor) {
			return fmt.Errorf("%w: spend %d anchor is not a canonical BN254 field element", ErrInvalidShieldedTx, i)
		}
		if len(spend.Proof) == 0 {
			return fmt.Errorf("%w: spend %d missing proof", ErrInvalidShieldedTx, i)
		}
		if len(spend.Proof) > shieldedMaxProofBytes {
			return fmt.Errorf("%w: spend %d proof exceeds %d bytes", ErrInvalidShieldedTx, i, shieldedMaxProofBytes)
		}
		if len(spend.EncryptedSpendData) > shieldedMaxEncryptedBytes {
			return fmt.Errorf("%w: spend %d encrypted data exceeds %d bytes", ErrInvalidShieldedTx, i, shieldedMaxEncryptedBytes)
		}
		if _, ok := seenNullifiers[spend.Nullifier]; ok {
			return fmt.Errorf("%w: duplicate nullifier %s", ErrInvalidShieldedTx, spend.Nullifier.Hex())
		}
		seenNullifiers[spend.Nullifier] = struct{}{}
	}
	if tx.BalanceCommitment == (common.Hash{}) {
		return fmt.Errorf("%w: zero balance commitment", ErrInvalidShieldedTx)
	}
	if !isCanonicalShieldedFieldHash(tx.BalanceCommitment) {
		return fmt.Errorf("%w: balance commitment is not a canonical BN254 field element", ErrInvalidShieldedTx)
	}
	seenCommitments := make(map[common.Hash]struct{}, len(tx.Outputs))
	for i, output := range tx.Outputs {
		if output.Commitment == (common.Hash{}) {
			return fmt.Errorf("%w: output %d zero commitment", ErrInvalidShieldedTx, i)
		}
		if !isCanonicalShieldedFieldHash(output.Commitment) {
			return fmt.Errorf("%w: output %d commitment is not a canonical BN254 field element", ErrInvalidShieldedTx, i)
		}
		if output.PayloadHash == (common.Hash{}) {
			return fmt.Errorf("%w: output %d zero payload hash", ErrInvalidShieldedTx, i)
		}
		if len(output.EncryptedPayload) < shieldedMinEncryptedOutputBytes {
			return fmt.Errorf("%w: output %d encrypted payload must be at least %d bytes", ErrInvalidShieldedTx, i, shieldedMinEncryptedOutputBytes)
		}
		if len(output.EncryptedPayload) > shieldedMaxEncryptedBytes {
			return fmt.Errorf("%w: output %d encrypted payload exceeds %d bytes", ErrInvalidShieldedTx, i, shieldedMaxEncryptedBytes)
		}
		if len(output.Nonce) < shieldedMinNonceBytes {
			return fmt.Errorf("%w: output %d nonce must be at least %d bytes", ErrInvalidShieldedTx, i, shieldedMinNonceBytes)
		}
		if _, ok := seenCommitments[output.Commitment]; ok {
			return fmt.Errorf("%w: duplicate commitment %s", ErrInvalidShieldedTx, output.Commitment.Hex())
		}
		seenCommitments[output.Commitment] = struct{}{}
	}
	return nil
}

func shieldedCommitmentSlot(commitment common.Hash) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_COMMITMENT_V1"), commitment.Bytes())
}

func shieldedNullifierSlot(nullifier common.Hash) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_NULLIFIER_V1"), nullifier.Bytes())
}
