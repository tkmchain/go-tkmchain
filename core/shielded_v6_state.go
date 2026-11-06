package core

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/holiman/uint256"
)

func ShieldedV6StateSlot(role string, key []byte) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELD6_STATE_V1/"+role+"/"), key)
}

func ShieldedV6OutputCount(st shieldedStateReader) uint64 {
	return uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-count", nil)))
}

// ShieldedV6KeyImageCount returns the number of canonical Shield6 spends.
// The sequential index lets wallets download the already-public key images
// and match them locally, instead of disclosing wallet-owned images to RPC.
func ShieldedV6KeyImageCount(st shieldedStateReader) uint64 {
	return uint64FromHash(st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-image-count", nil)))
}

// ShieldedV6KeyImageAt returns a canonical spent key image by public index.
func ShieldedV6KeyImageAt(st shieldedStateReader, index uint64) ([shielded6.PointSize]byte, error) {
	var image [shielded6.PointSize]byte
	if st == nil || index >= ShieldedV6KeyImageCount(st) {
		return image, ErrInvalidShieldedTx
	}
	stored := st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-image-index", shieldedV6IndexKey(index)))
	copy(image[:], stored[:])
	if shielded6.ValidatePublicKey(image[:]) != nil {
		return [shielded6.PointSize]byte{}, fmt.Errorf("%w: corrupt Shield6 key-image index %d", ErrInvalidShieldedTx, index)
	}
	return image, nil
}

func ShieldedV6OutputRoot(st shieldedStateReader) common.Hash {
	root := st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-root", nil))
	if root == (common.Hash{}) && ShieldedV6OutputCount(st) == 0 {
		return shieldedV6EmptyNodes()[ShieldedV6OutputTreeDepth]
	}
	return root
}

func shieldedV6IndexKey(index uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], index)
	return encoded[:]
}

func shieldedV6ReadOutput(st shieldedStateReader, index uint64) (shielded6.RingMember, error) {
	var member shielded6.RingMember
	if index >= ShieldedV6OutputCount(st) {
		return member, ErrInvalidShieldedTx
	}
	key := shieldedV6IndexKey(index)
	outputKey := st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-key", key))
	outputCommitment := st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-commitment", key))
	copy(member.OneTimeKey[:], outputKey[:])
	copy(member.Commitment[:], outputCommitment[:])
	if member.OneTimeKey == ([shielded6.PointSize]byte{}) || member.Commitment == ([shielded6.PointSize]byte{}) {
		return shielded6.RingMember{}, ErrInvalidShieldedTx
	}
	return member, nil
}

// ShieldedV6OutputAt returns the public key and commitment at a canonical
// accumulator position for wallet decoy construction. It does not disclose
// note amounts, decrypted records, or spend metadata.
func ShieldedV6OutputAt(st shieldedStateReader, index uint64) (shielded6.RingMember, error) {
	return shieldedV6ReadOutput(st, index)
}

// ShieldedV6OutputTransactionAt returns the canonical transaction hash that
// created an accumulator output. Wallet scanners can use the canonical tx
// lookup to retrieve its encrypted note payload without duplicating ciphertext
// in state.
func ShieldedV6OutputTransactionAt(st shieldedStateReader, index uint64) (common.Hash, error) {
	if st == nil || index >= ShieldedV6OutputCount(st) {
		return common.Hash{}, ErrInvalidShieldedTx
	}
	txHash := st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-tx", shieldedV6IndexKey(index)))
	if txHash == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("%w: missing Shield6 output transaction index", ErrInvalidShieldedTx)
	}
	return txHash, nil
}

// ShieldedV6KeyImageTransaction returns the canonical spend transaction for
// an already spent Shield6 key image, or the zero hash when unspent.
func ShieldedV6KeyImageTransaction(st shieldedStateReader, keyImage [shielded6.PointSize]byte) common.Hash {
	if st == nil || shielded6.ValidatePublicKey(keyImage[:]) != nil {
		return common.Hash{}
	}
	return st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-image", keyImage[:]))
}

