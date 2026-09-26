// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package params

import (
	"math/big"
	"testing"
)

func TestAntarticalFeatureCatalogUsesSingleActivation(t *testing.T) {
	catalog := RandomXChainConfig.AntarticalFeatureCatalog()
	if len(catalog) != len(antarticalFeatureInfo) {
		t.Fatalf("catalog length = %d, want %d", len(catalog), len(antarticalFeatureInfo))
	}
	for _, feature := range catalog {
		if feature.ActivationTime != MainnetAntarticalTime {
			t.Fatalf("%s activation = %d, want %d", feature.ID, feature.ActivationTime, MainnetAntarticalTime)
		}
	}
	if got := RandomXChainConfig.IsAntarticalFeatureActive(FeatureNativePrivacy, big.NewInt(0), MainnetAntarticalTime-1); got {
		t.Fatal("native privacy active before Antartical")
	}
	if got := RandomXChainConfig.IsAntarticalFeatureActive(FeatureNativePrivacy, big.NewInt(0), MainnetAntarticalTime); !got {
		t.Fatal("native privacy inactive at Antartical")
	}
}

func TestAntarticalRulesExposeAllFeatureGates(t *testing.T) {
	rules := RandomXChainConfig.Rules(big.NewInt(0), false, MainnetAntarticalTime)
	checks := []struct {
		name string
		got  bool
	}{
		{"account abstraction", rules.IsAccountAbstraction},
		{"parallel execution", rules.IsParallelExecution},
		{"alternative EVM", rules.IsAlternativeEVM},
		{"formal verification", rules.IsFormalVerification},
		{"multidimensional gas", rules.IsMultidimensionalGas},
		{"blob gas", rules.IsBlobGas},
		{"native privacy", rules.IsNativePrivacy},
		{"stateless Verkle", rules.IsStatelessVerkle},
		{"native randomness", rules.IsNativeRandomness},
		{"native oracles", rules.IsNativeOracles},
		{"cross-chain standards", rules.IsCrossChainStandards},
		{"EOF", rules.IsEOF},
		{"modular precompiles", rules.IsModularPrecompiles},
		{"deterministic gas", rules.IsDeterministicGas},
		{"single-slot finality", rules.IsSingleSlotFinality},
	}
	for _, check := range checks {
		if !check.got {
			t.Errorf("%s gate is inactive at Antartical", check.name)
		}
	}
	preFork := RandomXChainConfig.Rules(big.NewInt(0), false, MainnetAntarticalTime-1)
	if preFork.IsAccountAbstraction || preFork.IsNativePrivacy || preFork.IsSingleSlotFinality {
		t.Fatal("Antartical feature gate active before the fork")
	}
}

func TestMainnetBlobGasSharesAntarticalActivation(t *testing.T) {
	if RandomXChainConfig.CancunTime == nil || *RandomXChainConfig.CancunTime != MainnetAntarticalTime {
		t.Fatalf("mainnet Cancun activation = %v, want Antartical timestamp", RandomXChainConfig.CancunTime)
	}
	if !RandomXChainConfig.IsCancun(big.NewInt(0), MainnetAntarticalTime) {
		t.Fatal("EIP-4844 blob rules inactive at Antartical")
	}
}
