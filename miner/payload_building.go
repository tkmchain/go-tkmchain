// Copyright 2022 The go-ethereum Authors
// This file is part of the go-ethereum library.

package miner

import (
        "crypto/sha256"
        "encoding/binary"
        "math/big"
        "sync"

        "github.com/ethereum/go-ethereum/common"
        "github.com/ethereum/go-ethereum/common/hexutil"
        "github.com/ethereum/go-ethereum/core/stateless"
        "github.com/ethereum/go-ethereum/core/types"
        "github.com/ethereum/go-ethereum/rlp"
)

// PayloadID is an 8-byte identifier for a payload
type PayloadID [8]byte

// String implements the stringer interface.
func (id PayloadID) String() string {
        return hexutil.Encode(id[:])
}

// BuildPayloadArgs contains the parameters for building payload
type BuildPayloadArgs struct {
        Parent       common.Hash
        Timestamp    uint64
        FeeRecipient common.Address
        Random       common.Hash
        Withdrawals  types.Withdrawals
        Extra        []byte
}

// Id computes an 8-byte identifier
func (args *BuildPayloadArgs) Id() PayloadID {
        hasher := sha256.New()
        hasher.Write(args.Parent[:])
        binary.Write(hasher, binary.BigEndian, args.Timestamp)
        hasher.Write(args.Random[:])
        hasher.Write(args.FeeRecipient[:])
        rlp.Encode(hasher, args.Withdrawals)
        if len(args.Extra) > 0 {
                hasher.Write(args.Extra)
        }
        var out PayloadID
        copy(out[:], hasher.Sum(nil)[:8])
        return out
}

// Payload wraps the built payload
type Payload struct {
        id            PayloadID
        empty         *types.Block
        emptyWitness  *stateless.Witness
        full          *types.Block
        fullWitness   *stateless.Witness
        sidecars      []*types.BlobTxSidecar
        emptyRequests [][]byte
        requests      [][]byte
        fullFees      *big.Int
        stop          chan struct{}
        lock          sync.Mutex
        cond          *sync.Cond
}

// newPayload initializes the payload object
func newPayload(empty *types.Block, emptyRequests [][]byte, witness *stateless.Witness, id PayloadID) *Payload {
        payload := &Payload{
                id:            id,
                empty:         empty,
                emptyRequests: emptyRequests,
                emptyWitness:  witness,
                stop:          make(chan struct{}),
        }
        payload.cond = sync.NewCond(&payload.lock)
        return payload
}

// ExecutionPayloadEnvelope wraps the execution payload
type ExecutionPayloadEnvelope struct {
        Block    *types.Block
        Fees     *big.Int
        Sidecars []*types.BlobTxSidecar
        Requests [][]byte
        Witness  *hexutil.Bytes
}

// BlockToExecutableData converts a block to executable data
func BlockToExecutableData(block *types.Block, fees *big.Int, sidecars []*types.BlobTxSidecar, requests [][]byte) *ExecutionPayloadEnvelope {
        return &ExecutionPayloadEnvelope{
                Block:    block,
                Fees:     fees,
                Sidecars: sidecars,
                Requests: requests,
        }
}

// BuildTestingPayload is for testing
func BuildTestingPayload(block *types.Block) *ExecutionPayloadEnvelope {
        return BlockToExecutableData(block, big.NewInt(0), nil, nil)
}
