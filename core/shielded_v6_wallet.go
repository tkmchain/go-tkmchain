package core

import (
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

// BuildShieldedV6DepositEnvelope builds the explicit, public boundary from
// ordinary TKM into Shield6. The outer PQ transaction's public value is the
// deposit amount; the envelope proves that all confidential outputs sum to
// that amount. DepositMask is public only as the aggregate blinding factor
// needed to verify the commitment equation.
func BuildShieldedV6DepositEnvelope(tx *types.Transaction, outputs []ShieldedV6WalletOutput, random io.Reader) ([]byte, error) {
	if tx == nil || random == nil || len(outputs) == 0 || len(outputs) > ShieldedV6MaxOutputs {
		return nil, ErrInvalidShieldedTx
	}
	ephemerals := make([][32]byte, len(outputs))
	defer clear(ephemerals)

	chainID := tx.ChainId()
	to := tx.To()
	if tx.Type() != types.PQTkmTxType || chainID == nil || !chainID.IsUint64() || to == nil || *to != params.ShieldedPoolAddress || tx.Value() == nil || tx.Value().Sign() <= 0 || tx.Value().BitLen() > 128 {
		return nil, ErrInvalidShieldedTx
	}
	total := new(big.Int)
	defer clearShieldedV6BigInt(total)
	amounts := make([]*big.Int, len(outputs))
	for i, output := range outputs {
		if output.Address.ChainID != chainID.Uint64() || output.Address.Validate() != nil || output.Amount == nil || output.Amount.Sign() <= 0 || output.Amount.BitLen() > 128 {
			return nil, ErrInvalidShieldedTx
		}
		amounts[i] = new(big.Int).Set(output.Amount)
		total.Add(total, output.Amount)
	}
	if total.Cmp(tx.Value()) != 0 {
		return nil, shielded6.ErrUnbalanced
	}
	wide, proof, witnesses, err := shielded6.ProveRangeProof128WithWitness(amounts, random)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range witnesses {
			clear(witnesses[i].Mask[:])
			clear(witnesses[i].LimbMasks[0][:])
			clear(witnesses[i].LimbMasks[1][:])
		}
		for _, amount := range amounts {
			clearShieldedV6BigInt(amount)
		}
	}()
	if err := shielded6.VerifyRangeProof128(wide, proof); err != nil {
		return nil, err
	}
	mask, err := shielded6.SumWideWitnessMasks(witnesses)
	if err != nil {
		return nil, err
	}
	envelope := &ShieldedV6Transaction{
		Version: 6, AssetID: shielded6.AssetTKM, Deposit: true,
		DepositMask: mask, Outputs: make([]ShieldedV6Output, len(outputs)), RangeProof: proof,
	}
	clear(mask[:])
	for i, output := range outputs {
		record, err := createShieldedV6RecipientOutput(output, uint64(i), amounts[i], witnesses[i], random, &ephemerals[i])
		if err != nil {
			return nil, err
		}
		envelope.Outputs[i] = ShieldedV6OutputFromWallet(record)
		if envelope.Outputs[i].Commitment != wide[i].Commitment || envelope.Outputs[i].AmountLimbs != wide[i].Limbs {
			return nil, ErrInvalidShieldedTx
		}
	}
	if err := prepareShieldedV6Recipients(tx, envelope, outputs, false); err != nil {
		return nil, err
	}
	if err := proveShieldedV6Recipients(tx, envelope, outputs, ephemerals); err != nil {
		return nil, err
	}
	return EncodeShieldedV6Transaction(envelope)
}

// ShieldedV6WalletInput is a scanned spendable note and its canonical ring.
// RingIndices use a wallet-randomized order, and each member must correspond
// to the same index. Consensus independently resolves the indices and rejects
// duplicates; it does not sort them, so the real member's age rank is hidden.
type ShieldedV6WalletInput struct {
	Note        *shielded6.Note
	Ring        [shielded6.RingSize]shielded6.RingMember
	RingIndices [shielded6.RingSize]uint64
	SignerIndex uint64
	ValidUntil  uint64
}

