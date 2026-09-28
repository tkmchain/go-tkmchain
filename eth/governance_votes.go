package eth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

const governanceVoteSnapshotFile = "address-votes.json"

// GovernanceAddressVote is a local, block-derived audit record. The canonical
// state remains the source of truth; this file is only a restartable projection
// for operators and explorers.
type GovernanceAddressVote struct {
	BlockNumber uint64         `json:"blockNumber"`
	BlockHash   common.Hash    `json:"blockHash"`
	TxHash      common.Hash    `json:"txHash"`
	Voter       common.Address `json:"voter"`
	Target      common.Address `json:"target"`
	Unvote      bool           `json:"unvote"`
	Reason      string         `json:"reason,omitempty"`
}

type governanceVoteSnapshot struct {
	ChainID    string                  `json:"chainId"`
	HeadNumber uint64                  `json:"headNumber"`
	HeadHash   common.Hash             `json:"headHash"`
	UpdatedAt  uint64                  `json:"updatedAt"`
	Votes      []GovernanceAddressVote `json:"votes"`
}

func (svc *GovernanceService) voteSnapshotPath() string {
	if svc == nil || svc.govDir == "" {
		return ""
	}
	return filepath.Join(svc.govDir, governanceVoteSnapshotFile)
}

func (svc *GovernanceService) loadVoteSnapshotLocked() error {
	path := svc.voteSnapshotPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var snapshot governanceVoteSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("decode canonical governance vote snapshot: %w", err)
	}
	svc.voteSnap = snapshot
	return nil
}

