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

package eth

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// Tests that snap sync is disabled after a successful sync cycle. The node
// advertises eth/71 as its current supported capability; using that capability
// here keeps the snap extension handshake aligned with the production peer set.
func TestSnapSyncDisabling69(t *testing.T) { testSnapSyncDisabling(t, eth.ETH71, snap.SNAP1) }

// Tests that snap sync gets disabled as soon as a real block is successfully
// imported into the blockchain.
func testSnapSyncDisabling(t *testing.T, ethVer uint, snapVer uint) {
	t.Parallel()

	// Create an empty handler and ensure it's in snap sync mode
	empty := newTestHandler(ethconfig.SnapSync)
	defer empty.close()

	// Create a full handler and ensure snap sync ends up disabled
	full := newTestHandlerWithBlocks(1024, ethconfig.SnapSync)
	defer full.close()

	// Sync up the two handlers via both `eth` and `snap`
	caps := []p2p.Cap{{Name: "eth", Version: ethVer}, {Name: "snap", Version: snapVer}}

	emptyPipeEth, fullPipeEth := p2p.MsgPipe()
	defer emptyPipeEth.Close()
	defer fullPipeEth.Close()

	emptyPeerEth := eth.NewPeer(ethVer, p2p.NewPeer(enode.ID{1}, "", caps), emptyPipeEth, empty.txpool, nil)
	fullPeerEth := eth.NewPeer(ethVer, p2p.NewPeer(enode.ID{2}, "", caps), fullPipeEth, full.txpool, nil)
	defer emptyPeerEth.Close()
	defer fullPeerEth.Close()

	peerErr := make(chan error, 4)
	go func() {
		peerErr <- empty.handler.runEthPeer(emptyPeerEth, func(peer *eth.Peer) error {
			return eth.Handle((*ethHandler)(empty.handler), peer)
		})
	}()
	go func() {
		peerErr <- full.handler.runEthPeer(fullPeerEth, func(peer *eth.Peer) error {
			return eth.Handle((*ethHandler)(full.handler), peer)
		})
	}()

	emptyPipeSnap, fullPipeSnap := p2p.MsgPipe()
	defer emptyPipeSnap.Close()
	defer fullPipeSnap.Close()

	emptyPeerSnap := snap.NewPeer(snapVer, p2p.NewPeer(enode.ID{1}, "", caps), emptyPipeSnap)
	fullPeerSnap := snap.NewPeer(snapVer, p2p.NewPeer(enode.ID{2}, "", caps), fullPipeSnap)

	go func() {
		peerErr <- empty.handler.runSnapExtension(emptyPeerSnap, func(peer *snap.Peer) error {
			return snap.Handle((*snapHandler)(empty.handler), peer)
		})
	}()
	go func() {
		peerErr <- full.handler.runSnapExtension(fullPeerSnap, func(peer *snap.Peer) error {
			return snap.Handle((*snapHandler)(full.handler), peer)
		})
	}()
	// Wait for both protocol handlers to register their peers. Snapshot
	// generation can make the old fixed 250ms delay racy on slower builders.
	deadline := time.NewTimer(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer tick.Stop()
	for empty.handler.peers.len() == 0 || full.handler.peers.len() == 0 {
		select {
		case err := <-peerErr:
			if err != nil {
				t.Fatalf("protocol handler failed before peer registration: %v", err)
			}
		case <-deadline.C:
			t.Fatalf("protocol peers not registered: empty=%d full=%d", empty.handler.peers.len(), full.handler.peers.len())
		case <-tick.C:
		}
	}
	// The handler starts its normal best-peer synchronizer as soon as the
	// connection is registered. Stop that background attempt before invoking
	// the snap-sync cycle under test, otherwise the two calls race and the
	// explicit cycle can return downloader busy.
	empty.handler.downloader.Cancel()

	// Check that snap sync was disabled
	if err := empty.handler.downloader.RandomXSync(full.chain.CurrentBlock()); err != nil {
		t.Fatal("sync failed:", err)
	}
	// Snap sync and mode switching happen asynchronously, poll for completion.
	timeout := time.NewTimer(15 * time.Second)
	tickSync := time.NewTicker(100 * time.Millisecond)
	defer timeout.Stop()
	defer tickSync.Stop()

	for {
		select {
		case <-timeout.C:
			t.Fatalf("snap sync not disabled after successful synchronisation")
		case <-tickSync.C:
			if empty.handler.synced.Load() && empty.handler.downloader.ConfigSyncMode() == ethconfig.FullSync {
				return
			}
		}
	}
}
