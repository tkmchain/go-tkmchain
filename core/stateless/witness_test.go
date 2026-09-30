// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package stateless

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestWitnessKeysRoundTripAndCopy(t *testing.T) {
	parent := &types.Header{Number: common.Big0, Root: common.HexToHash("0x01")}
	w := &Witness{
		Headers: []*types.Header{parent},
		Codes:   make(map[string]struct{}),
		State:   make(map[string]struct{}),
		Keys:    make(map[string]struct{}),
	}
	w.AddKey([]byte("account-key"), []byte("storage-key"), []byte("account-key"))
	if len(w.Keys) != 2 {
		t.Fatalf("key set contains %d entries, want 2", len(w.Keys))
	}

	copy := w.Copy()
	copy.AddKey([]byte("copy-only"))
	if len(w.Keys) != 2 || len(copy.Keys) != 3 {
		t.Fatalf("copy changed original key set: original=%d copy=%d", len(w.Keys), len(copy.Keys))
	}

	encoded, err := rlp.EncodeToBytes(w)
	if err != nil {
		t.Fatalf("encode witness: %v", err)
	}
	var decoded Witness
	if err := rlp.DecodeBytes(encoded, &decoded); err != nil {
		t.Fatalf("decode witness: %v", err)
	}
	if len(decoded.Keys) != 2 {
		t.Fatalf("decoded key set contains %d entries, want 2", len(decoded.Keys))
	}
	for key := range w.Keys {
		if _, ok := decoded.Keys[key]; !ok {
			t.Fatalf("decoded witness missing key %x", []byte(key))
		}
	}
	if bytes.Equal(encoded, nil) {
		t.Fatal("encoded witness is empty")
	}
}

func TestWitnessEncodingIsDeterministic(t *testing.T) {
	parent := &types.Header{Number: common.Big0, Root: common.HexToHash("0x01")}
	first := &Witness{
		Headers: []*types.Header{parent},
		Codes:   map[string]struct{}{"z-code": {}, "a-code": {}},
		State:   map[string]struct{}{"node-z": {}, "node-a": {}},
		Keys:    map[string]struct{}{"key-z": {}, "key-a": {}},
	}
	second := &Witness{
		Headers: []*types.Header{parent},
		Codes:   map[string]struct{}{"a-code": {}, "z-code": {}},
		State:   map[string]struct{}{"node-a": {}, "node-z": {}},
		Keys:    map[string]struct{}{"key-a": {}, "key-z": {}},
	}
	a, err := rlp.EncodeToBytes(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rlp.EncodeToBytes(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("witness encoding depends on map iteration order")
	}
}
