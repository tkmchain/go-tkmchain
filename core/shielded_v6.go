package core

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

const (
	ShieldedV6Magic                  = "TKMSHIELD6"
	ShieldedV6MaxTxSize              = ShieldedV3MaxTxSize
	ShieldedV6MaxCiphertext          = 4096
	ShieldedV6MaxInputs              = 16
	ShieldedV6MaxOutputs             = 16
	ShieldedV6OutputTreeDepth        = 32
	ShieldedV6BaseVerifyGas   uint64 = 1_000_000
	ShieldedV6InputVerifyGas  uint64 = 250_000
	ShieldedV6OutputVerifyGas uint64 = 50_000
	// ShieldedV6StateWriteGas prices a state slot at the conservative EVM
	// zero-to-nonzero SSTORE rate. Shield6 state is written by the consensus
	// processor rather than bytecode, so the envelope must account for it here.
	ShieldedV6StateWriteGas uint64 = 20_000
	// Repeated key-image appends update the same nonzero count slot. After its
	// first write in a transaction, a warm nonzero-to-nonzero SSTORE costs 2,900.
	ShieldedV6StateUpdateGas uint64 = 2_900
)

type ShieldedV6Input struct {
	RingIndices  [shielded6.RingSize]uint64
	KeyImage     [shielded6.PointSize]byte
	PseudoOutput [shielded6.PointSize]byte
	Signature    [shielded6.CLSAGSize]byte
}

type ShieldedV6Output struct {
	OneTimeKey    [shielded6.PointSize]byte
	EphemeralKey  [shielded6.PointSize]byte
	Commitment    [shielded6.PointSize]byte
	AmountLimbs   [2][shielded6.PointSize]byte
	EncryptedNote []byte
}

// ShieldedV6OutputFromWallet maps a cryptographically constructed wallet
// output into the consensus envelope representation. outputPosition used by
// wallet derivation is the transaction-local list position; consensus assigns
// the separate accumulator index only after block inclusion.
func ShieldedV6OutputFromWallet(output shielded6.OutputRecord) ShieldedV6Output {
	return ShieldedV6Output{
		OneTimeKey: output.OneTimeKey, EphemeralKey: output.EphemeralKey,
		Commitment: output.Commitment, AmountLimbs: output.AmountLimbs,
		EncryptedNote: append([]byte(nil), output.EncryptedNote...),
	}
}

// ShieldedV6WalletOutput maps a consensus output into the wallet scanner's
// representation without changing any authenticated field.
func ShieldedV6WalletRecord(output ShieldedV6Output) shielded6.OutputRecord {
	return shielded6.OutputRecord{
		OneTimeKey: output.OneTimeKey, EphemeralKey: output.EphemeralKey,
		Commitment: output.Commitment, AmountLimbs: output.AmountLimbs,
		EncryptedNote: append([]byte(nil), output.EncryptedNote...),
	}
}

// ShieldedV6Transaction carries a private transfer or an explicit public
// shielding/unshielding boundary. Public boundary values are visible by
// design. Gas is paid by the outer account transaction.
type ShieldedV6Transaction struct {
	Version             uint64
	AssetID             uint64
	Deposit             bool
	DepositMask         [shielded6.ScalarSize]byte
	WithdrawalRecipient common.Address
	WithdrawalValue     *big.Int
	Inputs              []ShieldedV6Input
	Outputs             []ShieldedV6Output
	RangeProof          []byte
}

func HasShieldedV6Prefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(ShieldedV6Magic))
}

func EncodeShieldedV6Transaction(e *ShieldedV6Transaction) ([]byte, error) {
	if e == nil {
		return nil, ErrInvalidShieldedTx
	}
	payload, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	return append([]byte(ShieldedV6Magic), payload...), nil
}

func DecodeShieldedV6Transaction(data []byte) (*ShieldedV6Transaction, bool, error) {
	if !HasShieldedV6Prefix(data) {
		return nil, false, nil
	}
	if uint64(len(data)) > ShieldedV6MaxTxSize {
		return nil, true, ErrInvalidShieldedTx
	}
	var e ShieldedV6Transaction
	if err := rlp.DecodeBytes(data[len(ShieldedV6Magic):], &e); err != nil {
		return nil, true, fmt.Errorf("%w: malformed Shield6 envelope", ErrInvalidShieldedTx)
	}
	return &e, true, nil
}

