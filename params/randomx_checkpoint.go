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
// or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser General Public License
// for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import "github.com/ethereum/go-ethereum/common"

// RandomXCheckpointHash is the immutable checkpoint anchor preloaded into the
// RandomX checkpoint contract. Operators may override it at build time with:
//
//	-ldflags "-X github.com/ethereum/go-ethereum/params.RandomXCheckpointHash=0x..."
var RandomXCheckpointHash = "0x3112f6d283e7b57462a719697ea236528846ab98de270573a9d465888f7a4f9e"

func init() {
	// Keep the default RandomX genesis hash aligned with the pre-deployed
	// checkpoint contract included in the default genesis allocation.
	RandomXGenesisHash = common.HexToHash("0x3112f6d283e7b57462a719697ea236528846ab98de270573a9d465888f7a4f9e")
}
