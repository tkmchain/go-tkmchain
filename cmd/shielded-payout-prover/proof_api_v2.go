package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded"
)

func (p *Prover) activeShieldedVersion(ctx context.Context, chainID *big.Int) uint64 {
	if chainID == nil || params.MainnetChainConfig == nil || params.MainnetChainConfig.ChainID == nil || chainID.Cmp(params.MainnetChainConfig.ChainID) != 0 {
		return core.ShieldedTxVersionV1
	}
	header, err := p.client.HeaderByNumber(ctx, nil)
	if err == nil && header != nil && header.Time >= params.MainnetShieldedV2Time {
		return core.ShieldedTxVersionV2
	}
	return core.ShieldedTxVersionV1
}

func (p *Prover) shieldedGasSponsorshipActive(ctx context.Context, chainID *big.Int) bool {
	if chainID == nil || params.MainnetChainConfig == nil || params.MainnetChainConfig.ChainID == nil || chainID.Cmp(params.MainnetChainConfig.ChainID) != 0 {
		return false
	}
	header, err := p.client.HeaderByNumber(ctx, nil)
	return err == nil && header != nil && header.Time >= params.MainnetShieldedGasSponsorTime
}

func (p *Prover) buildDepositV2(ctx context.Context, req DepositRequest, amountWei *big.Int, assetID string, chainID *big.Int, nonce uint64, gasPrice *big.Int) (BuildShieldedResponse, error) {
	if p.pkV2 == nil || p.r1csV2 == nil {
		return BuildShieldedResponse{}, errors.New("recipient-bound V2 proving key is not ready")
	}
	viewKey, err := parseViewPublicKey(req.RecipientViewKey)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	spendData, err := shieldedSpendData(req.RequestID, req.ApplicationData)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	sender := common.HexToAddress(req.From)
	recipient := sender
	if strings.TrimSpace(req.To) != "" {
		if !isValidAddress(req.To) {
			return BuildShieldedResponse{}, fmt.Errorf("invalid recipient address %q", req.To)
		}
		recipient = common.HexToAddress(req.To)
	}
	asset, err := parseFieldElement(assetID)
	if err != nil {
		return BuildShieldedResponse{}, fmt.Errorf("invalid assetId: %w", err)
	}
	value := fieldElementFromBig(amountWei)
	recipients := [shielded.OutputSlots]fr.Element{fieldElementFromBig(recipient.Big()), zeroElement(), zeroElement(), zeroElement()}
	values := [shielded.OutputSlots]fr.Element{value, zeroElement(), zeroElement(), zeroElement()}
	randomness := [shielded.OutputSlots]fr.Element{randomElement(), randomElement(), randomElement(), randomElement()}
	var commitments [shielded.OutputSlots]fr.Element
	outputs := make([]core.ShieldedOutput, 0, shielded.OutputSlots)
	for i := 0; i < shielded.OutputSlots; i++ {
		commitments[i] = fieldHash(shielded.DomainNoteV2, recipients[i], asset, values[i], randomness[i])
		commitment := hashFromField(commitments[i])
		if i == 0 {
			nullifier := hashFromField(fieldHash(shielded.DomainNullV2, recipients[i], randomness[i]))
			output, err := encryptShieldedNote(commitment, noteOpeningV2(recipient, asset.BigInt(new(big.Int)), amountWei, randomness[i].BigInt(new(big.Int)), commitment, nullifier), viewKey)
			if err != nil {
				return BuildShieldedResponse{}, err
			}
			outputs = append(outputs, output)
			continue
		}
		output, err := decoyShieldedOutput(commitment)
		if err != nil {
			return BuildShieldedResponse{}, err
		}
		outputs = append(outputs, output)
	}
	outputRoot := hashFromField(fieldHash(shielded.DomainOutputV2, commitments[:]...))
	balance := fieldHash(shielded.DomainBalV2, zeroElement(), value, value, asset, fieldElementFromHash(outputRoot))
	envelope := &core.ShieldedTransaction{
		Version: core.ShieldedTxVersionV2,
		Spends: []core.ShieldedSpend{{
			Nullifier:          common.Hash{},
			Anchor:             common.Hash{},
			EncryptedSpendData: spendData,
		}},
		Outputs:           outputs,
		BalanceCommitment: hashFromField(balance),
		BindingSig:        make([]byte, common.HashLength),
	}
	data, err := core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	unsigned := p.unsignedTxWithValue(chainID, nonce, gasPrice, amountWei, data)
	intentHash, err := core.ShieldedTransactionIntentHash(unsigned, envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	assignment := newV2Assignment(chainID, sender, new(big.Int), intentHash, common.Hash{}, common.Hash{}, envelope.BalanceCommitment, amountWei, outputRoot, "0", "0", "0", assetID)
	fillCircuitArraysV2(assignment, make([]string, shielded.MerkleDepth), make([]string, shielded.MerkleDepth), recipients, values, randomness, commitments)
	binding := fieldHash(shielded.DomainBindV2,
		fieldElementFromBig(sender.Big()),
		zeroElement(),
		zeroElement(),
		fieldElementFromHash(outputRoot),
		balance,
		fieldElementFromBig(chainID),
		fieldElementFromBytes(intentHash[:16]),
		fieldElementFromBytes(intentHash[16:]),
	)
	bindingBytes := binding.Bytes()
	envelope.BindingSig = bindingBytes[:]
	assignment.BindingSigHash = binding.BigInt(new(big.Int)).String()
	proof, err := p.proveV2(assignment)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	envelope.Spends[0].Proof = proof
	data, err = core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	commitment := hashFromField(commitments[0])
	nullifier := hashFromField(fieldHash(shielded.DomainNullV2, recipients[0], randomness[0]))
	note := ShieldedNote{
		Version:        core.ShieldedTxVersionV2,
		ID:             "deposit-" + req.RequestID,
		Commitment:     commitment.Hex(),
		Nullifier:      nullifier.Hex(),
		NoteRandomness: randomness[0].BigInt(new(big.Int)).String(),
		NoteValueWei:   amountWei.String(),
		AssetID:        assetID,
		Status:         "pending",
		Source:         "deposit",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	created := []ShieldedNote(nil)
	if recipient == sender {
		created = []ShieldedNote{note}
	}
	return BuildShieldedResponse{
		Transaction:     makeUnsignedPQTransaction(chainID, nonce, gasPrice, p.cfg.GasLimit, amountWei, data),
		IntentHash:      intentHash.Hex(),
		CreatedNotes:    created,
		ShieldedVersion: core.ShieldedTxVersionV2,
		OutputOpenings: []ShieldedOutputOpening{{
			Index:      0,
			Recipient:  recipient.Hex(),
			AssetID:    assetID,
			ValueWei:   amountWei.String(),
			Randomness: randomness[0].BigInt(new(big.Int)).String(),
			Commitment: commitment.Hex(),
		}},
	}, nil
}

func (p *Prover) buildTransferV2(ctx context.Context, req BuildTransferRequest, amountWei, chainID *big.Int, nonce uint64, gasPrice *big.Int) (BuildShieldedResponse, error) {
	if p.pkV2 == nil || p.r1csV2 == nil {
		return BuildShieldedResponse{}, errors.New("recipient-bound V2 proving key is not ready")
	}
	recipientViewKey, err := parseViewPublicKey(req.RecipientViewKey)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	spendData, err := shieldedSpendData(req.RequestID, req.ApplicationData)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	changeViewKey, err := parseViewPublicKey(req.ChangeViewKey)
	if err != nil {
		return BuildShieldedResponse{}, fmt.Errorf("change %w", err)
	}
	noteValue, ok := parseDecimalBig(req.Note.NoteValueWei)
	if !ok || noteValue.Sign() <= 0 || noteValue.BitLen() > 64 || noteValue.Cmp(amountWei) < 0 {
		return BuildShieldedResponse{}, errors.New("input note is invalid or smaller than the transfer amount")
	}
	if err := validateMerkleWitness(req.Note.MerklePath, req.Note.MerklePathIndex); err != nil {
		return BuildShieldedResponse{}, err
	}
	assetID := strings.TrimSpace(req.Note.AssetID)
	if assetID == "" {
		assetID = "1"
	}
	asset, err := parseFieldElement(assetID)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	randomness, err := parseFieldElement(req.Note.NoteRandomness)
	if err != nil {
		return BuildShieldedResponse{}, fmt.Errorf("invalid note randomness: %w", err)
	}
	sender := common.HexToAddress(req.From)
	senderElement := fieldElementFromBig(sender.Big())
	gasSponsor, err := p.shieldedGasSponsorValue(ctx, chainID, sender, gasPrice)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	legacyInput := req.Note.Version < core.ShieldedTxVersionV2
	ownerSecret := "0"
	inputCommitment := fieldHash(shielded.DomainNoteV2, senderElement, asset, fieldElementFromBig(noteValue), randomness)
	nullifier := fieldHash(shielded.DomainNullV2, senderElement, randomness)
	if legacyInput {
		owner, err := parseFieldElement(req.Note.OwnerSecret)
		if err != nil || owner.Cmp(&senderElement) != 0 {
			return BuildShieldedResponse{}, errors.New("legacy V1 note is not owned by the sending PQ account")
		}
		ownerSecret = req.Note.OwnerSecret
		inputCommitment = fieldHash(shielded.DomainNote, owner, asset, fieldElementFromBig(noteValue), randomness)
		nullifier = fieldHash(shielded.DomainNull, owner, randomness)
	}
	anchor, err := computeAnchor(inputCommitment, req.Note.MerklePath, req.Note.MerklePathIndex)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	recipient := common.HexToAddress(req.To)
	requiredWei := new(big.Int).Add(amountWei, gasSponsor)
	if noteValue.Cmp(requiredWei) < 0 {
		return BuildShieldedResponse{}, fmt.Errorf("input note is smaller than transfer amount plus gas sponsorship (%s wei required)", requiredWei)
	}
	changeWei := new(big.Int).Sub(noteValue, requiredWei)
	recipients := [shielded.OutputSlots]fr.Element{fieldElementFromBig(recipient.Big()), senderElement, zeroElement(), zeroElement()}
	values := [shielded.OutputSlots]fr.Element{fieldElementFromBig(amountWei), fieldElementFromBig(changeWei), zeroElement(), zeroElement()}
	outputRandomness := [shielded.OutputSlots]fr.Element{randomElement(), randomElement(), randomElement(), randomElement()}
	var commitments [shielded.OutputSlots]fr.Element
	outputs := make([]core.ShieldedOutput, 0, shielded.OutputSlots)
	for i := 0; i < shielded.OutputSlots; i++ {
		commitments[i] = fieldHash(shielded.DomainNoteV2, recipients[i], asset, values[i], outputRandomness[i])
		commitment := hashFromField(commitments[i])
		switch {
		case i == 0:
			outputNullifier := hashFromField(fieldHash(shielded.DomainNullV2, recipients[i], outputRandomness[i]))
			output, err := encryptShieldedNote(commitment, noteOpeningV2(recipient, asset.BigInt(new(big.Int)), amountWei, outputRandomness[i].BigInt(new(big.Int)), commitment, outputNullifier), recipientViewKey)
			if err != nil {
				return BuildShieldedResponse{}, err
			}
			outputs = append(outputs, output)
		case i == 1 && changeWei.Sign() > 0:
			outputNullifier := hashFromField(fieldHash(shielded.DomainNullV2, recipients[i], outputRandomness[i]))
			output, err := encryptShieldedNote(commitment, noteOpeningV2(sender, asset.BigInt(new(big.Int)), changeWei, outputRandomness[i].BigInt(new(big.Int)), commitment, outputNullifier), changeViewKey)
			if err != nil {
				return BuildShieldedResponse{}, err
			}
			outputs = append(outputs, output)
		default:
			output, err := decoyShieldedOutput(commitment)
			if err != nil {
				return BuildShieldedResponse{}, err
			}
			outputs = append(outputs, output)
		}
	}
	outputRoot := hashFromField(fieldHash(shielded.DomainOutputV2, commitments[:]...))
	totalOutput := new(big.Int).Add(amountWei, changeWei)
	balance := fieldHash(shielded.DomainBalV2, fieldElementFromBig(noteValue), fieldElementFromBig(totalOutput), fieldElementFromBig(gasSponsor), asset, fieldElementFromHash(outputRoot))
	envelope := &core.ShieldedTransaction{
		Version: core.ShieldedTxVersionV2,
		Spends: []core.ShieldedSpend{{
			Nullifier:          hashFromField(nullifier),
			Anchor:             hashFromField(anchor),
			EncryptedSpendData: spendData,
		}},
		Outputs:           outputs,
		BalanceCommitment: hashFromField(balance),
		BindingSig:        make([]byte, common.HashLength),
		GasSponsorValue:   optionalPositiveBig(gasSponsor),
	}
	data, err := core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	unsigned := p.unsignedTx(chainID, nonce, gasPrice, data)
	intentHash, err := core.ShieldedTransactionIntentHash(unsigned, envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	legacyFlag := "0"
	if legacyInput {
		legacyFlag = "1"
	}
	assignment := newV2Assignment(chainID, sender, new(big.Int), intentHash, envelope.Spends[0].Nullifier, envelope.Spends[0].Anchor, envelope.BalanceCommitment, gasSponsor, outputRoot, legacyFlag, ownerSecret, noteValue.String(), assetID)
	assignment.NoteRandomness = req.Note.NoteRandomness
	fillCircuitArraysV2(assignment, req.Note.MerklePath, req.Note.MerklePathIndex, recipients, values, outputRandomness, commitments)
	binding := fieldHash(shielded.DomainBindV2,
		senderElement,
		randomness,
		nullifier,
		fieldElementFromHash(outputRoot),
		balance,
		fieldElementFromBig(chainID),
		fieldElementFromBytes(intentHash[:16]),
		fieldElementFromBytes(intentHash[16:]),
	)
	bindingBytes := binding.Bytes()
	envelope.BindingSig = bindingBytes[:]
	assignment.BindingSigHash = binding.BigInt(new(big.Int)).String()
	proof, err := p.proveV2(assignment)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	envelope.Spends[0].Proof = proof
	data, err = core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	created := make([]ShieldedNote, 0, 1)
	if changeWei.Sign() > 0 {
		created = append(created, ShieldedNote{
			Version:        core.ShieldedTxVersionV2,
			ID:             "change-" + req.RequestID,
			Commitment:     hashFromField(commitments[1]).Hex(),
			Nullifier:      hashFromField(fieldHash(shielded.DomainNullV2, recipients[1], outputRandomness[1])).Hex(),
			NoteRandomness: outputRandomness[1].BigInt(new(big.Int)).String(),
			NoteValueWei:   changeWei.String(),
			AssetID:        assetID,
			Status:         "pending",
			Source:         "change",
			CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	openings := []ShieldedOutputOpening{{
		Index:      0,
		Recipient:  recipient.Hex(),
		AssetID:    assetID,
		ValueWei:   amountWei.String(),
		Randomness: outputRandomness[0].BigInt(new(big.Int)).String(),
		Commitment: hashFromField(commitments[0]).Hex(),
	}}
	if changeWei.Sign() > 0 {
		openings = append(openings, ShieldedOutputOpening{
			Index:      1,
			Recipient:  sender.Hex(),
			AssetID:    assetID,
			ValueWei:   changeWei.String(),
			Randomness: outputRandomness[1].BigInt(new(big.Int)).String(),
			Commitment: hashFromField(commitments[1]).Hex(),
		})
	}
	return BuildShieldedResponse{
		Transaction:     makeUnsignedPQTransaction(chainID, nonce, gasPrice, p.cfg.GasLimit, new(big.Int), data),
		IntentHash:      intentHash.Hex(),
		SpentNullifier:  envelope.Spends[0].Nullifier.Hex(),
		CreatedNotes:    created,
		ShieldedVersion: core.ShieldedTxVersionV2,
		GasSponsorWei:   gasSponsor.String(),
		OutputOpenings:  openings,
	}, nil
}

func (p *Prover) buildWithdrawalV2(ctx context.Context, req BuildWithdrawalRequest, amountWei, chainID *big.Int, nonce uint64, gasPrice *big.Int) (BuildShieldedResponse, error) {
	spendData, err := shieldedSpendData(req.RequestID, req.ApplicationData)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	if p.pkV2 == nil || p.r1csV2 == nil {
		return BuildShieldedResponse{}, errors.New("recipient-bound V2 proving key is not ready")
	}
	changeViewKey, err := parseViewPublicKey(req.ChangeViewKey)
	if err != nil {
		return BuildShieldedResponse{}, fmt.Errorf("change %w", err)
	}
	noteValue, ok := parseDecimalBig(req.Note.NoteValueWei)
	if !ok || noteValue.Sign() <= 0 || noteValue.BitLen() > 64 || noteValue.Cmp(amountWei) < 0 {
		return BuildShieldedResponse{}, errors.New("input note is invalid or smaller than the withdrawal amount")
	}
	if err := validateMerkleWitness(req.Note.MerklePath, req.Note.MerklePathIndex); err != nil {
		return BuildShieldedResponse{}, err
	}
	assetID := strings.TrimSpace(req.Note.AssetID)
	if assetID == "" {
		assetID = "1"
	}
	asset, err := parseFieldElement(assetID)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	randomness, err := parseFieldElement(req.Note.NoteRandomness)
	if err != nil {
		return BuildShieldedResponse{}, fmt.Errorf("invalid note randomness: %w", err)
	}
	sender := common.HexToAddress(req.From)
	senderElement := fieldElementFromBig(sender.Big())
	gasSponsor, err := p.shieldedGasSponsorValue(ctx, chainID, sender, gasPrice)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	legacyInput := req.Note.Version < core.ShieldedTxVersionV2
	ownerSecret := "0"
	inputCommitment := fieldHash(shielded.DomainNoteV2, senderElement, asset, fieldElementFromBig(noteValue), randomness)
	nullifier := fieldHash(shielded.DomainNullV2, senderElement, randomness)
	if legacyInput {
		owner, err := parseFieldElement(req.Note.OwnerSecret)
		if err != nil || owner.Cmp(&senderElement) != 0 {
			return BuildShieldedResponse{}, errors.New("legacy V1 note is not owned by the withdrawing PQ account")
		}
		ownerSecret = req.Note.OwnerSecret
		inputCommitment = fieldHash(shielded.DomainNote, owner, asset, fieldElementFromBig(noteValue), randomness)
		nullifier = fieldHash(shielded.DomainNull, owner, randomness)
	}
	anchor, err := computeAnchor(inputCommitment, req.Note.MerklePath, req.Note.MerklePathIndex)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	publicValue := new(big.Int).Add(amountWei, gasSponsor)
	if noteValue.Cmp(publicValue) < 0 {
		return BuildShieldedResponse{}, fmt.Errorf("input note is smaller than withdrawal amount plus gas sponsorship (%s wei required)", publicValue)
	}
	changeWei := new(big.Int).Sub(noteValue, publicValue)
	recipients := [shielded.OutputSlots]fr.Element{senderElement, zeroElement(), zeroElement(), zeroElement()}
	values := [shielded.OutputSlots]fr.Element{fieldElementFromBig(changeWei), zeroElement(), zeroElement(), zeroElement()}
	outputRandomness := [shielded.OutputSlots]fr.Element{randomElement(), randomElement(), randomElement(), randomElement()}
	var commitments [shielded.OutputSlots]fr.Element
	outputs := make([]core.ShieldedOutput, 0, shielded.OutputSlots)
	for i := 0; i < shielded.OutputSlots; i++ {
		commitments[i] = fieldHash(shielded.DomainNoteV2, recipients[i], asset, values[i], outputRandomness[i])
		commitment := hashFromField(commitments[i])
		if i == 0 && changeWei.Sign() > 0 {
			outputNullifier := hashFromField(fieldHash(shielded.DomainNullV2, recipients[i], outputRandomness[i]))
			output, err := encryptShieldedNote(commitment, noteOpeningV2(sender, asset.BigInt(new(big.Int)), changeWei, outputRandomness[i].BigInt(new(big.Int)), commitment, outputNullifier), changeViewKey)
			if err != nil {
				return BuildShieldedResponse{}, err
			}
			outputs = append(outputs, output)
			continue
		}
		output, err := decoyShieldedOutput(commitment)
		if err != nil {
			return BuildShieldedResponse{}, err
		}
		outputs = append(outputs, output)
	}
	outputRoot := hashFromField(fieldHash(shielded.DomainOutputV2, commitments[:]...))
	balance := fieldHash(shielded.DomainBalV2, fieldElementFromBig(noteValue), fieldElementFromBig(changeWei), fieldElementFromBig(publicValue), asset, fieldElementFromHash(outputRoot))
	envelope := &core.ShieldedTransaction{
		Version: core.ShieldedTxVersionV2,
		Spends: []core.ShieldedSpend{{
			Nullifier:          hashFromField(nullifier),
			Anchor:             hashFromField(anchor),
			EncryptedSpendData: spendData,
		}},
		Outputs:             outputs,
		BalanceCommitment:   hashFromField(balance),
		BindingSig:          make([]byte, common.HashLength),
		WithdrawalRecipient: common.HexToAddress(req.To),
		WithdrawalValue:     new(big.Int).Set(amountWei),
		GasSponsorValue:     optionalPositiveBig(gasSponsor),
	}
	data, err := core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	unsigned := p.unsignedTx(chainID, nonce, gasPrice, data)
	intentHash, err := core.ShieldedTransactionIntentHash(unsigned, envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	legacyFlag := "0"
	if legacyInput {
		legacyFlag = "1"
	}
	assignment := newV2Assignment(chainID, sender, new(big.Int), intentHash, envelope.Spends[0].Nullifier, envelope.Spends[0].Anchor, envelope.BalanceCommitment, publicValue, outputRoot, legacyFlag, ownerSecret, noteValue.String(), assetID)
	assignment.NoteRandomness = req.Note.NoteRandomness
	fillCircuitArraysV2(assignment, req.Note.MerklePath, req.Note.MerklePathIndex, recipients, values, outputRandomness, commitments)
	binding := fieldHash(shielded.DomainBindV2,
		senderElement,
		randomness,
		nullifier,
		fieldElementFromHash(outputRoot),
		balance,
		fieldElementFromBig(chainID),
		fieldElementFromBytes(intentHash[:16]),
		fieldElementFromBytes(intentHash[16:]),
	)
	bindingBytes := binding.Bytes()
	envelope.BindingSig = bindingBytes[:]
	assignment.BindingSigHash = binding.BigInt(new(big.Int)).String()
	proof, err := p.proveV2(assignment)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	envelope.Spends[0].Proof = proof
	data, err = core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	created := make([]ShieldedNote, 0, 1)
	openings := make([]ShieldedOutputOpening, 0, 1)
	if changeWei.Sign() > 0 {
		changeCommitment := hashFromField(commitments[0])
		created = append(created, ShieldedNote{
			Version:        core.ShieldedTxVersionV2,
			ID:             "withdrawal-change-" + req.RequestID,
			Commitment:     changeCommitment.Hex(),
			Nullifier:      hashFromField(fieldHash(shielded.DomainNullV2, recipients[0], outputRandomness[0])).Hex(),
			NoteRandomness: outputRandomness[0].BigInt(new(big.Int)).String(),
			NoteValueWei:   changeWei.String(),
			AssetID:        assetID,
			Status:         "pending",
			Source:         "withdrawal-change",
			CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		})
		openings = append(openings, ShieldedOutputOpening{
			Index:      0,
			Recipient:  sender.Hex(),
			AssetID:    assetID,
			ValueWei:   changeWei.String(),
			Randomness: outputRandomness[0].BigInt(new(big.Int)).String(),
			Commitment: changeCommitment.Hex(),
		})
	}
	return BuildShieldedResponse{
		Transaction:     makeUnsignedPQTransaction(chainID, nonce, gasPrice, p.cfg.GasLimit, new(big.Int), data),
		IntentHash:      intentHash.Hex(),
		SpentNullifier:  envelope.Spends[0].Nullifier.Hex(),
		CreatedNotes:    created,
		ShieldedVersion: core.ShieldedTxVersionV2,
		GasSponsorWei:   gasSponsor.String(),
		OutputOpenings:  openings,
	}, nil
}

func optionalPositiveBig(value *big.Int) *big.Int {
	if value == nil || value.Sign() <= 0 {
		return nil
	}
	return new(big.Int).Set(value)
}

func newV2Assignment(chainID *big.Int, sender common.Address, blockNumber *big.Int, intentHash common.Hash, nullifier, anchor, balance common.Hash, publicValue *big.Int, outputRoot common.Hash, legacyInput, ownerSecret, noteValue, assetID string) *shielded.SpendCircuitV2 {
	return &shielded.SpendCircuitV2{
		ChainID:           chainID.String(),
		BlockNumber:       blockNumber.String(),
		TxHashHi:          new(big.Int).SetBytes(intentHash[:16]).String(),
		TxHashLo:          new(big.Int).SetBytes(intentHash[16:]).String(),
		SpendIndex:        "0",
		Nullifier:         nullifier.Big().String(),
		Anchor:            anchor.Big().String(),
		BalanceCommitment: balance.Big().String(),
		PublicValue:       publicValue.String(),
		OutputRoot:        outputRoot.Big().String(),
		BindingSigHash:    "0",
		SenderAddress:     sender.Big().String(),
		LegacyInput:       legacyInput,
		OwnerSecret:       ownerSecret,
		NoteRandomness:    "0",
		NoteValue:         noteValue,
		AssetID:           assetID,
	}
}

func fillCircuitArraysV2(c *shielded.SpendCircuitV2, path, pathIndex []string, recipients, values, randomness, commitments [shielded.OutputSlots]fr.Element) {
	for i := 0; i < shielded.MerkleDepth; i++ {
		if i < len(path) && strings.TrimSpace(path[i]) != "" {
			c.MerklePath[i] = path[i]
		} else {
			c.MerklePath[i] = "0"
		}
		if i < len(pathIndex) && strings.TrimSpace(pathIndex[i]) != "" {
			c.MerklePathIndex[i] = pathIndex[i]
		} else {
			c.MerklePathIndex[i] = "0"
		}
	}
	for i := 0; i < shielded.OutputSlots; i++ {
		c.OutputRecipient[i] = recipients[i].BigInt(new(big.Int)).String()
		c.OutputValue[i] = values[i].BigInt(new(big.Int)).String()
		c.OutputRandomness[i] = randomness[i].BigInt(new(big.Int)).String()
		c.OutputCommitment[i] = commitments[i].BigInt(new(big.Int)).String()
	}
}