func shieldedV6HashNode(left, right common.Hash) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_SHIELD6_ACCUMULATOR_NODE_V1"), left[:], right[:])
}

var shieldedV6Zeroes struct {
	once   sync.Once
	values [ShieldedV6OutputTreeDepth + 1]common.Hash
}

func shieldedV6EmptyNodes() [ShieldedV6OutputTreeDepth + 1]common.Hash {
	shieldedV6Zeroes.once.Do(func() {
		shieldedV6Zeroes.values[0] = crypto.Keccak256Hash([]byte("TKM_SHIELD6_ACCUMULATOR_EMPTY_LEAF_V1"))
		for level := 0; level < ShieldedV6OutputTreeDepth; level++ {
			shieldedV6Zeroes.values[level+1] = shieldedV6HashNode(shieldedV6Zeroes.values[level], shieldedV6Zeroes.values[level])
		}
	})
	return shieldedV6Zeroes.values
}

func shieldedV6AppendOutput(st shieldedStateWriter, output ShieldedV6Output, txHash common.Hash) error {
	if err := shielded6.ValidatePublicKey(output.OneTimeKey[:]); err != nil {
		return fmt.Errorf("%w: invalid Shield6 one-time key", ErrInvalidShieldedTx)
	}
	if err := shielded6.ValidateCommitmentEncoding(output.Commitment[:]); err != nil {
		return fmt.Errorf("%w: invalid Shield6 amount commitment", ErrInvalidShieldedTx)
	}
	count := ShieldedV6OutputCount(st)
	if count >= uint64(1)<<ShieldedV6OutputTreeDepth {
		return fmt.Errorf("%w: Shield6 output accumulator is full", ErrInvalidShieldedTx)
	}
	if st.GetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-index", output.OneTimeKey[:])) != (common.Hash{}) {
		return fmt.Errorf("%w: duplicate Shield6 one-time output key", ErrInvalidShieldedTx)
	}
	// Validate every frontier node that this append will consume before writing
	// any state. The normal block path reverts failed transactions, but keeping
	// this helper atomic also protects callers doing state rechecks and tests.
	for level := 0; level < ShieldedV6OutputTreeDepth; level++ {
		if count&(uint64(1)<<level) == 0 {
			continue
		}
		var levelBytes [4]byte
		binary.BigEndian.PutUint32(levelBytes[:], uint32(level))
		frontierSlot := ShieldedV6StateSlot("accumulator-frontier", levelBytes[:])
		if st.GetState(params.ShieldedPoolAddress, frontierSlot) == (common.Hash{}) {
			return fmt.Errorf("%w: missing Shield6 accumulator frontier node at level %d", ErrInvalidShieldedTx, level)
		}
	}
	var indexBytes [8]byte
	binary.BigEndian.PutUint64(indexBytes[:], count)
	leaf := crypto.Keccak256Hash([]byte("TKM_SHIELD6_ACCUMULATOR_LEAF_V1"), indexBytes[:], output.OneTimeKey[:], output.Commitment[:])
	frontierIndex := count
	current := leaf
	zeroes := shieldedV6EmptyNodes()
	for level := 0; level < ShieldedV6OutputTreeDepth; level++ {
		var levelBytes [4]byte
		binary.BigEndian.PutUint32(levelBytes[:], uint32(level))
		frontierSlot := ShieldedV6StateSlot("accumulator-frontier", levelBytes[:])
		if frontierIndex&1 == 0 {
			st.SetState(params.ShieldedPoolAddress, frontierSlot, current)
			current = shieldedV6HashNode(current, zeroes[level])
		} else {
			left := st.GetState(params.ShieldedPoolAddress, frontierSlot)
			current = shieldedV6HashNode(left, current)
		}
		frontierIndex >>= 1
	}
	key := shieldedV6IndexKey(count)
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-key", key), common.BytesToHash(output.OneTimeKey[:]))
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-commitment", key), common.BytesToHash(output.Commitment[:]))
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-tx", key), txHash)
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-index", output.OneTimeKey[:]), uint64Hash(count+1))
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-count", nil), uint64Hash(count+1))
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("output-root", nil), current)
	return nil
}

