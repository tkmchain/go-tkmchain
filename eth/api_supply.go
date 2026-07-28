package eth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	tkmSupplyLatestKey = []byte("tkm-supply-latest-v1")
	tkmSupplyEntryPref = []byte("tkm-supply-entry-v1-")
)

// SupplyEntry stores cumulative non-consensus supply accounting up to one canonical block.
type SupplyEntry struct {
	BlockNumber         hexutil.Uint64 `json:"blockNumber"`
	BlockHash           common.Hash    `json:"blockHash"`
	GenesisSupply       *hexutil.Big   `json:"genesisSupply"`
	TotalIssued         *hexutil.Big   `json:"totalIssued"`
	TotalSupply         *hexutil.Big   `json:"totalSupply"`
	MainKingRewards     *hexutil.Big   `json:"mainKingRewards"`
	RotatingKingRewards *hexutil.Big   `json:"rotatingKingRewards"`
	MinerRewards        *hexutil.Big   `json:"minerRewards"`
	IndexedTo           hexutil.Uint64 `json:"indexedTo"`
}

type supplyEntryDisk struct {
	BlockNumber         uint64
	BlockHash           common.Hash
	GenesisSupply       *big.Int
	TotalIssued         *big.Int
	TotalSupply         *big.Int
	MainKingRewards     *big.Int
	RotatingKingRewards *big.Int
	MinerRewards        *big.Int
}

type supplyDelta struct {
	mainKing     *big.Int
	rotatingKing *big.Int
	miner        *big.Int
}

type SupplyAPI struct {
	svc *SupplyService
}

// SupplyService builds and persists supply accounting from canonical block data.
type SupplyService struct {
	eth           *Ethereum
	db            ethdb.Database
	genesisSupply *big.Int
	mu            sync.Mutex
}

func NewSupplyAPI(e *Ethereum) *SupplyAPI {
	return &SupplyAPI{svc: NewSupplyService(e, e.chainDb)}
}

func NewSupplyService(e *Ethereum, db ethdb.Database) *SupplyService {
	return &SupplyService{eth: e, db: db}
}

// Latest returns cumulative supply accounting for the current canonical head.
func (api *SupplyAPI) Latest() (SupplyEntry, error) {
	return api.svc.AtBlock(api.svc.headNumber())
}

// AtBlock returns cumulative supply accounting at the requested canonical block height.
func (api *SupplyAPI) AtBlock(blockNumber hexutil.Uint64) (SupplyEntry, error) {
	return api.svc.AtBlock(uint64(blockNumber))
}

// Sync indexes supply accounting up to the requested canonical block height.
func (api *SupplyAPI) Sync(blockNumber hexutil.Uint64) (SupplyEntry, error) {
	return api.svc.AtBlock(uint64(blockNumber))
}

func (svc *SupplyService) AtBlock(target uint64) (SupplyEntry, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.eth == nil || svc.eth.blockchain == nil {
		return SupplyEntry{}, errors.New("supply service requires a blockchain")
	}
	head := svc.headNumber()
	if target > head {
		return SupplyEntry{}, fmt.Errorf("requested block %d is above canonical head %d", target, head)
	}
	if err := svc.ensureGenesisSupply(); err != nil {
		return SupplyEntry{}, err
	}
	entry, err := svc.indexTo(target)
	if err != nil {
		return SupplyEntry{}, err
	}
	return entry.view(), nil
}

func (svc *SupplyService) headNumber() uint64 {
	if svc.eth == nil || svc.eth.blockchain == nil || svc.eth.blockchain.CurrentBlock() == nil {
		return 0
	}
	return svc.eth.blockchain.CurrentBlock().Number.Uint64()
}

func (svc *SupplyService) ensureGenesisSupply() error {
	if svc.genesisSupply != nil {
		return nil
	}
	genesis, err := core.ReadGenesis(svc.db)
	if err != nil {
		return err
	}
	total := new(big.Int)
	for _, account := range genesis.Alloc {
		if account.Balance != nil {
			total.Add(total, account.Balance)
		}
	}
	svc.genesisSupply = total
	return nil
}

