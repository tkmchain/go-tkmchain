// Copyright 2026 The go-tkmchain Authors
// This file is part of the go-tkmchain library.
//
// The go-tkmchain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-tkmchain library. If not, see <http://www.gnu.org/licenses/>.

//go:build !cgo || !randomx
// +build !cgo !randomx

package randomx

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"
)

const (
	RandomXEpochLength = 2048
	TargetBlockTime    = 120
	EDAThreshold       = 7 * 60
)

type Config struct {
	Enabled        bool
	EpochLength    uint64
	CacheSize      uint64
	DatasetSize    uint64
	MinMemory      uint64
	PersistDataset bool
}

type Work struct {
	HeaderHash  string `json:"header_hash"`
	SeedHash    string `json:"seed_hash"`
	Target      string `json:"target"`
	Difficulty  string `json:"difficulty"`
	BlockNumber uint64 `json:"block_number"`
	Height      uint64 `json:"height"`
}

type RandomX struct {
	config                  *Config
	mainKing                common.Address
	rotatingKings           []common.Address
	rotatingKingActivations map[common.Address]uint64
	rotationInterval        uint64
	miningThreads           int
	fail                    uint64
	lock                    sync.RWMutex
}

var rotatingKingStateSlot = crypto.Keccak256Hash([]byte("randomx.rotatingking"))

func DefaultConfig() *Config {
	return &Config{
		Enabled:     true,
		EpochLength: RandomXEpochLength,
		CacheSize:   256,
		DatasetSize: 2,
		MinMemory:   4,
	}
}

func New(config *Config, threads int, mainKing common.Address, kingAddresses []common.Address) (*RandomX, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if config.EpochLength == 0 {
		config.EpochLength = RandomXEpochLength
	}
	if threads <= 0 {
		threads = 1
	}
	rx := &RandomX{
		config:                  config,
		mainKing:                mainKing,
		rotatingKingActivations: make(map[common.Address]uint64, len(kingAddresses)),
		rotationInterval:        100,
		miningThreads:           threads,
	}
	for _, king := range kingAddresses {
		rx.AddRotatingKing(king)
	}
	return rx, nil
}

func NewFaker() *RandomX {
	rx, _ := New(DefaultConfig(), 1, common.Address{}, nil)
	return rx
}

func NewFullFaker() *RandomX {
	return NewFaker()
}

func NewFakeFailer(fail uint64) *RandomX {
	rx := NewFaker()
	rx.fail = fail
	return rx
}

func (rx *RandomX) Close() error { return nil }

func (rx *RandomX) GetEpochLength() uint64 {
	if rx == nil || rx.config == nil || rx.config.EpochLength == 0 {
		return RandomXEpochLength
	}
	return rx.config.EpochLength
}

func (rx *RandomX) Hashrate() float64 { return 0 }

func (rx *RandomX) GetSharesFound() uint64 { return 0 }

func (rx *RandomX) GetWork() ([]string, error) {
	return nil, errors.New("randomx native mining is unavailable in this build")
}

func (rx *RandomX) SubmitWork(nonceHex string, headerHashHex string, mixDigestHex string) (bool, error) {
	return false, errors.New("randomx native mining is unavailable in this build")
}

func (rx *RandomX) Author(header *types.Header) (common.Address, error) { return header.Coinbase, nil }

func (rx *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
	if header == nil || header.Number == nil {
		return consensus.ErrInvalidNumber
	}
	if rx != nil && rx.fail > 0 && header.Number.Uint64() == rx.fail {
		return fmt.Errorf("invalid fake randomx header %d", rx.fail)
	}
	return nil
}

