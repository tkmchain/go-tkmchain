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
	if !rules.IsBlobGas || !rules.IsNativePrivacy || !rules.IsDeterministicGas || !rules.IsBlockHashAnchors {
		t.Fatal("implemented Antartical features are inactive at the fork")
	}
	if rules.IsAccountAbstraction || rules.IsParallelExecution || rules.IsAlternativeEVM ||
		rules.IsFormalVerification || rules.IsMultidimensionalGas || rules.IsStatelessVerkle ||
		rules.IsNativeRandomness || rules.IsNativeOracles || rules.IsCrossChainStandards ||
		rules.IsEOF || rules.IsModularPrecompiles || rules.IsSingleSlotFinality {
		t.Fatal("unfinished Antartical feature is active at the fork")
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

func TestTKMProfileVersionSharesAntarticalActivation(t *testing.T) {
	legacy := RandomXChainConfig.Rules(big.NewInt(0), false, MainnetAntarticalTime-1)
	if legacy.TKMProfileVersion != TKMProfileLegacyVersion {
		t.Fatalf("pre-fork TKM profile version = %d, want %d", legacy.TKMProfileVersion, TKMProfileLegacyVersion)
	}
	active := RandomXChainConfig.Rules(big.NewInt(0), false, MainnetAntarticalTime)
	if active.TKMProfileVersion != TKMProfileAntarticalVersion {
		t.Fatalf("Antartical TKM profile version = %d, want %d", active.TKMProfileVersion, TKMProfileAntarticalVersion)
	}
	egypt := EgyptChainConfig.Rules(big.NewInt(0), false, 0)
	if egypt.TKMProfileVersion != TKMProfileAntarticalVersion {
		t.Fatalf("Egypt TKM profile version = %d, want %d", egypt.TKMProfileVersion, TKMProfileAntarticalVersion)
	}
}