func (svc *SupplyService) indexTo(target uint64) (supplyEntryDisk, error) {
	if target == 0 {
		if stored, err := svc.readEntry(0); err == nil && stored != nil {
			block := svc.eth.blockchain.GetBlockByNumber(0)
			if block != nil && block.Hash() == stored.BlockHash {
				return *stored, nil
			}
		}
		return svc.genesisEntry()
	}
	latest := svc.readLatest()
	if latest != nil {
		latest = svc.rewindStale(latest, target)
	}
	var current supplyEntryDisk
	start := uint64(1)
	if latest != nil {
		current = latest.clone()
		start = latest.BlockNumber + 1
	} else {
		genesis, err := svc.genesisEntry()
		if err != nil {
			return supplyEntryDisk{}, err
		}
		current = genesis
	}
	if target < current.BlockNumber {
		stored, err := svc.readEntry(target)
		if err != nil {
			return supplyEntryDisk{}, err
		}
		if stored == nil {
			return supplyEntryDisk{}, fmt.Errorf("supply entry for block %d not found", target)
		}
		return *stored, nil
	}
	for number := start; number <= target; number++ {
		block := svc.eth.blockchain.GetBlockByNumber(number)
		if block == nil {
			return supplyEntryDisk{}, fmt.Errorf("canonical block %d not found", number)
		}
		delta := svc.blockRewardDelta(block)
		current.BlockNumber = number
		current.BlockHash = block.Hash()
		current.MainKingRewards.Add(current.MainKingRewards, delta.mainKing)
		current.RotatingKingRewards.Add(current.RotatingKingRewards, delta.rotatingKing)
		current.MinerRewards.Add(current.MinerRewards, delta.miner)
		current.TotalIssued.Add(current.TotalIssued, delta.mainKing)
		current.TotalIssued.Add(current.TotalIssued, delta.rotatingKing)
		current.TotalIssued.Add(current.TotalIssued, delta.miner)
		current.TotalSupply.Add(current.GenesisSupply, current.TotalIssued)
		if err := svc.writeEntry(current); err != nil {
			return supplyEntryDisk{}, err
		}
	}
	return current, nil
}

func (svc *SupplyService) genesisEntry() (supplyEntryDisk, error) {
	block := svc.eth.blockchain.GetBlockByNumber(0)
	if block == nil {
		return supplyEntryDisk{}, errors.New("genesis block not found")
	}
	entry := supplyEntryDisk{
		BlockNumber:         0,
		BlockHash:           block.Hash(),
		GenesisSupply:       new(big.Int).Set(svc.genesisSupply),
		TotalIssued:         new(big.Int),
		TotalSupply:         new(big.Int).Set(svc.genesisSupply),
		MainKingRewards:     new(big.Int),
		RotatingKingRewards: new(big.Int),
		MinerRewards:        new(big.Int),
	}
	if err := svc.writeEntry(entry); err != nil {
		return supplyEntryDisk{}, err
	}
	return entry, nil
}

func newSupplyDelta() supplyDelta {
	return supplyDelta{mainKing: new(big.Int), rotatingKing: new(big.Int), miner: new(big.Int)}
}

func (svc *SupplyService) blockRewardDelta(block *types.Block) supplyDelta {
	delta := newSupplyDelta()
	if block == nil || block.NumberU64() == 0 {
		return delta
	}
	if svc.markerRewardDelta(block, &delta) {
		return delta
	}
	header := block.Header()
	if header.Coinbase == (common.Address{}) {
		return delta
	}
	reward := randomx.CalculateBlockReward(block.NumberU64())
	if reward == nil || reward.Sign() == 0 {
		return delta
	}
	mainKing := new(big.Int).Mul(reward, big.NewInt(10))
	mainKing.Div(mainKing, big.NewInt(100))
	rotatingKing := new(big.Int).Mul(reward, big.NewInt(40))
	rotatingKing.Div(rotatingKing, big.NewInt(100))
	miner := new(big.Int).Sub(new(big.Int).Set(reward), new(big.Int).Add(mainKing, rotatingKing))
	mainKingAddress := svc.eth.GetMainKingAddress()
	if mainKingAddress == (common.Address{}) {
		miner.Add(miner, mainKing)
		miner.Add(miner, rotatingKing)
		mainKing = new(big.Int)
		rotatingKing = new(big.Int)
	} else if svc.eth.rotatingKingAt(block.NumberU64()) == (common.Address{}) {
		mainKing.Add(mainKing, rotatingKing)
		rotatingKing = new(big.Int)
	}
	delta.mainKing.Add(delta.mainKing, mainKing)
	delta.rotatingKing.Add(delta.rotatingKing, rotatingKing)
	delta.miner.Add(delta.miner, miner)
	return delta
}

func (svc *SupplyService) markerRewardDelta(block *types.Block, delta *supplyDelta) bool {
	found := false
	mainKing := svc.eth.GetMainKingAddress()
	for _, tx := range block.Transactions() {
		if !types.IsBlockRewardTx(tx) {
			continue
		}
		found = true
		value := tx.Value()
		if value == nil || value.Sign() == 0 {
			continue
		}
		to := tx.To()
		switch rewardTxKind(tx) {
		case types.BlockRewardMainKing:
			delta.mainKing.Add(delta.mainKing, value)
		case types.BlockRewardRotatingKing:
			if to == nil || *to == (common.Address{}) || *to == mainKing {
				delta.mainKing.Add(delta.mainKing, value)
			} else {
				delta.rotatingKing.Add(delta.rotatingKing, value)
			}
		case types.BlockRewardMiner:
			delta.miner.Add(delta.miner, value)
		}
	}
	return found
}

func rewardTxKind(tx *types.Transaction) int {
	data := tx.Data()
	if len(data) == 0 {
		return -1
	}
	return int(data[len(data)-1])
}

