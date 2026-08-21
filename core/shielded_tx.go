package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

const (
	shieldedTxMagic                 = "TKMSHIELD1"
	shieldedTxIntentDomain          = "TKM_SHIELDED_INTENT_V1"
	shieldedTxIntentDomainV2        = "TKM_SHIELDED_INTENT_V2"
	shieldedMinEncryptedOutputBytes = 32
	shieldedMinNonceBytes           = 12
	shieldedMaxEncryptedBytes       = 16 * 1024
	shieldedMaxProofBytes           = 192 * 1024
	shieldedMaxSpendsPerTx          = 16
	shieldedOutputSlots             = 4
	shieldedMerkleDepth             = 32
	shieldedDomainNode              = uint64(1002)

	// ShieldedMerkleDepth is the fixed tree depth used by the shielded circuit.
	ShieldedMerkleDepth = shieldedMerkleDepth
	ShieldedTxVersionV1 = uint64(1)
	ShieldedTxVersionV2 = uint64(2)
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
	// WithdrawalRecipient and WithdrawalValue are optional trailing fields so
	// previously encoded V1/V2 envelopes remain decodable. A non-zero pair asks
	// consensus to release a proof-backed public value from the shielded pool.
	WithdrawalRecipient common.Address `rlp:"optional"`
	WithdrawalValue     *big.Int       `rlp:"optional"`
}