// ShieldedV6WalletOutput describes one payment output. Change is an ordinary
// output and must be supplied explicitly by the caller when needed.
type ShieldedV6WalletOutput struct {
	Address   shielded6.Address
	Amount    *big.Int
	Recipient *ShieldedV6WalletRecipient
}

// BuildShieldedV6Envelope creates the confidential range proofs, encrypted
// outputs, balanced CLSAG signatures, and final Shield6 envelope. tx is an
// unsigned PQ account-transaction skeleton carrying the final chain ID,
// nonce, gas, fee caps, recipient pool address, and PQ public key. The caller
// must place the returned bytes into tx.Data and apply the account signature
// using the normal wallet signer.
//
// The function requires exact amount conservation. It does not select decoys:
// callers must obtain a ring from the canonical accumulator scanner, and the
// stateful consensus verifier rechecks every member against canonical state.
func BuildShieldedV6Envelope(tx *types.Transaction, inputs []ShieldedV6WalletInput, outputs []ShieldedV6WalletOutput, random io.Reader) ([]byte, error) {
	if tx == nil || random == nil || len(inputs) == 0 || len(inputs) > ShieldedV6MaxInputs || len(outputs) == 0 || len(outputs) > ShieldedV6MaxOutputs {
		return nil, ErrInvalidShieldedTx
	}
	ephemerals := make([][32]byte, len(outputs))
	defer clear(ephemerals)

	chainID := tx.ChainId()
	to := tx.To()
	if tx.Type() != types.PQTkmTxType || chainID == nil || !chainID.IsUint64() || to == nil || *to != params.ShieldedPoolAddress || tx.Value() == nil || tx.Value().Sign() != 0 {
		return nil, ErrInvalidShieldedTx
	}
	chainID64 := chainID.Uint64()
	totalInput := new(big.Int)
	defer clearShieldedV6BigInt(totalInput)
	for _, input := range inputs {
		if input.Note == nil || input.Note.ChainID != chainID64 || input.Note.AssetID != shielded6.AssetTKM || input.Note.Amount == nil || input.Note.Amount.Sign() <= 0 || input.SignerIndex >= shielded6.RingSize {
			return nil, ErrInvalidShieldedTx
		}
		if input.RingIndices[input.SignerIndex] != input.Note.Index {
			return nil, ErrInvalidShieldedTx
		}
		seenRingIndices := make(map[uint64]struct{}, shielded6.RingSize)
		for _, index := range input.RingIndices {
			if _, duplicate := seenRingIndices[index]; duplicate {
				return nil, ErrInvalidShieldedTx
			}
			seenRingIndices[index] = struct{}{}
		}
		prepared, err := input.Note.SigningInput(input.Ring, input.SignerIndex)
		if err != nil {
			return nil, err
		}
		prepared.Clear()
		totalInput.Add(totalInput, input.Note.Amount)
	}
	totalOutput := new(big.Int)
	defer clearShieldedV6BigInt(totalOutput)
	amounts := make([]*big.Int, len(outputs))
	for i, output := range outputs {
		if output.Address.ChainID != chainID64 || output.Address.Validate() != nil || output.Amount == nil || output.Amount.Sign() <= 0 || output.Amount.BitLen() > 128 {
			return nil, ErrInvalidShieldedTx
		}
		amounts[i] = new(big.Int).Set(output.Amount)
		totalOutput.Add(totalOutput, output.Amount)
	}
	if totalInput.Cmp(totalOutput) != 0 {
		return nil, shielded6.ErrUnbalanced
	}

	wideCommitments, rangeProof, outputWitnesses, err := shielded6.ProveRangeProof128WithWitness(amounts, random)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range outputWitnesses {
			clear(outputWitnesses[i].Mask[:])
			clear(outputWitnesses[i].LimbMasks[0][:])
			clear(outputWitnesses[i].LimbMasks[1][:])
		}
		for _, amount := range amounts {
			clearShieldedV6BigInt(amount)
		}
	}()
	if err := shielded6.VerifyRangeProof128(wideCommitments, rangeProof); err != nil {
		return nil, err
	}
	envelope := &ShieldedV6Transaction{
		Version: 6, AssetID: shielded6.AssetTKM,
		Inputs:  make([]ShieldedV6Input, len(inputs)),
		Outputs: make([]ShieldedV6Output, len(outputs)), RangeProof: rangeProof,
	}
	for i := range inputs {
		envelope.Inputs[i].RingIndices = inputs[i].RingIndices
	}
	for i, output := range outputs {
		walletOutput, err := createShieldedV6RecipientOutput(output, uint64(i), amounts[i], outputWitnesses[i], random, &ephemerals[i])
		if err != nil {
			return nil, err
		}
		envelope.Outputs[i] = ShieldedV6OutputFromWallet(walletOutput)
		if envelope.Outputs[i].Commitment != wideCommitments[i].Commitment || envelope.Outputs[i].AmountLimbs != wideCommitments[i].Limbs {
			return nil, ErrInvalidShieldedTx
		}
	}
	if err := prepareShieldedV6Recipients(tx, envelope, outputs, false); err != nil {
		return nil, err
	}
	intent, err := ShieldedV6Intent(tx, envelope)
	if err != nil {
		return nil, err
	}
	signingInputs := make([]shielded6.SigningInput128, len(inputs))
	for i := range inputs {
		signingInputs[i], err = inputs[i].Note.SigningInput(inputs[i].Ring, inputs[i].SignerIndex)
		if err != nil {
			return nil, err
		}
	}
	sumMasks, err := shielded6.SumWideWitnessMasks(outputWitnesses)
	if err != nil {
		return nil, err
	}
	signed, err := shielded6.SignCLSAGs128(intent, signingInputs, sumMasks, random)
	clear(sumMasks[:])
	for i := range signingInputs {
		signingInputs[i].Clear()
	}
	if err != nil {
		return nil, err
	}
	pseudoOutputs := make([][shielded6.PointSize]byte, len(signed))
	outputCommitments := make([][shielded6.PointSize]byte, len(envelope.Outputs))
	seenImages := make(map[[shielded6.PointSize]byte]struct{}, len(signed))
	for i := range signed {
		envelope.Inputs[i].KeyImage = signed[i].KeyImage
		envelope.Inputs[i].PseudoOutput = signed[i].PseudoOutput
		envelope.Inputs[i].Signature = signed[i].Signature
		pseudoOutputs[i] = signed[i].PseudoOutput
		if _, exists := seenImages[signed[i].KeyImage]; exists {
			return nil, ErrInvalidShieldedTx
		}
		seenImages[signed[i].KeyImage] = struct{}{}
	}
	for i := range envelope.Outputs {
		outputCommitments[i] = envelope.Outputs[i].Commitment
	}
	if err := shielded6.VerifyCommitmentBalance(pseudoOutputs, outputCommitments, 0); err != nil {
		return nil, err
	}
	for i := range inputs {
		if err := shielded6.VerifyCLSAG(intent, inputs[i].Ring[:], signed[i].KeyImage, signed[i].PseudoOutput, signed[i].Signature[:]); err != nil {
			return nil, err
		}
	}
	if err := proveShieldedV6Recipients(tx, envelope, outputs, ephemerals); err != nil {
		return nil, err
	}
	return EncodeShieldedV6Transaction(envelope)
}

