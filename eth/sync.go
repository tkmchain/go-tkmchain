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
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func (h *handler) chainSyncLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		h.synchroniseWithBestPeer()
		select {
		case <-ticker.C:
		case <-h.quitSync:
			return
		}
	}
}

func (h *handler) synchroniseWithBestPeer() {
	local := h.chain.CurrentBlock()
	if local == nil {
		return
	}
	var best *ethPeer
	var bestHead common.Hash
	var bestHeight uint64
	var bestTd = new(big.Int)
	peers := h.peers.all()
	for _, peer := range peers {
		head, td := peer.Head()
		if td == nil || !td.IsUint64() {
			continue
		}
		if height := td.Uint64(); height > local.Number.Uint64() && height > bestHeight {
			best = peer
			bestHead = head
			bestHeight = height
			bestTd = td
		}
	}
	if best == nil {
		if len(peers) > 0 {
			h.enableSyncedFeatures()
		}
		return
	}
	if err := h.downloader.Synchronise(best.ID(), bestHead, bestTd, downloader.FullSync); err == nil {
		h.enableSyncedFeatures()
	}
}

func (h *handler) synchroniseWithPeerRange(id string, update *eth.BlockRangeUpdatePacket) {
	local := h.chain.CurrentBlock()
	if local == nil || update.LatestBlock <= local.Number.Uint64() {
		return
	}
	if h.peers.peer(id) == nil {
		return
	}
	td := new(big.Int).SetUint64(update.LatestBlock)
	if err := h.downloader.Synchronise(id, update.LatestBlockHash, td, downloader.FullSync); err == nil {
		h.enableSyncedFeatures()
	}
}

// syncTransactions starts sending all currently pending transactions to the given peer.
func (h *handler) syncTransactions(p *eth.Peer) {
	var hashes []common.Hash
	pending, _ := h.txpool.Pending(txpool.PendingFilter{BlobTxs: false})
	for _, batch := range pending {
		for _, tx := range batch {
			hashes = append(hashes, tx.Hash)
		}
	}
	if len(hashes) == 0 {
		return
	}
	p.AsyncSendPooledTransactionHashes(hashes)
}
