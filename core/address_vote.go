package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// Address votes are an Antartical governance envelope. Votes are deliberately
// separate from Shield3 notes: the target and reason are public so an
// investigation can audit the basis for a suspension.
const (
	AddressVoteMagic     = "TKMVOTE1"
	AddressVoteVersion   = uint64(1)
	AddressVoteThreshold = uint64(15)
	AddressVoteMaxReason = 160
)

var (
	ErrAddressVoteAlreadyCast = errors.New("address vote already cast by this stamp owner")
	ErrAddressVoteNotFound    = errors.New("address vote not found for this stamp owner")
	ErrAddressVoteSuspended   = errors.New("address already has the address-vote threshold")
	ErrAddressSuspended       = errors.New("illegal transaction: address is suspended by address votes")
)

// AddressVote is the RLP payload after AddressVoteMagic. Unvote removes only
// the caller's own vote; no account can erase another voter's record.
type AddressVote struct {
	Version uint64
	Unvote  bool
	Target  common.Address
	Reason  string `rlp:"optional"`
}

// AddressVoteStatus is the consensus-derived status exposed by the governance
// RPC. Reasons remain in the signed vote transaction and are not hidden state.
type AddressVoteStatus struct {
	Address     common.Address `json:"address"`
	ActiveVotes uint64         `json:"activeVotes"`
	Threshold   uint64         `json:"threshold"`
	Suspended   bool           `json:"suspended"`
}

func HasAddressVotePrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(AddressVoteMagic))
}

func EncodeAddressVote(vote *AddressVote) ([]byte, error) {
	if vote == nil {
		return nil, ErrInvalidShieldedTx
	}
	payload, err := rlp.EncodeToBytes(vote)
	if err != nil {
		return nil, err
	}
	return append([]byte(AddressVoteMagic), payload...), nil
}

func DecodeAddressVote(data []byte) (*AddressVote, error) {
	if !HasAddressVotePrefix(data) || uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, ErrInvalidShieldedTx
	}
	var vote AddressVote
	if err := rlp.DecodeBytes(data[len(AddressVoteMagic):], &vote); err != nil {
		return nil, fmt.Errorf("invalid address vote: %w", err)
	}
	return &vote, nil
}

func validateAddressVotePayload(vote *AddressVote) error {
	if vote == nil || vote.Version != AddressVoteVersion || vote.Target == (common.Address{}) || vote.Target == params.ShieldedPoolAddress {
		return errors.New("invalid address vote target or version")
	}
	if vote.Unvote {
		if vote.Reason != "" {
			return errors.New("unvote must not include a reason")
		}
	} else if vote.Reason == "" || len(vote.Reason) > AddressVoteMaxReason || !utf8.ValidString(vote.Reason) {
		return fmt.Errorf("vote reason must be valid UTF-8 and between 1 and %d bytes", AddressVoteMaxReason)
	}
	return nil
}

// ValidateAddressVoteBasics validates fields that do not depend on state.
func ValidateAddressVoteBasics(config *params.ChainConfig, number *big.Int, blockTime uint64, tx *types.Transaction) error {
	if config == nil || config.ChainID == nil || !config.IsAntartical(number, blockTime) {
		return errors.New("address voting is not active until Antartical")
	}
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 || tx.ChainId() == nil || tx.ChainId().Cmp(config.ChainID) != 0 {
		return errors.New("address vote requires a zero-value PQ transaction to the reserved pool")
	}
	vote, err := DecodeAddressVote(tx.Data())
	if err != nil {
		return err
	}
	if err := validateAddressVotePayload(vote); err != nil {
		return err
	}
	return nil
}

func addressVoteOwner(st shieldedStateReader, voter common.Address) ([]byte, error) {
	if !IsAntarticalStamped(st, voter) {
		return nil, fmt.Errorf("%w: voter %s", ErrUnstampedAddress, voter)
	}
	owner, err := v3ReadDigest(st, "stamp/address-owner", voter.Bytes())
	if err != nil {
		return nil, fmt.Errorf("read voter stamp owner: %w", err)
	}
	if owner == (shielded3.Digest{}) {
		return nil, fmt.Errorf("%w: voter %s", ErrUnstampedAddress, voter)
	}
	return owner.Bytes(), nil
}

func addressVoteKey(target common.Address, owner []byte) []byte {
	key := make([]byte, 0, common.AddressLength+len(owner))
	key = append(key, target.Bytes()...)
	return append(key, owner...)
}

