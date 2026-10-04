package params

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"
)

func TestConfidentialLedgerFork(t *testing.T) {
	at := ProposedConfidentialLedgerTime
	if time.Unix(int64(at), 0).UTC().Format(time.RFC3339) != "2026-10-04T10:00:00Z" {
		t.Fatal("wrong requested activation time")
	}
	for _, network := range []*ChainConfig{MainnetChainConfig, RandomXChainConfig, EgyptChainConfig} {
		if network.ConfidentialLedgerTime != nil {
			t.Fatal("unreleased ledger activated on live network")
		}
		cfg := *network
		cfg.ConfidentialLedgerTime = &at
		if err := cfg.CheckConfigForkOrder(); err != nil {
			t.Fatal(err)
		}
		for _, ts := range []uint64{at - 1, at, at + 1} {
			if cfg.IsConfidentialLedger(big.NewInt(1), ts) != (ts >= at) || cfg.Rules(big.NewInt(1), false, ts).IsConfidentialLedger != (ts >= at) {
				t.Fatal("incorrect activation")
			}
		}
		b, err := json.Marshal(&cfg)
		if err != nil {
			t.Fatal(err)
		}
		var restored ChainConfig
		if err = json.Unmarshal(b, &restored); err != nil || restored.ConfidentialLedgerTime == nil || *restored.ConfidentialLedgerTime != at {
			t.Fatal("schedule lost")
		}
		if err := network.CheckCompatible(&cfg, 1, at-1); err != nil {
			t.Fatal(err)
		}
		if err := network.CheckCompatible(&cfg, 1, at); err == nil || err.What != "Confidential ledger timestamp" {
			t.Fatalf("retroactive fork accepted: %v", err)
		}
		cfg.ShieldedOnlyTime = &at
		if err := cfg.CheckConfigForkOrder(); err == nil {
			t.Fatal("conflicting privacy boundaries accepted")
		}
	}
}
