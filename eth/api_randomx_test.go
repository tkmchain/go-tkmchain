package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/rpc"
)

func TestRandomXAPIGetSeedHashAcceptsOptionalBlock(t *testing.T) {
	_, eth := newTestKingAPI(t, types.GenesisAlloc{})
	api := NewRandomXAPI(eth)

	wantNext := miner.RandomXSeedHash(eth.blockchain.Config(), eth.blockchain.CurrentBlock().Number.Uint64()+1)
	gotNext, err := api.GetSeedHash(nil)
	if err != nil {
		t.Fatalf("GetSeedHash(nil) failed: %v", err)
	}
	if gotNext != wantNext {
		t.Fatalf("GetSeedHash(nil) = %v, want %v", gotNext, wantNext)
	}

	block := hexutil.Uint64(2048)
	wantBlock := miner.RandomXSeedHash(eth.blockchain.Config(), uint64(block))
	gotBlock, err := api.GetSeedHash(&block)
	if err != nil {
		t.Fatalf("GetSeedHash(%d) failed: %v", block, err)
	}
	if gotBlock != wantBlock {
		t.Fatalf("GetSeedHash(%d) = %v, want %v", block, gotBlock, wantBlock)
	}
}

func TestRandomXRPCGetSeedHashAcceptsBlockArgument(t *testing.T) {
	_, eth := newTestKingAPI(t, types.GenesisAlloc{})
	server := rpc.NewServer()
	defer server.Stop()
	if err := server.RegisterName("randomx", NewRandomXAPI(eth)); err != nil {
		t.Fatal(err)
	}
	client := rpc.DialInProc(server)
	defer client.Close()

	block := hexutil.Uint64(2048)
	want := miner.RandomXSeedHash(eth.blockchain.Config(), uint64(block))
	var got common.Hash
	if err := client.Call(&got, "randomx_getSeedHash", block); err != nil {
		t.Fatalf("randomx_getSeedHash(%d) failed: %v", block, err)
	}
	if got != want {
		t.Fatalf("randomx_getSeedHash(%d) = %v, want %v", block, got, want)
	}
}