func (svc *SupplyService) rewindStale(latest *supplyEntryDisk, target uint64) *supplyEntryDisk {
	for latest != nil {
		if latest.BlockNumber > target {
			_ = svc.db.Delete(tkmSupplyEntryKey(latest.BlockNumber))
			latest = svc.readLatestBefore(latest.BlockNumber)
			continue
		}
		block := svc.eth.blockchain.GetBlockByNumber(latest.BlockNumber)
		if block != nil && block.Hash() == latest.BlockHash {
			_ = svc.writeLatest(latest.BlockNumber)
			return latest
		}
		_ = svc.db.Delete(tkmSupplyEntryKey(latest.BlockNumber))
		latest = svc.readLatestBefore(latest.BlockNumber)
	}
	_ = svc.db.Delete(tkmSupplyLatestKey)
	return nil
}

func (svc *SupplyService) readLatestBefore(number uint64) *supplyEntryDisk {
	if number == 0 {
		return nil
	}
	for n := number - 1; ; n-- {
		entry, err := svc.readEntry(n)
		if err == nil && entry != nil {
			return entry
		}
		if n == 0 {
			return nil
		}
	}
}

func (svc *SupplyService) readLatest() *supplyEntryDisk {
	data, err := svc.db.Get(tkmSupplyLatestKey)
	if err != nil || len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	entry, err := svc.readEntry(number)
	if err != nil {
		log.Warn("Failed to read persisted supply entry", "block", number, "err", err)
		return nil
	}
	return entry
}

func (svc *SupplyService) readEntry(number uint64) (*supplyEntryDisk, error) {
	data, err := svc.db.Get(tkmSupplyEntryKey(number))
	if err != nil || len(data) == 0 {
		return nil, err
	}
	var entry supplyEntryDisk
	if err := rlp.DecodeBytes(data, &entry); err != nil {
		return nil, err
	}
	entry.ensureBigInts()
	return &entry, nil
}

func (svc *SupplyService) writeEntry(entry supplyEntryDisk) error {
	entry.ensureBigInts()
	data, err := rlp.EncodeToBytes(&entry)
	if err != nil {
		return err
	}
	if err := svc.db.Put(tkmSupplyEntryKey(entry.BlockNumber), data); err != nil {
		return err
	}
	return svc.writeLatest(entry.BlockNumber)
}

func (svc *SupplyService) writeLatest(number uint64) error {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, number)
	return svc.db.Put(tkmSupplyLatestKey, data)
}

func (entry supplyEntryDisk) clone() supplyEntryDisk {
	entry.ensureBigInts()
	return supplyEntryDisk{
		BlockNumber:         entry.BlockNumber,
		BlockHash:           entry.BlockHash,
		GenesisSupply:       new(big.Int).Set(entry.GenesisSupply),
		TotalIssued:         new(big.Int).Set(entry.TotalIssued),
		TotalSupply:         new(big.Int).Set(entry.TotalSupply),
		MainKingRewards:     new(big.Int).Set(entry.MainKingRewards),
		RotatingKingRewards: new(big.Int).Set(entry.RotatingKingRewards),
		MinerRewards:        new(big.Int).Set(entry.MinerRewards),
	}
}

func (entry *supplyEntryDisk) ensureBigInts() {
	if entry.GenesisSupply == nil {
		entry.GenesisSupply = new(big.Int)
	}
	if entry.TotalIssued == nil {
		entry.TotalIssued = new(big.Int)
	}
	if entry.TotalSupply == nil {
		entry.TotalSupply = new(big.Int)
	}
	if entry.MainKingRewards == nil {
		entry.MainKingRewards = new(big.Int)
	}
	if entry.RotatingKingRewards == nil {
		entry.RotatingKingRewards = new(big.Int)
	}
	if entry.MinerRewards == nil {
		entry.MinerRewards = new(big.Int)
	}
}

func (entry supplyEntryDisk) view() SupplyEntry {
	entry.ensureBigInts()
	return SupplyEntry{
		BlockNumber:         hexutil.Uint64(entry.BlockNumber),
		BlockHash:           entry.BlockHash,
		GenesisSupply:       (*hexutil.Big)(new(big.Int).Set(entry.GenesisSupply)),
		TotalIssued:         (*hexutil.Big)(new(big.Int).Set(entry.TotalIssued)),
		TotalSupply:         (*hexutil.Big)(new(big.Int).Set(entry.TotalSupply)),
		MainKingRewards:     (*hexutil.Big)(new(big.Int).Set(entry.MainKingRewards)),
		RotatingKingRewards: (*hexutil.Big)(new(big.Int).Set(entry.RotatingKingRewards)),
		MinerRewards:        (*hexutil.Big)(new(big.Int).Set(entry.MinerRewards)),
		IndexedTo:           hexutil.Uint64(entry.BlockNumber),
	}
}

func tkmSupplyEntryKey(number uint64) []byte {
	key := make([]byte, len(tkmSupplyEntryPref)+8)
	copy(key, tkmSupplyEntryPref)
	binary.BigEndian.PutUint64(key[len(tkmSupplyEntryPref):], number)
	return key
}