func (svc *GovernanceService) saveVoteSnapshotLocked() error {
	path := svc.voteSnapshotPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(svc.govDir, 0700); err != nil {
		return err
	}
	svc.voteSnap.UpdatedAt = uint64(time.Now().Unix())
	data, err := json.MarshalIndent(&svc.voteSnap, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (svc *GovernanceService) appendVoteBlockLocked(header *types.Header, txs types.Transactions) bool {
	if svc.eth == nil || svc.eth.blockchain == nil || header == nil {
		return false
	}
	config := svc.eth.blockchain.Config()
	if config == nil {
		return false
	}
	signer := types.MakeSigner(config, header.Number, header.Time)
	changed := false
	for _, tx := range txs {
		if !core.HasAddressVotePrefix(tx.Data()) {
			continue
		}
		vote, err := core.DecodeAddressVote(tx.Data())
		if err != nil {
			log.Warn("Skipping malformed canonical address vote while building local journal", "tx", tx.Hash(), "err", err)
			continue
		}
		voter, err := types.Sender(signer, tx)
		if err != nil {
			log.Warn("Skipping address vote with unrecoverable sender", "tx", tx.Hash(), "err", err)
			continue
		}
		svc.voteSnap.Votes = append(svc.voteSnap.Votes, GovernanceAddressVote{
			BlockNumber: header.Number.Uint64(),
			BlockHash:   header.Hash(),
			TxHash:      tx.Hash(),
			Voter:       voter,
			Target:      vote.Target,
			Unvote:      vote.Unvote,
			Reason:      vote.Reason,
		})
		changed = true
	}
	svc.voteSnap.HeadNumber = header.Number.Uint64()
	svc.voteSnap.HeadHash = header.Hash()
	if config.ChainID != nil {
		svc.voteSnap.ChainID = config.ChainID.String()
	}
	return changed
}

func (svc *GovernanceService) rebuildVoteSnapshotLocked() error {
	if svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	head := svc.eth.blockchain.CurrentBlock()
	if head == nil {
		return nil
	}
	svc.voteSnap.Votes = nil
	svc.voteSnap.HeadNumber = 0
	svc.voteSnap.HeadHash = common.Hash{}
	for number := uint64(0); number <= head.Number.Uint64(); number++ {
		block := svc.eth.blockchain.GetBlockByNumber(number)
		if block == nil {
			return fmt.Errorf("canonical block %d is unavailable while rebuilding governance journal", number)
		}
		svc.appendVoteBlockLocked(block.Header(), block.Transactions())
	}
	return svc.saveVoteSnapshotLocked()
}

func (svc *GovernanceService) advanceVoteSnapshotLocked(headNumber uint64) error {
	if svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	for number := svc.voteSnap.HeadNumber + 1; number <= headNumber; number++ {
		block := svc.eth.blockchain.GetBlockByNumber(number)
		if block == nil {
			return fmt.Errorf("canonical block %d is unavailable while advancing governance journal", number)
		}
		svc.appendVoteBlockLocked(block.Header(), block.Transactions())
	}
	return svc.saveVoteSnapshotLocked()
}

func (svc *GovernanceService) syncVoteSnapshotLocked(eventBlock *core.ChainEvent) error {
	if svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	head := svc.eth.blockchain.CurrentBlock()
	if head == nil {
		return nil
	}
	if eventBlock == nil || eventBlock.Header == nil || svc.voteSnap.HeadHash == (common.Hash{}) || eventBlock.Header.ParentHash != svc.voteSnap.HeadHash || eventBlock.Header.Number.Uint64() != svc.voteSnap.HeadNumber+1 {
		return svc.rebuildVoteSnapshotLocked()
	}
	if svc.appendVoteBlockLocked(eventBlock.Header, eventBlock.Transactions) {
		return svc.saveVoteSnapshotLocked()
	}
	return nil
}

// StartVoteSync derives the local governance projection from canonical blocks
// and follows future canonical-head events. Local files never override chain
// state and are safe to delete; the next startup rebuilds them from blocks.
func (svc *GovernanceService) StartVoteSync() error {
	if svc == nil || svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	svc.voteMu.Lock()
	defer svc.voteMu.Unlock()
	if svc.voteSub != nil {
		return nil
	}
	if err := svc.loadVoteSnapshotLocked(); err != nil {
		return err
	}
	head := svc.eth.blockchain.CurrentBlock()
	if head != nil && svc.voteSnap.HeadHash != head.Hash() {
		canonicalSavedHead := svc.eth.blockchain.GetHeaderByNumber(svc.voteSnap.HeadNumber)
		if svc.voteSnap.HeadHash != (common.Hash{}) && canonicalSavedHead != nil && canonicalSavedHead.Hash() == svc.voteSnap.HeadHash && svc.voteSnap.HeadNumber < head.Number.Uint64() {
			if err := svc.advanceVoteSnapshotLocked(head.Number.Uint64()); err != nil {
				return err
			}
		} else {
			if err := svc.rebuildVoteSnapshotLocked(); err != nil {
				return err
			}
		}
	}
	ch := make(chan core.ChainEvent, 32)
	svc.voteSub = svc.eth.blockchain.SubscribeChainEvent(ch)
	svc.voteQuit = make(chan struct{})
	svc.voteWG.Add(1)
	go func() {
		defer svc.voteWG.Done()
		for {
			select {
			case eventBlock := <-ch:
				svc.voteMu.Lock()
				if err := svc.syncVoteSnapshotLocked(&eventBlock); err != nil {
					log.Warn("Failed to update canonical governance vote journal", "err", err)
				}
				svc.voteMu.Unlock()
			case <-svc.voteQuit:
				return
			}
		}
	}()
	return nil
}

func (svc *GovernanceService) StopVoteSync() {
	if svc == nil {
		return
	}
	svc.voteMu.Lock()
	sub := svc.voteSub
	quit := svc.voteQuit
	svc.voteSub = nil
	svc.voteQuit = nil
	svc.voteMu.Unlock()
	if sub != nil {
		sub.Unsubscribe()
	}
	if quit != nil {
		close(quit)
	}
	svc.voteWG.Wait()
}

func (svc *GovernanceService) AddressVoteJournal() []GovernanceAddressVote {
	if svc == nil {
		return nil
	}
	svc.voteMu.Lock()
	defer svc.voteMu.Unlock()
	out := make([]GovernanceAddressVote, len(svc.voteSnap.Votes))
	copy(out, svc.voteSnap.Votes)
	return out
}
