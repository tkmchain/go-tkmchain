// Copyright 2026 The go-ethereum Authors
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

// Package randomx implements the RandomX proof-of-work consensus engine.
package randomx

import (
    "bytes"
    "encoding/binary"
    "errors"
    "fmt"
    "math/big"
    "runtime"
    "sync"
    "time"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/consensus"
    "github.com/ethereum/go-ethereum/consensus/misc"
    "github.com/ethereum/go-ethereum/core/state"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/params"
    "github.com/ethereum/go-ethereum/rlp"
    "github.com/ethereum/go-ethereum/rpc"
    "golang.org/x/crypto/sha3"
    "modernc.org/randomx"
)

// RandomX proof-of-work consensus engine
type RandomX struct {
    config      *params.RandomXConfig
    cache       *randomx.Cache
    vm          *randomx.VM
    mu          sync.RWMutex
    cacheMu     sync.RWMutex
    cacheEpoch  uint64
    dataset     *randomx.Dataset
    
    // Mining related fields
    threads     int
    workerWg    sync.WaitGroup
    localWork   map[common.Hash]*types.Block
    localMu     sync.RWMutex
}

// New creates a new RandomX consensus engine
func New(config *params.RandomXConfig, threadCount int) *RandomX {
    if threadCount <= 0 {
        threadCount = runtime.NumCPU()
    }
    
    return &RandomX{
        config:    config,
        threads:   threadCount,
        localWork: make(map[common.Hash]*types.Block),
    }
}

// Author implements consensus.Engine, returning the coinbase from the header
func (r *RandomX) Author(header *types.Header) (common.Address, error) {
    return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules
func (r *RandomX) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
    return r.verifyHeader(chain, header, nil, seal)
}

func (r *RandomX) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parent *types.Header, seal bool) error {
    // Ensure that the header's extra-data section is of reasonable size
    if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
        return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
    }
    
    // Verify the header's timestamp
    if header.Time > uint64(time.Now().Add(10*time.Second).Unix()) {
        return consensus.ErrFutureBlock
    }
    
    // Verify the difficulty
    if parent == nil {
        parent = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
    }
    expectedDiff, err := r.CalcDifficulty(chain, header.Time, parent)
    if err != nil {
        return err
    }
    if expectedDiff.Cmp(header.Difficulty) != 0 {
        return fmt.Errorf("invalid difficulty: have %v, want %v", header.Difficulty, expectedDiff)
    }
    
    // Verify that the gas limit is within bounds
    if header.GasLimit > params.MaxGasLimit {
        return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, params.MaxGasLimit)
    }
    if header.GasLimit < params.MinGasLimit {
        return fmt.Errorf("invalid gasLimit: have %v, min %v", header.GasLimit, params.MinGasLimit)
    }
    
    // Verify that the gasUsed is <= gasLimit
    if header.GasUsed > header.GasLimit {
        return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
    }
    
    // Verify the block's difficulty to ensure it's valid
    if seal {
        if err := r.VerifySeal(chain, header); err != nil {
            return err
        }
    }
    
    return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers concurrently
func (r *RandomX) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
    abort := make(chan struct{})
    results := make(chan error, len(headers))
    
    go func() {
        for i, header := range headers {
            var parent *types.Header
            if i == 0 {
                parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
            } else if headers[i-1].Hash() == headers[i].ParentHash {
                parent = headers[i-1]
            }
            
            err := r.verifyHeader(chain, header, parent, seals[i])
            select {
            case <-abort:
                return
            case results <- err:
            }
        }
    }()
    
    return abort, results
}

// VerifyUncles implements consensus.Engine, but RandomX doesn't have uncles
func (r *RandomX) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
    if len(block.Uncles()) > 0 {
        return errors.New("uncles not allowed in RandomX consensus")
    }
    return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine
func (r *RandomX) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
    parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
    if parent == nil {
        return consensus.ErrUnknownAncestor
    }
    
    header.Difficulty = r.CalcDifficulty(chain, header.Time, parent)
    return nil
}

// Finalize implements consensus.Engine, accumulating the block rewards
func (r *RandomX) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header) (*types.Block, error) {
    // Accumulate any block and uncle rewards
    misc.AccumulateRewards(chain.Config(), state, header, uncles)
    
    // Root the block body
    header.Root = state.IntermediateRoot(true)
    
    // Generate the block
    return types.NewBlock(header, txs, uncles, nil), nil
}

