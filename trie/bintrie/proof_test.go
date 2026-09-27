// Copyright 2026 go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package bintrie

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
)

func TestBinaryTrieProve(t *testing.T) {
	key := common.HexToHash("0x0200000000000000000000000000000000000000000000000000000000000000")
	value := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	tr := makeTrie(t, [][2]common.Hash{{key, value}})
	proof := rawdb.NewMemoryDatabase()

	if err := tr.Prove(key[:], proof); err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if blob, err := proof.Get(tr.Hash().Bytes()); err != nil {
		t.Fatalf("read root proof: %v", err)
	} else if len(blob) == 0 {
		t.Fatal("proof did not contain the root node")
	}

	// Non-membership proofs still include the path's final stem node.
	missing := common.HexToHash("0x0300000000000000000000000000000000000000000000000000000000000000")
	proof = rawdb.NewMemoryDatabase()
	if err := tr.Prove(missing[:], proof); err != nil {
		t.Fatalf("non-membership Prove: %v", err)
	}
	if blob, err := proof.Get(tr.Hash().Bytes()); err != nil {
		t.Fatalf("read non-membership root proof: %v", err)
	} else if len(blob) == 0 {
		t.Fatal("non-membership proof did not contain the root node")
	}

	if err := tr.Prove(key[:31], rawdb.NewMemoryDatabase()); err == nil {
		t.Fatal("expected invalid key length error")
	}
}
