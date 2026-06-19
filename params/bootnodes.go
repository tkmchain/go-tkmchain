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
        "enode://cc01b3e649c0d1e6a8fc3a8037552237d2f778d29efb798b66537a6112e2343ecbf0ae95441b73d4590f1286aa32d30b4dfa2755e7692618e6053595c4975f57@187.124.217.73:3000",
        "enode://2c36e766ab52f04abfc129891b0d92d4d61dff6b8cf496910fd7046be7ca66afddc0086d527d9540003e766716a5337a2b866f8519708996fb8ff645e0b6b52e@129.151.164.202:3000",
        "enode://11533a81e1c318f91ba257bc6e4dec8476b7f70f4534280ce8a4107ba0d31e4613d285d2fa5712aa39741c41ee9f7eaecc759a4b3aea6825c50c8dae9174cb45@129.151.164.223:3000",
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
        "enr:-J24QDURbVyt4rhioV0Fc4Fhs2n8ihxCSsZ0G39VUb6qOTvqP-3rCAdzAlvMwc5s2LG_-RQW5XjHWUFyEFtOAwTCQ_CGAZ7gNIyeg2V0aMfGhBMEcTCAgmlkgnY0gmlwhLt82UmJc2VjcDI1NmsxoQPMAbPmScDR5qj8OoA3VSI30vd40p77eYtmU3phEuI0PoN0Y3CCC7iDdWRwggu4",
	"enr:-KO4QA-YD4j8Po5DdSDRJpfCfJmDo0eM-SwreDC6lKQi4KZ-JN0NTVyihf-g706wtLQ6up7EDWJlUXndqEiC3VR_EH2GAZ5BUnWUg2V0aMfGhJa7DLOAgmlkgnY0gmlwhIGXpMqJc2VjcDI1NmsxoQLjIoVsk8G4o5W5DuW_imiRJbusRl0XaVD5fI8UgSeFs4RzbmFwwIN0Y3CCC7iDdWRwggu4",
        "enr:-KO4QKaE8k2YQewzHbQbVoBxCTzTf0_VTBndEaahyw25y2GqdYcHBEn_h9mHtxBx2udDPQhxk6tVzUYiu_ZlxgAQMSmGAZ5BXkANg2V0aMfGhJa7DLOAgmlkgnY0gmlwhH8AAAGJc2VjcDI1NmsxoQPQEKDyQnRC0i52hxhsHFYDKUmCSxudn4H-WpGA6L7HhoRzbmFwwIN0Y3CCC7iDdWRwggu4",
}

const dnsPrefix = "enrtree://AKA3AM6LPBYEUDMVNU3BSVQJ5AD45Y7YPOHJLEF6W26QOE4VTUDPE@"

// KnownDNSNetwork returns the address of a public DNS-based node list.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	return dnsPrefix + protocol + ".randomx.comd"
}