// FinalizeAndAssemble implements consensus.Engine, creating the final block
func (r *RandomX) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
    // Accumulate any block and uncle rewards
    misc.AccumulateRewards(chain.Config(), state, header, uncles)
    
    // Root the block body
    header.Root = state.IntermediateRoot(true)
    header.Bloom = types.CreateBloom(receipts)
    
    // Assemble and return the final block for sealing
    return types.NewBlock(header, txs, uncles, receipts), nil
}

// Seal generates a new sealing request for the given input block and pushes
// the result into the given channel
func (r *RandomX) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
    header := block.Header()
    if header.Number.Sign() == 0 {
        return errors.New("sealing genesis block is not allowed")
    }
    
    // Update the dataset for the current epoch
    epoch := r.epoch(header.Number.Uint64())
    if err := r.updateCacheForEpoch(epoch); err != nil {
        return err
    }
    
    // Start mining on multiple threads
    r.workerWg.Add(r.threads)
    for i := 0; i < r.threads; i++ {
        go r.mine(block, chain, results, stop, i)
    }
    
    // Wait for mining to complete or stop
    go func() {
        r.workerWg.Wait()
        close(results)
    }()
    
    return nil
}

// mine is the actual mining thread that tries to find a valid nonce
func (r *RandomX) mine(block *types.Block, chain consensus.ChainHeaderReader, results chan<- *types.Block, stop <-chan struct{}, thread int) {
    defer r.workerWg.Done()
    
    header := block.Header()
    target := new(big.Int).Div(maxUint256, header.Difficulty)
    
    // Create a copy of the header for mining
    mineHeader := types.CopyHeader(header)
    
    // Seed for the random hash
    seed := r.seedHash(mineHeader.Number.Uint64())
    
    // Get the RandomX VM for this thread
    vm := r.getVMForThread(thread)
    defer r.releaseVMForThread(thread, vm)
    
    nonce := uint64(0)
    for {
        select {
        case <-stop:
            return
        default:
            // Try mining with current nonce
            mineHeader.Nonce = types.EncodeNonce(nonce)
            digest, result := r.hashimotoFull(mineHeader, seed, vm)
            
            if result.Cmp(target) <= 0 {
                // Seal found!
                mineHeader.MixDigest = digest
                sealedBlock := block.WithSeal(mineHeader)
                select {
                case results <- sealedBlock:
                case <-stop:
                }
                return
            }
            nonce++
            
            // Reset nonce after overflow
            if nonce == 0 {
                break
            }
        }
    }
}

// VerifySeal implements consensus.Engine, checking whether a given block satisfies
// the RandomX proof-of-work difficulty
func (r *RandomX) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header) error {
    // Ensure that the header's extra-data section is of reasonable size
    if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
        return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
    }
    
    // Verify the nonce and mix digest
    epoch := r.epoch(header.Number.Uint64())
    if err := r.updateCacheForEpoch(epoch); err != nil {
        return err
    }
    
    seed := r.seedHash(header.Number.Uint64())
    
    // Verify the PoW
    digest, result := r.hashimotoLight(header, seed)
    if !bytes.Equal(digest.Bytes(), header.MixDigest.Bytes()) {
        return errors.New("invalid mix digest")
    }
    
    target := new(big.Int).Div(maxUint256, header.Difficulty)
    if result.Cmp(target) > 0 {
        return fmt.Errorf("invalid proof-of-work: result %s > target %s", result.String(), target.String())
    }
    
    return nil
}

// CalcDifficulty is the difficulty adjustment algorithm for RandomX
func (r *RandomX) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) (*big.Int, error) {
    return r.config.DifficultyCalculator(time, parent)
}

// hashimotoLight aggregates the RandomX hash for light verification
func (r *RandomX) hashimotoLight(header *types.Header, seed common.Hash) (digest common.Hash, result *big.Int) {
    r.cacheMu.RLock()
    defer r.cacheMu.RUnlock()
    
    if r.cache == nil {
        return common.Hash{}, big.NewInt(0)
    }
    
    // Create a VM for verification
    vm := randomx.NewVM(r.cache, nil)
    defer vm.Close()
    
    return r.hashimoto(header, seed, vm)
}

