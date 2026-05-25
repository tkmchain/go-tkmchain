package downloader

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
)

// SyncMode is kept as a local alias for downloader internals.
type SyncMode = ethconfig.SyncMode

const (
	FullSync  SyncMode = ethconfig.FullSync
	FastSync  SyncMode = ethconfig.SnapSync
	SnapSync  SyncMode = ethconfig.SnapSync
	LightSync SyncMode = SyncMode(2)
)

// peerDropFn is a callback type for dropping misbehaving peers.
type peerDropFn func(id string)

// dataPack is a batch of peer-delivered content.
type dataPack interface {
	PeerId() string
	Items() int
	Stats() string
}

type headerPack struct {
	peerID  string
	headers []*types.Header
}

func (p *headerPack) PeerId() string { return p.peerID }
func (p *headerPack) Items() int     { return len(p.headers) }
func (p *headerPack) Stats() string  { return fmt.Sprintf("%d", len(p.headers)) }

type bodyPack struct {
	peerID       string
	transactions [][]*types.Transaction
	uncles       [][]*types.Header
}

func (p *bodyPack) PeerId() string { return p.peerID }
func (p *bodyPack) Items() int     { return len(p.transactions) }
func (p *bodyPack) Stats() string {
	return fmt.Sprintf("%d:%d", len(p.transactions), len(p.uncles))
}

type receiptPack struct {
	peerID   string
	receipts []types.Receipts
}

func (p *receiptPack) PeerId() string { return p.peerID }
func (p *receiptPack) Items() int     { return len(p.receipts) }
func (p *receiptPack) Stats() string  { return fmt.Sprintf("%d", len(p.receipts)) }

type statePack struct {
	peerID string
	states [][]byte
}

func (p *statePack) PeerId() string { return p.peerID }
func (p *statePack) Items() int     { return len(p.states) }
func (p *statePack) Stats() string  { return fmt.Sprintf("%d", len(p.states)) }
