// Copyright 2022 The go-ethereum Authors
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

package miner

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/telemetry"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"go.opentelemetry.io/otel/trace"
)

// PayloadID is an 8-byte identifier for a payload (no longer tied to engine API)
type PayloadID [8]byte

// String implements the stringer interface.
func (id PayloadID) String() string {
	return hexutil.Encode(id[:])
}

// BuildPayloadArgs contains the provided parameters for building payload.
type BuildPayloadArgs struct {
	Parent       common.Hash       // The parent block to build payload on top
	Timestamp    uint64            // The provided timestamp of generated payload
	FeeRecipient common.Address    // The provided recipient address for collecting transaction fee (miner gets 50%)
	Random       common.Hash       // The provided randomness value
	Withdrawals  types.Withdrawals // The provided withdrawals
	BeaconRoot   *common.Hash      // The provided beaconRoot (Cancun) - optional for RandomX
	SlotNum      *uint64           // The provided slotNumber - optional for RandomX

	// King addresses for reward distribution
	MainKingAddr     common.Address
	RotatingKingAddr common.Address
}

// Id computes an 8-byte identifier by hashing the components of the payload arguments.
func (args *BuildPayloadArgs) Id() PayloadID {
	hasher := sha256.New()
	hasher.Write(args.Parent[:])
	binary.Write(hasher, binary.BigEndian, args.Timestamp)
	hasher.Write(args.Random[:])
	hasher.Write(args.FeeRecipient[:])
	rlp.Encode(hasher, args.Withdrawals)
	if args.BeaconRoot != nil {
		hasher.Write(args.BeaconRoot[:])
	}
	if args.SlotNum != nil {
		binary.Write(hasher, binary.BigEndian, args.SlotNum)
	}
	// Include king addresses in ID for uniqueness
	if args.MainKingAddr != (common.Address{}) {
		hasher.Write(args.MainKingAddr[:])
	}
	if args.RotatingKingAddr != (common.Address{}) {
		hasher.Write(args.RotatingKingAddr[:])
	}
	var out PayloadID
	copy(out[:], hasher.Sum(nil)[:8])
	return out
}

// Payload wraps the built payload (block waiting for sealing).
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

// newPayload initializes the payload object.
func newPayload(empty *types.Block, emptyRequests [][]byte, witness *stateless.Witness, id PayloadID) *Payload {
	payload := &Payload{
		id:            id,
		empty:         empty,
		emptyRequests: emptyRequests,
		emptyWitness:  witness,
		stop:          make(chan struct{}),
	}
	log.Info("Starting work on payload", "id", payload.id)
	payload.cond = sync.NewCond(&payload.lock)
	return payload
}

// update updates the full-block with latest built version.
func (payload *Payload) update(r *newPayloadResult, elapsed time.Duration) (result bool) {
	payload.lock.Lock()
	defer payload.lock.Unlock()

	select {
	case <-payload.stop:
		return false // reject stale update
	default:
	}

	if payload.full == nil || r.fees.Cmp(payload.fullFees) > 0 {
		payload.full = r.block
		payload.fullFees = r.fees
		payload.sidecars = r.sidecars
		payload.requests = r.requests
		payload.fullWitness = r.witness

		feesInEther := new(big.Float).Quo(new(big.Float).SetInt(r.fees), big.NewFloat(params.Ether))
		log.Info("Updated payload",
			"id", payload.id,
			"number", r.block.NumberU64(),
			"hash", r.block.Hash(),
			"txs", len(r.block.Transactions()),
			"withdrawals", len(r.block.Withdrawals()),
			"gas", r.block.GasUsed(),
			"fees", feesInEther,
			"root", r.block.Root(),
			"elapsed", common.PrettyDuration(elapsed),
		)
		result = true
	}
	payload.cond.Broadcast()
	return
}

// Resolve returns the latest built payload.
func (payload *Payload) Resolve() *ExecutionPayloadEnvelope {
	payload.lock.Lock()
	defer payload.lock.Unlock()

	select {
	case <-payload.stop:
	default:
		close(payload.stop)
	}
	if payload.full != nil {
		envelope := BlockToExecutableData(payload.full, payload.fullFees, payload.sidecars, payload.requests)
		if payload.fullWitness != nil {
			envelope.Witness = new(hexutil.Bytes)
			*envelope.Witness, _ = rlp.EncodeToBytes(payload.fullWitness)
		}
		return envelope
	}
	envelope := BlockToExecutableData(payload.empty, big.NewInt(0), nil, payload.emptyRequests)
	if payload.emptyWitness != nil {
		envelope.Witness = new(hexutil.Bytes)
		*envelope.Witness, _ = rlp.EncodeToBytes(payload.emptyWitness)
	}
	return envelope
}

// ResolveEmpty returns the empty payload (for testing).
func (payload *Payload) ResolveEmpty() *ExecutionPayloadEnvelope {
	payload.lock.Lock()
	defer payload.lock.Unlock()

	envelope := BlockToExecutableData(payload.empty, big.NewInt(0), nil, payload.emptyRequests)
	if payload.emptyWitness != nil {
		envelope.Witness = new(hexutil.Bytes)
		*envelope.Witness, _ = rlp.EncodeToBytes(payload.emptyWitness)
	}
	return envelope
}

