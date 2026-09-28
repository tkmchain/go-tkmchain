package eth

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tkmasset"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/holiman/uint256"
)

type tkmAssetTestBackend struct {
	state  *state.StateDB
	config *params.ChainConfig
}

func (b tkmAssetTestBackend) StateAndHeaderByNumberOrHash(context.Context, rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	return b.state, &types.Header{Root: types.EmptyRootHash}, nil
}

func (b tkmAssetTestBackend) ChainConfig() *params.ChainConfig { return b.config }

func TestTKMAssetAPIClassifiesNativeAndEthereumCode(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")
	manifest := tkmasset.Manifest{Kind: tkmasset.KindMulti, ChainID: big.NewInt(8979), Name: "TKM Items", Symbol: "ITEM", MetadataURI: "ipfs://tkm/items.json"}
	code, err := tkmasset.AppendTrailer([]byte{0x60, 0x00, 0x56}, manifest)
	if err != nil {
		t.Fatal(err)
	}
	st.SetCode(address, code, tracing.CodeChangeContractCreation)
	st.SetBalance(address, uint256.NewInt(1), tracing.BalanceChangeUnspecified)
	api := NewTKMAssetAPI(tkmAssetTestBackend{state: st, config: params.MainnetChainConfig})
	info, err := api.GetAsset(context.Background(), address, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Native || info.Standard != "TKM-6909" || info.AssetID == (common.Hash{}) {
		t.Fatalf("unexpected native asset info: %+v", info)
	}
	plain := common.HexToAddress("0x9876543210987654321098765432109876543210")
	st.SetCode(plain, []byte{0x60, 0x00, 0x56}, tracing.CodeChangeContractCreation)
	plainInfo, err := api.GetAsset(context.Background(), plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plainInfo.Native || plainInfo.Network != "ethereum-compatible" {
		t.Fatalf("plain contract classified as native: %+v", plainInfo)
	}
}

func TestTKMAssetAPIMismatchedChainIsNotNative(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")
	manifest := tkmasset.Manifest{Kind: tkmasset.KindFungible, ChainID: big.NewInt(8980), Decimals: 18, Name: "Egypt TKM", Symbol: "ETKM", MetadataURI: "ipfs://tkm/egypt.json"}
	code, err := tkmasset.AppendTrailer(nil, manifest)
	if err != nil {
		t.Fatal(err)
	}
	st.SetCode(address, code, tracing.CodeChangeContractCreation)
	api := NewTKMAssetAPI(tkmAssetTestBackend{state: st, config: params.MainnetChainConfig})
	info, err := api.GetAsset(context.Background(), address, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.Native || info.AssetID != (common.Hash{}) {
		t.Fatalf("mismatched-chain manifest trusted: %+v", info)
	}
}
