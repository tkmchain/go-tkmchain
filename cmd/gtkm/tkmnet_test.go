// Copyright 2026 The TKMChain Authors.
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/params"
)

func TestTkmnetActivationFollowsCanonicalHead(t *testing.T) {
	before := big.NewInt(0)
	if tkmnetRequiredAt(params.MainnetChainConfig, before, params.MainnetAntarticalTime-1) {
		t.Fatal("TKMNet became required before the mainnet Antartical timestamp")
	}

	atFork := big.NewInt(0)
	if !tkmnetRequiredAt(params.MainnetChainConfig, atFork, params.MainnetAntarticalTime) {
		t.Fatal("TKMNet is not required at the mainnet Antartical timestamp")
	}

	config := &tkmnetConfig{}
	if !enableTkmnetAtFork(config, true) || !config.Enabled {
		t.Fatal("active Antartical fork did not enable TKMNet")
	}
	if enableTkmnetAtFork(config, true) {
		t.Fatal("already-enabled TKMNet was reported as newly enabled")
	}

	egypt := big.NewInt(0)
	if !tkmnetRequiredAt(params.EgyptChainConfig, egypt, 0) {
		t.Fatal("Egypt rehearsal network did not require TKMNet at genesis")
	}

	preFork := &tkmnetConfig{}
	if enableTkmnetAtFork(preFork, false) || preFork.Enabled {
		t.Fatal("pre-fork startup enabled TKMNet without the consensus gate")
	}
}
