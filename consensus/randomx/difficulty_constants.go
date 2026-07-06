// Copyright 2026 The go-tkmchain Authors
// This file is part of the go-tkmchain library.
//
// The go-tkmchain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-tkmchain library. If not, see <http://www.gnu.org/licenses/>.

package randomx

import "math/big"

var (
	GenesisDifficulty = big.NewInt(3)
	MinDifficulty     = big.NewInt(3)
	MaxDifficulty     = new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
)
