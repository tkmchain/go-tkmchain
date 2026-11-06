package core

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

// ShieldedV6BindingRegistration authorizes an opaque key commitment. Consensus
// takes its owner from the signed account's immutable stamp, never from this
// payload. Keys and openings remain private. Each owner registers once.
type ShieldedV6BindingRegistration struct {
	Owner      shielded3.Digest
	Commitment shielded3.Digest
	Proof      []byte
}

func ShieldedV6BindingCount(st shieldedStateReader) uint64 {
	return uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/count", nil)))
}

func ShieldedV6BindingRoot(st shieldedStateReader) (shielded3.Digest, error) {
	if ShieldedV6BindingCount(st) == 0 {
		zeroes, err := v3Zeroes()
		return zeroes[shielded3.MerkleDepth], err
	}
	return v3ReadDigest(st, "shield6-binding/current-root", nil)
}

func shieldedV6BindingNode(st shieldedStateReader, level int, index uint64, zeroes [shielded3.MerkleDepth + 1]shielded3.Digest) (shielded3.Digest, error) {
	key := v3NodeKey(level, index)
	if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/node-present", key)) == (common.Hash{}) {
		return zeroes[level], nil
	}
	return v3ReadDigest(st, "shield6-binding/node", key)
}

func validateShieldedV6BindingRegistration(st shieldedStateReader, from common.Address, commitment shielded3.Digest) (shielded3.Digest, error) {
	stamp, err := AntarticalStampForAddress(st, from)
	if err != nil || !stamp.Registered || IsAddressSuspended(st, from) {
		return shielded3.Digest{}, ErrUnstampedAddress
	}
	if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/owner-index", stamp.Owner.Bytes())) != (common.Hash{}) {
		return shielded3.Digest{}, fmt.Errorf("%w: recipient key binding is immutable and already registered", ErrInvalidShieldedTx)
	}
	if ShieldedV6BindingCount(st) >= uint64(1)<<shielded3.MerkleDepth {
		return shielded3.Digest{}, fmt.Errorf("%w: recipient registry full", ErrInvalidShieldedTx)
	}
	if _, err := shielded3.DigestFromBytes(commitment.Bytes()); err != nil || commitment == (shielded3.Digest{}) {
		return shielded3.Digest{}, ErrInvalidShieldedTx
	}
	return stamp.Owner, nil
}

func processShieldedV6Binding(st *state.StateDB, tx *types.Transaction, from common.Address, e *ShieldedV6Transaction) error {
	owner, err := validateShieldedV6BindingRegistration(st, from, e.Binding.Commitment)
	if err != nil {
		return err
	}
	if owner != e.Binding.Owner {
		return fmt.Errorf("%w: key-control owner does not match the stamped account", ErrInvalidShieldedTx)
	}
	current, err := recipient.AuthorizedBindingLeaf(tx.ChainId().Uint64(), owner, e.Binding.Commitment)
	if err != nil {
		return err
	}
	zeroes, err := v3Zeroes()
	if err != nil {
		return err
	}
	index := ShieldedV6BindingCount(st)
	cursor := index
	snapshot := st.Snapshot()
	for level := 0; level <= shielded3.MerkleDepth; level++ {
		key := v3NodeKey(level, cursor)
		v3WriteDigest(st, "shield6-binding/node", key, current)
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/node-present", key), uint64Hash(1))
		if level == shielded3.MerkleDepth {
			break
		}
		sibling, readErr := shieldedV6BindingNode(st, level, cursor^1, zeroes)
		if readErr != nil {
			st.RevertToSnapshot(snapshot)
			return readErr
		}
		if cursor&1 == 0 {
			current, err = shielded3.HashPair(current, sibling)
		} else {
			current, err = shielded3.HashPair(sibling, current)
		}
		if err != nil {
			st.RevertToSnapshot(snapshot)
			return err
		}
		cursor >>= 1
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/owner-index", owner.Bytes()), uint64Hash(index+1))
	v3WriteDigest(st, "shield6-binding/commitment", owner.Bytes(), e.Binding.Commitment)
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/count", nil), uint64Hash(index+1))
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/root", current.Bytes()), tx.Hash())
	v3WriteDigest(st, "shield6-binding/current-root", nil, current)
	return nil
}

// ShieldedV6BindingPath returns a canonical path to an authorized registry leaf.
// Wallets must match Commitment to their locally computed binding commitment.
func ShieldedV6BindingPath(st shieldedStateReader, owner shielded3.Digest) (ShieldedV3Path, error) {
	var result ShieldedV3Path
	index := uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/owner-index", owner.Bytes())))
	if index == 0 {
		return result, nil
	}
	if index > ShieldedV6BindingCount(st) {
		return result, ErrInvalidShieldedTx
	}
	result.Found, result.Index = true, index-1
	var err error
	result.Commitment, err = v3ReadDigest(st, "shield6-binding/commitment", owner.Bytes())
	if err != nil {
		return result, err
	}
	zeroes, err := v3Zeroes()
	if err != nil {
		return result, err
	}
	cursor := result.Index
	for level := range result.Path {
		result.Path[level], err = shieldedV6BindingNode(st, level, cursor^1, zeroes)
		if err != nil {
			return result, err
		}
		cursor >>= 1
	}
	result.Root, err = ShieldedV6BindingRoot(st)
	return result, err
}

// ShieldedV6RecipientAnchor commits to two canonical, append-only registry
// snapshots. Known historical roots remain usable while proofs are constructed;
// reorgs remove the root markers with all other journaled registry writes.
func ShieldedV6RecipientAnchor(chainID uint64, stampRoot, bindingRoot shielded3.Digest) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELD6_REGISTRY_ANCHOR_V2"), shieldedV6IndexKey(chainID), stampRoot.Bytes(), bindingRoot.Bytes())
}

func validateShieldedV6RecipientRoots(st shieldedStateReader, tx *types.Transaction, e *ShieldedV6Transaction) error {
	if e.Recipients == nil {
		return nil
	}
	if len(e.Recipients.Roots) != len(e.Outputs) {
		return ErrInvalidShieldedTx
	}
	for _, r := range e.Recipients.Roots {
		if tx.ChainId() == nil || !tx.ChainId().IsUint64() || r.Anchor != ShieldedV6RecipientAnchor(tx.ChainId().Uint64(), r.StampRoot, r.BindingRoot) ||
			st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/root", r.StampRoot.Bytes())) == (common.Hash{}) ||
			st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("shield6-binding/root", r.BindingRoot.Bytes())) == (common.Hash{}) {
			return fmt.Errorf("%w: unknown or reorganized recipient registry anchor", ErrInvalidShieldedTx)
		}
	}
	return nil
}
