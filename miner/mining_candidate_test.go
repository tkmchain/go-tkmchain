package miner

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestIsCurrentMiningCandidate(t *testing.T) {
	head := &types.Header{Number: big.NewInt(10)}
	valid := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(11), ParentHash: head.Hash()})
	if !isCurrentMiningCandidate(head, valid) {
		t.Fatal("direct child of current head was rejected")
	}

	tests := []*types.Block{
		nil,
		types.NewBlockWithHeader(&types.Header{Number: big.NewInt(10), ParentHash: head.Hash()}),
		types.NewBlockWithHeader(&types.Header{Number: big.NewInt(11), ParentHash: common.HexToHash("0x01")}),
	}
	for _, block := range tests {
		if isCurrentMiningCandidate(head, block) {
			t.Fatalf("stale or unrelated block was accepted: %v", block)
		}
	}
	if isCurrentMiningCandidate(nil, valid) {
		t.Fatal("candidate was accepted without a current head")
	}
}
