package state

import (
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

// ErrSpeculativeDelta is returned when a speculative result cannot be safely
// merged into the canonical StateDB. Callers must discard the speculative
// result and execute the transaction through the serial path.
var ErrSpeculativeDelta = errors.New("speculative state delta cannot be committed")

// SpeculativeSlot is one raw storage key mutation. Keys are deliberately kept
// in their pre-hash form so the canonical StateDB can apply them through
// SetState for both MPT and UBT backends.
type SpeculativeSlot struct {
	Key    common.Hash
	Origin common.Hash
	Value  common.Hash
}

// SpeculativeAccount is the write-set of one account. Origin is the account
// value observed by the speculative execution and Current contains the
// resulting account metadata. Storage contains only slots touched by the
// execution. Code is non-nil only when the transaction deployed or replaced
// contract code.
type SpeculativeAccount struct {
	Address common.Address
	Origin  *types.StateAccount
	Current *types.StateAccount
	Slots   []SpeculativeSlot
	Code    []byte
}

// SpeculativeDelta is an isolated transaction write-set. It intentionally
// excludes self-destructs, account deletion and other block-level effects;
// those transactions are required to use the serial executor.
type SpeculativeDelta struct {
	Accounts []SpeculativeAccount
}

func copyAccount(account *types.StateAccount) *types.StateAccount {
	if account == nil {
		return nil
	}
	return account.Copy()
}

// BuildSpeculativeDelta exports the mutations already present in this state.
// ApplyMessage finalises a transaction before returning, so pendingStorage is
// the complete raw-key write-set and can be merged without committing trie
// nodes from the speculative database.
func (s *StateDB) BuildSpeculativeDelta() (*SpeculativeDelta, error) {
	if s == nil || s.dbErr != nil {
		return nil, ErrSpeculativeDelta
	}
	// ApplyMessage normally finalises the transaction before this method is
	// called. Finalising here as well makes the API safe for callers that build a
	// delta directly after using StateDB setters.
	s.Finalise(true)
	delta := &SpeculativeDelta{Accounts: make([]SpeculativeAccount, 0, len(s.mutations))}
	for addr, mutation := range s.mutations {
		if mutation == nil || mutation.isDelete() {
			return nil, ErrSpeculativeDelta
		}
		obj := s.stateObjects[addr]
		if obj == nil || obj.selfDestructed || obj.origin == nil && obj.data.Root != types.EmptyRootHash {
			return nil, ErrSpeculativeDelta
		}
		account := SpeculativeAccount{
			Address: addr,
			Origin:  copyAccount(obj.origin),
			Current: obj.data.Copy(),
		}
		if obj.dirtyCode {
			account.Code = common.CopyBytes(obj.code)
		}
		account.Slots = make([]SpeculativeSlot, 0, len(obj.pendingStorage))
		for key, value := range obj.pendingStorage {
			origin, ok := obj.originStorage[key]
			if !ok {
				// SetState normally loads the origin before adding a pending
				// value. Refuse an incomplete delta instead of guessing.
				return nil, ErrSpeculativeDelta
			}
			account.Slots = append(account.Slots, SpeculativeSlot{Key: key, Origin: origin, Value: value})
		}
		sort.Slice(account.Slots, func(i, j int) bool { return account.Slots[i].Key.Hex() < account.Slots[j].Key.Hex() })
		delta.Accounts = append(delta.Accounts, account)
	}
	sort.Slice(delta.Accounts, func(i, j int) bool { return delta.Accounts[i].Address.Hex() < delta.Accounts[j].Address.Hex() })
	return delta, nil
}

func accountsEqual(a, b *types.StateAccount) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Nonce != b.Nonce || a.Root != b.Root || string(a.CodeHash) != string(b.CodeHash) {
		return false
	}
	if a.Balance == nil || b.Balance == nil {
		return a.Balance == b.Balance
	}
	return a.Balance.Cmp(b.Balance) == 0
}

// CanApplySpeculativeDelta checks every pre-state value without mutating the
// canonical state. It is used as a transactional validation phase before any
// write-set is applied.
func (s *StateDB) CanApplySpeculativeDelta(delta *SpeculativeDelta) error {
	if s == nil || delta == nil {
		return ErrSpeculativeDelta
	}
	for _, account := range delta.Accounts {
		obj := s.getStateObject(account.Address)
		var current *types.StateAccount
		if obj != nil {
			current = obj.data.Copy()
		}
		if !accountsEqual(current, account.Origin) {
			return ErrSpeculativeDelta
		}
		for _, slot := range account.Slots {
			if s.GetState(account.Address, slot.Key) != slot.Origin {
				return ErrSpeculativeDelta
			}
		}
	}
	return nil
}

// ApplySpeculativeDelta applies a previously validated write-set. The caller
// must invoke CanApplySpeculativeDelta for every delta first and apply them in
// canonical transaction order. Account roots are recalculated by the normal
// IntermediateRoot path after application.
func (s *StateDB) ApplySpeculativeDelta(delta *SpeculativeDelta) error {
	if err := s.CanApplySpeculativeDelta(delta); err != nil {
		return err
	}
	for _, account := range delta.Accounts {
		if account.Origin == nil {
			s.CreateAccount(account.Address)
		}
		if account.Current == nil || account.Current.Balance == nil {
			return ErrSpeculativeDelta
		}
		s.SetBalance(account.Address, new(uint256.Int).Set(account.Current.Balance), tracing.BalanceChangeUnspecified)
		s.SetNonce(account.Address, account.Current.Nonce, tracing.NonceChangeUnspecified)
		for _, slot := range account.Slots {
			s.SetState(account.Address, slot.Key, slot.Value)
		}
		if account.Code != nil {
			s.SetCode(account.Address, account.Code, tracing.CodeChangeUnspecified)
		}
	}
	return nil
}