func (rx *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		defer close(results)
		for _, header := range headers {
			err := rx.VerifyHeader(chain, header)
			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

func (rx *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return consensus.ErrUnknownAncestor
	}
	return nil
}

func (rx *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	if header.Number == nil {
		header.Number = new(big.Int)
	}
	if header.UncleHash == (common.Hash{}) {
		header.UncleHash = types.EmptyUncleHash
	}
	if header.TxHash == (common.Hash{}) {
		header.TxHash = types.EmptyTxsHash
	}
	if header.ReceiptHash == (common.Hash{}) {
		header.ReceiptHash = types.EmptyReceiptsHash
	}
	if header.Difficulty == nil || header.Difficulty.Sign() == 0 {
		header.Difficulty = new(big.Int).Set(GenesisDifficulty)
	}
	return nil
}

func (rx *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	select {
	case results <- block:
	case <-stop:
	}
	return nil
}

func (rx *RandomX) SealHash(header *types.Header) common.Hash { return header.Hash() }

func (rx *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	if parent == nil {
		return new(big.Int).Set(GenesisDifficulty)
	}
	return CalcDifficulty(nil, time, parent, nil)
}

func (rx *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, body *types.Body) {
	rx.finalizeRewards(header, state, body)
}

func (rx *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	if body == nil {
		body = &types.Body{}
	}
	rx.finalizeRewards(header, state, body)
	if header.Coinbase != (common.Address{}) {
		rewards := rx.RewardTransactions(header, receipts)
		before := len(body.Transactions)
		body.Transactions = appendRewardTransactions(body.Transactions, rewards)
		if added := body.Transactions[before:]; len(added) > 0 {
			receipts = append(receipts, rewardReceipts(added, header, header.GasUsed)...)
		}
	}
	if len(receipts) > 0 {
		header.Bloom = types.MergeBloom(receipts)
	}
	eip158 := false
	if chain != nil && chain.Config() != nil {
		eip158 = chain.Config().IsEIP158(header.Number)
	}
	header.Root = state.IntermediateRoot(eip158)
	return types.NewBlock(header, body, receipts, trie.NewStackTrie(nil)), nil
}

func (rx *RandomX) finalizeRewards(header *types.Header, state vm.StateDB, body *types.Body) {
	blockNumber := header.Number.Uint64()
	rx.writeRotatingKingToState(state, blockNumber)
	if header.Coinbase == (common.Address{}) {
		return
	}
	if rx.distributeBodyRewardTransactions(state, body) {
		return
	}
	blockReward := CalculateBlockReward(blockNumber)
	if blockReward.Sign() > 0 {
		rx.distributeRewardsToState(state, header, blockReward)
	}
}

func (rx *RandomX) distributeBodyRewardTransactions(state vm.StateDB, body *types.Body) bool {
	if body == nil || len(body.Transactions) == 0 {
		return false
	}
	start := -1
	for i, tx := range body.Transactions {
		if types.IsBlockRewardTx(tx) {
			start = i
			break
		}
	}
	if start < 0 {
		return false
	}
	for _, tx := range body.Transactions[start:] {
		if !types.IsBlockRewardTx(tx) {
			return false
		}
	}
	for _, tx := range body.Transactions[start:] {
		to := tx.To()
		if to == nil || tx.Value().Sign() == 0 {
			continue
		}
		recipient := *to
		if rewardKind(tx) == types.BlockRewardRotatingKing && recipient == (common.Address{}) && rx.mainKing != (common.Address{}) {
			recipient = rx.mainKing
		}
		if recipient == (common.Address{}) {
			continue
		}
		state.AddBalance(recipient, uint256.MustFromBig(tx.Value()), tracing.BalanceIncreaseRewardMineBlock)
	}
	return true
}

func rewardKind(tx *types.Transaction) int {
	data := tx.Data()
	if len(data) == 0 {
		return -1
	}
	return int(data[len(data)-1])
}

func (rx *RandomX) writeRotatingKingToState(state vm.StateDB, blockNumber uint64) {
	state.SetState(params.SystemAddress, rotatingKingStateSlot, common.BytesToHash(rx.getRotatingKing(blockNumber).Bytes()))
}

// RewardTransactions returns the deterministic synthetic transactions for block rewards.
// StateProcessor uses this during block import to validate synced reward markers.
func (rx *RandomX) RewardTransactions(header *types.Header, receipts []*types.Receipt) []*types.Transaction {
	blockNumber := header.Number.Uint64()
	blockReward := CalculateBlockReward(blockNumber)
	totalReward := CalculateTotalReward(blockReward, nil)
	mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward := rx.rewardMarkerShares(header, totalReward)
	rewards := make([]*types.Transaction, 0, 3)
	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMainKing, mainKing, mainKingReward))
	}
	if rotatingKingReward.Sign() > 0 {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward))
	}
	if minerReward.Sign() > 0 && miner != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, minerReward))
	}
	return rewards
}

func (rx *RandomX) CompatibleRewardTransactions(header *types.Header, receipts []*types.Receipt) [][]*types.Transaction {
	canonical := rx.RewardTransactions(header, receipts)
	candidates := [][]*types.Transaction{canonical}
	for _, candidate := range [][]*types.Transaction{
		rx.legacyRewardTransactions(header, receipts),
		rx.fallbackRewardTransactions(header, receipts),
	} {
		if len(candidate) > 0 && !containsRewardTransactionSet(candidates, candidate) {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func containsRewardTransactionSet(candidates [][]*types.Transaction, target []*types.Transaction) bool {
	for _, candidate := range candidates {
		if sameRewardTransactions(candidate, target) {
			return true
		}
	}
	return false
}

func (rx *RandomX) fallbackRewardTransactions(header *types.Header, receipts []*types.Receipt) []*types.Transaction {
	blockNumber := header.Number.Uint64()
	blockReward := CalculateBlockReward(blockNumber)
	totalReward := CalculateTotalReward(blockReward, nil)
	mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward := rx.rewardShares(header, totalReward)
	rewards := make([]*types.Transaction, 0, 3)
	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMainKing, mainKing, mainKingReward))
	}
	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward))
	} else if rotatingKing == (common.Address{}) && mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward))
	}
	if minerReward.Sign() > 0 && miner != (common.Address{}) {
		rewards = append(rewards, types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, minerReward))
	}
	return rewards
}

