// Copyright 2026 The go-tkmchain Authors
// This file is part of the go-tkmchain library.
//
// The go-tkmchain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-tkmchain library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestEgyptCheckpointSetUsesEgyptGenesis(t *testing.T) {
	SetActiveCheckpointGenesis(EgyptGenesisHash)
	defer SetActiveCheckpointGenesis(MainnetGenesisHash)

	got, ok := GetCheckpoint(0)
	if !ok {
		t.Fatal("missing Egypt genesis checkpoint")
	}
	if got != EgyptGenesisHash {
		t.Fatalf("Egypt genesis checkpoint = %s, want %s", got, EgyptGenesisHash)
	}
	if _, ok := GetCheckpoint(2370); ok {
		t.Fatal("Egypt checkpoint set inherited mainnet checkpoint 2370")
	}
	old := CheckpointValidationEnabled
	defer SetCheckpointValidation(old)
	SetCheckpointValidation(false)
	if ShouldValidateCheckpoint(2370) {
		t.Fatal("Egypt checkpoint set treats mainnet checkpoint 2370 as mandatory")
	}
}

func TestMandatoryRandomXCheckpoint2370(t *testing.T) {
	SetActiveCheckpointGenesis(MainnetGenesisHash)

	want := common.HexToHash("0xe10ff3179cc30f911c29326a822e6a24206f819dcaff2edfeeb5b2078dd95b17")
	got, ok := GetCheckpoint(2370)
	if !ok {
		t.Fatal("missing mandatory checkpoint at block 2370")
	}
	if got != want {
		t.Fatalf("checkpoint hash mismatch: have %s, want %s", got, want)
	}

	old := CheckpointValidationEnabled
	defer SetCheckpointValidation(old)
	SetCheckpointValidation(false)
	if !ShouldValidateCheckpoint(2370) {
		t.Fatal("mandatory checkpoint disabled with optional checkpoint validation")
	}
}
