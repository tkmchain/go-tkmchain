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
        "enode://2c36e766ab52f04abfc129891b0d92d4d61dff6b8cf496910fd7046be7ca66afddc0086d527d9540003e766716a5337a2b866f8519708996fb8ff645e0b6b52e@129.151.164.202:3000",
        "enode://551fa0a4ed4d8539e3c85181d4d73d0a4617164abc60f782ad77639db6e6d63a650c534993b0e968c8c4cc21a91984f24b2a762117d062e1e1514a4a1be6ab60@102.90.96.171:3000",
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
        "enr:-J24QNptZgMENguLj2GqIz5grYg6PG43Gp6zAJG3tg2kyEwvaboCh085uKL799yLWxt6ouU2en0YarV-MHH9ZqMgMnWGAZ7ZjOpRg2V0aMfGhBMEcTCAgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQIsNudmq1LwSr_BKYkbDZLU1h3_a4z0lpEP1wRr58pmr4N0Y3CCC7iDdWRwggu4",
	"enr:-KO4QA-YD4j8Po5DdSDRJpfCfJmDo0eM-SwreDC6lKQi4KZ-JN0NTVyihf-g706wtLQ6up7EDWJlUXndqEiC3VR_EH2GAZ5BUnWUg2V0aMfGhJa7DLOAgmlkgnY0gmlwhIGXpMqJc2VjcDI1NmsxoQLjIoVsk8G4o5W5DuW_imiRJbusRl0XaVD5fI8UgSeFs4RzbmFwwIN0Y3CCC7iDdWRwggu4",
        "enr:-KO4QKaE8k2YQewzHbQbVoBxCTzTf0_VTBndEaahyw25y2GqdYcHBEn_h9mHtxBx2udDPQhxk6tVzUYiu_ZlxgAQMSmGAZ5BXkANg2V0aMfGhJa7DLOAgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPQEKDyQnRC0i52hxhsHFYDKUmCSxudn4H-WpGA6L7HhoRzbmFwwIN0Y3CCC7iDdWRwggu4",
}

const dnsPrefix = "enrtree://AKA3AM6LPBYEUDMVNU3BSVQJ5AD45Y7YPOHJLEF6W26QOE4VTUDPE@"

// KnownDNSNetwork returns the address of a public DNS-based node list.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	return dnsPrefix + protocol + ".randomx.comd"
}
