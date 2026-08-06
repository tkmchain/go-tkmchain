package shielded

import (
	"encoding/json"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
)

type TestVectors struct {
	Circuit string     `json:"circuit"`
	Valid   VectorCase `json:"valid"`
	Invalid []BadCase  `json:"invalid"`
}

type BadCase struct {
	Name string     `json:"name"`
	Case VectorCase `json:"case"`
}

type VectorCase struct {
	Public  PublicInputs  `json:"public"`
	Private PrivateInputs `json:"private"`
}

type PublicInputs struct {
	ChainID           string `json:"chainId"`
	BlockNumber       string `json:"blockNumber"`
	TxHashHi          string `json:"txHashHi"`
	TxHashLo          string `json:"txHashLo"`
	SpendIndex        string `json:"spendIndex"`
	Nullifier         string `json:"nullifier"`
	Anchor            string `json:"anchor"`
	BalanceCommitment string `json:"balanceCommitment"`
	PublicValue       string `json:"publicValue"`
	OutputRoot        string `json:"outputRoot"`
	BindingSigHash    string `json:"bindingSigHash"`
}

type PrivateInputs struct {
	OwnerSecret      string   `json:"ownerSecret"`
	NoteRandomness   string   `json:"noteRandomness"`
	NoteValue        string   `json:"noteValue"`
	AssetID          string   `json:"assetId"`
	MerklePath       []string `json:"merklePath"`
	MerklePathIndex  []string `json:"merklePathIndex"`
	OutputRecipient  []string `json:"outputRecipient"`
	OutputValue      []string `json:"outputValue"`
	OutputRandomness []string `json:"outputRandomness"`
	OutputCommitment []string `json:"outputCommitment"`
}

func DeterministicTestVectors() TestVectors {
	valid := deterministicValidCase()
	badNullifier := cloneVectorCase(valid)
	badNullifier.Public.Nullifier = "999"
	badBalance := cloneVectorCase(valid)
	badBalance.Private.OutputValue[0] = "41"
	badAnchor := cloneVectorCase(valid)
	badAnchor.Public.Anchor = "123456"
	return TestVectors{
		Circuit: CircuitName,
		Valid:   valid,
		Invalid: []BadCase{
			{Name: "wrong-nullifier", Case: badNullifier},
			{Name: "unbalanced-output-value", Case: badBalance},
			{Name: "wrong-anchor", Case: badAnchor},
		},
	}
}

func cloneVectorCase(in VectorCase) VectorCase {
	out := in
	out.Private.MerklePath = append([]string(nil), in.Private.MerklePath...)
	out.Private.MerklePathIndex = append([]string(nil), in.Private.MerklePathIndex...)
	out.Private.OutputRecipient = append([]string(nil), in.Private.OutputRecipient...)
	out.Private.OutputValue = append([]string(nil), in.Private.OutputValue...)
	out.Private.OutputRandomness = append([]string(nil), in.Private.OutputRandomness...)
	out.Private.OutputCommitment = append([]string(nil), in.Private.OutputCommitment...)
	return out
}

func DeterministicTestVectorsJSON() ([]byte, error) {
	return json.MarshalIndent(DeterministicTestVectors(), "", "  ")
}

func deterministicValidCase() VectorCase {
	owner := elem(11)
	asset := elem(1)
	noteValue := elem(100)
	noteRandomness := elem(22)
	inputCommitment := hash(DomainNote, owner, asset, noteValue, noteRandomness)

	path := make([]fr.Element, MerkleDepth)
	index := make([]fr.Element, MerkleDepth)
	root := inputCommitment
	for i := 0; i < MerkleDepth; i++ {
		path[i] = elem(uint64(100 + i))
		if i%2 == 0 {
			index[i] = elem(0)
			root = hash(DomainNode, root, path[i])
		} else {
			index[i] = elem(1)
			root = hash(DomainNode, path[i], root)
		}
	}

	recipients := [OutputSlots]fr.Element{elem(31), elem(32), elem(33), elem(34)}
	values := [OutputSlots]fr.Element{elem(70), elem(30), elem(0), elem(0)}
	randomness := [OutputSlots]fr.Element{elem(41), elem(42), elem(43), elem(44)}
	var commitments [OutputSlots]fr.Element
	for i := 0; i < OutputSlots; i++ {
		commitments[i] = hash(DomainNote, recipients[i], asset, values[i], randomness[i])
	}
	outputRoot := hash(DomainOutput, commitments[:]...)
	publicValue := elem(0)
	totalOutput := elem(100)
	balance := hash(DomainBal, noteValue, totalOutput, publicValue, asset, outputRoot)
	chainID := elem(8979)
	blockNumber := elem(1)
	txHashHi := elem(0)
	txHashLo := elem(555)
	spendIndex := elem(0)
	nullifier := hash(DomainNull, owner, noteRandomness)
	binding := hash(DomainBind, owner, nullifier, outputRoot, balance, chainID, txHashHi, txHashLo)

	return VectorCase{
		Public: PublicInputs{
			ChainID:           str(chainID),
			BlockNumber:       str(blockNumber),
			TxHashHi:          str(txHashHi),
			TxHashLo:          str(txHashLo),
			SpendIndex:        str(spendIndex),
			Nullifier:         str(nullifier),
			Anchor:            str(root),
			BalanceCommitment: str(balance),
			PublicValue:       str(publicValue),
			OutputRoot:        str(outputRoot),
			BindingSigHash:    str(binding),
		},
		Private: PrivateInputs{
			OwnerSecret:      str(owner),
			NoteRandomness:   str(noteRandomness),
			NoteValue:        str(noteValue),
			AssetID:          str(asset),
			MerklePath:       strSlice(path[:]),
			MerklePathIndex:  strSlice(index[:]),
			OutputRecipient:  strSlice(recipients[:]),
			OutputValue:      strSlice(values[:]),
			OutputRandomness: strSlice(randomness[:]),
			OutputCommitment: strSlice(commitments[:]),
		},
	}
}

func hash(domain uint64, inputs ...fr.Element) fr.Element {
	h := mimc.NewMiMC()
	domainElement := elem(domain)
	domainBytes := domainElement.Bytes()
	h.Write(domainBytes[:])
	for _, input := range inputs {
		inputBytes := input.Bytes()
		h.Write(inputBytes[:])
	}
	sum := h.Sum(nil)
	var out fr.Element
	if err := out.SetBytesCanonical(sum); err != nil {
		panic(err)
	}
	return out
}

func elem(v uint64) fr.Element {
	var out fr.Element
	out.SetUint64(v)
	return out
}

func str(v fr.Element) string {
	return v.BigInt(new(big.Int)).String()
}

func strSlice(values []fr.Element) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = str(value)
	}
	return out
}
