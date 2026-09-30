package antartical

import (
	"fmt"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// OptimisticResult is the result of one speculative transaction execution.
// The callback must execute against an isolated snapshot and return the
// accesses it observed. Commit is always performed by the caller in index
// order, so the same result is produced on every client.
type OptimisticResult[T any] struct {
	Index  int
	Value  T
	Access AccessSet
}

// ExecuteOptimistic executes each canonical wave concurrently, validates the
// dynamic access sets, and retries a conflicting wave serially. This is the
// Block-STM rule used by Antartical: speculative work is parallel, while
// conflict resolution and commit order are deterministic. The function does
// not expose a shared mutable state object; callers provide isolated snapshots
// in run and commit returned values after this function returns.
func ExecuteOptimistic[T any](items []T, access []AccessSet, run func(index int, item T) (T, AccessSet, error)) ([]T, ConflictTranscript, error) {
	if len(items) != len(access) || run == nil {
		return nil, ConflictTranscript{}, fmt.Errorf("invalid optimistic execution input")
	}
	if len(items) == 0 {
		return []T{}, ConflictTranscript{}, nil
	}
	waves := BuildExecutionWaves(access)
	results := make([]T, len(items))
	dynamic := make([]AccessSet, len(items))
	for _, wave := range waves {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var firstErr error
		for _, index := range wave {
			index := index
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, observed, err := run(index, items[index])
				mu.Lock()
				defer mu.Unlock()
				if err != nil && firstErr == nil {
					firstErr = err
					return
				}
				results[index], dynamic[index] = value, observed
			}()
		}
		wg.Wait()
		if firstErr != nil {
			return nil, ConflictTranscript{}, firstErr
		}
		conflict := false
		for i, left := range wave {
			for _, right := range wave[i+1:] {
				if dynamic[left].conflicts(dynamic[right]) {
					conflict = true
				}
			}
		}
		if conflict {
			for _, index := range wave {
				value, observed, err := run(index, items[index])
				if err != nil {
					return nil, ConflictTranscript{}, err
				}
				results[index], dynamic[index] = value, observed
			}
		}
	}
	transcript := NewConflictTranscript(dynamic)
	return results, transcript, nil
}

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
