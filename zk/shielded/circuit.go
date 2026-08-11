package shielded

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
)

const (
	CircuitName  = "TKM_SHIELDED_SPEND_V1"
	MerkleDepth  = 32
	OutputSlots  = 4
	DomainNote   = 1001
	DomainNode   = 1002
	DomainNull   = 1003
	DomainOutput = 1004
	DomainBal    = 1005
	DomainBind   = 1006
)

// SpendCircuit proves either one private note spend or one transparent-to-shielded
// deposit into a fixed padded output set.
//
// Deposit mode is selected by setting Nullifier and Anchor to zero. In deposit
// mode, NoteValue must be zero and PublicValue must equal the sum of private
// outputs. Spend mode keeps the original equation:
//
//	NoteValue = sum(outputs) + PublicValue
type SpendCircuit struct {
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

func (c *SpendCircuit) Define(api frontend.API) error {
	api.ToBinary(c.ChainID, 64)
	api.ToBinary(c.BlockNumber, 64)
	api.ToBinary(c.TxHashHi, 128)
	api.ToBinary(c.TxHashLo, 128)
	api.ToBinary(c.SpendIndex, 64)
	api.ToBinary(c.NoteValue, 64)
	api.ToBinary(c.PublicValue, 64)
	api.ToBinary(c.AssetID, 64)

	isDeposit := api.IsZero(c.Nullifier)
	api.AssertIsEqual(isDeposit, api.IsZero(c.Anchor))

	inputCommitment := c.hash(api, DomainNote, c.OwnerSecret, c.AssetID, c.NoteValue, c.NoteRandomness)
	root := inputCommitment
	for i := 0; i < MerkleDepth; i++ {
		api.AssertIsBoolean(c.MerklePathIndex[i])
		left := api.Select(c.MerklePathIndex[i], c.MerklePath[i], root)
		right := api.Select(c.MerklePathIndex[i], root, c.MerklePath[i])
		root = c.hash(api, DomainNode, left, right)
	}
	api.AssertIsEqual(api.Select(isDeposit, 0, c.Anchor), api.Select(isDeposit, 0, root))

	nullifier := c.hash(api, DomainNull, c.OwnerSecret, c.NoteRandomness)
	api.AssertIsEqual(api.Select(isDeposit, 0, c.Nullifier), api.Select(isDeposit, 0, nullifier))

	totalOutput := frontend.Variable(0)
	for i := 0; i < OutputSlots; i++ {
		api.ToBinary(c.OutputValue[i], 64)
		commitment := c.hash(api, DomainNote, c.OutputRecipient[i], c.AssetID, c.OutputValue[i], c.OutputRandomness[i])
		api.AssertIsEqual(c.OutputCommitment[i], commitment)
		totalOutput = api.Add(totalOutput, c.OutputValue[i])
	}
	api.AssertIsEqual(c.NoteValue, api.Select(isDeposit, 0, api.Add(totalOutput, c.PublicValue)))
	api.AssertIsEqual(c.PublicValue, api.Select(isDeposit, totalOutput, c.PublicValue))

	outputRoot := c.hash(api, DomainOutput, c.OutputCommitment[:]...)
	api.AssertIsEqual(c.OutputRoot, outputRoot)

	balance := c.hash(api, DomainBal, c.NoteValue, totalOutput, c.PublicValue, c.AssetID, outputRoot)
	api.AssertIsEqual(c.BalanceCommitment, balance)

	binding := c.hash(api, DomainBind, c.OwnerSecret, c.Nullifier, outputRoot, balance, c.ChainID, c.TxHashHi, c.TxHashLo)
	api.AssertIsEqual(c.BindingSigHash, binding)
	return nil
}

func (c *SpendCircuit) hash(api frontend.API, domain frontend.Variable, inputs ...frontend.Variable) frontend.Variable {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		panic(err)
	}
	h.Write(domain)
	h.Write(inputs...)
	return h.Sum()
}
