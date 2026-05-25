// Copyright 2015 The go-ethereum Authors
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

package params

import "github.com/ethereum/go-ethereum/common"

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main RandomX network.
var MainnetBootnodes = []string{
	// Ethereum Foundation Go Bootnodes
        "enode://7c88351b493274c5abf111c0d67b84ba59b6a2cc1982904d65dd8273cc648b17d8dcf8f73882676406ad2f7a6d9e8c3ad9777ce843edf6c3f1fb73ea415dd1bf@40.82.130.91:3000",
	"enode://e322856c93c1b8a395b90ee5bf8a689125bbac465d176950f97c8f14812785b388dc17edd86d91786fbdcdf07050b4fc380bb2fbd1dd5815b4d77d6c60793902@129.151.164.202:3000",
        "enode://d010a0f2427442d22e7687186c1c56032949824b1b9d9f81fe5a9180e8bec786298528b5cfb0b64210ae1cc29d6afae09e5f80c64f42fcb96f59b12c5996c2c5@192.168.113.221:3000",
}

// TestnetBootnodes are bootstrap nodes for RandomX test networks.
var TestnetBootnodes = []string{
	"enode://ac906289e4b7f12df423d654c5a962b6ebe5b3a74cc9e06292a85221f9a64a6f1cfdd6b714ed6dacef51578f92b34c60ee91e9ede9c7f8fadc4d347326d95e2b@146.190.13.128:3000",
	"enode://a3435a0155a3e837c02f5e7f5662a2f1fbc25b48e4dc232016e1c51b544cb5b4510ef633ea3278c0e970fa8ad8141e2d4d0f9f95456c537ff05fdf9b31c15072@178.128.136.233:3000",
}

var (
	// HoleskyBootnodes are the enode URLs of the P2P bootstrap nodes running on
	// the Holesky test network.
	HoleskyBootnodes = TestnetBootnodes
	// SepoliaBootnodes are the enode URLs of the P2P bootstrap nodes running on
	// the Sepolia test network.
	SepoliaBootnodes = TestnetBootnodes
	// HoodiBootnodes are the enode URLs of the P2P bootstrap nodes running on
	// the Hoodi test network.
	HoodiBootnodes = TestnetBootnodes
)

var V5Bootnodes = []string{
	"enr:-KO4QA-YD4j8Po5DdSDRJpfCfJmDo0eM-SwreDC6lKQi4KZ-JN0NTVyihf-g706wtLQ6up7EDWJlUXndqEiC3VR_EH2GAZ5BUnWUg2V0aMfGhJa7DLOAgmlkgnY0gmlwhIGXpMqJc2VjcDI1NmsxoQLjIoVsk8G4o5W5DuW_imiRJbusRl0XaVD5fI8UgSeFs4RzbmFwwIN0Y3CCC7iDdWRwggu4",
        "enr:-KO4QKaE8k2YQewzHbQbVoBxCTzTf0_VTBndEaahyw25y2GqdYcHBEn_h9mHtxBx2udDPQhxk6tVzUYiu_ZlxgAQMSmGAZ5BXkANg2V0aMfGhJa7DLOAgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPQEKDyQnRC0i52hxhsHFYDKUmCSxudn4H-WpGA6L7HhoRzbmFwwIN0Y3CCC7iDdWRwggu4",
}

const dnsPrefix = "enrtree://AKA3AM6LPBYEUDMVNU3BSVQJ5AD45Y7YPOHJLEF6W26QOE4VTUDPE@"

// KnownDNSNetwork returns the address of a public DNS-based node list.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	return dnsPrefix + protocol + ".randomx.comd"
}
