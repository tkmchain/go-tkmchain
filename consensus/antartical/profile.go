package antartical

// This file contains the consensus-facing TKM profile primitives that sit
// above the EVM interpreter. They are deliberately deterministic and small:
// clients can implement them without depending on a particular execution
// backend, while block/state integration can use the same commitments.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tkmasset"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidTypedDomain        = errors.New("invalid TKM typed transaction domain")
	ErrInvalidTokenOperation     = errors.New("invalid TKM token operation")
	ErrTokenPolicyViolation      = errors.New("TKM token policy violation")
	ErrInvalidAssetRegistry      = errors.New("invalid TKM asset registry")
	ErrInvalidShieldedAssetBind  = errors.New("invalid shielded asset binding")
	ErrInvalidConflictTranscript = errors.New("invalid execution conflict transcript")
	ErrInvalidLightClientProof   = errors.New("invalid TKM stateless light-client proof")
)

// ReceiptTranscript is fork-integration metadata for an optimistic execution
// receipt. It is kept outside legacy receipt RLP until a network-wide receipt
// format fork is activated, avoiding receipt-root changes on old blocks.
type ReceiptTranscript struct {
	ReceiptIndex uint32
	Commitment   common.Hash
}

func (t ConflictTranscript) ReceiptMetadata(receiptIndex uint32) (ReceiptTranscript, error) {
	commitment, err := t.Commitment()
	if err != nil {
		return ReceiptTranscript{}, err
	}
	return ReceiptTranscript{ReceiptIndex: receiptIndex, Commitment: commitment}, nil
}

func (m ReceiptTranscript) Verify(t ConflictTranscript, receiptIndex uint32) bool {
	if m.ReceiptIndex != receiptIndex || m.Commitment == (common.Hash{}) {
		return false
	}
	commitment, err := t.Commitment()
	return err == nil && commitment == m.Commitment
}

// TypedTransactionDomain binds a signature to both the network and the
// contract receiving the operation. A chain ID alone is insufficient when a
// signature is replayable across two contracts on the same network.
type TypedTransactionDomain struct {
	ChainID  *big.Int
	Contract common.Address
	Type     uint8
}

func (d TypedTransactionDomain) validate() error {
	if d.ChainID == nil || d.ChainID.Sign() <= 0 || d.ChainID.BitLen() > 256 || d.Contract == (common.Address{}) || d.Type == 0 {
		return ErrInvalidTypedDomain
	}
	return nil
}