func addressVoteActiveSlot(target common.Address, owner []byte) common.Hash {
	return ShieldedV3StateSlot("governance/address-vote/active", addressVoteKey(target, owner))
}

func addressVoteReasonSlot(target common.Address, owner []byte) common.Hash {
	return ShieldedV3StateSlot("governance/address-vote/reason", addressVoteKey(target, owner))
}

func addressVoteCountSlot(target common.Address) common.Hash {
	return ShieldedV3StateSlot("governance/address-vote/count", target.Bytes())
}

func addressVoteSuspendedSlot(target common.Address) common.Hash {
	return ShieldedV3StateSlot("governance/address-vote/suspended", target.Bytes())
}

func IsAddressSuspended(st shieldedStateReader, address common.Address) bool {
	return st.GetState(params.ShieldedPoolAddress, addressVoteSuspendedSlot(address)) != (common.Hash{})
}

func AddressVoteCount(st shieldedStateReader, address common.Address) uint64 {
	return uint64FromHash(st.GetState(params.ShieldedPoolAddress, addressVoteCountSlot(address)))
}

func GetAddressVoteStatus(st shieldedStateReader, address common.Address) AddressVoteStatus {
	return AddressVoteStatus{Address: address, ActiveVotes: AddressVoteCount(st, address), Threshold: AddressVoteThreshold, Suspended: IsAddressSuspended(st, address)}
}

// ValidateAddressVoteState applies the unique-voter and suspension rules.
// Identity is the registered stamp owner, rather than the 20-byte address, so
// one owner cannot multiply its influence by registering several addresses.
func ValidateAddressVoteState(st shieldedStateReader, from common.Address, data []byte) error {
	vote, err := DecodeAddressVote(data)
	if err != nil {
		return err
	}
	if err := validateAddressVotePayload(vote); err != nil {
		return err
	}
	if vote.Target == from {
		return errors.New("an address cannot vote on itself")
	}
	owner, err := addressVoteOwner(st, from)
	if err != nil {
		return err
	}
	active := st.GetState(params.ShieldedPoolAddress, addressVoteActiveSlot(vote.Target, owner))
	count := AddressVoteCount(st, vote.Target)
	if vote.Unvote {
		if active == (common.Hash{}) {
			return ErrAddressVoteNotFound
		}
		return nil
	}
	if active != (common.Hash{}) {
		return ErrAddressVoteAlreadyCast
	}
	if count >= AddressVoteThreshold || IsAddressSuspended(st, vote.Target) {
		return ErrAddressVoteSuspended
	}
	return nil
}

// ProcessAddressVote applies a validated vote after the enclosing transaction
// has executed successfully. The transaction hash records the active vote and
// suspension event for auditability, while reason is committed as a digest.
func ProcessAddressVote(st *state.StateDB, from common.Address, data []byte, txHash common.Hash) error {
	vote, err := DecodeAddressVote(data)
	if err != nil {
		return err
	}
	if err := ValidateAddressVoteState(st, from, data); err != nil {
		return err
	}
	owner, err := addressVoteOwner(st, from)
	if err != nil {
		return err
	}
	activeSlot := addressVoteActiveSlot(vote.Target, owner)
	reasonSlot := addressVoteReasonSlot(vote.Target, owner)
	count := AddressVoteCount(st, vote.Target)
	if vote.Unvote {
		if count == 0 {
			return errors.New("address vote count underflow")
		}
		st.SetState(params.ShieldedPoolAddress, activeSlot, common.Hash{})
		st.SetState(params.ShieldedPoolAddress, reasonSlot, common.Hash{})
		count--
		st.SetState(params.ShieldedPoolAddress, addressVoteCountSlot(vote.Target), uint64Hash(count))
		if count < AddressVoteThreshold {
			st.SetState(params.ShieldedPoolAddress, addressVoteSuspendedSlot(vote.Target), common.Hash{})
		}
		return nil
	}
	if count >= AddressVoteThreshold {
		return ErrAddressVoteSuspended
	}
	st.SetState(params.ShieldedPoolAddress, activeSlot, txHash)
	st.SetState(params.ShieldedPoolAddress, reasonSlot, crypto.Keccak256Hash([]byte(vote.Reason)))
	count++
	st.SetState(params.ShieldedPoolAddress, addressVoteCountSlot(vote.Target), uint64Hash(count))
	if count >= AddressVoteThreshold {
		st.SetState(params.ShieldedPoolAddress, addressVoteSuspendedSlot(vote.Target), txHash)
	}
	return nil
}
