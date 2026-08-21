package shielded

import (
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
)

// DeterministicV2Assignment returns a valid, non-secret witness used to verify
// that V2 ceremony artifacts match the compiled recipient-bound circuit.
func DeterministicV2Assignment() *SpendCircuitV2 {
	sender := v2Elem(11)
	asset := v2Elem(1)
	noteValue := v2Elem(100)
	noteRandomness := v2Elem(22)
	inputCommitment := v2Hash(DomainNoteV2, sender, asset, noteValue, noteRandomness)
	nullifier := v2Hash(DomainNullV2, sender, noteRandomness)
	path := make([]fr.Element, MerkleDepth)
	index := make([]fr.Element, MerkleDepth)
	anchor := inputCommitment
	for i := 0; i < MerkleDepth; i++ {
		anchor = v2Hash(DomainNode, anchor, path[i])
	}

	recipients := [OutputSlots]fr.Element{v2Elem(22), sender, v2Elem(0), v2Elem(0)}
	values := [OutputSlots]fr.Element{v2Elem(70), v2Elem(30), v2Elem(0), v2Elem(0)}
	randomness := [OutputSlots]fr.Element{v2Elem(41), v2Elem(42), v2Elem(43), v2Elem(44)}
	var commitments [OutputSlots]fr.Element
	for i := range commitments {
		commitments[i] = v2Hash(DomainNoteV2, recipients[i], asset, values[i], randomness[i])
	}
	outputRoot := v2Hash(DomainOutputV2, commitments[:]...)
	balance := v2Hash(DomainBalV2, noteValue, noteValue, v2Elem(0), asset, outputRoot)
	binding := v2Hash(DomainBindV2, sender, noteRandomness, nullifier, outputRoot, balance, v2Elem(8979), v2Elem(0), v2Elem(555))

	c := &SpendCircuitV2{
		ChainID:           "8979",
		BlockNumber:       "0",
		TxHashHi:          "0",
		TxHashLo:          "555",
		SpendIndex:        "0",
		Nullifier:         v2String(nullifier),
		Anchor:            v2String(anchor),
		BalanceCommitment: v2String(balance),
		PublicValue:       "0",
		OutputRoot:        v2String(outputRoot),
		BindingSigHash:    v2String(binding),
		SenderAddress:     v2String(sender),
		LegacyInput:       "0",
		OwnerSecret:       v2String(sender),
		NoteRandomness:    v2String(noteRandomness),
		NoteValue:         v2String(noteValue),
		AssetID:           v2String(asset),
	}
	for i := 0; i < MerkleDepth; i++ {
		c.MerklePath[i] = v2String(path[i])
		c.MerklePathIndex[i] = v2String(index[i])
	}
	for i := 0; i < OutputSlots; i++ {
		c.OutputRecipient[i] = v2String(recipients[i])
		c.OutputValue[i] = v2String(values[i])
		c.OutputRandomness[i] = v2String(randomness[i])
		c.OutputCommitment[i] = v2String(commitments[i])
	}
	return c
}

func v2Elem(value uint64) fr.Element {
	var out fr.Element
	out.SetUint64(value)
	return out
}

func v2Hash(domain uint64, inputs ...fr.Element) fr.Element {
	h := mimc.NewMiMC()
	domainElement := v2Elem(domain)
	d := domainElement.Bytes()
	_, _ = h.Write(d[:])
	for i := range inputs {
		encoded := inputs[i].Bytes()
		_, _ = h.Write(encoded[:])
	}
	var out fr.Element
	out.SetBytes(h.Sum(nil))
	return out
}

func v2String(value fr.Element) string {
	return value.BigInt(new(big.Int)).String()
}