func (d TypedTransactionDomain) Digest(payload []byte) (common.Hash, error) {
	if err := d.validate(); err != nil {
		return common.Hash{}, err
	}
	encoded, err := rlp.EncodeToBytes([]interface{}{
		common.BytesToHash([]byte("TKM_TYPED_TX_DOMAIN_V1")), d.ChainID, d.Contract, d.Type, crypto.Keccak256Hash(payload),
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(encoded), nil
}

// TypedTransaction carries a signature over the TKM domain and arbitrary
// contract payload. ML-DSA-87 is the post-quantum sender policy; empty
// Algorithm selects the legacy secp256k1 compatibility path.
type TypedTransaction struct {
	Sender    common.Address
	Domain    TypedTransactionDomain
	Payload   []byte
	Algorithm string
	PublicKey []byte
	Signature []byte
}

func (tx TypedTransaction) Verify() error {
	if tx.Sender == (common.Address{}) {
		return ErrInvalidTypedDomain
	}
	digest, err := tx.Domain.Digest(tx.Payload)
	if err != nil {
		return err
	}
	if tx.Algorithm == pqcrypto.AlgorithmMLDSA87 {
		if !pqcrypto.VerifyMLDSA87(tx.PublicKey, digest[:], tx.Signature) {
			return ErrInvalidTypedDomain
		}
		address, err := pqcrypto.Address(tx.Algorithm, tx.PublicKey)
		if err != nil || address != tx.Sender {
			return ErrInvalidTypedDomain
		}
		return nil
	}
	if tx.Algorithm != "" || len(tx.PublicKey) != 0 || len(tx.Signature) != crypto.SignatureLength {
		return ErrInvalidTypedDomain
	}
	pub, err := crypto.SigToPub(digest.Bytes(), tx.Signature)
	if err != nil || crypto.PubkeyToAddress(*pub) != tx.Sender {
		return ErrInvalidTypedDomain
	}
	return nil
}

// TokenOperationKind is enforced against a manifest's capability flags.
type TokenOperationKind uint8

const (
	TokenTransfer TokenOperationKind = iota + 1
	TokenMint
	TokenBurn
	TokenPause
	TokenUnpause
	TokenShield
)

type TokenPolicy struct {
	AssetID          common.Hash
	Admin            common.Address
	Flags            tkmasset.Flags
	MaxSupply        *big.Int
	RoyaltyBPS       uint16
	RoyaltyRecipient common.Address
}

// Commitment is the canonical policy commitment carried by a tkmasset
// manifest. AssetID is intentionally excluded: it is derived from the
// manifest hash and deployment address, while this commitment describes the
// policy itself.
func (p TokenPolicy) Commitment() (common.Hash, error) {
	if p.Admin == (common.Address{}) || p.MaxSupply != nil && p.MaxSupply.Sign() < 0 || p.RoyaltyBPS > 10_000 || p.RoyaltyBPS > 0 && p.RoyaltyRecipient == (common.Address{}) {
		return common.Hash{}, ErrInvalidTokenOperation
	}
	var maxSupply []byte
	if p.MaxSupply != nil {
		maxSupply = p.MaxSupply.Bytes()
	}
	blob, err := rlp.EncodeToBytes([]interface{}{
		[]byte("TKM_TOKEN_POLICY_V1"), p.Admin, uint32(p.Flags), maxSupply, p.RoyaltyBPS, p.RoyaltyRecipient,
	})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

// ValidateManifest binds executable policy enforcement to the immutable
// manifest fields. A manifest with a different capability set or policy hash
// cannot be used with this policy state machine.
func (p TokenPolicy) ValidateManifest(m tkmasset.Manifest) error {
	if err := m.Validate(); err != nil || m.Flags != p.Flags {
		return ErrTokenPolicyViolation
	}
	commitment, err := p.Commitment()
	if err != nil || m.PolicyHash != commitment {
		return ErrTokenPolicyViolation
	}
	return nil
}

type TokenOperation struct {
	Kind   TokenOperationKind
	Actor  common.Address
	From   common.Address
	To     common.Address
	Amount *big.Int
}

type TokenState struct {
	TotalSupply *big.Int
	Balances    map[common.Address]*big.Int
	Paused      bool
}

func (s *TokenState) balance(address common.Address) *big.Int {
	if s.Balances == nil {
		s.Balances = make(map[common.Address]*big.Int)
	}
	if s.Balances[address] == nil {
		s.Balances[address] = new(big.Int)
	}
	return s.Balances[address]
}

func (s *TokenState) Apply(policy TokenPolicy, op TokenOperation) error {
	if policy.AssetID == (common.Hash{}) || policy.Admin == (common.Address{}) || op.Actor == (common.Address{}) || policy.MaxSupply != nil && policy.MaxSupply.Sign() < 0 || policy.RoyaltyBPS > 10_000 || policy.RoyaltyBPS > 0 && policy.RoyaltyRecipient == (common.Address{}) {
		return ErrInvalidTokenOperation
	}
	if op.Kind != TokenPause && op.Kind != TokenUnpause && (op.Amount == nil || op.Amount.Sign() < 0) {
		return ErrInvalidTokenOperation
	}
	if s.TotalSupply == nil {
		s.TotalSupply = new(big.Int)
	}
	if op.Kind != TokenPause && op.Kind != TokenUnpause && op.Kind != TokenShield && op.Amount.Sign() == 0 {
		return ErrInvalidTokenOperation
	}
	requireAdmin := func() error {
		if op.Actor != policy.Admin {
			return ErrTokenPolicyViolation
		}
		return nil
	}
	if s.Paused && op.Kind != TokenUnpause {
		return ErrTokenPolicyViolation
	}
	switch op.Kind {
	case TokenMint:
		if policy.Flags&tkmasset.FlagMintable == 0 || requireAdmin() != nil || op.To == (common.Address{}) {
			return ErrTokenPolicyViolation
		}
		next := new(big.Int).Add(s.TotalSupply, op.Amount)
		if policy.MaxSupply != nil && next.Cmp(policy.MaxSupply) > 0 {
			return ErrTokenPolicyViolation
		}
		s.TotalSupply.Set(next)
		s.balance(op.To).Add(s.balance(op.To), op.Amount)
	case TokenBurn:
		if policy.Flags&tkmasset.FlagBurnable == 0 || op.Actor != op.From || op.From == (common.Address{}) || s.balance(op.From).Cmp(op.Amount) < 0 {
			return ErrTokenPolicyViolation
		}
		s.balance(op.From).Sub(s.balance(op.From), op.Amount)
		s.TotalSupply.Sub(s.TotalSupply, op.Amount)
	case TokenTransfer:
		if op.From == (common.Address{}) || op.To == (common.Address{}) || op.Actor != op.From || s.balance(op.From).Cmp(op.Amount) < 0 {
			return ErrTokenPolicyViolation
		}
		fee := new(big.Int)
		if policy.Flags&tkmasset.FlagRoyalty != 0 && policy.RoyaltyBPS > 0 {
			if policy.RoyaltyRecipient == (common.Address{}) || policy.RoyaltyBPS > 10_000 {
				return ErrTokenPolicyViolation
			}
			fee.Mul(op.Amount, new(big.Int).SetUint64(uint64(policy.RoyaltyBPS)))
			fee.Div(fee, big.NewInt(10_000))
		}
		net := new(big.Int).Sub(op.Amount, fee)
		s.balance(op.From).Sub(s.balance(op.From), op.Amount)
		s.balance(op.To).Add(s.balance(op.To), net)
		if fee.Sign() > 0 {
			s.balance(policy.RoyaltyRecipient).Add(s.balance(policy.RoyaltyRecipient), fee)
		}
	case TokenPause:
		if policy.Flags&tkmasset.FlagPausable == 0 || requireAdmin() != nil {
			return ErrTokenPolicyViolation
		}
		s.Paused = true
	case TokenUnpause:
		if policy.Flags&tkmasset.FlagPausable == 0 || requireAdmin() != nil {
			return ErrTokenPolicyViolation
		}
		s.Paused = false
	case TokenShield:
		if policy.Flags&tkmasset.FlagShielded == 0 {
			return ErrTokenPolicyViolation
		}
	default:
		return ErrInvalidTokenOperation
	}
	return nil
}

// AssetRegistryEntry is the canonical state-independent registry record.
type AssetRegistryEntry struct {
	AssetID         common.Hash
	ManifestHash    common.Hash
	RuntimeCodeHash common.Hash
}

const assetRegistryExtraMagic = "TKM_ASSET_REGISTRY_V1"

func AssetRegistryCommitment(entries []AssetRegistryEntry) (common.Hash, error) {
	if len(entries) == 0 {
		return crypto.Keccak256Hash([]byte(assetRegistryExtraMagic), []byte("empty")), nil
	}
	items := append([]AssetRegistryEntry(nil), entries...)
	sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i].AssetID[:], items[j].AssetID[:]) < 0 })
	leaves := make([]common.Hash, len(items))
	for i, entry := range items {
		if entry.AssetID == (common.Hash{}) || entry.ManifestHash == (common.Hash{}) || entry.RuntimeCodeHash == (common.Hash{}) || (i > 0 && items[i-1].AssetID == entry.AssetID) {
			return common.Hash{}, ErrInvalidAssetRegistry
		}
		blob, err := rlp.EncodeToBytes([]interface{}{[]byte("TKM_ASSET_REGISTRY_LEAF_V1"), entry.AssetID, entry.ManifestHash, entry.RuntimeCodeHash})
		if err != nil {
			return common.Hash{}, err
		}
		leaves[i] = crypto.Keccak256Hash(blob)
	}
	for len(leaves) > 1 {
		next := make([]common.Hash, 0, (len(leaves)+1)/2)
		for i := 0; i < len(leaves); i += 2 {
			right := leaves[i]
			if i+1 < len(leaves) {
				right = leaves[i+1]
			}
			next = append(next, crypto.Keccak256Hash([]byte("TKM_ASSET_REGISTRY_NODE_V1"), leaves[i][:], right[:]))
		}
		leaves = next
	}
	return crypto.Keccak256Hash([]byte(assetRegistryExtraMagic), leaves[0][:]), nil
}

