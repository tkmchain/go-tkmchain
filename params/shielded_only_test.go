package params

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestShieldedOnlyForkSchedule(t *testing.T) {
	for name, network := range map[string]*ChainConfig{"mainnet": MainnetChainConfig, "randomx": RandomXChainConfig, "egypt": EgyptChainConfig} {
		t.Run(name, func(t *testing.T) {
			if network.ShieldedOnlyTime != nil {
				t.Fatal("cutoff must not strand live-network funding")
			}
			cfg := *network
			cfg.ShieldedOnlyTime = newUint64(MainnetShieldedOnlyTime)
			if cfg.ShieldedOnlyTime == nil || *cfg.ShieldedOnlyTime != MainnetShieldedOnlyTime {
				t.Fatal("missing shielded-only schedule")
			}
			if err := cfg.CheckConfigForkOrder(); err != nil {
				t.Fatal(err)
			}
			for _, timestamp := range []uint64{MainnetShieldedOnlyTime - 1, MainnetShieldedOnlyTime, MainnetShieldedOnlyTime + 1} {
				want := timestamp >= MainnetShieldedOnlyTime
				if cfg.IsShieldedOnly(big.NewInt(1), timestamp) != want || cfg.Rules(big.NewInt(1), false, timestamp).IsShieldedOnly != want {
					t.Fatalf("incorrect activation at %d", timestamp)
				}
			}
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var restored ChainConfig
			if err := json.Unmarshal(data, &restored); err != nil || restored.ShieldedOnlyTime == nil || *restored.ShieldedOnlyTime != MainnetShieldedOnlyTime {
				t.Fatalf("schedule not preserved by database JSON: %v", err)
			}
		})
	}
}

func TestShieldedOnlyConfigCompatibility(t *testing.T) {
	scheduled := *MainnetChainConfig
	scheduled.ShieldedOnlyTime = newUint64(MainnetShieldedOnlyTime)
	stored := scheduled
	stored.ShieldedOnlyTime = nil
	if err := stored.CheckCompatible(&scheduled, 1, MainnetShieldedOnlyTime-1); err != nil {
		t.Fatalf("could not schedule future fork: %v", err)
	}
	for _, timestamp := range []uint64{MainnetShieldedOnlyTime, MainnetShieldedOnlyTime + 1} {
		err := stored.CheckCompatible(&scheduled, 1, timestamp)
		if err == nil || err.What != "Shielded-only fork timestamp" || err.RewindToTime != MainnetShieldedOnlyTime-1 {
			t.Fatalf("retroactive configuration accepted: %v", err)
		}
		if err := scheduled.CheckCompatible(&stored, 1, timestamp); err == nil {
			t.Fatal("activated fork could be removed")
		}
	}
}

func TestShieldedOnlyRequiresEarlierPrivacyForks(t *testing.T) {
	for _, mutate := range []func(*ChainConfig){
		func(c *ChainConfig) { c.AntarticalTime = nil },
		func(c *ChainConfig) { c.PrivacyCommitmentTime = nil },
		func(c *ChainConfig) { c.QuantumResistantTime = nil },
		func(c *ChainConfig) { c.LondonBlock = nil },
		func(c *ChainConfig) { c.ShieldedOnlyTime = newUint64(MainnetAntarticalTime - 1) },
	} {
		cfg := *MainnetChainConfig
		cfg.ShieldedOnlyTime = newUint64(MainnetShieldedOnlyTime)
		mutate(&cfg)
		if err := cfg.CheckConfigForkOrder(); err == nil {
			t.Fatal("inconsistent schedule accepted")
		}
	}
	cfg := *MainnetChainConfig
	cfg.ShieldedOnlyTime = nil
	if cfg.IsShieldedOnly(big.NewInt(1), MainnetShieldedOnlyTime+1) || (*ChainConfig)(nil).IsShieldedOnly(big.NewInt(1), 0) {
		t.Fatal("unscheduled fork active")
	}
}
