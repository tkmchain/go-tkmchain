// Copyright 2026 The TKMChain Authors.

package tkmnet

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

const (
	descriptorProtocolName    = "tkmnet"
	descriptorProtocolVersion = 1
	descriptorProtocolLength  = 1
	descriptorMessageCode     = 0
	descriptorMessageMaxSize  = 16 * 1024
	descriptorCacheLimit      = 256
	descriptorSendQueue       = descriptorCacheLimit
)

// DescriptorGossip exchanges authenticated TKMNet descriptors with connected
// gtkm peers. Received records are only accepted after descriptor signature,
// key-derived ID, onion hostname, and expiry checks. Directory authorization
// remains subject to the separate pinned-key policy in PinnedDirectoryPeer.
type DescriptorGossip struct {
	mu       sync.RWMutex
	known    map[[LayerNextIDSize]byte]Descriptor
	sessions map[enode.ID]chan Descriptor
	onAdd    func(Descriptor)
}

func (g *DescriptorGossip) SetOnAdd(callback func(Descriptor)) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.onAdd = callback
	g.mu.Unlock()
}

func NewDescriptorGossip() *DescriptorGossip {
	return &DescriptorGossip{
		known:    make(map[[LayerNextIDSize]byte]Descriptor),
		sessions: make(map[enode.ID]chan Descriptor),
	}
}

// Add adds a locally generated or received descriptor after strict validation.
func (g *DescriptorGossip) Add(d Descriptor) (bool, error) {
	if g == nil {
		return false, errors.New("tkmnet: descriptor gossip is unavailable")
	}
	if err := d.Verify(time.Now()); err != nil {
		return false, fmt.Errorf("tkmnet: reject unverified gossip descriptor: %w", err)
	}
	g.mu.Lock()
	now := time.Now()
	for id, cached := range g.known {
		if cached.Verify(now) != nil {
			delete(g.known, id)
		}
	}
	old, exists := g.known[d.ID]
	if exists {
		if string(old.PublicKey) != string(d.PublicKey) || string(old.SigningPublicKey) != string(d.SigningPublicKey) {
			g.mu.Unlock()
			return false, errors.New("tkmnet: descriptor identity attempted to replace its pinned keys")
		}
		if old.Expires >= d.Expires {
			g.mu.Unlock()
			return false, nil
		}
	}
	if !exists && len(g.known) >= descriptorCacheLimit {
		g.mu.Unlock()
		return false, errors.New("tkmnet: descriptor gossip cache is full")
	}
	g.known[d.ID] = cloneDescriptor(d)
	callback := g.onAdd
	queues := make([]chan Descriptor, 0, len(g.sessions))
	for _, queue := range g.sessions {
		queues = append(queues, queue)
	}
	g.mu.Unlock()
	if callback != nil {
		callback(cloneDescriptor(d))
	}
	for _, queue := range queues {
		select {
		case queue <- cloneDescriptor(d):
		default: // bounded queues prevent a slow peer from back-pressuring P2P.
		}
	}
	return true, nil
}

// Descriptors returns a copy of the currently valid descriptor cache.
func (g *DescriptorGossip) Descriptors() []Descriptor {
	if g == nil {
		return nil
	}
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	result := make([]Descriptor, 0, len(g.known))
	for id, d := range g.known {
		if d.Verify(now) != nil {
			delete(g.known, id)
			continue
		}
		result = append(result, cloneDescriptor(d))
	}
	return result
}

func (g *DescriptorGossip) Protocols() []p2p.Protocol {
	return []p2p.Protocol{{
		Name: descriptorProtocolName, Version: descriptorProtocolVersion,
		Length: descriptorProtocolLength, Run: g.run,
		NodeInfo: func() interface{} { return map[string]int{"verifiedDescriptors": len(g.Descriptors())} },
	}}
}

func (g *DescriptorGossip) run(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	if g == nil || peer == nil || rw == nil {
		return errors.New("tkmnet: invalid descriptor gossip session")
	}
	peerID := peer.ID()
	queue := make(chan Descriptor, descriptorSendQueue)
	g.mu.Lock()
	if _, exists := g.sessions[peerID]; exists {
		g.mu.Unlock()
		return errors.New("tkmnet: duplicate descriptor gossip session")
	}
	g.sessions[peerID] = queue
	initial := make([]Descriptor, 0, len(g.known))
	for _, d := range g.known {
		initial = append(initial, cloneDescriptor(d))
	}
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.sessions, peerID)
		g.mu.Unlock()
	}()

	for _, d := range initial {
		if err := p2p.Send(rw, descriptorMessageCode, &d); err != nil {
			return err
		}
	}
	reads := make(chan descriptorRead, 1)
	stopReader := make(chan struct{})
	defer close(stopReader)
	go func() {
		for {
			msg, err := rw.ReadMsg()
			if err != nil {
				select {
				case reads <- descriptorRead{err: err}:
				case <-stopReader:
				}
				return
			}
			var d Descriptor
			if msg.Code != descriptorMessageCode || msg.Size == 0 || msg.Size > descriptorMessageMaxSize {
				_ = msg.Discard()
				select {
				case reads <- descriptorRead{err: errors.New("tkmnet: invalid descriptor gossip message")}:
				case <-stopReader:
				}
				return
			}
			if err := msg.Decode(&d); err != nil {
				select {
				case reads <- descriptorRead{err: fmt.Errorf("tkmnet: decode descriptor gossip: %w", err)}:
				case <-stopReader:
				}
				return
			}
			select {
			case reads <- descriptorRead{descriptor: d}:
			case <-stopReader:
				return
			}
		}
	}()
	for {
		select {
		case incoming := <-reads:
			if incoming.err != nil {
				return incoming.err
			}
			if _, err := g.Add(incoming.descriptor); err != nil {
				return err
			}
		case d := <-queue:
			if err := p2p.Send(rw, descriptorMessageCode, &d); err != nil {
				return err
			}
		}
	}
}

type descriptorRead struct {
	descriptor Descriptor
	err        error
}

func cloneDescriptor(d Descriptor) Descriptor {
	d.PublicKey = append([]byte(nil), d.PublicKey...)
	d.SigningPublicKey = append([]byte(nil), d.SigningPublicKey...)
	d.Signature = append([]byte(nil), d.Signature...)
	return d
}
