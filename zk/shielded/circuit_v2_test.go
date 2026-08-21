package shielded

import (
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/test"
)

func TestSpendCircuitV2BindsInputToSender(t *testing.T) {
	assert := test.NewAssert(t)
	valid := deterministicV2Spend(false)
	assert.ProverSucceeded(&SpendCircuitV2{}, valid, test.WithCurves(ecc.BN254))

	wrongSender := *valid
	wrongSender.SenderAddress = "22"
	assert.ProverFailed(&SpendCircuitV2{}, &wrongSender, test.WithCurves(ecc.BN254))
}

func TestSpendCircuitV2MigratesOnlySenderOwnedV1Note(t *testing.T) {
	assert := test.NewAssert(t)
	valid := deterministicV2Spend(true)
	assert.ProverSucceeded(&SpendCircuitV2{}, valid, test.WithCurves(ecc.BN254))

	wrongOwner := *valid
	wrongOwner.OwnerSecret = "22"
	assert.ProverFailed(&SpendCircuitV2{}, &wrongOwner, test.WithCurves(ecc.BN254))
}

func TestSpendCircuitV2Compiles(t *testing.T) {
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &SpendCircuitV2{})
	if err != nil {
		t.Fatal(err)
	}
	if ccs.GetNbConstraints() == 0 {
		t.Fatal("shielded V2 spend circuit compiled with no constraints")
	}
}

func deterministicV2Spend(legacy bool) *SpendCircuitV2 {
	sender := elem(11)
	owner := sender
	asset := elem(1)
	noteValue := elem(100)
	noteRandomness := elem(22)
	legacyFlag := elem(0)
	inputCommitment := hash(DomainNoteV2, sender, asset, noteValue, noteRandomness)
	nullifier := hash(DomainNullV2, sender, noteRandomness)
	if legacy {
		legacyFlag = elem(1)
		inputCommitment = hash(DomainNote, owner, asset, noteValue, noteRandomness)
		nullifier = hash(DomainNull, owner, noteRandomness)
	}
	path := make([]fr.Element, MerkleDepth)
	index := make([]fr.Element, MerkleDepth)
	anchor := inputCommitment
	for i := 0; i < MerkleDepth; i++ {
		anchor = hash(DomainNode, anchor, path[i])
	}

	recipients := [OutputSlots]fr.Element{elem(22), sender, elem(0), elem(0)}
	values := [OutputSlots]fr.Element{elem(70), elem(30), elem(0), elem(0)}
	randomness := [OutputSlots]fr.Element{elem(41), elem(42), elem(43), elem(44)}
	var commitments [OutputSlots]fr.Element
	for i := range commitments {
		commitments[i] = hash(DomainNoteV2, recipients[i], asset, values[i], randomness[i])
	}
	outputRoot := hash(DomainOutputV2, commitments[:]...)
	balance := hash(DomainBalV2, noteValue, noteValue, elem(0), asset, outputRoot)
	binding := hash(DomainBindV2, sender, noteRandomness, nullifier, outputRoot, balance, elem(8979), elem(0), elem(555))

	c := &SpendCircuitV2{
		ChainID:           "8979",
		BlockNumber:       "0",
		TxHashHi:          "0",
		TxHashLo:          "555",
		SpendIndex:        "0",
		Nullifier:         str(nullifier),
		Anchor:            str(anchor),
		BalanceCommitment: str(balance),
		PublicValue:       "0",
		OutputRoot:        str(outputRoot),
		BindingSigHash:    str(binding),
		SenderAddress:     str(sender),
		LegacyInput:       str(legacyFlag),
		OwnerSecret:       str(owner),
		NoteRandomness:    str(noteRandomness),
		NoteValue:         str(noteValue),
		AssetID:           str(asset),
	}
	for i := 0; i < MerkleDepth; i++ {
		c.MerklePath[i] = str(path[i])
		c.MerklePathIndex[i] = str(index[i])
	}
	for i := 0; i < OutputSlots; i++ {
		c.OutputRecipient[i] = str(recipients[i])
		c.OutputValue[i] = str(values[i])
		c.OutputRandomness[i] = str(randomness[i])
		c.OutputCommitment[i] = str(commitments[i])
	}
	return c
}