func AttachAssetRegistryCommitment(extra []byte, root common.Hash) []byte {
	result := append([]byte(nil), extra...)
	result = append(result, []byte(assetRegistryExtraMagic)...)
	return append(result, root[:]...)
}

func AssetRegistryCommitmentFromHeaderExtra(extra []byte) (common.Hash, bool, error) {
	marker := []byte(assetRegistryExtraMagic)
	index := bytes.LastIndex(extra, marker)
	if index < 0 {
		return common.Hash{}, false, nil
	}
	if len(extra) != index+len(marker)+common.HashLength {
		return common.Hash{}, true, ErrInvalidAssetRegistry
	}
	return common.BytesToHash(extra[index+len(marker):]), true, nil
}

// ShieldedAssetNullifierBinding makes a token ID and nullifier inseparable in
// the proof-domain transcript. The same nullifier can therefore not be reused
// for a different asset or token ID.
func ShieldedAssetNullifierBinding(chainID uint64, assetID, tokenID, nullifier common.Hash) common.Hash {
	var chain [8]byte
	binary.BigEndian.PutUint64(chain[:], chainID)
	return crypto.Keccak256Hash([]byte("TKM_SHIELDED_ASSET_NULLIFIER_V1"), chain[:], assetID[:], tokenID[:], nullifier[:])
}

