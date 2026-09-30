package core

import (
	"context"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

type speculativeTransaction struct {
	receipt  *types.Receipt
	delta    *state.SpeculativeDelta
	gas      *GasPool
	access   antartical.AccessSet
	peakGas  uint64
	gasLimit uint64
}

// executeParallelTransfers is the commit phase of Antartical optimistic
// execution. It deliberately admits only transactions whose destination is an
// account without code. Such transfers have a complete, explicit account
// write-set (sender and recipient), so the state deltas can be checked and
// merged without guessing about internal contract calls. Contract execution,
// account creation, and consensus envelopes remain on the serial path until
// their dynamic access witnesses are available.
func (p *StateProcessor) executeParallelTransfers(ctx context.Context, block *types.Block, statedb *state.StateDB, blockContext vm.BlockContext, signer types.Signer, cfg vm.Config, gp *GasPool) (types.Receipts, []*types.Log, common.Hash, bool, error) {
	if cfg.Tracer != nil || block == nil || len(block.Transactions()) < 2 {
		return nil, nil, common.Hash{}, false, nil
	}
	txs := block.Transactions()
	for _, tx := range txs {
		if tx == nil || types.IsBlockRewardTx(tx) || tx.Type() == types.PQTkmTxType || tx.To() == nil || hasConsensusSideEffects(tx.Data()) {
			return nil, nil, common.Hash{}, false, nil
		}
		if statedb.GetCodeSize(*tx.To()) != 0 {
			return nil, nil, common.Hash{}, false, nil
		}
	}

	access := make([]antartical.AccessSet, len(txs))
	for i, tx := range txs {
		msg, err := TransactionToMessage(tx, signer, block.BaseFee())
		if err != nil {
			return nil, nil, common.Hash{}, false, err
		}
		// A simple transfer has no hidden storage access. Include both account
		// writes so transfers sharing a sender or recipient are serialized.
		access[i] = antartical.AccessSet{Reads: []common.Address{msg.From}, Writes: []common.Address{msg.From, *tx.To()}}
		sort.Slice(access[i].Reads, func(a, b int) bool { return access[i].Reads[a].Hex() < access[i].Reads[b].Hex() })
		sort.Slice(access[i].Writes, func(a, b int) bool { return access[i].Writes[a].Hex() < access[i].Writes[b].Hex() })
	}

	results, transcript, err := antartical.ExecuteOptimisticResults(txs, access, func(index int, tx *types.Transaction) (speculativeTransaction, antartical.AccessSet, error) {
		copyState := statedb.Copy()
		copyState.SetTxContext(tx.Hash(), index)
		message, err := TransactionToMessage(tx, signer, block.BaseFee())
		if err != nil {
			return speculativeTransaction{}, antartical.AccessSet{}, err
		}
		localGas := NewGasPool(block.GasLimit())
		evm := vm.NewEVM(blockContext, copyState, p.chainConfig(), cfg)
		defer evm.Release()
		receipt, err := ApplyTransactionWithEVM(message, localGas, copyState, block.Number(), block.Hash(), block.Time(), tx, evm)
		if err != nil {
			return speculativeTransaction{}, antartical.AccessSet{}, err
		}
		delta, err := copyState.BuildSpeculativeDelta()
		if err != nil {
			return speculativeTransaction{}, antartical.AccessSet{}, err
		}
		return speculativeTransaction{receipt: receipt, delta: delta, gas: localGas, peakGas: localGas.Used(), gasLimit: tx.Gas()}, access[index], nil
	})
	if err != nil {
		return nil, nil, common.Hash{}, false, err
	}
	for _, result := range results {
		if err := statedb.CanApplySpeculativeDelta(result.delta); err != nil {
			// A state changed between speculation and commit. The caller retries
			// the complete block through the canonical serial loop.
			return nil, nil, common.Hash{}, false, nil
		}
	}

	// Validate the gas schedule on a copy before mutating the canonical pool.
	gasCheck := gp.Snapshot()
	for i, result := range results {
		if err := gasCheck.SubGas(result.gasLimit); err != nil {
			return nil, nil, common.Hash{}, false, fmt.Errorf("parallel transaction %d: %w", i, err)
		}
		returned := result.gasLimit - result.peakGas
		if !p.chainConfig().IsAmsterdam(block.Number(), block.Time()) {
			returned = result.gasLimit - result.receipt.GasUsed
		}
		if err := gasCheck.ReturnGas(returned, result.receipt.GasUsed); err != nil {
			return nil, nil, common.Hash{}, false, err
		}
	}

	// Apply deltas to a throw-away state first. This makes trie/read errors a
	// precondition failure rather than leaving the canonical state half merged.
	checkState := statedb.Copy()
	for i, result := range results {
		checkState.SetTxContext(txs[i].Hash(), i)
		if err := checkState.ApplySpeculativeDelta(result.delta); err != nil {
			return nil, nil, common.Hash{}, false, nil
		}
		if checkState.IntermediateRoot(p.chainConfig().IsEIP158(block.Number())) == (common.Hash{}) {
			return nil, nil, common.Hash{}, false, nil
		}
	}
	for i, result := range results {
		statedb.SetTxContext(txs[i].Hash(), i)
		if err := statedb.ApplySpeculativeDelta(result.delta); err != nil {
			return nil, nil, common.Hash{}, false, fmt.Errorf("parallel state commit %d: %w", i, err)
		}
		if statedb.IntermediateRoot(p.chainConfig().IsEIP158(block.Number())) == (common.Hash{}) {
			return nil, nil, common.Hash{}, false, fmt.Errorf("parallel state commit %d: empty state root", i)
		}
	}
	gp.Set(gasCheck)

	receipts := make(types.Receipts, len(results))
	logs := make([]*types.Log, 0)
	for i, result := range results {
		result.receipt.CumulativeGasUsed = gasCheck.CumulativeUsed() // corrected below in order
		receipts[i] = result.receipt
		logs = append(logs, result.receipt.Logs...)
		// Recompute the cumulative value at the exact transaction boundary.
		boundary := NewGasPool(block.GasLimit())
		for j := 0; j <= i; j++ {
			_ = boundary.SubGas(results[j].gasLimit)
			returned := results[j].gasLimit - results[j].peakGas
			if !p.chainConfig().IsAmsterdam(block.Number(), block.Time()) {
				returned = results[j].gasLimit - results[j].receipt.GasUsed
			}
			_ = boundary.ReturnGas(returned, results[j].receipt.GasUsed)
		}
		result.receipt.CumulativeGasUsed = boundary.CumulativeUsed()
	}
	commitment, err := transcript.Commitment()
	if err != nil {
		return nil, nil, common.Hash{}, false, err
	}
	return receipts, logs, commitment, true, nil
}

// speculativeConflictTranscript executes eligible transactions on independent
// snapshots. The canonical StateDB is still committed by Process's ordered
// pass; this phase supplies a deterministic Block-STM schedule and catches
// transactions that cannot execute from the declared pre-state. Any operation
// with consensus-side effects is deliberately routed through the serial path.
func (p *StateProcessor) speculativeConflictTranscript(ctx context.Context, block *types.Block, statedb *state.StateDB, blockHash common.Hash, context vm.BlockContext, signer types.Signer, cfg vm.Config) (antartical.ConflictTranscript, bool, error) {
	transactions := block.Transactions()
	if len(transactions) < 2 {
		return antartical.ConflictTranscript{}, false, nil
	}
	access := make([]antartical.AccessSet, len(transactions))
	senders := make(map[common.Address]struct{}, len(transactions))
	for i, tx := range transactions {
		if tx == nil || tx.Type() == types.PQTkmTxType || tx.To() == nil || hasConsensusSideEffects(tx.Data()) {
			return antartical.ConflictTranscript{}, false, nil
		}
		access[i] = antartical.AccessSetForTransaction(tx)
		if access[i].Unknown {
			return antartical.ConflictTranscript{}, false, nil
		}
		msg, err := TransactionToMessage(tx, signer, block.BaseFee())
		if err != nil {
			return antartical.ConflictTranscript{}, false, err
		}
		if _, exists := senders[msg.From]; exists {
			// Sender nonce and balance changes are implicit dependencies even
			// when the access list omits the sender account.
			return antartical.ConflictTranscript{}, false, nil
		}
		senders[msg.From] = struct{}{}
	}
	_, transcript, err := antartical.ExecuteOptimistic(transactions, access, func(index int, tx *types.Transaction) (*types.Transaction, antartical.AccessSet, error) {
		copyState := statedb.Copy()
		copyState.SetTxContext(tx.Hash(), index)
		message, err := TransactionToMessage(tx, signer, block.BaseFee())
		if err != nil {
			return nil, antartical.AccessSet{}, err
		}
		gasPool := NewGasPool(block.GasLimit())
		evm := vm.NewEVM(context, copyState, p.chainConfig(), cfg)
		defer evm.Release()
		if _, err := ApplyTransactionWithEVM(message, gasPool, copyState, block.Number(), blockHash, block.Time(), tx, evm); err != nil {
			return nil, antartical.AccessSet{}, fmt.Errorf("speculative tx %d: %w", index, err)
		}
		return tx, access[index], nil
	})
	if err != nil {
		return antartical.ConflictTranscript{}, false, err
	}
	return transcript, true, nil
}

func hasConsensusSideEffects(data []byte) bool {
	return HasAntarticalStampPrefix(data) || HasPrivateTVMPrefix(data) || HasAccountAbstractionPrefix(data) || HasOracleObservationPrefix(data) || HasCrossChainMessagePrefix(data) || HasAddressVotePrefix(data) || HasValidatorRegistrationPrefix(data) || HasValidatorSlashPrefix(data) || HasValidatorExitPrefix(data) || HasValidatorWithdrawalPrefix(data) || HasShieldedV3Prefix(data) || HasShieldedV4Prefix(data)
}