func shieldedV6IntentEnvelope(e *ShieldedV6Transaction) *ShieldedV6Transaction {
	clean := *e
	clean.Inputs = append([]ShieldedV6Input(nil), e.Inputs...)
	for i := range clean.Inputs {
		// CLSAG signing derives the key image and pseudo-output together with
		// its signature. They are still bound by the CLSAG challenge itself;
		// omitting them here avoids a signing-message circular dependency.
		clean.Inputs[i].KeyImage = [shielded6.PointSize]byte{}
		clean.Inputs[i].PseudoOutput = [shielded6.PointSize]byte{}
		clean.Inputs[i].Signature = [shielded6.CLSAGSize]byte{}
	}
	return &clean
}

// ShieldedV6Intent binds the transaction signature to the chain, fee fields,
// ring indices, range proof, output commitments and encrypted payloads. Key
// images and pseudo-outputs are bound by each CLSAG transcript; only the
// resulting CLSAG signatures are excluded to avoid a circular dependency.
func ShieldedV6Intent(tx *types.Transaction, e *ShieldedV6Transaction) ([32]byte, error) {
	var result [32]byte
	if tx == nil || e == nil {
		return result, ErrInvalidShieldedTx
	}
	data, err := EncodeShieldedV6Transaction(shieldedV6IntentEnvelope(e))
	if err != nil {
		return result, err
	}
	algorithm, publicKey, _, _ := tx.PQTkmFields()
	payload := shieldedIntentPayload{
		Domain: []byte("TKM_SHIELD6_CLSAG_RINGCT_INTENT_V1"),
		TxType: tx.Type(), ChainID: tx.ChainId(), Nonce: tx.Nonce(),
		GasTipCap: tx.GasTipCap(), GasFeeCap: tx.GasFeeCap(), Gas: tx.Gas(),
		To: tx.To(), Value: tx.Value(), AccessList: tx.AccessList(),
		PQAlgorithm: algorithm, PQPublicKey: publicKey, Envelope: data,
	}
	encoded, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return result, err
	}
	copy(result[:], crypto.Keccak256(encoded))
	return result, nil
}

