package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

func TestSupplyMarkerRewardDeltaAndPersistence(t *testing.T) {
	mainKing := common.HexToAddress("0x1000000000000000000000000000000000000001")
	rotatingKing := common.HexToAddress("0x2000000000000000000000000000000000000002")
	miner := common.HexToAddress("0x3000000000000000000000000000000000000003")
	db := rawdb.NewMemoryDatabase()
	eth := &Ethereum{mainKingAddress: mainKing, chainDb: db}
	svc := NewSupplyService(eth, db)

	blockNumber := uint64(7)
	txs := []*types.Transaction{
		types.NewBlockRewardTx(blockNumber, types.BlockRewardMainKing, mainKing, big.NewInt(20)),
		types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, rotatingKing, big.NewInt(80)),
		types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, big.NewInt(100)),
	}
	block := types.NewBlock(&types.Header{Number: big.NewInt(int64(blockNumber)), Coinbase: miner}, &types.Body{Transactions: txs}, nil, trie.NewStackTrie(nil))
	delta := svc.blockRewardDelta(block)
	if delta.mainKing.Cmp(big.NewInt(20)) != 0 {
		t.Fatalf("main king delta = %v, want 20", delta.mainKing)
	}
	if delta.rotatingKing.Cmp(big.NewInt(80)) != 0 {
		t.Fatalf("rotating king delta = %v, want 80", delta.rotatingKing)
	}
	if delta.miner.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("miner delta = %v, want 100", delta.miner)
	}

	entry := supplyEntryDisk{
		BlockNumber:         blockNumber,
		BlockHash:           block.Hash(),
		GenesisSupply:       big.NewInt(1000),
		TotalIssued:         big.NewInt(200),
		TotalSupply:         big.NewInt(1200),
		MainKingRewards:     big.NewInt(20),
		RotatingKingRewards: big.NewInt(80),
		MinerRewards:        big.NewInt(100),
	}
	if err := svc.writeEntry(entry); err != nil {
		t.Fatalf("writeEntry failed: %v", err)
	}
	reloaded := NewSupplyService(eth, db)
	stored := reloaded.readLatest()
	if stored == nil {
		t.Fatal("missing persisted latest supply entry")
	}
	if stored.BlockNumber != blockNumber || stored.BlockHash != block.Hash() {
		t.Fatalf("stored entry = block %d hash %s, want block %d hash %s", stored.BlockNumber, stored.BlockHash, blockNumber, block.Hash())
	}
	if stored.TotalSupply.Cmp(big.NewInt(1200)) != 0 || stored.TotalIssued.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("stored totals = supply %v issued %v, want 1200/200", stored.TotalSupply, stored.TotalIssued)
	}
}

func TestSupplyRotatingZeroMarkerCountsAsMainKingReceived(t *testing.T) {
	mainKing := common.HexToAddress("0x1000000000000000000000000000000000000001")
	miner := common.HexToAddress("0x3000000000000000000000000000000000000003")
	db := rawdb.NewMemoryDatabase()
	eth := &Ethereum{mainKingAddress: mainKing, chainDb: db}
	svc := NewSupplyService(eth, db)

	blockNumber := uint64(8)
	txs := []*types.Transaction{
		types.NewBlockRewardTx(blockNumber, types.BlockRewardRotatingKing, common.Address{}, big.NewInt(80)),
		types.NewBlockRewardTx(blockNumber, types.BlockRewardMiner, miner, big.NewInt(100)),
	}
	block := types.NewBlock(&types.Header{Number: big.NewInt(int64(blockNumber)), Coinbase: miner}, &types.Body{Transactions: txs}, nil, trie.NewStackTrie(nil))
	delta := svc.blockRewardDelta(block)
	if delta.mainKing.Cmp(big.NewInt(80)) != 0 {
		t.Fatalf("zero rotating marker main king delta = %v, want 80", delta.mainKing)
	}
	if delta.rotatingKing.Sign() != 0 {
		t.Fatalf("zero rotating marker rotating delta = %v, want 0", delta.rotatingKing)
	}
}
