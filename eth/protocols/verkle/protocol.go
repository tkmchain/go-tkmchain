// Copyright 2026 The TKMChain Authors.
//
// Package verkle implements the TKM stateless witness protocol. The wire
// format deliberately carries the existing stateless witness RLP so clients
// can validate it with the same root and header rules used for local execution.
package verkle

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	VERKLE1        = 1
	ProtocolName   = "verkle"
	maxMessageSize = 8 * 1024 * 1024
	maxWitnessSize = 4 * 1024 * 1024
)

var ProtocolVersions = []uint{VERKLE1}
var protocolLengths = map[uint]uint64{VERKLE1: 3}

const (
	StatusMsg     = 0x00
	GetWitnessMsg = 0x01
	WitnessMsg    = 0x02
)

var (
	errInvalidMessage  = errors.New("invalid verkle protocol message")
	errNetworkMismatch = errors.New("verkle protocol network mismatch")
)

type Packet interface {
	Name() string
	Kind() byte
}

// StatusPacket binds the witness protocol to the same chain as the eth
// protocol. A witness from another chain or another head is never accepted.
type StatusPacket struct {
	Version    uint32
	ChainID    *big.Int
	Genesis    common.Hash
	Latest     uint64
	LatestHash common.Hash
	StateRoot  common.Hash
}

type GetWitnessPacket struct {
	ID          uint64
	BlockHash   common.Hash
	BlockNumber uint64
	MaxBytes    uint64
}

type WitnessPacket struct {
	ID          uint64
	BlockHash   common.Hash
	BlockNumber uint64
	StateRoot   common.Hash
	Witness     rlp.RawValue
	Error       []byte
}

func (*StatusPacket) Name() string     { return "Status" }
func (*StatusPacket) Kind() byte       { return StatusMsg }
func (*GetWitnessPacket) Name() string { return "GetWitness" }
func (*GetWitnessPacket) Kind() byte   { return GetWitnessMsg }
func (*WitnessPacket) Name() string    { return "Witness" }
func (*WitnessPacket) Kind() byte      { return WitnessMsg }

type Handler func(*Peer) error

// Backend is intentionally small: the chain supplies witness construction,
// while the protocol package owns all framing and validation.
type Backend interface {
	Chain() *core.BlockChain
	RunPeer(*Peer, Handler) error
	GetWitness(context.Context, common.Hash, uint64, uint64) (*stateless.ExtWitness, error)
	PeerInfo(enode.ID) interface{}
}

// MakeProtocols registers verkle/1 with the node's protocol stack.
func MakeProtocols(backend Backend) []p2p.Protocol {
	protocols := make([]p2p.Protocol, len(ProtocolVersions))
	for i, version := range ProtocolVersions {
		protocols[i] = p2p.Protocol{
			Name: ProtocolName, Version: version, Length: protocolLengths[version],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := NewPeer(version, p, rw)
				return backend.RunPeer(peer, func(peer *Peer) error {
					defer peer.Close()
					return Handle(backend, peer)
				})
			},
			NodeInfo:   func() interface{} { return makeNodeInfo(backend.Chain()) },
			PeerInfo:   func(id enode.ID) interface{} { return backend.PeerInfo(id) },
			Attributes: []enr.Entry{&enrEntry{}},
		}
	}
	return protocols
}

func Handle(backend Backend, peer *Peer) error {
	for {
		msg, err := peer.rw.ReadMsg()
		if err != nil {
			return err
		}
		if msg.Size > maxMessageSize {
			msg.Discard()
			return fmt.Errorf("verkle message exceeds %d bytes", maxMessageSize)
		}
		switch msg.Code {
		case GetWitnessMsg:
			var request GetWitnessPacket
			if err := msg.Decode(&request); err != nil || request.MaxBytes == 0 || request.MaxBytes > maxWitnessSize {
				msg.Discard()
				return errInvalidMessage
			}
			if err := msg.Discard(); err != nil {
				return err
			}
			witness, err := backend.GetWitness(context.Background(), request.BlockHash, request.BlockNumber, request.MaxBytes)
			response := WitnessPacket{ID: request.ID, BlockHash: request.BlockHash, BlockNumber: request.BlockNumber}
			if err != nil {
				response.Error = []byte(err.Error())
			} else {
				encoded, encodeErr := rlp.EncodeToBytes(witness)
				if encodeErr != nil || len(encoded) > int(request.MaxBytes) || len(encoded) > maxWitnessSize {
					response.Error = []byte("witness exceeds requested size")
				} else {
					response.Witness = encoded
					if len(witness.Headers) == 0 {
						response.Error = []byte("witness has no parent header")
					} else if witness.Headers[0].Number == nil || witness.Headers[0].Number.Sign() < 0 || witness.Headers[0].Number.Uint64()+1 != request.BlockNumber {
						response.Error = []byte("witness parent header does not match requested block")
					} else {
						response.StateRoot = witness.Headers[0].Root
					}
				}
			}
			if err := p2p.Send(peer.rw, WitnessMsg, &response); err != nil {
				return err
			}
		case WitnessMsg:
			var response WitnessPacket
			if err := msg.Decode(&response); err != nil {
				msg.Discard()
				return errInvalidMessage
			}
			if err := msg.Discard(); err != nil {
				return err
			}
			peer.deliverWitness(response)
		default:
			msg.Discard()
			return fmt.Errorf("unknown verkle message code 0x%x", msg.Code)
		}
	}
}

type nodeInfoData struct {
	ChainID string `json:"chainId"`
}

func makeNodeInfo(chain *core.BlockChain) *nodeInfoData {
	if chain == nil || chain.Config() == nil || chain.Config().ChainID == nil {
		return &nodeInfoData{}
	}
	return &nodeInfoData{ChainID: chain.Config().ChainID.String()}
}

type enrEntry struct{}

func (*enrEntry) ENRKey() string { return ProtocolName }
