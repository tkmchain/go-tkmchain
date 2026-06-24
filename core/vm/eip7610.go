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

package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

// eip7610Accounts lists the addresses eligible for contract deployment
// rejection under EIP-7610, keyed by chain ID. Only networks that adopted
// EIP-158 after genesis need an entry; all others have no pre-existing
// address collisions to guard against.
//
// For RandomX chains, we don't have any pre-existing address collisions
// to guard against since it's a new chain with no prior state. The list
// is empty for RandomX.
var eip7610Accounts = map[uint64][]common.Address{
	// RandomX chain - no pre-existing accounts
	params.RandomXChainConfig.ChainID.Uint64(): {},
}

// eip7610AccountSets is the membership-lookup form of eip7610Accounts,
// built once at init for O(1) containment checks.
var eip7610AccountSets = func() map[uint64]map[common.Address]struct{} {
	sets := make(map[uint64]map[common.Address]struct{}, len(eip7610Accounts))
	for chainID, addrs := range eip7610Accounts {
		set := make(map[common.Address]struct{}, len(addrs))
		for _, a := range addrs {
			set[a] = struct{}{}
		}
		sets[chainID] = set
	}
	return sets
}()

// isEIP7610RejectedAccount reports whether the account identified by the
// address is eligible for contract deployment rejection due to having
// non-empty storage.
//
// Note that, historically, there has been no case where a contract deployment
// targets an already existing account in Ethereum. This situation would only
// occur in the event of an address collision, which is extremely unlikely.
//
// This check is skipped for blocks prior to EIP-158, serving as a safeguard
// against potential address collisions in the future. Chains that are not
// registered in eip7610Accounts are assumed to have no rejected accounts,
// and false is returned for them.
func isEIP7610RejectedAccount(chainID *big.Int, addr common.Address, isEIP158 bool) bool {
	// Short circuit for blocks prior to EIP-158.
	if !isEIP158 {
		return false
	}
	// Unknown chains fall through as a nil set; the second lookup then
	// returns the zero value (false), treating the chain as empty.
	_, exist := eip7610AccountSets[chainID.Uint64()][addr]
	return exist
}
