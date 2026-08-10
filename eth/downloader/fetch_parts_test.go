// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package downloader

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

func TestFetchPartsRequestErrorDoesNotPanic(t *testing.T) {
	peer := newPeerConnection("peer", 69, nil, log.New("peer", "peer"))
	peers := newPeerSet()
	if err := peers.Register(peer); err != nil {
		t.Fatal(err)
	}
	d := &Downloader{
		peers:    peers,
		cancelCh: make(chan struct{}),
	}

	fetchErr := errors.New("request assignment rejected")
	request := &fetchRequest{
		Peer: peer,
		Headers: []*types.Header{{
			Number: big.NewInt(1),
		}},
	}
	pending := 1
	var canceled, idled bool

	err := d.fetchParts(
		errCancelBodyFetch,
		make(chan dataPack),
		func(dataPack) (int, error) { return 0, nil },
		make(chan bool),
		func() map[string]int { return nil },
		func() int { return pending },
		func() bool { return false },
		func() bool { return false },
		func(p *peerConnection, count int) (*fetchRequest, bool, error) {
			if p != peer {
				t.Fatalf("reserved unexpected peer %q", p.id)
			}
			pending = 0
			return request, false, nil
		},
		nil,
		func(p *peerConnection, req *fetchRequest) error {
			if p != peer || req != request {
				t.Fatalf("fetch called with unexpected request")
			}
			return fetchErr
		},
		func(req *fetchRequest) {
			if req != request {
				t.Fatalf("canceled unexpected request")
			}
			canceled = true
		},
		func(*peerConnection) int { return 1 },
		func() ([]*peerConnection, int) { return []*peerConnection{peer}, 1 },
		func(p *peerConnection, accepted int) {
			if p != peer || accepted != 0 {
				t.Fatalf("idled unexpected peer or accepted count")
			}
			idled = true
		},
		"bodies",
	)
	if err == nil {
		t.Fatal("fetchParts returned nil after fetch assignment error")
	}
	if !strings.Contains(err.Error(), "bodies fetch assignment failed") || !errors.Is(err, fetchErr) {
		t.Fatalf("fetchParts error = %v, want wrapped assignment error", err)
	}
	if !canceled {
		t.Fatal("fetch request was not canceled")
	}
	if !idled {
		t.Fatal("peer was not returned to idle state")
	}
}