func (rx *RandomX) legacyRewardTransactions(header *types.Header, receipts []*types.Receipt) []*types.Transaction {
	blockNumber := header.Number.Uint64()
	blockReward := CalculateBlockReward(blockNumber)
	totalReward := CalculateTotalReward(blockReward, nil)
	mainKing := rx.mainKing
	rotatingKing := rx.getRotatingKing(blockNumber)
	miner := header.Coinbase

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)
	if totalReward != nil && totalReward.Sign() > 0 {
		totalRewardBig := new(big.Int).Set(totalReward)
		mainKingReward.Mul(totalRewardBig, big.NewInt(10))
		mainKingReward.Div(mainKingReward, big.NewInt(100))
		rotatingKingReward.Mul(totalRewardBig, big.NewInt(40))
		rotatingKingReward.Div(rotatingKingReward, big.NewInt(100))
		minerReward.Mul(totalRewardBig, big.NewInt(50))
		minerReward.Div(minerReward, big.NewInt(100))
		actualTotal := new(big.Int).Add(mainKingReward, rotatingKingReward)
		actualTotal.Add(actualTotal, minerReward)
		if actualTotal.Cmp(totalRewardBig) != 0 {
			minerReward.Add(minerReward, new(big.Int).Sub(totalRewardBig, actualTotal))
		}
	}
	return []*types.Transaction{
		types.NewBlockRewardTx(blockNumber, types.BlockRewardMainKing, mainKing, mainKingReward),
		types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, rotatingKingReward),
		types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, minerReward),
	}
}

func (rx *RandomX) rewardMarkerShares(header *types.Header, totalReward *big.Int) (common.Address, *big.Int, common.Address, *big.Int, common.Address, *big.Int) {
	blockNumber := header.Number.Uint64()
	mainKing := rx.mainKing
	rotatingKing := rx.getRotatingKing(blockNumber)
	miner := header.Coinbase

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)
	if totalReward == nil || totalReward.Sign() == 0 {
		return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
	}
	totalRewardBig := new(big.Int).Set(totalReward)
	mainKingReward.Mul(totalRewardBig, big.NewInt(10))
	mainKingReward.Div(mainKingReward, big.NewInt(100))
	rotatingKingReward.Mul(totalRewardBig, big.NewInt(40))
	rotatingKingReward.Div(rotatingKingReward, big.NewInt(100))
	minerReward.Mul(totalRewardBig, big.NewInt(50))
	minerReward.Div(minerReward, big.NewInt(100))

	actualTotal := new(big.Int).Add(mainKingReward, rotatingKingReward)
	actualTotal.Add(actualTotal, minerReward)
	if actualTotal.Cmp(totalRewardBig) != 0 {
		minerReward.Add(minerReward, new(big.Int).Sub(totalRewardBig, actualTotal))
	}
	if mainKing == (common.Address{}) {
		minerReward.Add(minerReward, mainKingReward)
		minerReward.Add(minerReward, rotatingKingReward)
		mainKingReward = new(big.Int)
		rotatingKingReward = new(big.Int)
	}
	return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
}

func sameRewardTransactions(a, b []*types.Transaction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Hash() != b[i].Hash() {
			return false
		}
	}
	return true
}

func appendRewardTransactions(txs []*types.Transaction, rewards []*types.Transaction) []*types.Transaction {
	if len(rewards) == 0 {
		return txs
	}
	if len(txs) >= len(rewards) {
		matches := true
		start := len(txs) - len(rewards)
		for i := range rewards {
			if txs[start+i].Hash() != rewards[i].Hash() {
				matches = false
				break
			}
		}
		if matches {
			return txs
		}
	}
	return append(txs, rewards...)
}

func rewardReceipts(txs []*types.Transaction, header *types.Header, cumulativeGas uint64) []*types.Receipt {
	receipts := make([]*types.Receipt, 0, len(txs))
	for _, tx := range txs {
		receipts = append(receipts, &types.Receipt{
			Type:              tx.Type(),
			Status:            types.ReceiptStatusSuccessful,
			CumulativeGasUsed: cumulativeGas,
			TxHash:            tx.Hash(),
			GasUsed:           0,
			EffectiveGasPrice: new(big.Int),
		})
	}
	return receipts
}

