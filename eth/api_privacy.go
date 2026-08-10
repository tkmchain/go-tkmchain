package eth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

const (
	privacyActivationConfirmations  = uint64(5)
	privacyMinEncryptedPayloadBytes = 32
	privacyMinNonceBytes            = 12
	privacyMinViewTagBytes          = 1
	privacyMaxEncryptedPayloadBytes = 16 * 1024
	privacyMaxSpendPayloadBytes     = 16 * 1024
)

var privacyActivationFee = new(big.Int).SetUint64(params.Ether)

type privacyActivationInfo struct {
	PaymentHash    common.Hash
	PaidHeight     uint64
	ActivateHeight uint64
	Amount         *big.Int
}

type privacyCommitmentInfo struct {
	EncryptedPayload []byte
	Nonce            []byte
	PaidHeight       uint64
	ActivateHeight   uint64
	Amount           *big.Int
	PayloadHash      common.Hash
	EphemeralPubKey  []byte
	ViewTag          []byte
	Nullifier        common.Hash
	SpentHeight      uint64
	SpendProofHash   common.Hash
	SpendCiphertext  []byte
}

type privacyNullifierInfo struct {
	ProofHash          common.Hash
	EncryptedSpendData []byte
	SpentHeight        uint64
}

// PrivacyActivationStatus describes one address privacy activation.
type PrivacyActivationStatus struct {
	Address        common.Address `json:"address"`
	PaymentHash    common.Hash    `json:"paymentHash"`
	PaidHeight     hexutil.Uint64 `json:"paidHeight"`
	ActivateHeight hexutil.Uint64 `json:"activateHeight"`
	Confirmations  hexutil.Uint64 `json:"confirmations"`
	Active         bool           `json:"active"`
	Amount         *hexutil.Big   `json:"amount"`
}

// PrivacyCommitmentStatus describes one encrypted commitment privacy record.
type PrivacyCommitmentStatus struct {
	Commitment       common.Hash    `json:"commitment"`
	EncryptedPayload hexutil.Bytes  `json:"encryptedPayload"`
	Nonce            hexutil.Bytes  `json:"nonce"`
	PayloadHash      common.Hash    `json:"payloadHash"`
	EphemeralPubKey  hexutil.Bytes  `json:"ephemeralPubKey"`
	ViewTag          hexutil.Bytes  `json:"viewTag"`
	PaidHeight       hexutil.Uint64 `json:"paidHeight"`
	ActivateHeight   hexutil.Uint64 `json:"activateHeight"`
	Confirmations    hexutil.Uint64 `json:"confirmations"`
	Active           bool           `json:"active"`
	Nullifier        common.Hash    `json:"nullifier"`
	SpentHeight      hexutil.Uint64 `json:"spentHeight"`
	Spent            bool           `json:"spent"`
	SpendProofHash   common.Hash    `json:"spendProofHash"`
	SpendCiphertext  hexutil.Bytes  `json:"spendCiphertext"`
	Amount           *hexutil.Big   `json:"amount"`
}

// PrivacyNullifierStatus describes one private note spend marker.
type PrivacyNullifierStatus struct {
	Nullifier          common.Hash    `json:"nullifier"`
	ProofHash          common.Hash    `json:"proofHash"`
	EncryptedSpendData hexutil.Bytes  `json:"encryptedSpendData"`
	SpentHeight        hexutil.Uint64 `json:"spentHeight"`
	Spent              bool           `json:"spent"`
}

// PrivacyDefaults describes privacy-preserving client defaults.
type PrivacyDefaults struct {
	CommitmentMode           bool           `json:"commitmentMode"`
	LegacyAddressRegistry    bool           `json:"legacyAddressRegistry"`
	ActivationConfirmations  hexutil.Uint64 `json:"activationConfirmations"`
	MinEncryptedPayloadBytes hexutil.Uint64 `json:"minEncryptedPayloadBytes"`
	MinNonceBytes            hexutil.Uint64 `json:"minNonceBytes"`
	MinViewTagBytes          hexutil.Uint64 `json:"minViewTagBytes"`
	Nullifiers               bool           `json:"nullifiers"`
}

// PrivacyAPI provides address privacy activation RPC methods.
type PrivacyAPI struct {
	e *Ethereum
}

func NewPrivacyAPI(e *Ethereum) *PrivacyAPI {
	return &PrivacyAPI{e: e}
}

// Fee returns the minimum fee paid to the main king to activate address privacy.
func (api *PrivacyAPI) Fee() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(privacyActivationFee))
}

// Confirmations returns the number of confirmations required before privacy is active.
func (api *PrivacyAPI) Confirmations() hexutil.Uint64 {
	return hexutil.Uint64(privacyActivationConfirmations)
}

// CommitmentActivationTime returns the encrypted commitment fork activation timestamp.
func (api *PrivacyAPI) CommitmentActivationTime() *hexutil.Uint64 {
	t := api.e.privacyCommitmentActivationTime()
	if t == nil {
		return nil
	}
	v := hexutil.Uint64(*t)
	return &v
}

// CommitmentActive reports whether encrypted privacy commitments are active at the chain head.
func (api *PrivacyAPI) CommitmentActive() bool {
	return api.e.privacyCommitmentsActive()
}

