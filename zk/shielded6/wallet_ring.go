// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package shielded6

import (
	cryptorand "crypto/rand"
	"io"
	"math/big"
	"sort"
)

// IndexedRingMember is a public output candidate fetched from the canonical
// Shield6 accumulator. Candidates must be ordered by increasing Index.
type IndexedRingMember struct {
	Index  uint64
	Member RingMember
}

// SelectRing selects RingSize-1 decoys uniformly without replacement, then
// shuffles the final ring order locally. The real output's position is
// returned as SignerIndex. Keeping the signed ring order independent of
// accumulator age avoids leaking the real output's rank among its decoys.
// Selection randomness is local wallet entropy; do not ask a remote RPC
// service to select a ring after revealing the real index.
//
// Uniform selection is a version-1 baseline. Wallets should obtain the
// candidate set from a sufficiently large, canonical accumulator range. The
// caller remains responsible for applying any future, versioned output-age
// policy before calling this function.
func SelectRing(realIndex uint64, candidates []IndexedRingMember, random io.Reader) (ring [RingSize]RingMember, indices [RingSize]uint64, signerIndex uint64, err error) {
	if random == nil {
		random = cryptorand.Reader
	}
	if len(candidates) < RingSize {
		return ring, indices, 0, ErrInvalidRing
	}
	realPosition := sort.Search(len(candidates), func(i int) bool { return candidates[i].Index >= realIndex })
	if realPosition == len(candidates) || candidates[realPosition].Index != realIndex {
		return ring, indices, 0, ErrInvalidRing
	}
	for i := 1; i < len(candidates); i++ {
		if candidates[i].Index <= candidates[i-1].Index {
			return ring, indices, 0, ErrInvalidRing
		}
	}
	population := len(candidates) - 1 // real output is excluded from decoys
	selected := make(map[int]struct{}, RingSize-1)
	// Floyd's algorithm samples a fixed-size subset with bounded O(RingSize)
	// random draws and no modulo bias, even for very large candidate ranges.
	for j := population - (RingSize - 1); j < population; j++ {
		n, readErr := cryptorand.Int(random, big.NewInt(int64(j+1)))
		if readErr != nil {
			return ring, indices, 0, readErr
		}
		candidate := int(n.Int64())
		if _, exists := selected[candidate]; exists {
			candidate = j
		}
		selected[candidate] = struct{}{}
	}
	positions := make([]int, 0, RingSize)
	for i := range candidates {
		if i != realPosition {
			positions = append(positions, i)
		}
	}
	chosen := make([]int, 0, RingSize)
	chosen = append(chosen, realPosition)
	for i := range selected {
		chosen = append(chosen, positions[i])
	}
	sort.Slice(chosen, func(i, j int) bool { return candidates[chosen[i]].Index < candidates[chosen[j]].Index })
	// The accumulator index order is useful while resolving and checking
	// candidates, but must not become the transaction's ring order: otherwise
	// the real member's position is correlated with its age rank. Fisher-Yates
	// uses rejection-sampled indices to avoid modulo bias.
	for i := len(chosen) - 1; i > 0; i-- {
		n, readErr := cryptorand.Int(random, big.NewInt(int64(i+1)))
		if readErr != nil {
			return ring, indices, 0, readErr
		}
		j := int(n.Int64())
		chosen[i], chosen[j] = chosen[j], chosen[i]
	}
	seenKeys := make(map[[PointSize]byte]struct{}, RingSize)
	for i, position := range chosen {
		candidate := candidates[position]
		if err := ValidatePublicKey(candidate.Member.OneTimeKey[:]); err != nil {
			return ring, indices, 0, ErrInvalidRing
		}
		if err := ValidateCommitmentEncoding(candidate.Member.Commitment[:]); err != nil {
			return ring, indices, 0, ErrInvalidRing
		}
		if _, duplicate := seenKeys[candidate.Member.OneTimeKey]; duplicate {
			return ring, indices, 0, ErrInvalidRing
		}
		seenKeys[candidate.Member.OneTimeKey] = struct{}{}
		ring[i], indices[i] = candidate.Member, candidate.Index
		if candidate.Index == realIndex {
			signerIndex = uint64(i)
		}
	}
	return ring, indices, signerIndex, nil
}
