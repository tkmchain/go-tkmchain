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
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var checkpointPrefix = []byte("checkpoint-")

func checkpointKey(number uint64) []byte {
	key := make([]byte, len(checkpointPrefix)+8)
	copy(key, checkpointPrefix)
	binary.BigEndian.PutUint64(key[len(checkpointPrefix):], number)
	return key
}

// ReadCheckpoint retrieves an immutable validation checkpoint by block number.
func ReadCheckpoint(db ethdb.KeyValueReader, number uint64) (common.Hash, bool) {
	data, _ := db.Get(checkpointKey(number))
	if len(data) == 0 {
		return common.Hash{}, false
	}
	return common.BytesToHash(data), true
}

// WriteCheckpoint stores a validation checkpoint unless a different hash already exists.
func WriteCheckpoint(db ethdb.KeyValueStore, number uint64, hash common.Hash) error {
	if existing, ok := ReadCheckpoint(db, number); ok {
		if existing == hash {
			return nil
		}
		return fmt.Errorf("checkpoint already set at block %d: have %s, want %s", number, existing, hash)
	}
	return db.Put(checkpointKey(number), hash.Bytes())
}

// ReadCheckpoints retrieves all persisted validation checkpoints.
func ReadCheckpoints(db ethdb.Iteratee) []params.Checkpoint {
	it := db.NewIterator(checkpointPrefix, nil)
	defer it.Release()

	var checkpoints []params.Checkpoint
	for it.Next() {
		key := it.Key()
		if len(key) != len(checkpointPrefix)+8 {
			continue
		}
		checkpoints = append(checkpoints, params.Checkpoint{
			Number: binary.BigEndian.Uint64(key[len(checkpointPrefix):]),
			Hash:   common.BytesToHash(it.Value()),
		})
	}
	if err := it.Error(); err != nil {
		log.Error("Failed to iterate checkpoint database", "err", err)
	}
	return checkpoints
}