// BuildShieldedV6WithdrawalEnvelope constructs a public exit from the
// confidential Shield6 pool. The recipient and amount are intentionally
// public. If outputs is empty this is a full exit; otherwise the outputs are
// confidential change. Consensus independently requires the withdrawal
// recipient to be stamped and verifies the ring members against canonical
// pre-transaction state.
func BuildShieldedV6WithdrawalEnvelope(tx *types.Transaction, inputs []ShieldedV6WalletInput, outputs []ShieldedV6WalletOutput, recipient common.Address, withdrawal *big.Int, random io.Reader) ([]byte, error) {
	return buildShieldedV6WithdrawalEnvelope(tx, inputs, outputs, recipient, withdrawal, random, false)
}

func BuildShieldedV6BoundWithdrawalEnvelope(tx *types.Transaction, inputs []ShieldedV6WalletInput, outputs []ShieldedV6WalletOutput, recipient common.Address, withdrawal *big.Int, random io.Reader) ([]byte, error) {
	return buildShieldedV6WithdrawalEnvelope(tx, inputs, outputs, recipient, withdrawal, random, true)
}

func buildShieldedV6WithdrawalEnvelope(tx *types.Transaction, inputs []ShieldedV6WalletInput, outputs []ShieldedV6WalletOutput, recipient common.Address, withdrawal *big.Int, random io.Reader, forceBound bool) ([]byte, error) {
	if tx == nil || random == nil || len(inputs) == 0 || len(inputs) > ShieldedV6MaxInputs || len(outputs) > ShieldedV6MaxOutputs || recipient == (common.Address{}) || withdrawal == nil || withdrawal.Sign() <= 0 || withdrawal.BitLen() > 128 {
		return nil, ErrInvalidShieldedTx
	}
	ephemerals := make([][32]byte, len(outputs))
	defer clear(ephemerals)

	chainID, to := tx.ChainId(), tx.To()
	if tx.Type() != types.PQTkmTxType || chainID == nil || !chainID.IsUint64() || to == nil || *to != params.ShieldedPoolAddress || tx.Value() == nil || tx.Value().Sign() != 0 {
		return nil, ErrInvalidShieldedTx
	}
	chainID64 := chainID.Uint64()
	totalInput := new(big.Int)
	defer clearShieldedV6BigInt(totalInput)
	for _, input := range inputs {
		if input.Note == nil || input.Note.ChainID != chainID64 || input.Note.AssetID != shielded6.AssetTKM || input.Note.Amount == nil || input.Note.Amount.Sign() <= 0 || input.SignerIndex >= shielded6.RingSize || input.RingIndices[input.SignerIndex] != input.Note.Index {
			return nil, ErrInvalidShieldedTx
		}
		seenRingIndices := make(map[uint64]struct{}, shielded6.RingSize)
		for _, index := range input.RingIndices {
			if _, duplicate := seenRingIndices[index]; duplicate {
				return nil, ErrInvalidShieldedTx
			}
			seenRingIndices[index] = struct{}{}
		}
		prepared, err := input.Note.SigningInput(input.Ring, input.SignerIndex)
		if err != nil {
			return nil, err
		}
		prepared.Clear()
		totalInput.Add(totalInput, input.Note.Amount)
	}
	amounts := make([]*big.Int, len(outputs))
	totalOutput := new(big.Int)
	defer clearShieldedV6BigInt(totalOutput)
	for i, output := range outputs {
		if output.Address.ChainID != chainID64 || output.Address.Validate() != nil || output.Amount == nil || output.Amount.Sign() <= 0 || output.Amount.BitLen() > 128 {
			return nil, ErrInvalidShieldedTx
		}
		amounts[i] = new(big.Int).Set(output.Amount)
		totalOutput.Add(totalOutput, output.Amount)
	}
	totalExit := new(big.Int).Add(totalOutput, withdrawal)
	defer clearShieldedV6BigInt(totalExit)
	if totalExit.Cmp(totalInput) != 0 {
		return nil, shielded6.ErrUnbalanced
	}
	var wide []shielded6.WideCommitment
	var proof []byte
	var witnesses []shielded6.WideWitness
	if len(outputs) > 0 {
		var err error
		wide, proof, witnesses, err = shielded6.ProveRangeProof128WithWitness(amounts, random)
		if err != nil {
			return nil, err
		}
		defer func() {
			for i := range witnesses {
				clear(witnesses[i].Mask[:])
				clear(witnesses[i].LimbMasks[0][:])
				clear(witnesses[i].LimbMasks[1][:])
			}
		}()
		if err := shielded6.VerifyRangeProof128(wide, proof); err != nil {
			return nil, err
		}
	}
	defer func() {
		for _, amount := range amounts {
			clearShieldedV6BigInt(amount)
		}
	}()
	envelope := &ShieldedV6Transaction{
		Version: 6, AssetID: shielded6.AssetTKM, WithdrawalRecipient: recipient,
		WithdrawalValue: new(big.Int).Set(withdrawal), Inputs: make([]ShieldedV6Input, len(inputs)),
		Outputs: make([]ShieldedV6Output, len(outputs)), RangeProof: proof,
	}
	for i := range inputs {
		envelope.Inputs[i].RingIndices = inputs[i].RingIndices
	}
	for i, output := range outputs {
		record, err := createShieldedV6RecipientOutput(output, uint64(i), amounts[i], witnesses[i], random, &ephemerals[i])
		if err != nil {
			return nil, err
		}
		envelope.Outputs[i] = ShieldedV6OutputFromWallet(record)
		if envelope.Outputs[i].Commitment != wide[i].Commitment || envelope.Outputs[i].AmountLimbs != wide[i].Limbs {
			return nil, ErrInvalidShieldedTx
		}
	}
	if forceBound && len(outputs) == 0 {
		envelope.ValidUntil = inputs[0].ValidUntil
	}
	if err := prepareShieldedV6Recipients(tx, envelope, outputs, forceBound); err != nil {
		return nil, err
	}
	intent, err := ShieldedV6Intent(tx, envelope)
	if err != nil {
		return nil, err
	}
	signingInputs := make([]shielded6.SigningInput128, len(inputs))
	for i := range inputs {
		signingInputs[i], err = inputs[i].Note.SigningInput(inputs[i].Ring, inputs[i].SignerIndex)
		if err != nil {
			return nil, err
		}
	}
	var outputMask [shielded6.ScalarSize]byte
	if len(witnesses) > 0 {
		outputMask, err = shielded6.SumWideWitnessMasks(witnesses)
		if err != nil {
			return nil, err
		}
	}
	signed, err := shielded6.SignCLSAGs128(intent, signingInputs, outputMask, random)
	clear(outputMask[:])
	for i := range signingInputs {
		signingInputs[i].Clear()
	}
	if err != nil {
		return nil, err
	}
	pseudoOutputs := make([][shielded6.PointSize]byte, len(signed))
	outputCommitments := make([][shielded6.PointSize]byte, len(envelope.Outputs))
	seenImages := make(map[[shielded6.PointSize]byte]struct{}, len(signed))
	for i := range signed {
		envelope.Inputs[i].KeyImage, envelope.Inputs[i].PseudoOutput, envelope.Inputs[i].Signature = signed[i].KeyImage, signed[i].PseudoOutput, signed[i].Signature
		pseudoOutputs[i] = signed[i].PseudoOutput
		if _, exists := seenImages[signed[i].KeyImage]; exists {
			return nil, ErrInvalidShieldedTx
		}
		seenImages[signed[i].KeyImage] = struct{}{}
	}
	for i := range envelope.Outputs {
		outputCommitments[i] = envelope.Outputs[i].Commitment
	}
	if err := shielded6.VerifyCommitmentBalance128(pseudoOutputs, outputCommitments, withdrawal); err != nil {
		return nil, err
	}
	for i := range inputs {
		if err := shielded6.VerifyCLSAG(intent, inputs[i].Ring[:], signed[i].KeyImage, signed[i].PseudoOutput, signed[i].Signature[:]); err != nil {
			return nil, err
		}
	}
	if err := proveShieldedV6Recipients(tx, envelope, outputs, ephemerals); err != nil {
		return nil, err
	}
	return EncodeShieldedV6Transaction(envelope)
}

// clearShieldedV6BigInt wipes private amount copies before resetting them.
// big.Int.SetInt64(0) changes its logical value but can leave old backing words
// in the allocated heap storage.
func clearShieldedV6BigInt(value *big.Int) {
	if value == nil {
		return
	}
	words := value.Bits()
	clear(words)
	value.SetInt64(0)
}
