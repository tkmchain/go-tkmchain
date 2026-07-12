package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/params"
)

func newReadyToMineTestEthereum(t *testing.T, config *params.ChainConfig) *Ethereum {
	t.Helper()
	db := rawdb.NewMemoryDatabase()
	genesis := &core.Genesis{Config: config}
	chain, err := core.NewBlockChain(db, genesis, randomx.NewFaker(), nil)
	if err != nil {
		t.Fatalf("failed to create test chain: %v", err)
	}
	t.Cleanup(chain.Stop)
	return &Ethereum{blockchain: chain}
}

func TestReadyToMineEgyptDoesNotWaitForPeers(t *testing.T) {
	eth := newReadyToMineTestEthereum(t, params.EgyptChainConfig)
	ready, reason, local, highest := eth.readyToMine()
	if !ready {
		t.Fatalf("Egypt readyToMine = false, reason %q local %d highest %d", reason, local, highest)
	}
}

func TestReadyToMineOtherRandomXNetworksWaitForPeers(t *testing.T) {
	eth := newReadyToMineTestEthereum(t, params.TestChainConfig)
	ready, reason, _, _ := eth.readyToMine()
	if ready {
		t.Fatal("non-Egypt readyToMine = true without peers")
	}
	if reason != "waiting for peers" {
		t.Fatalf("non-Egypt readyToMine reason = %q, want waiting for peers", reason)
	}
}
