package miner

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
)

type generateParams struct {
	timestamp         uint64
	forceTime         bool
	parentHash        common.Hash
	coinbase          common.Address
	random            common.Hash
	withdrawals       types.Withdrawals
	beaconRoot        *common.Hash
	slotNum           *uint64
	noTxs             bool
	forceOverrides    bool
	overrideExtraData []byte
	overrideTxs       []*types.Transaction
}

type newPayloadResult struct {
	block    *types.Block
	fees     *big.Int
	sidecars []*types.BlobTxSidecar
	requests [][]byte
	witness  *stateless.Witness
	err      error
}

func (miner *Miner) generateWork(_ context.Context, _ *generateParams, _ bool) *newPayloadResult {
	return &newPayloadResult{err: errors.New("generateWork not implemented")}
}