// ResolveFull returns the full payload (for testing).
func (payload *Payload) ResolveFull() *ExecutionPayloadEnvelope {
	payload.lock.Lock()
	defer payload.lock.Unlock()

	if payload.full == nil {
		select {
		case <-payload.stop:
			return nil
		default:
		}
		payload.cond.Wait()
	}
	select {
	case <-payload.stop:
	default:
		close(payload.stop)
	}
	envelope := BlockToExecutableData(payload.full, payload.fullFees, payload.sidecars, payload.requests)
	if payload.fullWitness != nil {
		envelope.Witness = new(hexutil.Bytes)
		*envelope.Witness, _ = rlp.EncodeToBytes(payload.fullWitness)
	}
	return envelope
}

// ExecutionPayloadEnvelope wraps the execution payload for delivery
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

func (miner *Miner) runBuildIteration(ctx context.Context, start time.Time, iteration int, payload *Payload, params *generateParams, witness bool) {
	ctx, span, spanEnd := telemetry.StartSpan(ctx, "miner.buildIteration",
		telemetry.Int64Attribute("iteration", int64(iteration)),
	)
	var err error
	defer spanEnd(&err)

	r := miner.generateWork(ctx, params, witness)
	err = r.err
	if err == nil {
		accepted := payload.update(r, time.Since(start))
		span.SetAttributes(telemetry.BoolAttribute("update.accepted", accepted))
	} else {
		log.Info("Error while generating work", "id", payload.id, "err", err)
	}
}

// buildPayload builds the payload according to the provided parameters.
func (miner *Miner) buildPayload(ctx context.Context, args *BuildPayloadArgs, witness bool) (result *Payload, err error) {
	payloadID := args.Id()
	ctx, _, spanEnd := telemetry.StartSpan(ctx, "miner.buildPayload",
		telemetry.StringAttribute("payload.id", payloadID.String()),
		telemetry.StringAttribute("parent.hash", args.Parent.String()),
		telemetry.Int64Attribute("timestamp", int64(args.Timestamp)),
	)
	defer spanEnd(&err)

	// Build the initial version with no transaction included
	emptyParams := &generateParams{
		timestamp:   args.Timestamp,
		forceTime:   true,
		parentHash:  args.Parent,
		coinbase:    args.FeeRecipient,
		random:      args.Random,
		withdrawals: args.Withdrawals,
		beaconRoot:  args.BeaconRoot,
		slotNum:     args.SlotNum,
		noTxs:       true,
	}
	empty := miner.generateWork(ctx, emptyParams, witness)
	if empty.err != nil {
		return nil, empty.err
	}

	payload := newPayload(empty.block, empty.requests, empty.witness, payloadID)

	// Spin up a routine for updating the payload in background
	go func() {
		var iteration int
		bCtx, bSpan, bSpanEnd := telemetry.StartSpan(ctx, "miner.background",
			telemetry.Int64Attribute("block.number", int64(empty.block.NumberU64())),
		)
		defer func() {
			bSpan.SetAttributes(telemetry.Int64Attribute("iterations.total", int64(iteration)))
			bSpanEnd(nil)
		}()

		timer := time.NewTimer(0)
		defer timer.Stop()

		// 12 second timeout for slot (typical for Ethereum)
		endTimer := time.NewTimer(time.Second * 12)

		fullParams := &generateParams{
			timestamp:   args.Timestamp,
			forceTime:   true,
			parentHash:  args.Parent,
			coinbase:    args.FeeRecipient,
			random:      args.Random,
			withdrawals: args.Withdrawals,
			beaconRoot:  args.BeaconRoot,
			slotNum:     args.SlotNum,
			noTxs:       false,
		}

		for {
			select {
			case <-timer.C:
				select {
				case <-payload.stop:
					payload.updateSpanForDelivery(bSpan)
					log.Info("Stopping work on payload", "id", payload.id, "reason", "delivery")
					return
				default:
				}
				start := time.Now()
				iteration++
				miner.runBuildIteration(bCtx, start, iteration, payload, fullParams, witness)
				timer.Reset(max(0, miner.recommit-time.Since(start)))
			case <-payload.stop:
				payload.updateSpanForDelivery(bSpan)
				log.Info("Stopping work on payload", "id", payload.id, "reason", "delivery")
				return
			case <-endTimer.C:
				bSpan.SetAttributes(telemetry.StringAttribute("exit.reason", "timeout"))
				log.Info("Stopping work on payload", "id", payload.id, "reason", "timeout")
				return
			}
		}
	}()
	return payload, nil
}

func (payload *Payload) updateSpanForDelivery(bSpan trace.Span) {
	payload.lock.Lock()
	emptyDelivered := payload.full == nil
	payload.lock.Unlock()
	bSpan.SetAttributes(
		telemetry.StringAttribute("exit.reason", "delivery"),
		telemetry.BoolAttribute("empty.delivered", emptyDelivered),
	)
}

// BuildTestingPayload is for testing purposes only.
func (miner *Miner) BuildTestingPayload(args *BuildPayloadArgs, transactions []*types.Transaction, empty bool, extraData []byte) (*ExecutionPayloadEnvelope, error) {
	fullParams := &generateParams{
		timestamp:         args.Timestamp,
		forceTime:         true,
		parentHash:        args.Parent,
		coinbase:          args.FeeRecipient,
		random:            args.Random,
		withdrawals:       args.Withdrawals,
		beaconRoot:        args.BeaconRoot,
		slotNum:           args.SlotNum,
		noTxs:             empty,
		forceOverrides:    true,
		overrideExtraData: extraData,
		overrideTxs:       transactions,
	}
	res := miner.generateWork(context.Background(), fullParams, false)
	if res.err != nil {
		return nil, res.err
	}
	return BlockToExecutableData(res.block, res.fees, res.sidecars, res.requests), nil
}

// Helper function
func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
