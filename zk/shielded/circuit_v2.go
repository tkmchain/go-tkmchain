package shielded

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
)

const (
	CircuitNameV2  = "TKM_SHIELDED_SPEND_V2"
	DomainNoteV2   = 2001
	DomainNullV2   = 2003
	DomainOutputV2 = 2004
	DomainBalV2    = 2005
	DomainBindV2   = 2006
)

// SpendCircuitV2 binds every private note to the PQ account that must sign the
// spending transaction. SenderAddress is supplied by consensus from the
// verified transaction signature; it is never accepted from envelope data.
//
// LegacyInput permits a V1 self-note to migrate once into V2. Such a migration
// is valid only when the V1 owner secret equals SenderAddress. All outputs use
// the V2 commitment domain and therefore cannot be spent by the V1 circuit.
type SpendCircuitV2 struct {
	ChainID           frontend.Variable `gnark:",public"`
	BlockNumber       frontend.Variable `gnark:",public"`
	TxHashHi          frontend.Variable `gnark:",public"`
	TxHashLo          frontend.Variable `gnark:",public"`
	SpendIndex        frontend.Variable `gnark:",public"`
	Nullifier         frontend.Variable `gnark:",public"`
	Anchor            frontend.Variable `gnark:",public"`
	BalanceCommitment frontend.Variable `gnark:",public"`
	PublicValue       frontend.Variable `gnark:",public"`
	OutputRoot        frontend.Variable `gnark:",public"`
	BindingSigHash    frontend.Variable `gnark:",public"`
	SenderAddress     frontend.Variable `gnark:",public"`

	LegacyInput    frontend.Variable
	OwnerSecret    frontend.Variable
	NoteRandomness frontend.Variable
	NoteValue      frontend.Variable
	AssetID        frontend.Variable

	MerklePath      [MerkleDepth]frontend.Variable
	MerklePathIndex [MerkleDepth]frontend.Variable

	OutputRecipient  [OutputSlots]frontend.Variable
	OutputValue      [OutputSlots]frontend.Variable
	OutputRandomness [OutputSlots]frontend.Variable
	OutputCommitment [OutputSlots]frontend.Variable
}

func (c *SpendCircuitV2) Define(api frontend.API) error {
	api.ToBinary(c.ChainID, 64)
	api.ToBinary(c.BlockNumber, 64)
	api.ToBinary(c.TxHashHi, 128)
	api.ToBinary(c.TxHashLo, 128)
	api.ToBinary(c.SpendIndex, 64)
	api.ToBinary(c.NoteValue, 64)
	api.ToBinary(c.PublicValue, 64)
	api.ToBinary(c.AssetID, 64)
	api.ToBinary(c.SenderAddress, 160)
	api.AssertIsBoolean(c.LegacyInput)

	isDeposit := api.IsZero(c.Nullifier)
	api.AssertIsEqual(isDeposit, api.IsZero(c.Anchor))
	// Deposits have no legacy input to migrate.
	api.AssertIsEqual(api.Mul(isDeposit, c.LegacyInput), 0)
	// A V1 note was committed directly to its owner's address. Binding this
	// witness to the recovered sender prevents a public prover from migrating it.
	api.AssertIsEqual(api.Mul(c.LegacyInput, api.Sub(c.OwnerSecret, c.SenderAddress)), 0)

	legacyCommitment := c.hash(api, DomainNote, c.OwnerSecret, c.AssetID, c.NoteValue, c.NoteRandomness)
	v2Commitment := c.hash(api, DomainNoteV2, c.SenderAddress, c.AssetID, c.NoteValue, c.NoteRandomness)
	inputCommitment := api.Select(c.LegacyInput, legacyCommitment, v2Commitment)
	root := inputCommitment
	for i := 0; i < MerkleDepth; i++ {
		api.AssertIsBoolean(c.MerklePathIndex[i])
		left := api.Select(c.MerklePathIndex[i], c.MerklePath[i], root)
		right := api.Select(c.MerklePathIndex[i], root, c.MerklePath[i])
		root = c.hash(api, DomainNode, left, right)
	}
	api.AssertIsEqual(api.Select(isDeposit, 0, c.Anchor), api.Select(isDeposit, 0, root))

	legacyNullifier := c.hash(api, DomainNull, c.OwnerSecret, c.NoteRandomness)
	v2Nullifier := c.hash(api, DomainNullV2, c.SenderAddress, c.NoteRandomness)
	nullifier := api.Select(c.LegacyInput, legacyNullifier, v2Nullifier)
	api.AssertIsEqual(api.Select(isDeposit, 0, c.Nullifier), api.Select(isDeposit, 0, nullifier))

	totalOutput := frontend.Variable(0)
	for i := 0; i < OutputSlots; i++ {
		api.ToBinary(c.OutputRecipient[i], 160)
		api.ToBinary(c.OutputValue[i], 64)
		commitment := c.hash(api, DomainNoteV2, c.OutputRecipient[i], c.AssetID, c.OutputValue[i], c.OutputRandomness[i])
		api.AssertIsEqual(c.OutputCommitment[i], commitment)
		totalOutput = api.Add(totalOutput, c.OutputValue[i])
	}
	api.AssertIsEqual(c.NoteValue, api.Select(isDeposit, 0, api.Add(totalOutput, c.PublicValue)))
	api.AssertIsEqual(c.PublicValue, api.Select(isDeposit, totalOutput, c.PublicValue))

	outputRoot := c.hash(api, DomainOutputV2, c.OutputCommitment[:]...)
	api.AssertIsEqual(c.OutputRoot, outputRoot)

	balance := c.hash(api, DomainBalV2, c.NoteValue, totalOutput, c.PublicValue, c.AssetID, outputRoot)
	api.AssertIsEqual(c.BalanceCommitment, balance)

	binding := c.hash(api, DomainBindV2, c.SenderAddress, c.NoteRandomness, c.Nullifier, outputRoot, balance, c.ChainID, c.TxHashHi, c.TxHashLo)
	api.AssertIsEqual(c.BindingSigHash, binding)
	return nil
}

func (c *SpendCircuitV2) hash(api frontend.API, domain frontend.Variable, inputs ...frontend.Variable) frontend.Variable {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		panic(err)
	}
	h.Write(domain)
	h.Write(inputs...)
	return h.Sum()
}
