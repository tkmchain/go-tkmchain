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

// MainnetBootnodes are the onion enode URLs of the P2P bootstrap nodes running
// on the main RandomX network. The node identity is retained while its
// transport endpoint is carried by a Tor onion service.
var MainnetBootnodes = []string{
	"enode://9f8ff5bda3629e9da4b2f1f4d4bd2385f38a382fa7f063b7f21c913411ef852dd224c7ee292450d587c3cbee5bd2d4a79999f0b9002070d7a559f8fe9a04baa2@4aof7abdduh4vftejgdpdfqeosvxxco3xmpu4uqypnpdbi7wjuzfqhqd.onion:3000?discport=0",
}

// TestnetBootnodes intentionally has no defaults until testnet operators
// publish onion services for their existing node identities.
var TestnetBootnodes = []string{}

var (
	HoleskyBootnodes = TestnetBootnodes
	SepoliaBootnodes = TestnetBootnodes
	HoodiBootnodes   = TestnetBootnodes
	EgyptBootnodes   = TestnetBootnodes
)

// V5Bootnodes is empty because the UDP ENR discovery transport is not part of
// the onion-only network.
var V5Bootnodes = []string{}

// KnownDNSNetwork returns no URL because DNS discovery is disabled for the onion-only network.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	// DNS discovery exposes clearnet addresses and is disabled for all network
	// profiles. Onion peers are configured explicitly as enode URLs.
	return ""
}
