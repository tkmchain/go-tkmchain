package core

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

const (
	ShieldedV6BoundVersion              = 7
	ShieldedV6BindingVersion            = 8
	ShieldedV6MaxRecipientProof         = recipient.MaxProofBytes
	ShieldedV6RecipientVerifyGas uint64 = 250_000
)

// ValidateShieldedV6Time is shared with canonical-head txpool pruning. The
// historical experimental V6 encoding has no deadline; new envelopes do.
func ValidateShieldedV6Time(e *ShieldedV6Transaction, time uint64) error {
	if e == nil {
		return ErrInvalidShieldedTx
	}
	if e.Version == 6 {
		if e.ValidUntil != 0 {
			return ErrInvalidShieldedTx
		}
		return nil
	}
	if (e.Version != ShieldedV6BoundVersion && e.Version != ShieldedV6BindingVersion) || e.ValidUntil <= time || e.ValidUntil-time > ShieldedV6MaxLifetime {
		return fmt.Errorf("%w: recipient envelope expired or exceeds one hour", ErrInvalidShieldedTx)
	}
	return nil
}

type ShieldedV6RecipientRoots struct {
	StampRoot   shielded3.Digest `json:"stampRoot"`
	BindingRoot shielded3.Digest `json:"bindingRoot"`
	Anchor      common.Hash      `json:"anchor"`
}

type ShieldedV6RecipientProofs struct {
	Roots  []ShieldedV6RecipientRoots
	Proofs [][]byte
}

func validShieldedV6BindingShape(e *ShieldedV6Transaction) bool {
	return e != nil && e.Version == ShieldedV6BindingVersion && e.AssetID == 1 && e.Binding != nil && e.Recipients == nil && !e.Deposit && e.DepositMask == ([32]byte{}) &&
		e.WithdrawalRecipient == (common.Address{}) && (e.WithdrawalValue == nil || e.WithdrawalValue.Sign() == 0) && len(e.Inputs) == 0 && len(e.Outputs) == 0 && len(e.RangeProof) == 0
}

func validateShieldedV6BindingBasics(tx *types.Transaction, e *ShieldedV6Transaction) (*ShieldedV6Transaction, error) {
	if !validShieldedV6BindingShape(e) || e.ValidUntil == 0 || tx == nil || tx.Value() == nil || tx.Value().Sign() != 0 || tx.ChainId() == nil || !tx.ChainId().IsUint64() || tx.ChainId().Sign() == 0 {
		return nil, ErrInvalidShieldedTx
	}
	if _, err := shielded3.DigestFromBytes(e.Binding.Commitment.Bytes()); err != nil || e.Binding.Commitment == (shielded3.Digest{}) {
		return nil, ErrInvalidShieldedTx
	}
	if _, err := shielded3.DigestFromBytes(e.Binding.Owner.Bytes()); err != nil || e.Binding.Owner == (shielded3.Digest{}) || len(e.Binding.Proof) == 0 || len(e.Binding.Proof) > ShieldedV6MaxRecipientProof {
		return nil, ErrInvalidShieldedTx
	}
	return e, nil
}

type shieldedV6VerifierCache struct {
	sync.Mutex
	pin      common.Hash
	verifier *recipient.Verifier
}

var shieldedV6RecipientVerifierCache, shieldedV6RegistrationVerifierCache shieldedV6VerifierCache

func shieldedV6RecipientVerifier(config *params.ChainConfig) (*recipient.Verifier, error) {
	if config == nil {
		return nil, ErrInvalidShieldedTx
	}
	return shieldedV6PinnedVerifier(config.Shield6RecipientKey, config.Shield6RecipientKeyHash, &shieldedV6RecipientVerifierCache)
}
func shieldedV6RegistrationVerifier(config *params.ChainConfig) (*recipient.Verifier, error) {
	if config == nil {
		return nil, ErrInvalidShieldedTx
	}
	return shieldedV6PinnedVerifier(config.Shield6RegistrationKey, config.Shield6RegistrationKeyHash, &shieldedV6RegistrationVerifierCache)
}
func shieldedV6PinnedVerifier(encoded []byte, pin common.Hash, cache *shieldedV6VerifierCache) (*recipient.Verifier, error) {
	if len(encoded) == 0 || len(encoded) > recipient.MaxVerifyingKeyBytes || pin == (common.Hash{}) || common.Hash(sha256.Sum256(encoded)) != pin {
		return nil, fmt.Errorf("%w: recipient verification key unavailable or unpinned", ErrInvalidShieldedTx)
	}
	cache.Lock()
	defer cache.Unlock()
	if cache.verifier != nil && cache.pin == pin {
		return cache.verifier, nil
	}
	v, err := recipient.NewVerifier(encoded, [32]byte(pin))
	if err != nil {
		return nil, err
	}
	cache.pin, cache.verifier = pin, v
	return v, nil
}