func ValidateShieldedAssetNullifierBinding(chainID uint64, assetID, tokenID, nullifier, binding common.Hash) error {
	if chainID == 0 || assetID == (common.Hash{}) || nullifier == (common.Hash{}) || binding == (common.Hash{}) || ShieldedAssetNullifierBinding(chainID, assetID, tokenID, nullifier) != binding {
		return ErrInvalidShieldedAssetBind
	}
	return nil
}

// ProtocolGasVector charges EVM, TVM, proof-verification, and blob resources
// independently. This prevents a cheap EVM operation from consuming a proof
// or TVM budget accidentally.
type ProtocolGasVector struct {
	EVM, TVM, Proof, Blob uint64
}

func (g ProtocolGasVector) Add(other ProtocolGasVector) ProtocolGasVector {
	return ProtocolGasVector{saturatingAdd(g.EVM, other.EVM), saturatingAdd(g.TVM, other.TVM), saturatingAdd(g.Proof, other.Proof), saturatingAdd(g.Blob, other.Blob)}
}

func (g ProtocolGasVector) Fits(limit ProtocolGasVector) bool {
	return g.EVM <= limit.EVM && g.TVM <= limit.TVM && g.Proof <= limit.Proof && g.Blob <= limit.Blob
}

func (g ProtocolGasVector) Charge(limit *ProtocolGasVector, amount ProtocolGasVector) error {
	if limit == nil || g.EVM > ^uint64(0)-amount.EVM || g.TVM > ^uint64(0)-amount.TVM || g.Proof > ^uint64(0)-amount.Proof || g.Blob > ^uint64(0)-amount.Blob || !g.Add(amount).Fits(*limit) {
		return ErrGasDimensionExceeded
	}
	return nil
}

// ConflictTranscript is the deterministic receipt commitment for optimistic
// execution. Waves are canonical transaction indexes; Access is retained so a
// light client can independently recompute the schedule.
type ConflictTranscript struct {
	Waves  [][]uint32
	Access []AccessSet
}

func NewConflictTranscript(access []AccessSet) ConflictTranscript {
	waves := BuildExecutionWaves(access)
	encoded := make([][]uint32, len(waves))
	for i, wave := range waves {
		encoded[i] = make([]uint32, len(wave))
		for j, index := range wave {
			encoded[i][j] = uint32(index)
		}
	}
	return ConflictTranscript{Waves: encoded, Access: append([]AccessSet(nil), access...)}
}

func (t ConflictTranscript) Commitment() (common.Hash, error) {
	if err := t.Validate(); err != nil {
		return common.Hash{}, err
	}
	reads := make([][]common.Address, len(t.Access))
	writes := make([][]common.Address, len(t.Access))
	unknown := make([]bool, len(t.Access))
	for i, set := range t.Access {
		reads[i], writes[i], unknown[i] = set.Reads, set.Writes, set.Unknown
	}
	blob, err := rlp.EncodeToBytes([]interface{}{[]byte("TKM_CONFLICT_TRANSCRIPT_V1"), t.Waves, reads, writes, unknown})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

// Validate ensures the supplied waves are exactly the canonical greedy
// schedule for the supplied access sets. This prevents a producer from
// committing an arbitrary transcript while still making it look deterministic
// to a light client.
func (t ConflictTranscript) Validate() error {
	if len(t.Waves) == 0 || len(t.Access) == 0 {
		return ErrInvalidConflictTranscript
	}
	canonical := NewConflictTranscript(t.Access)
	if !equalWaves(canonical.Waves, t.Waves) {
		return ErrInvalidConflictTranscript
	}
	return nil
}

func (t ConflictTranscript) Verify(access []AccessSet, expected common.Hash) bool {
	canonical := NewConflictTranscript(access)
	if !equalWaves(canonical.Waves, t.Waves) || len(canonical.Access) != len(t.Access) {
		return false
	}
	got, err := t.Commitment()
	return err == nil && got == expected
}

func equalWaves(a, b [][]uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(uint32Bytes(a[i]), uint32Bytes(b[i])) {
			return false
		}
	}
	return true
}

func uint32Bytes(values []uint32) []byte {
	result := make([]byte, 4*len(values))
	for i, value := range values {
		binary.BigEndian.PutUint32(result[i*4:], value)
	}
	return result
}
