// Copyright 2022 The go-ethereum Authors
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

package miner

import (
        "math/big"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethereum/go-ethereum/core/stateless"
        "github.com/ethereum/go-ethereum/core/types"
)

// generateParams contains the parameters for generating work
type generateParams struct {
        timestamp   uint64
        forceTime   bool
        parentHash  common.Hash
        coinbase    common.Address
        random      common.Hash
        withdrawals types.Withdrawals
        beaconRoot  *common.Hash // Optional, for compatibility
        slotNum     *uint64      // Optional, for compatibility
        noTxs       bool
        extra       []byte // Extra data for RandomX
        
        // Testing overrides
        forceOverrides    bool
        overrideExtraData []byte
        overrideTxs       []*types.Transaction
}

// newPayloadResult contains the result of generating work
type newPayloadResult struct {
        block    *types.Block
        fees     *big.Int
        sidecars []*types.BlobTxSidecar
        requests [][]byte
        witness  *stateless.Witness
        err      error
}
