// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or
// modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.

package eth

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/params"
)

// ProtocolFeatureState is the RPC representation of an Antartical capability.
// Active is derived from the canonical chain head, never from local wall clock
// time or a command-line override.
type ProtocolFeatureState struct {
	params.AntarticalFeatureInfo
	Active bool `json:"active"`
}

// ProtocolAPI exposes the fork feature contract to wallets, explorers, and
// alternative execution clients. It is read-only and cannot change consensus
// activation.
type ProtocolAPI struct {
	eth *Ethereum
}

func NewProtocolAPI(eth *Ethereum) *ProtocolAPI { return &ProtocolAPI{eth: eth} }

func (api *ProtocolAPI) AntarticalFeatures() []ProtocolFeatureState {
	if api == nil || api.eth == nil || api.eth.blockchain == nil {
		return nil
	}
	chain := api.eth.blockchain
	config := chain.Config()
	if config == nil {
		return nil
	}
	head := chain.CurrentHeader()
	var number *big.Int
	var timestamp uint64
	if head != nil {
		number = new(big.Int).Set(head.Number)
		timestamp = head.Time
	} else {
		number = new(big.Int)
	}
	catalog := config.AntarticalFeatureCatalog()
	result := make([]ProtocolFeatureState, 0, len(catalog))
	for _, feature := range catalog {
		result = append(result, ProtocolFeatureState{
			AntarticalFeatureInfo: feature,
			Active:                config.IsAntarticalFeatureActive(feature.ID, number, timestamp),
		})
	}
	return result
}

// AntarticalStatus returns the activation timestamp and the current head used
// to evaluate every feature gate.
func (api *ProtocolAPI) AntarticalStatus() map[string]interface{} {
	status := map[string]interface{}{"active": false}
	if api == nil || api.eth == nil || api.eth.blockchain == nil {
		return status
	}
	config := api.eth.blockchain.Config()
	if config == nil {
		return status
	}
	if config.AntarticalTime != nil {
		status["activationTime"] = hexutil.Uint64(*config.AntarticalTime)
	}
	head := api.eth.blockchain.CurrentHeader()
	if head != nil {
		status["headNumber"] = hexutil.Uint64(head.Number.Uint64())
		status["headTime"] = hexutil.Uint64(head.Time)
		status["active"] = config.IsAntartical(head.Number, head.Time)
	}
	return status
}