// Defaults returns privacy-focused client defaults.
func (api *PrivacyAPI) Defaults() PrivacyDefaults {
	return PrivacyDefaults{
		CommitmentMode:           true,
		LegacyAddressRegistry:    false,
		ActivationConfirmations:  hexutil.Uint64(privacyActivationConfirmations),
		MinEncryptedPayloadBytes: hexutil.Uint64(privacyMinEncryptedPayloadBytes),
		MinNonceBytes:            hexutil.Uint64(privacyMinNonceBytes),
		MinViewTagBytes:          hexutil.Uint64(privacyMinViewTagBytes),
		Nullifiers:               true,
	}
}

// Register records a canonical payment transaction as an address privacy activation.
func (api *PrivacyAPI) Register(address common.Address, paymentHash common.Hash) (PrivacyActivationStatus, error) {
	if address == (common.Address{}) {
		return PrivacyActivationStatus{}, fmt.Errorf("privacy address cannot be zero")
	}
	info, err := api.e.registerPrivacyActivation(address, paymentHash)
	if err != nil {
		return PrivacyActivationStatus{}, err
	}
	return api.e.privacyStatus(address, info), nil
}

// RegisterCommitment records an encrypted privacy commitment from a canonical shielded transaction.
func (api *PrivacyAPI) RegisterCommitment(commitment common.Hash, encryptedPayload hexutil.Bytes, nonce hexutil.Bytes, paymentHash common.Hash) (PrivacyCommitmentStatus, error) {
	info, err := api.e.registerPrivacyCommitment(commitment, []byte(encryptedPayload), []byte(nonce), common.Hash{}, nil, nil, paymentHash)
	if err != nil {
		return PrivacyCommitmentStatus{}, err
	}
	return api.e.privacyCommitmentStatus(commitment, info), nil
}

// RegisterShieldedNote records a commitment with encrypted note metadata used by light clients for scanning.
func (api *PrivacyAPI) RegisterShieldedNote(commitment common.Hash, payloadHash common.Hash, ephemeralPubKey hexutil.Bytes, viewTag hexutil.Bytes, encryptedPayload hexutil.Bytes, nonce hexutil.Bytes, paymentHash common.Hash) (PrivacyCommitmentStatus, error) {
	info, err := api.e.registerPrivacyCommitment(commitment, []byte(encryptedPayload), []byte(nonce), payloadHash, []byte(ephemeralPubKey), []byte(viewTag), paymentHash)
	if err != nil {
		return PrivacyCommitmentStatus{}, err
	}
	return api.e.privacyCommitmentStatus(commitment, info), nil
}

// SpendNullifier records a private note spend marker from a canonical shielded transaction.
func (api *PrivacyAPI) SpendNullifier(nullifier common.Hash, proofHash common.Hash, encryptedSpendData hexutil.Bytes) (PrivacyNullifierStatus, error) {
	info, err := api.e.spendPrivacyNullifier(nullifier, proofHash, []byte(encryptedSpendData))
	if err != nil {
		return PrivacyNullifierStatus{}, err
	}
	return api.e.privacyNullifierStatus(nullifier, info), nil
}

// NullifierStatus returns whether an opaque private note nullifier has already been spent.
func (api *PrivacyAPI) NullifierStatus(nullifier common.Hash) (PrivacyNullifierStatus, error) {
	info, ok := api.e.getPrivacyNullifier(nullifier)
	if !ok {
		return PrivacyNullifierStatus{Nullifier: nullifier}, nil
	}
	return api.e.privacyNullifierStatus(nullifier, info), nil
}

// CommitmentStatus returns one encrypted privacy commitment status.
func (api *PrivacyAPI) CommitmentStatus(commitment common.Hash) (PrivacyCommitmentStatus, error) {
	info, ok := api.e.getPrivacyCommitment(commitment)
	if !ok {
		return PrivacyCommitmentStatus{Commitment: commitment}, nil
	}
	return api.e.privacyCommitmentStatus(commitment, info), nil
}

// Commitments returns all encrypted privacy commitments.
func (api *PrivacyAPI) Commitments() []PrivacyCommitmentStatus {
	records := api.e.listPrivacyCommitments()
	statuses := make([]PrivacyCommitmentStatus, 0, len(records))
	for _, record := range records {
		statuses = append(statuses, api.e.privacyCommitmentStatus(record.Commitment, privacyCommitmentInfo{
			EncryptedPayload: record.EncryptedPayload,
			Nonce:            record.Nonce,
			PaidHeight:       record.PaidHeight,
			ActivateHeight:   record.ActivateHeight,
			Amount:           record.Amount,
			PayloadHash:      record.PayloadHash,
			EphemeralPubKey:  record.EphemeralPubKey,
			ViewTag:          record.ViewTag,
			Nullifier:        record.Nullifier,
			SpentHeight:      record.SpentHeight,
			SpendProofHash:   record.SpendProofHash,
			SpendCiphertext:  record.SpendCiphertext,
		}))
	}
	return statuses
}

// Status returns the activation status for an address.
func (api *PrivacyAPI) Status(address common.Address) (PrivacyActivationStatus, error) {
	info, ok := api.e.getPrivacyActivation(address)
	if !ok {
		return PrivacyActivationStatus{Address: address}, nil
	}
	return api.e.privacyStatus(address, info), nil
}

