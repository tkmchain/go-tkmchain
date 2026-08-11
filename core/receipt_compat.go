package core

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
)

var historicalBrokenPqReceiptRootBlocks = map[uint64]common.Hash{
	20146: common.HexToHash("0x4ad9f0d3caec69b43c547254b041060514dd689c2b50bf03cfec32e9559b749d"),
	20173: common.HexToHash("0xca3393c0164ea2b7ae776c3e5b90ae4a717ee46a81c0303895eb1344b2b7ab5c"),
}

type historicalBrokenPqReceipts struct {
	types.Receipts
}

func (rs historicalBrokenPqReceipts) EncodeIndex(i int, w *bytes.Buffer) {
	if rs.Receipts[i].Type == types.PQTkmTxType {
		w.WriteByte(types.PQTkmTxType)
		return
	}
	rs.Receipts.EncodeIndex(i, w)
}

func acceptHistoricalBrokenPqReceiptRoot(number uint64, blockHash common.Hash, receiptHash common.Hash, receipts types.Receipts) bool {
	wantHash, ok := historicalBrokenPqReceiptRootBlocks[number]
	if !ok || blockHash != wantHash {
		return false
	}
	checkpoint, ok := params.GetCheckpoint(number)
	if !ok || checkpoint != blockHash {
		return false
	}
	hasPQReceipt := false
	for _, receipt := range receipts {
		if receipt.Type == types.PQTkmTxType {
			hasPQReceipt = true
			break
		}
	}
	if !hasPQReceipt {
		return false
	}
	compatSha := types.DeriveSha(historicalBrokenPqReceipts{receipts}, trie.NewStackTrie(nil))
	return compatSha == receiptHash
}
