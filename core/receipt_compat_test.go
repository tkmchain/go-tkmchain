package core

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
)

func TestAcceptHistoricalBrokenPqReceiptRootBlock20146(t *testing.T) {
	params.SetActiveCheckpointGenesis(params.MainnetGenesisHash)
	t.Cleanup(func() { params.SetActiveCheckpointGenesis(params.MainnetGenesisHash) })

	receipts := types.Receipts{
		testReceipt(types.PQTkmTxType, 0xa03c),
		testReceipt(types.LegacyTxType, 0xa03c),
		testReceipt(types.LegacyTxType, 0xa03c),
		testReceipt(types.LegacyTxType, 0xa03c),
	}
	headerReceiptHash := common.HexToHash("0x03d054783d9a4ae45b029f3de84e4ce93afa65cd6a24bdac6a329d4ce253441a")
	blockHash := common.HexToHash("0x4ad9f0d3caec69b43c547254b041060514dd689c2b50bf03cfec32e9559b749d")

	normalRoot := types.DeriveSha(receipts, trie.NewStackTrie(nil))
	if want := common.HexToHash("0x52649d49da4ab184368f7cc5a7a9a411dd66ad32362974b6154bf353aefb6990"); normalRoot != want {
		t.Fatalf("normal typed PQ receipt root = %s, want %s", normalRoot, want)
	}
	if normalRoot == headerReceiptHash {
		t.Fatal("test fixture does not reproduce the historical receipt root mismatch")
	}
	compatRoot := types.DeriveSha(historicalBrokenPqReceipts{receipts}, trie.NewStackTrie(nil))
	if compatRoot != headerReceiptHash {
		t.Fatalf("historical broken PQ receipt root = %s, want %s", compatRoot, headerReceiptHash)
	}
	if !acceptHistoricalBrokenPqReceiptRoot(20146, blockHash, headerReceiptHash, receipts) {
		t.Fatal("checkpointed historical PQ receipt root was rejected")
	}
	if acceptHistoricalBrokenPqReceiptRoot(20146, common.Hash{}, headerReceiptHash, receipts) {
		t.Fatal("historical PQ receipt root accepted without the checkpointed block hash")
	}
}

func TestAcceptHistoricalBrokenPqReceiptRootBlock20173(t *testing.T) {
	params.SetActiveCheckpointGenesis(params.MainnetGenesisHash)
	t.Cleanup(func() { params.SetActiveCheckpointGenesis(params.MainnetGenesisHash) })

	receipts := types.Receipts{
		testReceipt(types.PQTkmTxType, 0x9fa4),
		testReceipt(types.PQTkmTxType, 0x13f58),
		testReceipt(types.LegacyTxType, 0x13f58),
		testReceipt(types.LegacyTxType, 0x13f58),
		testReceipt(types.LegacyTxType, 0x13f58),
	}
	headerReceiptHash := common.HexToHash("0x5694c071e8ce1c97803f9239257d082efb95bca381bc69ead46e69cca3c89d0f")
	blockHash := common.HexToHash("0xca3393c0164ea2b7ae776c3e5b90ae4a717ee46a81c0303895eb1344b2b7ab5c")

	if normalRoot := types.DeriveSha(receipts, trie.NewStackTrie(nil)); normalRoot == headerReceiptHash {
		t.Fatal("test fixture does not reproduce the block 20173 receipt root mismatch")
	}
	if compatRoot := types.DeriveSha(historicalBrokenPqReceipts{receipts}, trie.NewStackTrie(nil)); compatRoot != headerReceiptHash {
		t.Fatalf("historical broken PQ receipt root = %s, want %s", compatRoot, headerReceiptHash)
	}
	if !acceptHistoricalBrokenPqReceiptRoot(20173, blockHash, headerReceiptHash, receipts) {
		t.Fatal("checkpointed historical PQ receipt root was rejected")
	}
}

func testReceipt(typ uint8, cumulativeGasUsed uint64) *types.Receipt {
	return &types.Receipt{
		Type:              typ,
		Status:            types.ReceiptStatusSuccessful,
		CumulativeGasUsed: cumulativeGasUsed,
	}
}
