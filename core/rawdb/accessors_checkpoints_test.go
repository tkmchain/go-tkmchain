// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
// or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser General Public License
// for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package rawdb

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestCheckpointPersistenceIsImmutable(t *testing.T) {
	db := NewMemoryDatabase()
	hash := common.HexToHash("0x01")
	if err := WriteCheckpoint(db, 98, hash); err != nil {
		t.Fatalf("failed to write checkpoint: %v", err)
	}
	if err := WriteCheckpoint(db, 98, hash); err != nil {
		t.Fatalf("failed to rewrite same checkpoint: %v", err)
	}
	if err := WriteCheckpoint(db, 98, common.HexToHash("0x02")); err == nil {
		t.Fatalf("rewrote checkpoint with a different hash")
	}
	stored, ok := ReadCheckpoint(db, 98)
	if !ok || stored != hash {
		t.Fatalf("stored checkpoint mismatch: have %v %v, want %v true", stored, ok, hash)
	}
	checkpoints := ReadCheckpoints(db)
	if len(checkpoints) != 1 || checkpoints[0].Number != 98 || checkpoints[0].Hash != hash {
		t.Fatalf("unexpected checkpoints: %#v", checkpoints)
	}
}
