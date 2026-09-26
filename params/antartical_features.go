// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or
// modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.

package params

import "math/big"

// AntarticalFeature identifies a protocol capability that is scheduled to
// become part of the TKM mainnet rules at the Antartical timestamp. Keeping
// the identifiers in one package prevents clients, the EVM, and the txpool
// from inventing slightly different activation conditions.
type AntarticalFeature string

const (
	FeatureAccountAbstraction  AntarticalFeature = "account-abstraction"
	FeatureParallelExecution   AntarticalFeature = "parallel-execution"
	FeatureAlternativeEVM      AntarticalFeature = "alternative-evm"
	FeatureFormalVerification  AntarticalFeature = "formal-verification"
	FeatureMultidimensionalGas AntarticalFeature = "multidimensional-gas"
	FeatureBlobGas             AntarticalFeature = "eip-4844-blobs"
	FeatureNativePrivacy       AntarticalFeature = "native-privacy"
	FeatureStatelessVerkle     AntarticalFeature = "stateless-verkle"
	FeatureNativeRandomness    AntarticalFeature = "native-randomness"
	FeatureOracles             AntarticalFeature = "native-oracles"
	FeatureCrossChain          AntarticalFeature = "cross-chain-standards"
	FeatureEOF                 AntarticalFeature = "evm-object-format"
	FeatureModularPrecompiles  AntarticalFeature = "modular-precompiles"
	FeatureDeterministicGas    AntarticalFeature = "deterministic-gas"
	FeatureSingleSlotFinality  AntarticalFeature = "single-slot-finality"
)

// AntarticalFeatureInfo is the machine-readable activation contract exposed
// to tooling and documentation. A feature may be scheduled before all of its
// execution-engine work is merged; ConsensusReady records that distinction so
// a node never silently treats a design document as consensus code.
type AntarticalFeatureInfo struct {
	ID              AntarticalFeature `json:"id"`
	Name            string            `json:"name"`
	ActivationTime  uint64            `json:"activationTime"`
	ConsensusReady  bool              `json:"consensusReady"`
	ImplementedArea string            `json:"implementedArea"`
}

// antarticalFeatureInfo is intentionally kept as a value table. It is copied
// on return so callers cannot mutate process-wide consensus metadata.
var antarticalFeatureInfo = [...]struct {
	id              AntarticalFeature
	name            string
	consensusReady  bool
	implementedArea string
}{
	{FeatureAccountAbstraction, "Native account abstraction (EIP-4337/RIP-7560)", false, "consensus/antartical UserOperation hashing and secp256k1/ML-DSA authorization"},
	{FeatureParallelExecution, "Parallel execution (optimistic Block-STM)", false, "consensus/antartical deterministic access-set wave scheduler"},
	{FeatureAlternativeEVM, "Alternative EVM engines (Revm/evmone/Rust)", false, "engine interface is not consensus-swappable yet"},
	{FeatureFormalVerification, "Formal verification tooling", false, "consensus/antartical execution claims plus zkEVM witness/proof tooling"},
	{FeatureMultidimensionalGas, "Multidimensional gas", false, "consensus/antartical deterministic gas vectors"},
	{FeatureBlobGas, "EIP-4844 blob gas", true, "existing Cancun blob transaction and blob pool implementation"},
	{FeatureNativePrivacy, "Native private EVM/TVM and zk execution", true, "Shield3/Shield4 and zkEVM witness/proof paths"},
	{FeatureStatelessVerkle, "Stateless clients (Verkle witnesses)", false, "Verkle transition storage plus consensus/antartical state-witness commitments"},
	{FeatureNativeRandomness, "Native randomness", false, "consensus/antartical parent-mix/block/slot derivation"},
	{FeatureOracles, "Native oracle interface", false, "consensus/antartical signed observation envelopes"},
	{FeatureCrossChain, "Cross-chain standards", false, "consensus/antartical domain-separated replay keys"},
	{FeatureEOF, "EVM Object Format", false, "consensus/antartical EOF v1 container validation"},
	{FeatureModularPrecompiles, "Modular precompiles", false, "consensus/antartical deterministic module registry"},
	{FeatureDeterministicGas, "Deterministic gas metering", true, "canonical intrinsic and EIP-1559/blob gas rules"},
	{FeatureSingleSlotFinality, "Single-slot finality", false, "consensus/antartical quorum certificate validation"},
}

// AntarticalFeatureCatalog returns the complete activation catalog. Every
// feature is scheduled at the same timestamp; ConsensusReady deliberately
// distinguishes the pieces already safe to enforce from work that still needs
// an execution/consensus implementation.
func (c *ChainConfig) AntarticalFeatureCatalog() []AntarticalFeatureInfo {
	activation := MainnetAntarticalTime
	if c != nil && c.AntarticalTime != nil {
		activation = *c.AntarticalTime
	}
	out := make([]AntarticalFeatureInfo, 0, len(antarticalFeatureInfo))
	for _, feature := range antarticalFeatureInfo {
		out = append(out, AntarticalFeatureInfo{
			ID:              feature.id,
			Name:            feature.name,
			ActivationTime:  activation,
			ConsensusReady:  feature.consensusReady,
			ImplementedArea: feature.implementedArea,
		})
	}
	return out
}

// IsAntarticalFeatureActive applies the single fork gate used by every
// scheduled capability. It intentionally does not claim that an unimplemented
// engine is consensus-ready; callers should check the catalog before enabling
// consensus behavior.
func (c *ChainConfig) IsAntarticalFeatureActive(feature AntarticalFeature, num *big.Int, timestamp uint64) bool {
	if c == nil || !c.IsAntartical(num, timestamp) {
		return false
	}
	for _, entry := range antarticalFeatureInfo {
		if entry.id == feature {
			return true
		}
	}
	return false
}

// AntarticalFeatureConsensusReady reports whether the feature has a complete
// deterministic implementation in this binary. This check is separate from
// activation so a node cannot accidentally activate a partial design.
func AntarticalFeatureConsensusReady(feature AntarticalFeature) bool {
	for _, entry := range antarticalFeatureInfo {
		if entry.id == feature {
			return entry.consensusReady
		}
	}
	return false
}
