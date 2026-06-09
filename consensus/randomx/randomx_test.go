//go:build cgo && randomx
// +build cgo,randomx

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
// or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package randomx

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestVerifySealAcceptsZeroMixDigestDuringBootstrap(t *testing.T) {
	rx := &RandomX{}
	header := &types.Header{Number: big.NewInt(1)}

	if err := rx.VerifySeal(nil, header); err != nil {
		t.Fatalf("unexpected bootstrap seal rejection: %v", err)
	}
}

func TestVerifySealRejectsZeroMixDigestAfterBootstrap(t *testing.T) {
	rx := &RandomX{}
	header := &types.Header{Number: big.NewInt(21)}

	if err := rx.VerifySeal(nil, header); err != errInvalidMixHash {
		t.Fatalf("unexpected seal error: have %v, want %v", err, errInvalidMixHash)
	}
}