func (rx *RandomX) rewardShares(header *types.Header, totalReward *big.Int) (common.Address, *big.Int, common.Address, *big.Int, common.Address, *big.Int) {
	blockNumber := header.Number.Uint64()
	mainKing := rx.mainKing
	rotatingKing := rx.getRotatingKing(blockNumber)
	miner := header.Coinbase

	mainKingReward := new(big.Int)
	rotatingKingReward := new(big.Int)
	minerReward := new(big.Int)
	if totalReward == nil || totalReward.Sign() == 0 {
		return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
	}
	totalRewardBig := new(big.Int).Set(totalReward)
	mainKingReward.Mul(totalRewardBig, big.NewInt(10))
	mainKingReward.Div(mainKingReward, big.NewInt(100))
	rotatingKingReward.Mul(totalRewardBig, big.NewInt(40))
	rotatingKingReward.Div(rotatingKingReward, big.NewInt(100))
	minerReward.Mul(totalRewardBig, big.NewInt(50))
	minerReward.Div(minerReward, big.NewInt(100))

	actualTotal := new(big.Int).Add(mainKingReward, rotatingKingReward)
	actualTotal.Add(actualTotal, minerReward)
	if actualTotal.Cmp(totalRewardBig) != 0 {
		minerReward.Add(minerReward, new(big.Int).Sub(totalRewardBig, actualTotal))
	}
	if mainKing == (common.Address{}) {
		minerReward.Add(minerReward, mainKingReward)
		mainKingReward = new(big.Int)
	}
	if rotatingKing == (common.Address{}) {
		if mainKing != (common.Address{}) {
			mainKingReward.Add(mainKingReward, rotatingKingReward)
		} else {
			minerReward.Add(minerReward, rotatingKingReward)
		}
		rotatingKingReward = new(big.Int)
	}
	return mainKing, mainKingReward, rotatingKing, rotatingKingReward, miner, minerReward
}

func (rx *RandomX) distributeRewardsToState(state vm.StateDB, header *types.Header, totalReward *big.Int) {
	mainKing, mainKingReward, rotatingKing, rotatingKingReward, coinbase, minerReward := rx.rewardShares(header, totalReward)
	if mainKingReward.Sign() > 0 && mainKing != (common.Address{}) {
		state.AddBalance(mainKing, uint256.MustFromBig(mainKingReward), tracing.BalanceIncreaseRewardMineBlock)
	}
	if rotatingKingReward.Sign() > 0 && rotatingKing != (common.Address{}) {
		state.AddBalance(rotatingKing, uint256.MustFromBig(rotatingKingReward), tracing.BalanceIncreaseRewardMineBlock)
	}
	if minerReward.Sign() > 0 && coinbase != (common.Address{}) {
		state.AddBalance(coinbase, uint256.MustFromBig(minerReward), tracing.BalanceIncreaseRewardMineBlock)
	}
}

func (rx *RandomX) SetRotationInterval(interval uint64) {
	if interval == 0 {
		return
	}
	rx.lock.Lock()
	defer rx.lock.Unlock()
	rx.rotationInterval = interval
}

func (rx *RandomX) SetThreads(threads int) {
	if threads <= 0 {
		return
	}
	rx.lock.Lock()
	defer rx.lock.Unlock()
	rx.miningThreads = threads
}

func (rx *RandomX) AddRotatingKing(address common.Address) {
	rx.AddRotatingKingAt(address, 0)
}

func (rx *RandomX) AddRotatingKingAt(address common.Address, activationHeight uint64) {
	if address == (common.Address{}) || address == rx.mainKing {
		return
	}
	rx.lock.Lock()
	defer rx.lock.Unlock()
	for _, existing := range rx.rotatingKings {
		if existing == address {
			if current, ok := rx.rotatingKingActivations[address]; !ok || activationHeight < current {
				rx.rotatingKingActivations[address] = activationHeight
			}
			return
		}
	}
	rx.rotatingKings = append(rx.rotatingKings, address)
	rx.rotatingKingActivations[address] = activationHeight
}

func (rx *RandomX) getRotatingKing(blockNumber uint64) common.Address {
	rx.lock.RLock()
	defer rx.lock.RUnlock()
	active := rx.activeRotatingKingsAtLocked(blockNumber)
	if len(active) == 0 {
		return common.Address{}
	}
	interval := rx.rotationInterval
	if interval == 0 {
		interval = 100
	}
	return active[(blockNumber/interval)%uint64(len(active))]
}

func (rx *RandomX) activeRotatingKingsAtLocked(blockNumber uint64) []common.Address {
	active := make([]common.Address, 0, len(rx.rotatingKings))
	for _, address := range rx.rotatingKings {
		if activation, ok := rx.rotatingKingActivations[address]; !ok || blockNumber >= activation {
			active = append(active, address)
		}
	}
	return active
}

func (rx *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return nil
}