func shieldedV6Basics(config *params.ChainConfig, number *big.Int, time uint64, tx *types.Transaction) (*ShieldedV6Transaction, error) {
	fail := func(reason string) (*ShieldedV6Transaction, error) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidShieldedTx, reason)
	}
	if config == nil || !config.IsShield6(number, time) {
		return fail("Shield6 is not active")
	}
	if err := ValidateShieldedOnlyTransaction(config, number, time, tx); err != nil {
		return nil, err
	}
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value() == nil {
		return fail("Shield6 requires a PQ transaction to the shielded pool")
	}
	if config.ChainID == nil || tx.ChainId().Cmp(config.ChainID) != 0 {
		return fail("Shield6 chain ID mismatch")
	}
	e, _, err := DecodeShieldedV6Transaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if e == nil || e.Version != 6 {
		return fail("expected Shield6 envelope version 6")
	}
	if e.AssetID != shielded3.AssetTKM {
		return fail("Shield6 currently accepts only the native TKM asset domain")
	}
	withdrawal := new(big.Int)
	if e.WithdrawalValue != nil {
		if e.WithdrawalValue.Sign() < 0 || e.WithdrawalValue.BitLen() > 128 {
			return fail("invalid Shield6 withdrawal amount")
		}
		withdrawal.Set(e.WithdrawalValue)
	}
	if (e.Deposit || withdrawal.Sign() > 0) && !config.IsConfidentialLedger(number, time) {
		return fail("Shield6 public boundaries require the confidential backing ledger")
	}
	if e.Deposit {
		if tx.Value().Sign() <= 0 || tx.Value().BitLen() > 128 || withdrawal.Sign() != 0 || e.WithdrawalRecipient != (common.Address{}) || len(e.Inputs) != 0 || len(e.Outputs) == 0 || len(e.Outputs) > ShieldedV6MaxOutputs {
			return fail("invalid Shield6 public deposit")
		}
		if err := shielded6.ValidateScalarEncoding(e.DepositMask[:]); err != nil {
			return fail("noncanonical Shield6 deposit mask")
		}
	} else {
		if tx.Value().Sign() != 0 || e.DepositMask != ([shielded6.ScalarSize]byte{}) {
			return fail("Shield6 spend must have zero public value and no deposit mask")
		}
		if withdrawal.Sign() > 0 {
			if e.WithdrawalRecipient == (common.Address{}) || len(e.Inputs) == 0 || len(e.Inputs) > ShieldedV6MaxInputs || len(e.Outputs) > ShieldedV6MaxOutputs {
				return fail("invalid Shield6 public withdrawal")
			}
		} else if e.WithdrawalRecipient != (common.Address{}) || len(e.Inputs) == 0 || len(e.Inputs) > ShieldedV6MaxInputs || len(e.Outputs) == 0 || len(e.Outputs) > ShieldedV6MaxOutputs {
			return fail("invalid Shield6 private input or output count")
		}
	}
	if len(e.Outputs) > 0 {
		if len(e.RangeProof) == 0 || len(e.RangeProof) > shielded6.MaxRangeProofBundleSize {
			return fail("invalid Shield6 paired range-proof bundle size")
		}
	} else if len(e.RangeProof) != 0 {
		return fail("Shield6 withdrawal without outputs must not carry a range proof")
	}
	seenImages := make(map[[shielded6.PointSize]byte]struct{}, len(e.Inputs))
	for i, input := range e.Inputs {
		if input.KeyImage == ([shielded6.PointSize]byte{}) || input.PseudoOutput == ([shielded6.PointSize]byte{}) || input.Signature == ([shielded6.CLSAGSize]byte{}) {
			return fail(fmt.Sprintf("invalid Shield6 input %d", i))
		}
		if err := shielded6.ValidatePublicKey(input.KeyImage[:]); err != nil {
			return fail(fmt.Sprintf("invalid Shield6 key image %d", i))
		}
		if err := shielded6.ValidateCommitmentEncoding(input.PseudoOutput[:]); err != nil {
			return fail(fmt.Sprintf("invalid Shield6 pseudo-output %d", i))
		}
		if _, ok := seenImages[input.KeyImage]; ok {
			return fail("duplicate Shield6 key image in transaction")
		}
		seenImages[input.KeyImage] = struct{}{}
		seenRingIndices := make(map[uint64]struct{}, len(input.RingIndices))
		for _, index := range input.RingIndices {
			if _, duplicate := seenRingIndices[index]; duplicate {
				return fail("duplicate Shield6 ring index")
			}
			seenRingIndices[index] = struct{}{}
		}
	}
	seenOutputs := make(map[[shielded6.PointSize]byte]struct{}, len(e.Outputs))
	for i, output := range e.Outputs {
		if output.OneTimeKey == ([shielded6.PointSize]byte{}) || output.EphemeralKey == ([shielded6.PointSize]byte{}) || output.Commitment == ([shielded6.PointSize]byte{}) || len(output.EncryptedNote) == 0 || len(output.EncryptedNote) > ShieldedV6MaxCiphertext {
			return fail(fmt.Sprintf("invalid Shield6 output %d", i))
		}
		if err := shielded6.ValidatePublicKey(output.OneTimeKey[:]); err != nil {
			return fail(fmt.Sprintf("invalid Shield6 one-time key %d", i))
		}
		if err := shielded6.ValidatePublicKey(output.EphemeralKey[:]); err != nil {
			return fail(fmt.Sprintf("invalid Shield6 ephemeral key %d", i))
		}
		if _, ok := seenOutputs[output.OneTimeKey]; ok {
			return fail("duplicate Shield6 one-time output key")
		}
		seenOutputs[output.OneTimeKey] = struct{}{}
	}
	return e, nil
}

func ValidateShieldedV6Proof(tx *types.Transaction) error {
	if tx == nil {
		return ErrInvalidShieldedTx
	}
	e, ok, err := DecodeShieldedV6Transaction(tx.Data())
	if err != nil || !ok || e == nil {
		return ErrInvalidShieldedTx
	}
	wideCommitments := make([]shielded6.WideCommitment, len(e.Outputs))
	outputCommitments := make([][shielded6.PointSize]byte, len(e.Outputs))
	pseudoOutputs := make([][shielded6.PointSize]byte, len(e.Inputs))
	for i := range e.Outputs {
		wideCommitments[i] = shielded6.WideCommitment{Commitment: e.Outputs[i].Commitment, Limbs: e.Outputs[i].AmountLimbs}
		outputCommitments[i] = e.Outputs[i].Commitment
	}
	for i := range e.Inputs {
		pseudoOutputs[i] = e.Inputs[i].PseudoOutput
		// The ring keys and commitments are loaded from the canonical output
		// accumulator during stateful validation. Stateless proof verification
		// is therefore deferred to ValidateShieldedV6State.
	}
	// Range proofs and public-boundary commitment equations are stateless.
	if len(e.Outputs) > 0 {
		if err := shielded6.VerifyRangeProof128(wideCommitments, e.RangeProof); err != nil {
			return fmt.Errorf("%w: Shield6 range proof: %v", ErrInvalidShieldedTx, err)
		}
	}
	if e.Deposit {
		if err := shielded6.VerifyDepositCommitment(outputCommitments, tx.Value(), e.DepositMask); err != nil {
			return fmt.Errorf("%w: Shield6 deposit balance: %v", ErrInvalidShieldedTx, err)
		}
	} else {
		publicWithdrawal := new(big.Int)
		if e.WithdrawalValue != nil {
			publicWithdrawal.Set(e.WithdrawalValue)
		}
		if err := shielded6.VerifyCommitmentBalance128(pseudoOutputs, outputCommitments, publicWithdrawal); err != nil {
			return fmt.Errorf("%w: Shield6 balance: %v", ErrInvalidShieldedTx, err)
		}
	}
	// CLSAG verification uses accumulator-resolved ring members in the
	// stateful path below, where those members are anchored to canonical state.
	return nil
}