// ShieldedProofContext is the exact public input passed into the ZK verifier.
type ShieldedProofContext struct {
	Version           uint64
	Sender            common.Address
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

// ShieldedMerklePath is the stored commitment witness used by wallet/prover
// tooling after a deposit or shielded output becomes canonical.
type ShieldedMerklePath struct {
	Commitment common.Hash
	Index      uint64
	Root       common.Hash
	Path       []common.Hash
	PathIndex  []uint8
}

type shieldedStateReader interface {
	GetState(common.Address, common.Hash) common.Hash
}

type shieldedStateWriter interface {
	shieldedStateReader
	SetState(common.Address, common.Hash, common.Hash) common.Hash
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

type fallbackShieldedVerifier struct {
	primary  ShieldedProofVerifier
	fallback ShieldedProofVerifier
}

func (v fallbackShieldedVerifier) VerifyShieldedSpend(ctx ShieldedProofContext, proof []byte) error {
	var primaryErr error
	if v.primary != nil {
		if err := v.primary.VerifyShieldedSpend(ctx, proof); err == nil {
			return nil
		} else {
			primaryErr = err
		}
	}
	if v.fallback != nil {
		if err := v.fallback.VerifyShieldedSpend(ctx, proof); err == nil {
			return nil
		} else if primaryErr == nil {
			primaryErr = err
		}
	}
	if primaryErr != nil {
		return primaryErr
	}
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

// ProcessShieldedTransaction applies consensus shielded commitment state for tx.
// The seen map should be shared across all transactions in the candidate block.
func ProcessShieldedTransaction(config *params.ChainConfig, blockNumber *big.Int, blockTime uint64, statedb *state.StateDB, tx *types.Transaction, seen map[common.Hash]struct{}) error {
	if seen == nil {
		seen = make(map[common.Hash]struct{})
	}
	return processShieldedTransaction(config, blockNumber, blockTime, statedb, tx, seen)
}

// ValidateShieldedTransactionBasics validates the stateless privacy-envelope
// rules enforced by ProcessShieldedTransaction. It deliberately avoids note-root,
// nullifier and commitment state checks so txpool validation can reject malformed
// or transparent post-privacy transactions without needing a StateDB.
func ValidateShieldedTransactionBasics(config *params.ChainConfig, blockNumber *big.Int, blockTime uint64, tx *types.Transaction) error {
	_, err := validateShieldedTransactionEnvelope(config, blockNumber, blockTime, tx)
	return err
}

// ShieldedTransactionIntentHash returns the transaction binding hash used as
// the shielded circuit's TxHash public input. Proof bytes, the binding hash and
// authentication metadata are excluded so the proof can be generated before the
// final envelope is signed and assembled without creating a self-referential
// hash.
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
	intentDomain := shieldedTxIntentDomain
	if envelope.Version == ShieldedTxVersionV2 {
		intentDomain = shieldedTxIntentDomainV2
	}
	payload := shieldedIntentPayload{
		Domain:                []byte(intentDomain),
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
		Version:             tx.Version,
		Spends:              make([]ShieldedSpend, len(tx.Spends)),
		Outputs:             make([]ShieldedOutput, len(tx.Outputs)),
		BalanceCommitment:   tx.BalanceCommitment,
		BindingSig:          make([]byte, common.HashLength),
		WithdrawalRecipient: tx.WithdrawalRecipient,
		WithdrawalValue:     copyBigInt(tx.WithdrawalValue),
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
	envelope, err := validateShieldedTransactionEnvelope(config, blockNumber, blockTime, tx)
	if err != nil {
		return err
	}
	if envelope == nil {
		return nil
	}
	isDeposit := isShieldedDeposit(envelope, tx.Value())
	publicValue := shieldedPublicValue(envelope, tx.Value())
	withdrawalValue := shieldedWithdrawalValue(envelope)
	if withdrawalValue.Sign() > 0 {
		amount := uint256.MustFromBig(withdrawalValue)
		if statedb.GetBalance(params.ShieldedPoolAddress).Cmp(amount) < 0 {
			return fmt.Errorf("%w: shielded pool balance is smaller than withdrawal value", ErrInvalidShieldedTx)
		}
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
	var sender common.Address
	if envelope.Version == ShieldedTxVersionV2 {
		var err error
		sender, err = types.Sender(types.MakeSigner(config, blockNumber, blockTime), tx)
		if err != nil {
			return fmt.Errorf("%w: recover V2 PQ sender: %v", ErrInvalidShieldedTx, err)
		}
	}
	for i, spend := range envelope.Spends {
		if isDeposit {
			if i != 0 {
				return fmt.Errorf("%w: shielded deposit must carry exactly one deposit proof", ErrInvalidShieldedTx)
			}
		} else if !shieldedMerkleRootKnown(statedb, spend.Anchor) {
			return fmt.Errorf("%w: unknown shielded note root %s", ErrInvalidShieldedTx, spend.Anchor.Hex())
		}
		if _, ok := seen[spend.Nullifier]; ok {
			return fmt.Errorf("%w: duplicate nullifier in block %s", ErrInvalidShieldedTx, spend.Nullifier.Hex())
		}
		if !isDeposit {
			if stored := statedb.GetState(params.ShieldedPoolAddress, shieldedNullifierSlot(spend.Nullifier)); stored != (common.Hash{}) {
				return fmt.Errorf("%w: nullifier already spent %s", ErrInvalidShieldedTx, spend.Nullifier.Hex())
			}
		}
		ctx := ShieldedProofContext{
			Version:           envelope.Version,
			Sender:            sender,
			ChainID:           config.ChainID,
			BlockNumber:       blockNumber,
			TxHash:            intentHash,
			SpendIndex:        i,
			Nullifier:         spend.Nullifier,
			Anchor:            spend.Anchor,
			BalanceCommitment: envelope.BalanceCommitment,
			PublicValue:       new(big.Int).Set(publicValue),
			OutputCommitments: outputCommitments,
			BindingSig:        append([]byte(nil), envelope.BindingSig...),
		}
		if err := verifyShieldedSpend(config, blockTime, envelope.Version, ctx, spend.Proof); err != nil {
			return fmt.Errorf("%w: spend proof %d: %w", ErrInvalidShieldedTx, i, err)
		}
		if !isDeposit {
			seen[spend.Nullifier] = struct{}{}
			statedb.SetState(params.ShieldedPoolAddress, shieldedNullifierSlot(spend.Nullifier), txHash)
		}
	}
	if withdrawalValue.Sign() > 0 {
		amount := uint256.MustFromBig(withdrawalValue)
		statedb.SubBalance(params.ShieldedPoolAddress, amount, tracing.BalanceChangeTransfer)
		statedb.AddBalance(envelope.WithdrawalRecipient, amount, tracing.BalanceChangeTransfer)
	}
	for _, output := range envelope.Outputs {
		slot := shieldedCommitmentSlot(output.Commitment)
		if stored := statedb.GetState(params.ShieldedPoolAddress, slot); stored != (common.Hash{}) {
			return fmt.Errorf("%w: commitment already exists %s", ErrInvalidShieldedTx, output.Commitment.Hex())
		}
		statedb.SetState(params.ShieldedPoolAddress, slot, output.PayloadHash)
		if err := appendShieldedMerkleLeaf(statedb, output.Commitment, txHash); err != nil {
			return err
		}
	}
	return nil
}

func validateShieldedTransactionEnvelope(config *params.ChainConfig, blockNumber *big.Int, blockTime uint64, tx *types.Transaction) (*ShieldedTransaction, error) {
	envelope, ok, err := DecodeShieldedTransaction(tx.Data())
	if err != nil {
		return nil, fmt.Errorf("%w: malformed envelope: %v", ErrInvalidShieldedTx, err)
	}
	if !ok {
		if config != nil && config.IsPrivacyCommitments(blockNumber, blockTime) {
			if config.IsPQMigrationAllowed(blockNumber, blockTime) && types.IsPQMigrationTx(tx) {
				return nil, nil
			}
			return nil, fmt.Errorf("%w: transparent transactions are disabled after privacy activation", ErrInvalidShieldedTx)
		}
		return nil, nil
	}
	if config == nil || !config.IsPrivacyCommitments(blockNumber, blockTime) {
		return nil, fmt.Errorf("%w: privacy commitments are not active", ErrInvalidShieldedTx)
	}
	if to := tx.To(); to == nil || *to != params.ShieldedPoolAddress {
		return nil, fmt.Errorf("%w: shielded tx must target %s", ErrInvalidShieldedTx, params.ShieldedPoolAddress.Hex())
	}
	expectedVersion := ShieldedTxVersionV1
	if params.IsMainnetShieldedV2(config, blockTime) {
		expectedVersion = ShieldedTxVersionV2
	}
	if err := validateShieldedEnvelope(envelope, expectedVersion); err != nil {
		return nil, err
	}
	isDeposit := isShieldedDeposit(envelope, tx.Value())
	if envelope.WithdrawalValue != nil && envelope.WithdrawalValue.Sign() < 0 {
		return nil, fmt.Errorf("%w: shielded withdrawal value cannot be negative", ErrInvalidShieldedTx)
	}
	withdrawalValue := shieldedWithdrawalValue(envelope)
	if tx.Value().Sign() != 0 && !isDeposit {
		return nil, fmt.Errorf("%w: transparent value is only allowed for shielded deposits", ErrInvalidShieldedTx)
	}
	if withdrawalValue.Sign() > 0 {
		if isDeposit || tx.Value().Sign() != 0 {
			return nil, fmt.Errorf("%w: shielded deposit and withdrawal cannot be combined", ErrInvalidShieldedTx)
		}
		if len(envelope.Spends) != 1 {
			return nil, fmt.Errorf("%w: shielded withdrawal must spend exactly one private note", ErrInvalidShieldedTx)
		}
		if envelope.WithdrawalRecipient == (common.Address{}) {
			return nil, fmt.Errorf("%w: shielded withdrawal recipient is required", ErrInvalidShieldedTx)
		}
		if withdrawalValue.BitLen() > 64 {
			return nil, fmt.Errorf("%w: shielded withdrawal value exceeds uint64", ErrInvalidShieldedTx)
		}
	} else if envelope.WithdrawalRecipient != (common.Address{}) {
		return nil, fmt.Errorf("%w: shielded withdrawal value is required", ErrInvalidShieldedTx)
	}
	if tx.Value().Sign() == 0 && len(envelope.Spends) == 0 {
		return nil, fmt.Errorf("%w: shielded tx must spend at least one private note", ErrInvalidShieldedTx)
	}
	return envelope, nil
}

func verifyShieldedSpend(config *params.ChainConfig, blockTime uint64, version uint64, ctx ShieldedProofContext, proof []byte) error {
	verifier := activeShieldedProofVerifier(config, blockTime, version)
	err := verifier.VerifyShieldedSpend(ctx, proof)
	if err == nil {
		return nil
	}
	if ctx.BlockNumber == nil || ctx.BlockNumber.Sign() == 0 {
		return err
	}
	durable := ctx
	durable.BlockNumber = new(big.Int)
	if durableErr := verifier.VerifyShieldedSpend(durable, proof); durableErr == nil {
		return nil
	}
	return err
}

func activeShieldedProofVerifier(config *params.ChainConfig, blockTime uint64, version uint64) ShieldedProofVerifier {
	shieldedVerifierMu.RLock()
	verifier := shieldedVerifier
	configured := shieldedVerifierConfigured
	shieldedVerifierMu.RUnlock()
	if configured {
		return verifier
	}
	if version == ShieldedTxVersionV2 {
		if v2Verifier := v2ShieldedGroth16VerifierFromParams(config); v2Verifier != nil {
			return v2Verifier
		}
		return verifier
	}
	configVerifier := shieldedGroth16VerifierFromChainConfig(config)
	upgradeVerifier := upgradedShieldedGroth16VerifierFromParams(config)
	if blockTime >= params.MainnetShieldedGroth16RecoveryTime {
		if recoveryVerifier := recoveryShieldedGroth16VerifierFromParams(config); recoveryVerifier != nil {
			return recoveryVerifier
		}
	}
	if upgradeVerifier != nil && configVerifier != nil {
		return fallbackShieldedVerifier{primary: upgradeVerifier, fallback: configVerifier}
	}
	if upgradeVerifier != nil {
		return upgradeVerifier
	}
	if configVerifier != nil {
		return configVerifier
	}
	return verifier
}

func validateShieldedEnvelope(tx *ShieldedTransaction, expectedVersion uint64) error {
	if tx.Version != expectedVersion {
		return fmt.Errorf("%w: envelope version %d is not active; expected version %d", ErrInvalidShieldedTx, tx.Version, expectedVersion)
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
		isDepositProof := spend.Nullifier == (common.Hash{}) && spend.Anchor == (common.Hash{})
		if !isDepositProof {
			if spend.Nullifier == (common.Hash{}) {
				return fmt.Errorf("%w: spend %d zero nullifier", ErrInvalidShieldedTx, i)
			}
			if spend.Anchor == (common.Hash{}) {
				return fmt.Errorf("%w: spend %d zero anchor", ErrInvalidShieldedTx, i)
			}
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
		if !isDepositProof {
			if _, ok := seenNullifiers[spend.Nullifier]; ok {
				return fmt.Errorf("%w: duplicate nullifier %s", ErrInvalidShieldedTx, spend.Nullifier.Hex())
			}
			seenNullifiers[spend.Nullifier] = struct{}{}
			continue
		}
		if len(tx.Spends) != 1 {
			return fmt.Errorf("%w: deposit proof cannot be mixed with private spends", ErrInvalidShieldedTx)
		}
		if i != 0 {
			return fmt.Errorf("%w: duplicate nullifier %s", ErrInvalidShieldedTx, spend.Nullifier.Hex())
		}
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

// ShieldedMerkleNextIndex returns the next shielded commitment leaf index.
func ShieldedMerkleNextIndex(statedb shieldedStateReader) uint64 {
	return uint64FromHash(statedb.GetState(params.ShieldedPoolAddress, ShieldedMerkleNextIndexSlot()))
}

// ShieldedCommitmentPath returns the stored Merkle witness for a canonical
// shielded output commitment.
func ShieldedCommitmentPath(statedb shieldedStateReader, commitment common.Hash) (ShieldedMerklePath, bool) {
	indexPlusOne := uint64FromHash(statedb.GetState(params.ShieldedPoolAddress, ShieldedCommitmentIndexSlot(commitment)))
	if indexPlusOne == 0 {
		return ShieldedMerklePath{Commitment: commitment}, false
	}
	index := indexPlusOne - 1
	path := make([]common.Hash, shieldedMerkleDepth)
	pathIndex := make([]uint8, shieldedMerkleDepth)
	bits := index
	for level := 0; level < shieldedMerkleDepth; level++ {
		path[level] = statedb.GetState(params.ShieldedPoolAddress, ShieldedCommitmentPathSlot(commitment, level))
		if bits&(uint64(1)<<uint(level)) != 0 {
			pathIndex[level] = 1
		}
	}
	root := computeShieldedMerkleRoot(commitment, path, pathIndex)
	return ShieldedMerklePath{
		Commitment: commitment,
		Index:      index,
		Root:       root,
		Path:       path,
		PathIndex:  pathIndex,
	}, true
}

// ShieldedNextMerklePath returns the witness that will be assigned to the next
// appended commitment if no earlier shielded output is included first.
func ShieldedNextMerklePath(statedb shieldedStateReader, commitment common.Hash) ShieldedMerklePath {
	index := ShieldedMerkleNextIndex(statedb)
	path := make([]common.Hash, shieldedMerkleDepth)
	pathIndex := make([]uint8, shieldedMerkleDepth)
	zeroes := shieldedMerkleZeroHashes()
	for level := 0; level < shieldedMerkleDepth; level++ {
		if index&(uint64(1)<<uint(level)) == 0 {
			path[level] = zeroes[level]
		} else {
			path[level] = statedb.GetState(params.ShieldedPoolAddress, ShieldedMerkleFrontierSlot(level))
			pathIndex[level] = 1
		}
	}
	root := computeShieldedMerkleRoot(commitment, path, pathIndex)
	return ShieldedMerklePath{
		Commitment: commitment,
		Index:      index,
		Root:       root,
		Path:       path,
		PathIndex:  pathIndex,
	}
}

// ShieldedMerkleZeroHashes returns the empty-tree sibling value for each level.
func ShieldedMerkleZeroHashes() []common.Hash {
	return shieldedMerkleZeroHashes()
}

// ShieldedMerkleNextIndexSlot is the storage slot holding the next append index.
func ShieldedMerkleNextIndexSlot() common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_MERKLE_NEXT_INDEX_V1"))
}

// ShieldedMerkleFrontierSlot is the storage slot holding the incremental tree
// frontier for one level.
func ShieldedMerkleFrontierSlot(level int) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_MERKLE_FRONTIER_V1"), uint64Hash(uint64(level)).Bytes())
}

// ShieldedCommitmentIndexSlot stores index+1 for a commitment. Zero means absent.
func ShieldedCommitmentIndexSlot(commitment common.Hash) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_COMMITMENT_INDEX_V1"), commitment.Bytes())
}

// ShieldedCommitmentPathSlot stores one sibling from the commitment's append-time path.
func ShieldedCommitmentPathSlot(commitment common.Hash, level int) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_COMMITMENT_PATH_V1"), commitment.Bytes(), uint64Hash(uint64(level)).Bytes())
}

// ShieldedMerkleRootSlot stores known roots accepted by future shielded spends.
func ShieldedMerkleRootSlot(root common.Hash) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_MERKLE_ROOT_V1"), root.Bytes())
}

func isShieldedDeposit(envelope *ShieldedTransaction, value *big.Int) bool {
	return envelope != nil &&
		value != nil &&
		value.Sign() > 0 &&
		len(envelope.Spends) == 1 &&
		envelope.Spends[0].Nullifier == (common.Hash{}) &&
		envelope.Spends[0].Anchor == (common.Hash{})
}

func shieldedWithdrawalValue(envelope *ShieldedTransaction) *big.Int {
	if envelope == nil || envelope.WithdrawalValue == nil || envelope.WithdrawalValue.Sign() <= 0 {
		return new(big.Int)
	}
	return envelope.WithdrawalValue
}

func shieldedPublicValue(envelope *ShieldedTransaction, transactionValue *big.Int) *big.Int {
	if withdrawal := shieldedWithdrawalValue(envelope); withdrawal.Sign() > 0 {
		return withdrawal
	}
	if transactionValue == nil {
		return new(big.Int)
	}
	return transactionValue
}

func copyBigInt(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func shieldedMerkleRootKnown(statedb shieldedStateReader, root common.Hash) bool {
	if root == (common.Hash{}) {
		return false
	}
	return statedb.GetState(params.ShieldedPoolAddress, ShieldedMerkleRootSlot(root)) != (common.Hash{})
}

func appendShieldedMerkleLeaf(statedb shieldedStateWriter, commitment common.Hash, txHash common.Hash) error {
	index := ShieldedMerkleNextIndex(statedb)
	if index >= uint64(1)<<shieldedMerkleDepth {
		return fmt.Errorf("%w: shielded commitment tree is full", ErrInvalidShieldedTx)
	}
	zeroes := shieldedMerkleZeroHashes()
	current := commitment
	path := make([]common.Hash, shieldedMerkleDepth)
	for level := 0; level < shieldedMerkleDepth; level++ {
		var sibling common.Hash
		if index&(uint64(1)<<uint(level)) == 0 {
			sibling = zeroes[level]
			statedb.SetState(params.ShieldedPoolAddress, ShieldedMerkleFrontierSlot(level), current)
			current = shieldedHashNode(current, sibling)
		} else {
			sibling = statedb.GetState(params.ShieldedPoolAddress, ShieldedMerkleFrontierSlot(level))
			current = shieldedHashNode(sibling, current)
		}
		path[level] = sibling
		statedb.SetState(params.ShieldedPoolAddress, ShieldedCommitmentPathSlot(commitment, level), sibling)
	}
	statedb.SetState(params.ShieldedPoolAddress, ShieldedCommitmentIndexSlot(commitment), uint64Hash(index+1))
	statedb.SetState(params.ShieldedPoolAddress, ShieldedMerkleNextIndexSlot(), uint64Hash(index+1))
	statedb.SetState(params.ShieldedPoolAddress, ShieldedMerkleRootSlot(current), txHash)
	return nil
}

func computeShieldedMerkleRoot(commitment common.Hash, path []common.Hash, pathIndex []uint8) common.Hash {
	root := commitment
	for level := 0; level < shieldedMerkleDepth; level++ {
		var sibling common.Hash
		if level < len(path) {
			sibling = path[level]
		}
		var direction uint8
		if level < len(pathIndex) {
			direction = pathIndex[level]
		}
		if direction == 1 {
			root = shieldedHashNode(sibling, root)
		} else {
			root = shieldedHashNode(root, sibling)
		}
	}
	return root
}

func shieldedMerkleZeroHashes() []common.Hash {
	zeroes := make([]common.Hash, shieldedMerkleDepth)
	for level := 1; level < shieldedMerkleDepth; level++ {
		zeroes[level] = shieldedHashNode(zeroes[level-1], zeroes[level-1])
	}
	return zeroes
}

func shieldedHashNode(left, right common.Hash) common.Hash {
	leftElement := fieldElementFromHash(left)
	rightElement := fieldElementFromHash(right)
	node := shieldedFieldHash(shieldedDomainNode, leftElement, rightElement)
	return hashFromShieldedField(node)
}

func shieldedFieldHash(domain uint64, inputs ...fr.Element) fr.Element {
	hasher := mimc.NewMiMC()
	domainElement := fieldElementFromUint64(domain)
	domainBytes := domainElement.Bytes()
	hasher.Write(domainBytes[:])
	for _, input := range inputs {
		inputBytes := input.Bytes()
		hasher.Write(inputBytes[:])
	}
	sum := hasher.Sum(nil)
	var out fr.Element
	if err := out.SetBytesCanonical(sum); err != nil {
		panic(err)
	}
	return out
}

func hashFromShieldedField(element fr.Element) common.Hash {
	var out common.Hash
	elementBytes := element.Bytes()
	copy(out[:], elementBytes[:])
	return out
}

func uint64Hash(v uint64) common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(v))
}

func uint64FromHash(hash common.Hash) uint64 {
	return new(big.Int).SetBytes(hash.Bytes()).Uint64()
}
