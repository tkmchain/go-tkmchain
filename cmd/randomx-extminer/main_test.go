//go:build cgo && randomx
// +build cgo,randomx

package main

import (
	"math/big"
	"testing"
)

func TestDigestMeetsTargetUsesLittleEndianHash(t *testing.T) {
	low := make([]byte, 32)
	low[0] = 1
	if !digestMeetsTarget(low, big.NewInt(1)) {
		t.Fatal("little-endian hash value 1 should meet target 1")
	}

	high := make([]byte, 32)
	high[31] = 1
	if digestMeetsTarget(high, big.NewInt(1)) {
		t.Fatal("little-endian high hash should exceed target 1")
	}
}