func ShieldedV6GasData(data []byte) ([]byte, uint64, error) {
	e, ok, err := DecodeShieldedV6Transaction(data)
	if err != nil || !ok {
		return data, 0, err
	}
	if len(e.Inputs) > ShieldedV6MaxInputs || len(e.Outputs) > ShieldedV6MaxOutputs || len(e.RangeProof) > shielded6.MaxRangeProofBundleSize || (len(e.Inputs) == 0 && !e.Deposit) || (len(e.Outputs) == 0 && (e.WithdrawalValue == nil || e.WithdrawalValue.Sign() == 0)) {
		return nil, 0, ErrInvalidShieldedTx
	}
	clean := *e
	clean.RangeProof = nil
	clean.Inputs = append([]ShieldedV6Input(nil), e.Inputs...)
	for i := range clean.Inputs {
		clean.Inputs[i].Signature = [shielded6.CLSAGSize]byte{}
	}
	encoded, err := EncodeShieldedV6Transaction(&clean)
	if err != nil {
		return nil, 0, err
	}
	inputGas := uint64(len(e.Inputs)) * ShieldedV6InputVerifyGas
	outputGas := uint64(len(e.Outputs)) * ShieldedV6OutputVerifyGas
	proofGas := uint64(len(e.RangeProof)) * 100
	// Appending an output writes its key, commitment, creating-tx reference,
	// key-to-index entry, and up to one frontier node per tree level. Charge
	// worst-case frontier writes, not the average, so transaction cost does not
	// depend on accumulator position or underprice adversarial append patterns.
	var outputStateGas uint64
	if len(e.Outputs) > 0 {
		writesPerOutput := uint64(ShieldedV6OutputTreeDepth + 4) // 32 frontier + 4 leaf indexes
		outputStateGas = uint64(len(e.Outputs)) * writesPerOutput * ShieldedV6StateWriteGas
		outputStateGas += 2 * ShieldedV6StateWriteGas // output count and accumulator root
	}
	// Each input writes its indexed key image and lookup entry; the shared
	// sequential count is charged once per transaction.
	var inputStateGas uint64
	if len(e.Inputs) > 0 {
		inputStateGas = (2*uint64(len(e.Inputs)) + 1) * ShieldedV6StateWriteGas
		inputStateGas += (uint64(len(e.Inputs)) - 1) * ShieldedV6StateUpdateGas
	}
	// Public deposits and withdrawals also update the confidential backing
	// ledger's deposited, withdrawn, and backing slots.
	var boundaryStateGas uint64
	if e.Deposit || e.WithdrawalValue != nil && e.WithdrawalValue.Sign() > 0 {
		boundaryStateGas = 3 * ShieldedV6StateWriteGas
	}
	gas := ShieldedV6BaseVerifyGas
	for _, cost := range []uint64{inputGas, outputGas, proofGas, inputStateGas, outputStateGas, boundaryStateGas} {
		if ^uint64(0)-gas < cost {
			return nil, 0, ErrGasUintOverflow
		}
		gas += cost
	}
	if gas < ShieldedV6BaseVerifyGas {
		return nil, 0, ErrGasUintOverflow
	}
	return encoded, gas, nil
}

func IsShieldedV6To(tx *types.Transaction, to common.Address) bool {
	return tx != nil && tx.To() != nil && *tx.To() == to && HasShieldedV6Prefix(tx.Data())
}

// ShieldedV6KeyImages extracts the key images reserved by a Shield6 transaction.
// It returns (nil, nil) for transactions outside Shield6 so txpool callers can
// apply the reservation policy without duplicating envelope parsing.
func ShieldedV6KeyImages(tx *types.Transaction) ([][shielded6.PointSize]byte, error) {
	if tx == nil || !HasShieldedV6Prefix(tx.Data()) {
		return nil, nil
	}
	e, ok, err := DecodeShieldedV6Transaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if !ok || e == nil {
		return nil, ErrInvalidShieldedTx
	}
	images := make([][shielded6.PointSize]byte, len(e.Inputs))
	for i := range e.Inputs {
		images[i] = e.Inputs[i].KeyImage
	}
	return images, nil
}