// hashimotoFull aggregates the RandomX hash for full mining
func (r *RandomX) hashimotoFull(header *types.Header, seed common.Hash, vm *randomx.VM) (digest common.Hash, result *big.Int) {
    return r.hashimoto(header, seed, vm)
}

// hashimoto is the core RandomX hash function
func (r *RandomX) hashimoto(header *types.Header, seed common.Hash, vm *randomx.VM) (digest common.Hash, result *big.Int) {
    // Prepare the input for RandomX
    input := make([]byte, 40)
    copy(input[:32], seed.Bytes())
    copy(input[32:], header.Nonce.Bytes())
    
    // Additional header data for uniqueness
    headerData, _ := rlp.EncodeToBytes([]interface{}{
        header.ParentHash,
        header.Coinbase,
        header.Root,
        header.TxHash,
        header.ReceiptHash,
        header.Bloom,
        header.Difficulty,
        header.Number,
        header.GasLimit,
        header.GasUsed,
        header.Time,
        header.Extra,
    })
    
    hasher := sha3.NewLegacyKeccak256()
    hasher.Write(headerData)
    hasher.Write(input)
    finalInput := hasher.Sum(nil)
    
    // Execute RandomX
    output := make([]byte, 32)
    vm.CalculateHash(finalInput, output)
    
    // First 32 bytes are the mix digest, convert the whole output to big.Int
    copy(digest[:], output[:32])
    result = new(big.Int).SetBytes(output[:])
    
    return digest, result
}

// seedHash computes the seed hash for a given block number
func (r *RandomX) seedHash(blockNum uint64) common.Hash {
    epoch := r.epoch(blockNum)
    seed := make([]byte, 32)
    
    // Calculate seed based on epoch
    for i := uint64(0); i < epoch; i++ {
        hasher := sha3.NewLegacyKeccak256()
        hasher.Write(seed)
        seed = hasher.Sum(nil)
    }
    
    return common.BytesToHash(seed)
}

// epoch returns the epoch for a given block number
func (r *RandomX) epoch(blockNum uint64) uint64 {
    return blockNum / r.config.EpochLength
}

// updateCacheForEpoch ensures the RandomX cache and dataset are ready for the given epoch
func (r *RandomX) updateCacheForEpoch(epoch uint64) error {
    r.cacheMu.Lock()
    defer r.cacheMu.Unlock()
    
    if r.cacheEpoch != epoch {
        seed := r.seedHash(epoch * r.config.EpochLength)
        
        // Create new cache
        cache, err := randomx.NewCache(seed.Bytes())
        if err != nil {
            return fmt.Errorf("failed to create RandomX cache: %v", err)
        }
        
        // Close old cache
        if r.cache != nil {
            r.cache.Close()
        }
        
        r.cache = cache
        r.cacheEpoch = epoch
        
        // Initialize dataset for mining
        if r.dataset != nil {
            r.dataset.Close()
        }
        r.dataset = randomx.NewDataset(r.cache)
    }
    
    return nil
}

// getVMForThread gets or creates a RandomX VM for a specific thread
func (r *RandomX) getVMForThread(thread int) *randomx.VM {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    if r.vm != nil {
        // Clone VM for thread safety
        return r.vm.Clone()
    }
    return nil
}

// releaseVMForThread releases a VM back to the pool
func (r *RandomX) releaseVMForThread(thread int, vm *randomx.VM) {
    if vm != nil {
        vm.Close()
    }
}

// APIs returns the RPC APIs this consensus engine provides
func (r *RandomX) APIs(chain consensus.ChainHeaderReader) []rpc.API {
    return []rpc.API{
        {
            Namespace: "randomx",
            Version:   "1.0",
            Service:   &API{chain: chain, randomx: r},
            Public:    true,
        },
    }
}

// Close terminates any background threads maintained by the consensus engine
func (r *RandomX) Close() error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if r.cache != nil {
        r.cache.Close()
    }
    if r.dataset != nil {
        r.dataset.Close()
    }
    
    return nil
}

// maxUint256 is a big integer representing 2^256-1
var maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
