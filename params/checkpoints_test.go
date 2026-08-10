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

func TestMandatoryRandomXCheckpoints(t *testing.T) {
	SetActiveCheckpointGenesis(MainnetGenesisHash)

	tests := map[uint64]common.Hash{
		2370:  common.HexToHash("0xe10ff3179cc30f911c29326a822e6a24206f819dcaff2edfeeb5b2078dd95b17"),
		6000:  common.HexToHash("0x4d3cd743aec4b40c276174d6582049189901a0a78fa6fc280b8c5cfd946fa660"),
		7165:  common.HexToHash("0xcd0abe2c94903b0a7584ac5892ff812d2f2450853fc6a055cbb09807ce8c9f53"),
		20004: common.HexToHash("0x8918c6621a4fd423b86fedbae903277078a2286cbce5057a9afbc436c46dc7da"),
	}
	for number, want := range tests {
		got, ok := GetCheckpoint(number)
		if !ok {
			t.Fatalf("missing mandatory checkpoint at block %d", number)
		}
		if got != want {
			t.Fatalf("checkpoint %d hash mismatch: have %s, want %s", number, got, want)
		}
	}

	old := CheckpointValidationEnabled
	defer SetCheckpointValidation(old)
	SetCheckpointValidation(false)
	for number := range tests {
		if !ShouldValidateCheckpoint(number) {
			t.Fatalf("mandatory checkpoint %d disabled with optional checkpoint validation", number)
		}
	}
}
