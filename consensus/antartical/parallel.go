package antartical

import (
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// AccessSet is the conservative state-access summary used by the optimistic
// executor. Unknown accesses are kept serial; known disjoint accesses may be
// evaluated in one deterministic wave and committed in transaction order.
type AccessSet struct {
	Reads   []common.Address
	Writes  []common.Address
	Unknown bool
}

func (a AccessSet) conflicts(b AccessSet) bool {
	if a.Unknown || b.Unknown {
		return true
	}
	for _, x := range a.Writes {
		for _, y := range append(append([]common.Address{}, b.Reads...), b.Writes...) {
			if x == y {
				return true
			}
		}
	}
	for _, x := range b.Writes {
		for _, y := range a.Reads {
			if x == y {
				return true
			}
		}
	}
	return false
}

// BuildExecutionWaves returns transaction indexes grouped into deterministic
// optimistic waves. The greedy order is canonical, and every unknown or
// contract-creation transaction forms a serial wave.
func BuildExecutionWaves(access []AccessSet) [][]int {
	waves := make([][]int, 0, len(access))
	for i, item := range access {
		placed := false
		for w := range waves {
			ok := true
			for _, j := range waves[w] {
				if item.conflicts(access[j]) {
					ok = false
					break
				}
			}
			if ok {
				waves[w] = append(waves[w], i)
				placed = true
				break
			}
		}
		if !placed {
			waves = append(waves, []int{i})
		}
	}
	return waves
}

// AccessSetForTransaction derives a conservative summary from the EVM access
// list. Transactions without an access list are kept serial because their
// storage effects cannot be proven statically.
func AccessSetForTransaction(tx *types.Transaction) AccessSet {
	if tx == nil || len(tx.AccessList()) == 0 || tx.To() == nil {
		return AccessSet{Unknown: true}
	}
	reads := make([]common.Address, 0, len(tx.AccessList()))
	for _, tuple := range tx.AccessList() {
		reads = append(reads, tuple.Address)
	}
	sort.Slice(reads, func(i, j int) bool { return reads[i].Hex() < reads[j].Hex() })
	return AccessSet{Reads: reads, Writes: []common.Address{*tx.To()}}
}
