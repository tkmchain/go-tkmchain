package forkid

import (
	"testing"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
)

func TestShieldedOnlyForkID(t *testing.T) {
	genesis := core.DefaultGenesisBlock().ToBlock()
	old := *params.MainnetChainConfig
	old.ShieldedOnlyTime = nil
	cfg := old
	at := params.MainnetShieldedOnlyTime
	cfg.ShieldedOnlyTime = &at
	const height = 50000
	before := NewID(&cfg, genesis, height, params.MainnetShieldedOnlyTime-1)
	oldBefore := NewID(&old, genesis, height, params.MainnetShieldedOnlyTime-1)
	if before.Hash != oldBefore.Hash || before.Next != params.MainnetShieldedOnlyTime {
		t.Fatalf("pre-fork handshake does not advertise the scheduled cutoff: %+v %+v", before, oldBefore)
	}
	after := NewID(&cfg, genesis, height, params.MainnetShieldedOnlyTime)
	oldAfter := NewID(&old, genesis, height, params.MainnetShieldedOnlyTime)
	if after.Hash == oldAfter.Hash || after.Hash == before.Hash {
		t.Fatal("fork ID did not change at the privacy cutoff")
	}
}
