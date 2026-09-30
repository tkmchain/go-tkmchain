// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	"bytes"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

// The EIP27928 reader utilizes a hierarchical architecture to optimize state
// access during block execution:
//
// - Base layer: The reader is initialized with the pre-transition state root,
//   providing the access of the state.
//
// - Prefetching Layer: This base reader is wrapped by newPrefetchStateReader.
//   Using an Access List hint, it asynchronously fetches required state data
//   in the background, minimizing I/O blocking during transaction processing.
//
// - Execution Layer: To support parallel transaction execution within the EIP
//   7928 context, readers are wrapped in ReaderWithBlockLevelAccessList.
//   This layer provides a "unified view" by merging the pre-transition state
//   with mutated states from preceding transactions in the block.
//
// - Tracking Layer: Finally, the readerTracker wraps the execution reader to
//   capture all state reads made during a specific transaction. These individual
//   reads are subsequently merged to construct a comprehensive access list
//   for the entire block.
//
// The architecture can be illustrated by the diagram below:
//
//   ┌──────────────┴──────────────┐    ┌──────────────┴──────────────┐
//   │ ReaderWithBlockLevelAL      │    │ ReaderWithBlockLevelAL      │
//   │ (Pre-state + Mutations)     │    │ (Pre-state + Mutations)     │
//   └──────────────┬──────────────┘    └──────────────┬──────────────┘
//                  │                                  │
//                  └────────────────┬─────────────────┘
//                                   │
//                    ┌──────────────┴──────────────┐
//                    │    newPrefetchStateReader   │ (Async I/O)
//                    │  (Access List Hint driven)  │
//                    └──────────────┬──────────────┘
//                                   │
//                    ┌──────────────┴──────────────┐
//                    │        Base Reader          │ (State Root)
//                    │ (State & Contract Code)     │
//                    └─────────────────────────────┘

// Note: The block producer, which is responsible for generating the block
// along with the block-level access list, does not maintain the internal
// hierarchy (e.g., PrefetchStateReader or ReaderWithBlockLevelAL).
// Instead, it directly utilizes the readerTracker, wrapped around the
// base reader, to construct the access list.

type fetchTask struct {
	addr  common.Address
	slots []common.Hash
}

func (t *fetchTask) weight() int { return 1 + len(t.slots) }

type prefetchStateReader struct {
	StateReader

	tasks     []*fetchTask
	nThreads  int
	done      chan struct{}
	term      chan struct{}
	closeOnce sync.Once
}

// nolint:unused
func newPrefetchStateReader(reader StateReader, accessList map[common.Address][]common.Hash, nThreads int) *prefetchStateReader {
	tasks := make([]*fetchTask, 0, len(accessList))
	for addr, slots := range accessList {
		tasks = append(tasks, &fetchTask{
			addr:  addr,
			slots: slots,
		})
	}
	return newPrefetchStateReaderInternal(reader, tasks, nThreads)
}

func newPrefetchStateReaderInternal(reader StateReader, tasks []*fetchTask, nThreads int) *prefetchStateReader {
	r := &prefetchStateReader{
		StateReader: reader,
		tasks:       tasks,
		nThreads:    nThreads,
		done:        make(chan struct{}),
		term:        make(chan struct{}),
	}
	go r.prefetch()
	return r
}

func (r *prefetchStateReader) Close() {
	r.closeOnce.Do(func() {
		close(r.term)
		<-r.done
	})
}

func (r *prefetchStateReader) Wait() error {
	select {
	case <-r.term:
		return nil
	case <-r.done:
		return nil
	}
}

func (r *prefetchStateReader) prefetch() {
	defer close(r.done)

	if len(r.tasks) == 0 {
		return
	}
	var total int
	for _, t := range r.tasks {
		total += t.weight()
	}
	var (
		wg   sync.WaitGroup
		unit = (total + r.nThreads - 1) / r.nThreads // round-up the per worker unit
	)
	for i := 0; i < r.nThreads; i++ {
		start := i * unit
		if start >= total {
			break
		}
		limit := (i + 1) * unit
		if i == r.nThreads-1 {
			limit = total
		}
		// Schedule the worker for prefetching, the items on the range [start, limit)
		// is exclusively assigned for this worker.
		wg.Add(1)
		go func(workerID, startW, endW int) {
			r.process(startW, endW)
			wg.Done()
		}(i, start, limit)
	}
	wg.Wait()
}

func (r *prefetchStateReader) process(start, limit int) {
	var total = 0
	for _, t := range r.tasks {
		tw := t.weight()
		if total+tw > start {
			s := 0
			if start > total {
				s = start - total
			}
			l := tw
			if limit < total+tw {
				l = limit - total
			}
			for j := s; j < l; j++ {
				select {
				case <-r.term:
					return
				default:
					if j == 0 {
						r.StateReader.Account(t.addr)
					} else {
						r.StateReader.Storage(t.addr, t.slots[j-1])
					}
				}
			}
		}
		total += tw
		if total >= limit {
			return
		}
	}
}

// ReaderWithBlockLevelAccessList provides state access that reflects the
// pre-transition state combined with the mutations made by transactions
// prior to TxIndex.
type ReaderWithBlockLevelAccessList struct {
	Reader
	AccessList *bal.ConstructionBlockAccessList
	TxIndex    int
}

var _ Reader = (*ReaderWithBlockLevelAccessList)(nil)