func shieldedV6AppendKeyImage(st shieldedStateWriter, image [shielded6.PointSize]byte, txHash common.Hash) error {
	if shielded6.ValidatePublicKey(image[:]) != nil || txHash == (common.Hash{}) {
		return fmt.Errorf("%w: invalid Shield6 key image", ErrInvalidShieldedTx)
	}
	lookupSlot := ShieldedV6StateSlot("key-image", image[:])
	if st.GetState(params.ShieldedPoolAddress, lookupSlot) != (common.Hash{}) {
		return fmt.Errorf("%w: Shield6 key image already spent", ErrInvalidShieldedTx)
	}
	count := ShieldedV6KeyImageCount(st)
	if count == ^uint64(0) {
		return fmt.Errorf("%w: Shield6 key-image index is full", ErrInvalidShieldedTx)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-image-index", shieldedV6IndexKey(count)), common.BytesToHash(image[:]))
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-image-count", nil), uint64Hash(count+1))
	st.SetState(params.ShieldedPoolAddress, lookupSlot, txHash)
	return nil
}

func validateShieldedV6State(st *state.StateDB, tx *types.Transaction, e *ShieldedV6Transaction) error {
	if st == nil || tx == nil || e == nil {
		return ErrInvalidShieldedTx
	}
	if e.Version == ShieldedV6BindingVersion {
		if _, err := validateShieldedV6BindingBasics(tx, e); err != nil {
			return err
		}
		from, err := types.Sender(types.NewQuantumSigner(tx.ChainId()), tx)
		if err != nil {
			return err
		}
		owner, err := validateShieldedV6BindingRegistration(st, from, e.Binding.Commitment)
		if err == nil && owner != e.Binding.Owner {
			return ErrInvalidShieldedTx
		}
		return err
	}
	if len(e.Outputs) > 0 {
		if err := validateShieldedV6RecipientRoots(st, tx, e); err != nil {
			return err
		}
	}
	count := ShieldedV6OutputCount(st)
	if count > uint64(1)<<ShieldedV6OutputTreeDepth {
		return fmt.Errorf("%w: corrupt Shield6 output count", ErrInvalidShieldedTx)
	}
	intent, err := ShieldedV6Intent(tx, e)
	if err != nil {
		return err
	}
	seenImages := make(map[[shielded6.PointSize]byte]struct{}, len(e.Inputs))
	for i, input := range e.Inputs {
		members := make([]shielded6.RingMember, shielded6.RingSize)
		for j, index := range input.RingIndices {
			if index >= count {
				return fmt.Errorf("%w: Shield6 ring index is not in the pre-transaction accumulator", ErrInvalidShieldedTx)
			}
			member, err := shieldedV6ReadOutput(st, index)
			if err != nil {
				return fmt.Errorf("%w: invalid Shield6 ring member %d/%d", ErrInvalidShieldedTx, i, j)
			}
			members[j] = member
		}
		keyImageKey := ShieldedV6StateSlot("key-image", input.KeyImage[:])
		if st.GetState(params.ShieldedPoolAddress, keyImageKey) != (common.Hash{}) {
			return fmt.Errorf("%w: Shield6 key image already spent", ErrInvalidShieldedTx)
		}
		if _, duplicate := seenImages[input.KeyImage]; duplicate {
			return fmt.Errorf("%w: duplicate Shield6 key image in block", ErrInvalidShieldedTx)
		}
		seenImages[input.KeyImage] = struct{}{}
		if err := shielded6.VerifyCLSAG(intent, members, input.KeyImage, input.PseudoOutput, input.Signature[:]); err != nil {
			return fmt.Errorf("%w: Shield6 CLSAG input %d: %v", ErrInvalidShieldedTx, i, err)
		}
	}
	return nil
}

func processShieldedV6(config *params.ChainConfig, number *big.Int, time uint64, st *state.StateDB, tx *types.Transaction, seen map[common.Hash]struct{}) error {
	if st == nil || st.GetCodeSize(params.ShieldedPoolAddress) != 0 {
		return fmt.Errorf("%w: Shield6 pool must not contain executable code", ErrInvalidShieldedTx)
	}
	e, err := shieldedV6Basics(config, number, time, tx)
	if err != nil {
		return err
	}
	from, err := types.Sender(types.MakeSigner(config, number, time), tx)
	if err != nil {
		return err
	}
	if err := ValidateAntarticalStampState(st, from, tx.To(), tx.Value(), tx.Data()); err != nil {
		return err
	}
	if e.Version == ShieldedV6BindingVersion {
		return processShieldedV6Binding(st, tx, from, e)
	}
	if e.WithdrawalValue != nil && e.WithdrawalValue.Sign() > 0 && !IsAntarticalStamped(st, e.WithdrawalRecipient) {
		return fmt.Errorf("%w: Shield6 public withdrawal recipient is not stamped", ErrInvalidShieldedTx)
	}
	if err := ValidateShieldedV6Proof(tx); err != nil {
		return err
	}
	if err := validateShieldedV6State(st, tx, e); err != nil {
		return err
	}
	for _, input := range e.Inputs {
		key := ShieldedV6StateSlot("block-key-image", input.KeyImage[:])
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate Shield6 key image in block", ErrInvalidShieldedTx)
		}
	}
	var ledger ConfidentialLedgerTotals
	if e.Deposit || (e.WithdrawalValue != nil && e.WithdrawalValue.Sign() > 0) {
		deposit, withdrawal := new(big.Int), new(big.Int)
		if e.Deposit {
			deposit.Set(tx.Value())
		}
		if e.WithdrawalValue != nil {
			withdrawal.Set(e.WithdrawalValue)
		}
		ledger, err = ledgerTransition(st, shielded3.AssetTKM, deposit, withdrawal, new(big.Int))
		if err != nil {
			return fmt.Errorf("%w: Shield6 public boundary backing: %v", ErrInvalidShieldedTx, err)
		}
	}
	snapshot := st.Snapshot()
	for _, output := range e.Outputs {
		if err := shieldedV6AppendOutput(st, output, tx.Hash()); err != nil {
			st.RevertToSnapshot(snapshot)
			return err
		}
	}
	for _, input := range e.Inputs {
		if err := shieldedV6AppendKeyImage(st, input.KeyImage, tx.Hash()); err != nil {
			st.RevertToSnapshot(snapshot)
			return err
		}
	}
	for _, input := range e.Inputs {
		seen[ShieldedV6StateSlot("block-key-image", input.KeyImage[:])] = struct{}{}
	}
	if e.Deposit || (e.WithdrawalValue != nil && e.WithdrawalValue.Sign() > 0) {
		commitLedger(st, shielded3.AssetTKM, ledger)
	}
	if e.WithdrawalValue != nil && e.WithdrawalValue.Sign() > 0 {
		value := uint256.MustFromBig(e.WithdrawalValue)
		st.SubBalance(params.ShieldedPoolAddress, value, tracing.BalanceChangeTransfer)
		st.AddBalance(e.WithdrawalRecipient, value, tracing.BalanceChangeTransfer)
	}
	return nil
}

// ValidateShieldedV6State is the stateful mempool/reorg recheck for ring
// membership and key-image replay. It deliberately performs full CLSAG checks
// so ring indices are interpreted against the current canonical accumulator.
func ValidateShieldedV6State(st *state.StateDB, tx *types.Transaction) error {
	if st == nil || tx == nil || !HasShieldedV6Prefix(tx.Data()) {
		return nil
	}
	e, _, err := DecodeShieldedV6Transaction(tx.Data())
	if err != nil {
		return err
	}
	return validateShieldedV6State(st, tx, e)
}