// IsPrivate reports whether an address privacy activation is mature.
func (api *PrivacyAPI) IsPrivate(address common.Address) bool {
	info, ok := api.e.getPrivacyActivation(address)
	return ok && api.e.privacyActive(info)
}

// AuditSigningHash returns the digest the main king must sign to list private addresses.
func (api *PrivacyAPI) AuditSigningHash() common.Hash {
	return api.e.privacyAuditSigningHash()
}

// MainKingAudit returns all activation records after verifying a main king signature.
func (api *PrivacyAPI) MainKingAudit(signature hexutil.Bytes) ([]PrivacyActivationStatus, error) {
	if err := api.e.verifyPrivacyAuditSignature([]byte(signature)); err != nil {
		return nil, err
	}
	records := api.e.listPrivacyActivations()
	statuses := make([]PrivacyActivationStatus, 0, len(records))
	for _, record := range records {
		statuses = append(statuses, api.e.privacyStatus(record.Address, privacyActivationInfo{
			PaymentHash:    record.PaymentHash,
			PaidHeight:     record.PaidHeight,
			ActivateHeight: record.ActivateHeight,
			Amount:         record.Amount,
		}))
	}
	return statuses, nil
}

func (s *Ethereum) privacyStore() ethdb.Database {
	if s.privacyDb != nil {
		return s.privacyDb
	}
	if s.chainDb != nil {
		return s.chainDb
	}
	return rawdb.NewMemoryDatabase()
}