// NewReaderWithBlockLevelAccessList constructs a reader for accessing states
// with the mutations made by transactions prior to txIndex.
//
// The txIndex refers to the call frame as such:
// - 0 for pre‑execution system contract calls.
// - 1 … n for transactions (in block order).
// - n + 1 for post‑execution system contract calls.
func NewReaderWithBlockLevelAccessList(base Reader, accessList *bal.ConstructionBlockAccessList, txIndex int) *ReaderWithBlockLevelAccessList {
	return &ReaderWithBlockLevelAccessList{
		Reader:     base,
		AccessList: accessList,
		TxIndex:    txIndex,
	}
}

// Account implements Reader, returning the account with the specific address.
func (r *ReaderWithBlockLevelAccessList) Account(addr common.Address) (*types.StateAccount, error) {
	account, err := r.Reader.Account(addr)
	if err != nil {
		return nil, err
	}
	if account == nil {
		account = types.NewEmptyStateAccount()
	} else {
		account = account.Copy()
	}
	access := r.accountAccess(addr)
	if access == nil {
		if account.Nonce == 0 && account.Balance.IsZero() && bytes.Equal(account.CodeHash, types.EmptyCodeHash[:]) && account.Root == types.EmptyRootHash {
			// Preserve the underlying reader's non-existent-account result.
			if original, originalErr := r.Reader.Account(addr); originalErr == nil && original == nil {
				return nil, nil
			}
		}
		return account, nil
	}
	if balance, ok := latestBalance(access.BalanceChanges, r.TxIndex); ok {
		account.Balance = balance
	}
	if nonce, ok := latestNonce(access.NonceChanges, r.TxIndex); ok {
		account.Nonce = nonce
	}
	if code, ok := latestCode(access.CodeChange, r.TxIndex); ok {
		account.CodeHash = crypto.Keccak256(code)
	}
	return account, nil
}

// Storage implements Reader, returning the storage slot with the specific
// address and slot key.
func (r *ReaderWithBlockLevelAccessList) Storage(addr common.Address, slot common.Hash) (common.Hash, error) {
	value, err := r.Reader.Storage(addr, slot)
	if err != nil {
		return common.Hash{}, err
	}
	access := r.accountAccess(addr)
	if access == nil {
		return value, nil
	}
	if writes, ok := access.StorageWrites[slot]; ok {
		if written, ok := latestStorage(writes, r.TxIndex); ok {
			return written, nil
		}
	}
	return value, nil
}

// Has implements Reader, returning the flag indicating whether the contract
// code with specified address and hash exists or not.
func (r *ReaderWithBlockLevelAccessList) Has(addr common.Address, codeHash common.Hash) bool {
	code, changed := r.latestCode(addr)
	if changed {
		return common.BytesToHash(crypto.Keccak256(code)) == codeHash
	}
	return r.Reader.Has(addr, codeHash)
}

// Code implements Reader, returning the contract code with specified address
// and hash.
func (r *ReaderWithBlockLevelAccessList) Code(addr common.Address, codeHash common.Hash) []byte {
	if code, changed := r.latestCode(addr); changed {
		if common.BytesToHash(crypto.Keccak256(code)) != codeHash {
			return nil
		}
		return common.CopyBytes(code)
	}
	return common.CopyBytes(r.Reader.Code(addr, codeHash))
}

// CodeSize implements Reader, returning the contract code size with specified
// address and hash.
func (r *ReaderWithBlockLevelAccessList) CodeSize(addr common.Address, codeHash common.Hash) int {
	if code, changed := r.latestCode(addr); changed {
		if common.BytesToHash(crypto.Keccak256(code)) != codeHash {
			return 0
		}
		return len(code)
	}
	return len(r.Reader.Code(addr, codeHash))
}

func (r *ReaderWithBlockLevelAccessList) accountAccess(addr common.Address) *bal.ConstructionAccountAccess {
	if r == nil || r.AccessList == nil || r.AccessList.Accounts == nil {
		return nil
	}
	return r.AccessList.Accounts[addr]
}

func (r *ReaderWithBlockLevelAccessList) latestCode(addr common.Address) ([]byte, bool) {
	access := r.accountAccess(addr)
	if access == nil {
		return nil, false
	}
	return latestCode(access.CodeChange, r.TxIndex)
}

func latestBalance(changes map[uint32]*uint256.Int, txIndex int) (*uint256.Int, bool) {
	var (
		value *uint256.Int
		index uint32
		found bool
	)
	for tx, balance := range changes {
		if txIndex < 0 || tx >= uint32(txIndex) || (found && tx <= index) {
			continue
		}
		value, index, found = balance.Clone(), tx, true
	}
	return value, found
}

func latestNonce(changes map[uint32]uint64, txIndex int) (uint64, bool) {
	var (
		value uint64
		index uint32
		found bool
	)
	for tx, nonce := range changes {
		if txIndex < 0 || tx >= uint32(txIndex) || (found && tx <= index) {
			continue
		}
		value, index, found = nonce, tx, true
	}
	return value, found
}

func latestStorage(changes map[uint32]common.Hash, txIndex int) (common.Hash, bool) {
	var (
		value common.Hash
		index uint32
		found bool
	)
	for tx, slot := range changes {
		if txIndex < 0 || tx >= uint32(txIndex) || (found && tx <= index) {
			continue
		}
		value, index, found = slot, tx, true
	}
	return value, found
}

func latestCode(changes map[uint32][]byte, txIndex int) ([]byte, bool) {
	var (
		value []byte
		index uint32
		found bool
	)
	for tx, code := range changes {
		if txIndex < 0 || tx >= uint32(txIndex) || (found && tx <= index) {
			continue
		}
		value, index, found = common.CopyBytes(code), tx, true
	}
	return value, found
}