func ShieldedV6RegistrationStatement(tx *types.Transaction, e *ShieldedV6Transaction) (recipient.RegistrationStatement, error) {
	if tx == nil || e == nil || e.Binding == nil || tx.ChainId() == nil || !tx.ChainId().IsUint64() {
		return recipient.RegistrationStatement{}, ErrInvalidShieldedTx
	}
	intent, err := ShieldedV6Intent(tx, e)
	if err != nil {
		return recipient.RegistrationStatement{}, err
	}
	return recipient.RegistrationStatement{ChainID: tx.ChainId().Uint64(), Owner: e.Binding.Owner, Commitment: e.Binding.Commitment, Intent: intent}, nil
}
func ValidateShieldedV6RegistrationProof(config *params.ChainConfig, tx *types.Transaction, e *ShieldedV6Transaction) error {
	if _, err := validateShieldedV6BindingBasics(tx, e); err != nil {
		return err
	}
	verifier, err := shieldedV6RegistrationVerifier(config)
	if err != nil {
		return err
	}
	statement, err := ShieldedV6RegistrationStatement(tx, e)
	if err != nil {
		return err
	}
	if err := verifier.VerifyRegistration(statement, e.Binding.Proof); err != nil {
		return fmt.Errorf("%w: recipient key-control proof", ErrInvalidShieldedTx)
	}
	return nil
}

// ShieldedV6RecipientStatement constructs facts from the actual signed envelope.
// Callers never substitute a digest, ciphertext hash or output supplied by RPC.
func ShieldedV6RecipientStatement(tx *types.Transaction, e *ShieldedV6Transaction, index int) (recipient.Statement, error) {
	if tx == nil || e == nil || e.Recipients == nil || tx.ChainId() == nil || !tx.ChainId().IsUint64() || index < 0 || index >= len(e.Outputs) || len(e.Recipients.Roots) != len(e.Outputs) {
		return recipient.Statement{}, ErrInvalidShieldedTx
	}
	intent, err := ShieldedV6Intent(tx, e)
	if err != nil {
		return recipient.Statement{}, err
	}
	r := e.Recipients.Roots[index]
	return recipient.Statement{ChainID: tx.ChainId().Uint64(), AssetID: e.AssetID, OutputPosition: uint64(index), StampRoot: r.StampRoot, BindingRoot: r.BindingRoot, StateAnchor: [32]byte(r.Anchor), Intent: intent, Output: ShieldedV6WalletRecord(e.Outputs[index])}, nil
}

func ValidateShieldedV6RecipientProof(config *params.ChainConfig, tx *types.Transaction, e *ShieldedV6Transaction) error {
	if e == nil || e.Version != ShieldedV6BoundVersion || e.Recipients == nil || len(e.Recipients.Proofs) != len(e.Outputs) || len(e.Recipients.Roots) != len(e.Outputs) {
		return ErrInvalidShieldedTx
	}
	// Even full public withdrawals use the new version and authenticated pin;
	// roots/proofs are empty only when there are no confidential outputs.
	v, err := shieldedV6RecipientVerifier(config)
	if err != nil {
		return err
	}

	for i, proof := range e.Recipients.Proofs {
		if len(proof) == 0 || len(proof) > ShieldedV6MaxRecipientProof {
			return ErrInvalidShieldedTx
		}
		s, err := ShieldedV6RecipientStatement(tx, e, i)
		if err != nil {
			return err
		}
		if err := v.Verify(s, proof); err != nil {
			return fmt.Errorf("%w: recipient proof %d", ErrInvalidShieldedTx, i)
		}
	}
	return nil
}