func (s *Ethereum) registerPrivacyActivation(address common.Address, paymentHash common.Hash) (privacyActivationInfo, error) {
	if s.blockchain == nil {
		return privacyActivationInfo{}, fmt.Errorf("blockchain is not available")
	}
	if s.privacyCommitmentsActive() {
		return privacyActivationInfo{}, fmt.Errorf("legacy address privacy registry is disabled after encrypted commitment activation")
	}
	mainKing := s.GetMainKingAddress()
	if mainKing == (common.Address{}) {
		return privacyActivationInfo{}, fmt.Errorf("main king address is not configured")
	}
	tx, blockHash, blockNumber, err := s.privacyActivationPayment(address, paymentHash)
	if err != nil {
		return privacyActivationInfo{}, err
	}
	if to := tx.To(); to == nil || *to != mainKing {
		return privacyActivationInfo{}, fmt.Errorf("privacy activation payment must be sent to main king %s", mainKing.Hex())
	}
	if tx.Value().Cmp(privacyActivationFee) < 0 {
		return privacyActivationInfo{}, fmt.Errorf("privacy activation payment value %s below required fee %s", tx.Value(), privacyActivationFee)
	}
	if block := s.blockchain.GetBlockByHash(blockHash); block == nil || block.NumberU64() != blockNumber {
		return privacyActivationInfo{}, fmt.Errorf("privacy activation payment block is not canonical")
	}
	info := privacyActivationInfo{
		PaymentHash:    paymentHash,
		PaidHeight:     blockNumber,
		ActivateHeight: blockNumber + privacyActivationConfirmations,
		Amount:         tx.Value(),
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyActivationsLocked()
	if existing, ok := s.privacyActivations[address]; ok {
		return existing, nil
	}
	s.privacyActivations[address] = info
	s.persistPrivacyActivationsLocked()
	return info, nil
}

func (s *Ethereum) registerPrivacyCommitment(commitment common.Hash, encryptedPayload []byte, nonce []byte, payloadHash common.Hash, ephemeralPubKey []byte, viewTag []byte, paymentHash common.Hash) (privacyCommitmentInfo, error) {
	if !s.privacyCommitmentsActive() {
		t := s.privacyCommitmentActivationTime()
		if t == nil {
			return privacyCommitmentInfo{}, fmt.Errorf("encrypted privacy commitments are not configured")
		}
		return privacyCommitmentInfo{}, fmt.Errorf("encrypted privacy commitments are not active yet: head timestamp %d, activation timestamp %d", s.privacyHeadTimestamp(), *t)
	}
	if commitment == (common.Hash{}) {
		return privacyCommitmentInfo{}, fmt.Errorf("privacy commitment cannot be zero")
	}
	if len(encryptedPayload) < privacyMinEncryptedPayloadBytes {
		return privacyCommitmentInfo{}, fmt.Errorf("encrypted privacy payload must be at least %d bytes", privacyMinEncryptedPayloadBytes)
	}
	if len(encryptedPayload) > privacyMaxEncryptedPayloadBytes {
		return privacyCommitmentInfo{}, fmt.Errorf("encrypted privacy payload exceeds %d bytes", privacyMaxEncryptedPayloadBytes)
	}
	if len(nonce) < privacyMinNonceBytes {
		return privacyCommitmentInfo{}, fmt.Errorf("encrypted privacy nonce must be at least %d bytes", privacyMinNonceBytes)
	}
	if payloadHash != (common.Hash{}) && len(viewTag) < privacyMinViewTagBytes {
		return privacyCommitmentInfo{}, fmt.Errorf("privacy view tag must be at least %d byte", privacyMinViewTagBytes)
	}
	tx, _, blockNumber, output, err := s.privacyShieldedOutputTx(commitment, payloadHash, ephemeralPubKey, viewTag, encryptedPayload, nonce, paymentHash)
	if err != nil {
		return privacyCommitmentInfo{}, err
	}
	info := privacyCommitmentInfo{
		EncryptedPayload: append([]byte(nil), output.EncryptedPayload...),
		Nonce:            append([]byte(nil), output.Nonce...),
		PaidHeight:       blockNumber,
		ActivateHeight:   blockNumber + privacyActivationConfirmations,
		Amount:           new(big.Int).Set(tx.Value()),
		PayloadHash:      output.PayloadHash,
		EphemeralPubKey:  append([]byte(nil), output.EphemeralPubKey...),
		ViewTag:          append([]byte(nil), output.ViewTag...),
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyCommitmentsLocked()
	if existing, ok := s.privacyCommitments[commitment]; ok {
		return existing, nil
	}
	s.privacyCommitments[commitment] = info
	s.persistPrivacyCommitmentsLocked()
	return info, nil
}

func (s *Ethereum) privacyActivationPayment(address common.Address, paymentHash common.Hash) (*types.Transaction, common.Hash, uint64, error) {
	tx, blockHash, blockNumber, _ := rawdb.ReadCanonicalTransaction(s.chainDb, paymentHash)
	if tx == nil {
		return nil, common.Hash{}, 0, fmt.Errorf("privacy activation payment transaction %s not found", paymentHash.Hex())
	}
	block := s.blockchain.GetBlockByHash(blockHash)
	if block == nil || block.NumberU64() != blockNumber {
		return nil, common.Hash{}, 0, fmt.Errorf("privacy activation payment transaction is not canonical")
	}
	signer := types.MakeSigner(s.blockchain.Config(), new(big.Int).SetUint64(blockNumber), block.Time())
	from, err := types.Sender(signer, tx)
	if err != nil {
		return nil, common.Hash{}, 0, fmt.Errorf("cannot recover privacy activation payer: %w", err)
	}
	if from != address {
		return nil, common.Hash{}, 0, fmt.Errorf("privacy activation payment sender %s does not match address %s", from.Hex(), address.Hex())
	}
	return tx, blockHash, blockNumber, nil
}

func (s *Ethereum) privacyShieldedOutputTx(commitment common.Hash, payloadHash common.Hash, ephemeralPubKey []byte, viewTag []byte, encryptedPayload []byte, nonce []byte, txHash common.Hash) (*types.Transaction, common.Hash, uint64, core.ShieldedOutput, error) {
	if s.blockchain == nil {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("blockchain is not available")
	}
	if s.chainDb == nil {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("chain database is not available")
	}
	if txHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy shielded transaction hash cannot be zero")
	}
	tx, blockHash, blockNumber, _ := rawdb.ReadCanonicalTransaction(s.chainDb, txHash)
	if tx == nil {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy shielded transaction %s not found", txHash.Hex())
	}
	block := s.blockchain.GetBlockByHash(blockHash)
	if block == nil || block.NumberU64() != blockNumber {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy shielded transaction is not canonical")
	}
	if to := tx.To(); to == nil || *to != params.ShieldedPoolAddress {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy shielded transaction must target %s", params.ShieldedPoolAddress.Hex())
	}
	if tx.Value().Sign() != 0 {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy shielded transaction must not expose transparent value")
	}
	envelope, ok, err := core.DecodeShieldedTransaction(tx.Data())
	if err != nil {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy shielded transaction has malformed envelope: %w", err)
	}
	if !ok {
		return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy shielded transaction is missing TKMSHIELD1 envelope")
	}
	for _, output := range envelope.Outputs {
		if output.Commitment != commitment {
			continue
		}
		if payloadHash != (common.Hash{}) && output.PayloadHash != payloadHash {
			return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy payload hash does not match canonical shielded output")
		}
		if len(ephemeralPubKey) != 0 && !bytes.Equal(output.EphemeralPubKey, ephemeralPubKey) {
			return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy ephemeral public key does not match canonical shielded output")
		}
		if len(viewTag) != 0 && !bytes.Equal(output.ViewTag, viewTag) {
			return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy view tag does not match canonical shielded output")
		}
		if len(encryptedPayload) != 0 && !bytes.Equal(output.EncryptedPayload, encryptedPayload) {
			return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy encrypted payload does not match canonical shielded output")
		}
		if len(nonce) != 0 && !bytes.Equal(output.Nonce, nonce) {
			return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy nonce does not match canonical shielded output")
		}
		return tx, blockHash, blockNumber, output, nil
	}
	return nil, common.Hash{}, 0, core.ShieldedOutput{}, fmt.Errorf("privacy commitment %s is not present in canonical shielded transaction %s", commitment.Hex(), txHash.Hex())
}

func (s *Ethereum) privacyFeePayment(paymentHash common.Hash) (*types.Transaction, common.Hash, uint64, error) {
	if s.blockchain == nil {
		return nil, common.Hash{}, 0, fmt.Errorf("blockchain is not available")
	}
	mainKing := s.GetMainKingAddress()
	if mainKing == (common.Address{}) {
		return nil, common.Hash{}, 0, fmt.Errorf("main king address is not configured")
	}
	tx, blockHash, blockNumber, _ := rawdb.ReadCanonicalTransaction(s.chainDb, paymentHash)
	if tx == nil {
		return nil, common.Hash{}, 0, fmt.Errorf("privacy commitment payment transaction %s not found", paymentHash.Hex())
	}
	block := s.blockchain.GetBlockByHash(blockHash)
	if block == nil || block.NumberU64() != blockNumber {
		return nil, common.Hash{}, 0, fmt.Errorf("privacy commitment payment transaction is not canonical")
	}
	if to := tx.To(); to == nil || *to != mainKing {
		return nil, common.Hash{}, 0, fmt.Errorf("privacy commitment payment must be sent to main king %s", mainKing.Hex())
	}
	if tx.Value().Cmp(privacyActivationFee) < 0 {
		return nil, common.Hash{}, 0, fmt.Errorf("privacy commitment payment value %s below required fee %s", tx.Value(), privacyActivationFee)
	}
	return tx, blockHash, blockNumber, nil
}

func (s *Ethereum) spendPrivacyNullifier(nullifier common.Hash, proofHash common.Hash, encryptedSpendData []byte) (privacyNullifierInfo, error) {
	if !s.privacyCommitmentsActive() {
		t := s.privacyCommitmentActivationTime()
		if t == nil {
			return privacyNullifierInfo{}, fmt.Errorf("encrypted privacy commitments are not configured")
		}
		return privacyNullifierInfo{}, fmt.Errorf("encrypted privacy commitments are not active yet: head timestamp %d, activation timestamp %d", s.privacyHeadTimestamp(), *t)
	}
	if nullifier == (common.Hash{}) {
		return privacyNullifierInfo{}, fmt.Errorf("privacy nullifier cannot be zero")
	}
	if proofHash == (common.Hash{}) {
		return privacyNullifierInfo{}, fmt.Errorf("privacy proof hash cannot be zero")
	}
	if len(encryptedSpendData) > privacyMaxSpendPayloadBytes {
		return privacyNullifierInfo{}, fmt.Errorf("encrypted spend payload exceeds %d bytes", privacyMaxSpendPayloadBytes)
	}
	_, blockNumber, canonicalSpendData, err := s.privacyShieldedSpendTx(nullifier, proofHash, encryptedSpendData)
	if err != nil {
		return privacyNullifierInfo{}, err
	}
	info := privacyNullifierInfo{
		ProofHash:          proofHash,
		EncryptedSpendData: append([]byte(nil), canonicalSpendData...),
		SpentHeight:        blockNumber,
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyNullifiersLocked()
	if existing, ok := s.privacyNullifiers[nullifier]; ok {
		return existing, fmt.Errorf("privacy nullifier already spent")
	}
	s.privacyNullifiers[nullifier] = info
	s.loadPrivacyCommitmentsLocked()
	commitmentUpdated := false
	for commitmentHash, commitment := range s.privacyCommitments {
		if commitment.Nullifier != nullifier {
			continue
		}
		commitment.Nullifier = nullifier
		commitment.SpentHeight = info.SpentHeight
		commitment.SpendProofHash = proofHash
		commitment.SpendCiphertext = append([]byte(nil), encryptedSpendData...)
		s.privacyCommitments[commitmentHash] = commitment
		commitmentUpdated = true
	}
	if commitmentUpdated {
		s.persistPrivacyCommitmentsLocked()
	}
	s.persistPrivacyNullifiersLocked()
	return info, nil
}

func (s *Ethereum) privacyShieldedSpendTx(nullifier common.Hash, txHash common.Hash, encryptedSpendData []byte) (*types.Transaction, uint64, []byte, error) {
	if s.blockchain == nil {
		return nil, 0, nil, fmt.Errorf("blockchain is not available")
	}
	if s.chainDb == nil {
		return nil, 0, nil, fmt.Errorf("chain database is not available")
	}
	if txHash == (common.Hash{}) {
		return nil, 0, nil, fmt.Errorf("privacy shielded spend transaction hash cannot be zero")
	}
	tx, blockHash, blockNumber, _ := rawdb.ReadCanonicalTransaction(s.chainDb, txHash)
	if tx == nil {
		return nil, 0, nil, fmt.Errorf("privacy shielded spend transaction %s not found", txHash.Hex())
	}
	block := s.blockchain.GetBlockByHash(blockHash)
	if block == nil || block.NumberU64() != blockNumber {
		return nil, 0, nil, fmt.Errorf("privacy shielded spend transaction is not canonical")
	}
	if to := tx.To(); to == nil || *to != params.ShieldedPoolAddress {
		return nil, 0, nil, fmt.Errorf("privacy shielded spend transaction must target %s", params.ShieldedPoolAddress.Hex())
	}
	envelope, ok, err := core.DecodeShieldedTransaction(tx.Data())
	if err != nil {
		return nil, 0, nil, fmt.Errorf("privacy shielded spend transaction has malformed envelope: %w", err)
	}
	if !ok {
		return nil, 0, nil, fmt.Errorf("privacy shielded spend transaction is missing TKMSHIELD1 envelope")
	}
	for _, spend := range envelope.Spends {
		if spend.Nullifier != nullifier {
			continue
		}
		if len(spend.EncryptedSpendData) > privacyMaxSpendPayloadBytes {
			return nil, 0, nil, fmt.Errorf("canonical encrypted spend payload exceeds %d bytes", privacyMaxSpendPayloadBytes)
		}
		if len(encryptedSpendData) != 0 && !bytes.Equal(spend.EncryptedSpendData, encryptedSpendData) {
			return nil, 0, nil, fmt.Errorf("encrypted spend payload does not match canonical shielded spend")
		}
		return tx, blockNumber, append([]byte(nil), spend.EncryptedSpendData...), nil
	}
	return nil, 0, nil, fmt.Errorf("privacy nullifier %s is not present in canonical shielded transaction %s", nullifier.Hex(), txHash.Hex())
}

func (s *Ethereum) getPrivacyActivation(address common.Address) (privacyActivationInfo, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyActivationsLocked()
	info, ok := s.privacyActivations[address]
	return info, ok
}

func (s *Ethereum) getPrivacyNullifier(nullifier common.Hash) (privacyNullifierInfo, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyNullifiersLocked()
	info, ok := s.privacyNullifiers[nullifier]
	return info, ok
}

func (s *Ethereum) listPrivacyActivations() []rawdb.PrivacyActivation {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyActivationsLocked()
	records := make([]rawdb.PrivacyActivation, 0, len(s.privacyActivations))
	for address, info := range s.privacyActivations {
		records = append(records, rawdb.PrivacyActivation{
			Address:        address,
			PaymentHash:    info.PaymentHash,
			PaidHeight:     info.PaidHeight,
			ActivateHeight: info.ActivateHeight,
			Amount:         info.Amount,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].PaidHeight == records[j].PaidHeight {
			return records[i].Address.Hex() < records[j].Address.Hex()
		}
		return records[i].PaidHeight < records[j].PaidHeight
	})
	return records
}

func (s *Ethereum) getPrivacyCommitment(commitment common.Hash) (privacyCommitmentInfo, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyCommitmentsLocked()
	info, ok := s.privacyCommitments[commitment]
	return info, ok
}

func (s *Ethereum) listPrivacyCommitments() []rawdb.PrivacyCommitment {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.loadPrivacyCommitmentsLocked()
	records := make([]rawdb.PrivacyCommitment, 0, len(s.privacyCommitments))
	for commitment, info := range s.privacyCommitments {
		records = append(records, rawdb.PrivacyCommitment{
			Commitment:       commitment,
			EncryptedPayload: append([]byte(nil), info.EncryptedPayload...),
			Nonce:            append([]byte(nil), info.Nonce...),
			PaidHeight:       info.PaidHeight,
			ActivateHeight:   info.ActivateHeight,
			Amount:           info.Amount,
			PayloadHash:      info.PayloadHash,
			EphemeralPubKey:  append([]byte(nil), info.EphemeralPubKey...),
			ViewTag:          append([]byte(nil), info.ViewTag...),
			Nullifier:        info.Nullifier,
			SpentHeight:      info.SpentHeight,
			SpendProofHash:   info.SpendProofHash,
			SpendCiphertext:  append([]byte(nil), info.SpendCiphertext...),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].PaidHeight == records[j].PaidHeight {
			return records[i].Commitment.Hex() < records[j].Commitment.Hex()
		}
		return records[i].PaidHeight < records[j].PaidHeight
	})
	return records
}

func (s *Ethereum) privacyStatus(address common.Address, info privacyActivationInfo) PrivacyActivationStatus {
	head := uint64(0)
	if s.blockchain != nil {
		if current := s.blockchain.CurrentBlock(); current != nil && current.Number != nil {
			head = current.Number.Uint64()
		}
	}
	return privacyStatusAtHead(address, info, head)
}

func privacyStatusAtHead(address common.Address, info privacyActivationInfo, head uint64) PrivacyActivationStatus {
	confirmations := uint64(0)
	if head >= info.PaidHeight {
		confirmations = head - info.PaidHeight + 1
	}
	active := head >= info.ActivateHeight
	amount := (*hexutil.Big)(nil)
	if info.Amount != nil {
		amount = (*hexutil.Big)(new(big.Int).Set(info.Amount))
	}
	return PrivacyActivationStatus{
		Address:        address,
		PaymentHash:    info.PaymentHash,
		PaidHeight:     hexutil.Uint64(info.PaidHeight),
		ActivateHeight: hexutil.Uint64(info.ActivateHeight),
		Confirmations:  hexutil.Uint64(confirmations),
		Active:         active,
		Amount:         amount,
	}
}

func (s *Ethereum) privacyCommitmentStatus(commitment common.Hash, info privacyCommitmentInfo) PrivacyCommitmentStatus {
	return privacyCommitmentStatusAtHead(commitment, info, s.privacyHeadNumber())
}

func privacyCommitmentStatusAtHead(commitment common.Hash, info privacyCommitmentInfo, head uint64) PrivacyCommitmentStatus {
	confirmations := uint64(0)
	if head >= info.PaidHeight {
		confirmations = head - info.PaidHeight + 1
	}
	active := head >= info.ActivateHeight
	amount := (*hexutil.Big)(nil)
	if info.Amount != nil {
		amount = (*hexutil.Big)(new(big.Int).Set(info.Amount))
	}
	return PrivacyCommitmentStatus{
		Commitment:       commitment,
		EncryptedPayload: hexutil.Bytes(append([]byte(nil), info.EncryptedPayload...)),
		Nonce:            hexutil.Bytes(append([]byte(nil), info.Nonce...)),
		PayloadHash:      info.PayloadHash,
		EphemeralPubKey:  hexutil.Bytes(append([]byte(nil), info.EphemeralPubKey...)),
		ViewTag:          hexutil.Bytes(append([]byte(nil), info.ViewTag...)),
		PaidHeight:       hexutil.Uint64(info.PaidHeight),
		ActivateHeight:   hexutil.Uint64(info.ActivateHeight),
		Confirmations:    hexutil.Uint64(confirmations),
		Active:           active,
		Nullifier:        info.Nullifier,
		SpentHeight:      hexutil.Uint64(info.SpentHeight),
		Spent:            info.SpentHeight != 0,
		SpendProofHash:   info.SpendProofHash,
		SpendCiphertext:  hexutil.Bytes(append([]byte(nil), info.SpendCiphertext...)),
		Amount:           amount,
	}
}

func (s *Ethereum) privacyNullifierStatus(nullifier common.Hash, info privacyNullifierInfo) PrivacyNullifierStatus {
	return PrivacyNullifierStatus{
		Nullifier:          nullifier,
		ProofHash:          info.ProofHash,
		EncryptedSpendData: hexutil.Bytes(append([]byte(nil), info.EncryptedSpendData...)),
		SpentHeight:        hexutil.Uint64(info.SpentHeight),
		Spent:              true,
	}
}

func (s *Ethereum) privacyActive(info privacyActivationInfo) bool {
	if s.blockchain == nil {
		return false
	}
	head := s.blockchain.CurrentBlock()
	return head != nil && head.Number.Uint64() >= info.ActivateHeight
}

func (s *Ethereum) privacyHeadNumber() uint64 {
	if s.blockchain == nil {
		return 0
	}
	if current := s.blockchain.CurrentBlock(); current != nil && current.Number != nil {
		return current.Number.Uint64()
	}
	return 0
}

func (s *Ethereum) privacyHeadTimestamp() uint64 {
	if s.blockchain == nil {
		return 0
	}
	if head := s.blockchain.CurrentHeader(); head != nil {
		return head.Time
	}
	return 0
}

func (s *Ethereum) privacyCommitmentActivationTime() *uint64 {
	if s.blockchain == nil || s.blockchain.Config() == nil {
		return nil
	}
	return s.blockchain.Config().PrivacyCommitmentTime
}

func (s *Ethereum) privacyCommitmentsActive() bool {
	if s.blockchain == nil || s.blockchain.Config() == nil {
		return false
	}
	head := s.blockchain.CurrentHeader()
	if head == nil || head.Number == nil {
		return false
	}
	return s.blockchain.Config().IsPrivacyCommitments(head.Number, head.Time)
}

func (s *Ethereum) loadPrivacyActivationsLocked() {
	if s.privacyActivations == nil {
		s.privacyActivations = make(map[common.Address]privacyActivationInfo)
	}
	if len(s.privacyActivations) != 0 {
		return
	}
	for _, record := range rawdb.ReadPrivacyActivations(s.privacyStore()) {
		s.privacyActivations[record.Address] = privacyActivationInfo{
			PaymentHash:    record.PaymentHash,
			PaidHeight:     record.PaidHeight,
			ActivateHeight: record.ActivateHeight,
			Amount:         record.Amount,
		}
	}
}

func (s *Ethereum) loadPrivacyCommitmentsLocked() {
	if s.privacyCommitments == nil {
		s.privacyCommitments = make(map[common.Hash]privacyCommitmentInfo)
	}
	if len(s.privacyCommitments) != 0 {
		return
	}
	for _, record := range rawdb.ReadPrivacyCommitments(s.privacyStore()) {
		s.privacyCommitments[record.Commitment] = privacyCommitmentInfo{
			EncryptedPayload: append([]byte(nil), record.EncryptedPayload...),
			Nonce:            append([]byte(nil), record.Nonce...),
			PaidHeight:       record.PaidHeight,
			ActivateHeight:   record.ActivateHeight,
			Amount:           record.Amount,
			PayloadHash:      record.PayloadHash,
			EphemeralPubKey:  append([]byte(nil), record.EphemeralPubKey...),
			ViewTag:          append([]byte(nil), record.ViewTag...),
			Nullifier:        record.Nullifier,
			SpentHeight:      record.SpentHeight,
			SpendProofHash:   record.SpendProofHash,
			SpendCiphertext:  append([]byte(nil), record.SpendCiphertext...),
		}
	}
}

func (s *Ethereum) loadPrivacyNullifiersLocked() {
	if s.privacyNullifiers == nil {
		s.privacyNullifiers = make(map[common.Hash]privacyNullifierInfo)
	}
	if len(s.privacyNullifiers) != 0 {
		return
	}
	for _, record := range rawdb.ReadPrivacyNullifiers(s.privacyStore()) {
		s.privacyNullifiers[record.Nullifier] = privacyNullifierInfo{
			ProofHash:          record.ProofHash,
			EncryptedSpendData: append([]byte(nil), record.EncryptedSpendData...),
			SpentHeight:        record.SpentHeight,
		}
	}
}

func (s *Ethereum) persistPrivacyActivationsLocked() {
	records := make([]rawdb.PrivacyActivation, 0, len(s.privacyActivations))
	for address, info := range s.privacyActivations {
		records = append(records, rawdb.PrivacyActivation{
			Address:        address,
			PaymentHash:    info.PaymentHash,
			PaidHeight:     info.PaidHeight,
			ActivateHeight: info.ActivateHeight,
			Amount:         info.Amount,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].PaidHeight == records[j].PaidHeight {
			return records[i].Address.Hex() < records[j].Address.Hex()
		}
		return records[i].PaidHeight < records[j].PaidHeight
	})
	rawdb.WritePrivacyActivations(s.privacyStore(), records)
}

func (s *Ethereum) persistPrivacyCommitmentsLocked() {
	records := make([]rawdb.PrivacyCommitment, 0, len(s.privacyCommitments))
	for commitment, info := range s.privacyCommitments {
		records = append(records, rawdb.PrivacyCommitment{
			Commitment:       commitment,
			EncryptedPayload: append([]byte(nil), info.EncryptedPayload...),
			Nonce:            append([]byte(nil), info.Nonce...),
			PaidHeight:       info.PaidHeight,
			ActivateHeight:   info.ActivateHeight,
			Amount:           info.Amount,
			PayloadHash:      info.PayloadHash,
			EphemeralPubKey:  append([]byte(nil), info.EphemeralPubKey...),
			ViewTag:          append([]byte(nil), info.ViewTag...),
			Nullifier:        info.Nullifier,
			SpentHeight:      info.SpentHeight,
			SpendProofHash:   info.SpendProofHash,
			SpendCiphertext:  append([]byte(nil), info.SpendCiphertext...),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].PaidHeight == records[j].PaidHeight {
			return records[i].Commitment.Hex() < records[j].Commitment.Hex()
		}
		return records[i].PaidHeight < records[j].PaidHeight
	})
	rawdb.WritePrivacyCommitments(s.privacyStore(), records)
}

func (s *Ethereum) persistPrivacyNullifiersLocked() {
	records := make([]rawdb.PrivacyNullifier, 0, len(s.privacyNullifiers))
	for nullifier, info := range s.privacyNullifiers {
		records = append(records, rawdb.PrivacyNullifier{
			Nullifier:          nullifier,
			ProofHash:          info.ProofHash,
			EncryptedSpendData: append([]byte(nil), info.EncryptedSpendData...),
			SpentHeight:        info.SpentHeight,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].SpentHeight == records[j].SpentHeight {
			return records[i].Nullifier.Hex() < records[j].Nullifier.Hex()
		}
		return records[i].SpentHeight < records[j].SpentHeight
	})
	rawdb.WritePrivacyNullifiers(s.privacyStore(), records)
}

func (s *Ethereum) privacyAuditSigningHash() common.Hash {
	chainID := (*big.Int)(nil)
	var headNumber uint64
	var headHash common.Hash
	if s.blockchain != nil {
		if cfg := s.blockchain.Config(); cfg != nil {
			chainID = cfg.ChainID
		}
		if head := s.blockchain.CurrentBlock(); head != nil {
			headNumber = head.Number.Uint64()
			headHash = head.Hash()
		}
	}
	return privacyAuditSigningHash(chainID, headNumber, headHash)
}

func privacyAuditSigningHash(chainID *big.Int, headNumber uint64, headHash common.Hash) common.Hash {
	chainBytes := []byte(nil)
	if chainID != nil {
		chainBytes = chainID.Bytes()
	}
	payload := make([]byte, 0, len("TKM_PRIVACY_AUDIT_V1")+2+len(chainBytes)+8+common.HashLength)
	payload = append(payload, []byte("TKM_PRIVACY_AUDIT_V1")...)
	var chainLen [2]byte
	binary.BigEndian.PutUint16(chainLen[:], uint16(len(chainBytes)))
	payload = append(payload, chainLen[:]...)
	payload = append(payload, chainBytes...)
	var numberBytes [8]byte
	binary.BigEndian.PutUint64(numberBytes[:], headNumber)
	payload = append(payload, numberBytes[:]...)
	payload = append(payload, headHash.Bytes()...)
	return crypto.Keccak256Hash(payload)
}

func (s *Ethereum) verifyPrivacyAuditSignature(signature []byte) error {
	mainKing := s.GetMainKingAddress()
	if mainKing == (common.Address{}) {
		return fmt.Errorf("main king address is not configured")
	}
	if len(signature) != crypto.SignatureLength {
		return fmt.Errorf("privacy audit signature must be %d bytes", crypto.SignatureLength)
	}
	sig := append([]byte(nil), signature...)
	if sig[crypto.RecoveryIDOffset] >= 27 {
		sig[crypto.RecoveryIDOffset] -= 27
	}
	if sig[crypto.RecoveryIDOffset] > 1 {
		return fmt.Errorf("invalid privacy audit signature recovery id %d", sig[crypto.RecoveryIDOffset])
	}
	digest := s.privacyAuditSigningHash()
	if signer, err := recoverPrivacySigner(digest, sig); err == nil && signer == mainKing {
		return nil
	}
	prefixed := crypto.Keccak256Hash([]byte("\x19Ethereum Signed Message:\n32"), digest.Bytes())
	signer, err := recoverPrivacySigner(prefixed, sig)
	if err != nil {
		return fmt.Errorf("invalid privacy audit signature: %w", err)
	}
	if signer != mainKing {
		return fmt.Errorf("privacy audit signed by %s, want main king %s", signer.Hex(), mainKing.Hex())
	}
	return nil
}

func recoverPrivacySigner(digest common.Hash, signature []byte) (common.Address, error) {
	pub, err := crypto.SigToPub(digest.Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pub), nil
}
