package core

import (
	"context"
	"crypto/sha512"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded4"
	"github.com/holiman/uint256"
)

// Shield4-only replay state has its own namespace. The commitment tree and
// legacy nullifier namespace remain shared with Shield3, which is what makes
// the membership set genuinely full-chain across the Antartical transition.
func ShieldedV4StateSlot(role string, data []byte) common.Hash {
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD4_STATE_V1/" + role + "/"))
	h.Write(data)
	return common.BytesToHash(h.Sum(nil))
}

func ShieldedV4LinkTagTransaction(st shieldedStateReader, tag shielded4.Digest) common.Hash {
	return st.GetState(params.ShieldedPoolAddress, ShieldedV4StateSlot("link-tag", tag.Bytes()))
}

func validateShieldedV4State(st *state.StateDB, tx *types.Transaction, e *ShieldedV4Transaction) error {
	if st.GetCodeSize(params.ShieldedPoolAddress) != 0 {
		return fmt.Errorf("%w: Shield4 pool must not contain executable code", ErrInvalidShieldedTx)
	}
	if e.StampRoot == (shielded4.Digest{}) || st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/root", e.StampRoot.Bytes())) == (common.Hash{}) {
		return fmt.Errorf("%w: unknown stamp registry root", ErrInvalidShieldedTx)
	}
	if e.WithdrawalValue.Sign() > 0 && !IsAntarticalStamped(st, e.WithdrawalRecipient) {
		return ErrUnstampedAddress
	}
	nullifiers, err := ShieldedV4Nullifiers(e)
	if err != nil {
		return err
	}
	if !e.Deposit {
		for _, n := range nullifiers {
			if ShieldedV3NullifierTransaction(st, n) != (common.Hash{}) {
				return fmt.Errorf("%w: Shield4 nullifier already spent", ErrInvalidShieldedTx)
			}
		}
		if ShieldedV4LinkTagTransaction(st, e.LinkTag) != (common.Hash{}) {
			return fmt.Errorf("%w: Shield4 linkability tag already spent", ErrInvalidShieldedTx)
		}
		if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("root", e.Anchor.Bytes())) == (common.Hash{}) {
			return fmt.Errorf("%w: unknown full-chain note root", ErrInvalidShieldedTx)
		}
	}
	for _, out := range e.Outputs {
		if st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("commitment", out.Commitment.Bytes())) != (common.Hash{}) {
			return fmt.Errorf("%w: Shield4 commitment already exists", ErrInvalidShieldedTx)
		}
	}
	release := new(big.Int).Add(e.WithdrawalValue, e.GasSponsorValue)
	if release.BitLen() > 256 || st.GetBalance(params.ShieldedPoolAddress).Cmp(uint256.MustFromBig(release)) < 0 {
		return fmt.Errorf("%w: insufficient shielded pool reserve", ErrInvalidShieldedTx)
	}
	if ShieldedV3NextIndex(st) >= (uint64(1)<<shielded4.MerkleDepth)-3 {
		return fmt.Errorf("%w: full-chain tree is full", ErrInvalidShieldedTx)
	}
	return nil
}

func processShieldedV4(config *params.ChainConfig, number *big.Int, time uint64, st *state.StateDB, tx *types.Transaction, seen map[common.Hash]struct{}) error {
	e, err := shieldedV4Basics(config, number, time, tx)
	if err != nil {
		return err
	}
	if err = validateShieldedV4State(st, tx, e); err != nil {
		return err
	}
	nullifiers, err := ShieldedV4Nullifiers(e)
	if err != nil {
		return err
	}
	for _, n := range nullifiers {
		if _, ok := seen[ShieldedV3StateSlot("block-nullifier", n.Bytes())]; ok {
			return fmt.Errorf("%w: duplicate Shield4 nullifier in block", ErrInvalidShieldedTx)
		}
	}
	linkKey := ShieldedV4StateSlot("block-link-tag", e.LinkTag.Bytes())
	if _, ok := seen[linkKey]; ok {
		return fmt.Errorf("%w: duplicate Shield4 linkability tag in block", ErrInvalidShieldedTx)
	}
	statement, err := ShieldedV4Statement(tx, e)
	if err != nil {
		return err
	}
	if err = (shielded4.NativeBackend{}).Verify(context.Background(), statement, e.Proof); err != nil {
		return fmt.Errorf("%w: Shield4 proof: %w", ErrInvalidShieldedTx, err)
	}
	sender, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	snapshot := st.Snapshot()
	for _, out := range e.Outputs {
		if err = appendShieldedV3Leaf(st, out.Commitment, tx.Hash()); err != nil {
			st.RevertToSnapshot(snapshot)
			return err
		}
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("commitment", out.Commitment.Bytes()), tx.Hash())
	}
	for _, n := range nullifiers {
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("nullifier", n.Bytes()), tx.Hash())
		seen[ShieldedV3StateSlot("block-nullifier", n.Bytes())] = struct{}{}
	}
	if !e.Deposit {
		st.SetState(params.ShieldedPoolAddress, ShieldedV4StateSlot("link-tag", e.LinkTag.Bytes()), tx.Hash())
		seen[linkKey] = struct{}{}
	}
	for _, release := range []struct {
		to    common.Address
		value *big.Int
	}{{e.WithdrawalRecipient, e.WithdrawalValue}, {sender, e.GasSponsorValue}} {
		if release.value.Sign() > 0 {
			amount := uint256.MustFromBig(release.value)
			st.SubBalance(params.ShieldedPoolAddress, amount, tracing.BalanceChangeTransfer)
			st.AddBalance(release.to, amount, tracing.BalanceChangeTransfer)
		}
	}
	return nil
}
