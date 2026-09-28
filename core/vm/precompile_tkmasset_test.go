package vm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tkmasset"
	"github.com/ethereum/go-ethereum/params"
)

func TestTKMAssetIDPrecompile(t *testing.T) {
	m := tkmasset.Manifest{Kind: tkmasset.KindNonFungible, ChainID: big.NewInt(8979), Name: "TKM Collectible", Symbol: "TKMNFT", MetadataURI: "ipfs://tkm/nft.json"}
	hash, err := m.ManifestHash()
	if err != nil {
		t.Fatal(err)
	}
	contract := common.HexToAddress("0x1234567890123456789012345678901234567890")
	input, err := tkmasset.PrecompileInput(m.ChainID, contract, m.Kind, hash)
	if err != nil {
		t.Fatal(err)
	}
	want, err := tkmasset.AssetIDFromPrecompileInput(input)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := RunPrecompiledContract(nil, &tkmAssetIDPrecompile{}, TKMAssetIDPrecompileAddr, input, NewGasBudget(100_000), nil, params.Rules{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if common.BytesToHash(got) != want {
		t.Fatalf("asset ID = %x, want %x", got, want)
	}
	if _, _, err := RunPrecompiledContract(nil, &tkmAssetIDPrecompile{}, TKMAssetIDPrecompileAddr, input[:len(input)-1], NewGasBudget(100_000), nil, params.Rules{}, true); err == nil {
		t.Fatal("malformed asset ID input accepted")
	}
}

func TestTKMAssetIDPrecompileActivatesAtCancunBoundary(t *testing.T) {
	if _, ok := ActivePrecompiledContracts(params.Rules{IsBerlin: true})[TKMAssetIDPrecompileAddr]; ok {
		t.Fatal("TKM asset identity precompile active before Cancun")
	}
	if _, ok := ActivePrecompiledContracts(params.Rules{IsCancun: true})[TKMAssetIDPrecompileAddr]; !ok {
		t.Fatal("TKM asset identity precompile missing at Cancun")
	}
}
