package eth

import (
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestGovernanceVoteSnapshotPersistsInGovernanceDirectory(t *testing.T) {
	dir := t.TempDir()
	want := GovernanceAddressVote{
		BlockNumber: 12,
		BlockHash:   common.HexToHash("0x12"),
		TxHash:      common.HexToHash("0x34"),
		Voter:       common.HexToAddress("0x1001"),
		Target:      common.HexToAddress("0x2002"),
		Reason:      "fraud",
	}
	svc := &GovernanceService{govDir: dir, voteSnap: governanceVoteSnapshot{ChainID: "8979", HeadNumber: 12, HeadHash: want.BlockHash, Votes: []GovernanceAddressVote{want}}}
	if err := svc.saveVoteSnapshotLocked(); err != nil {
		t.Fatal(err)
	}
	path := svc.voteSnapshotPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("snapshot permissions = %o, want 600", mode)
	}
	reloaded := &GovernanceService{govDir: dir}
	if err := reloaded.loadVoteSnapshotLocked(); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.voteSnap.Votes) != 1 || reloaded.voteSnap.Votes[0] != want || reloaded.voteSnap.HeadHash != want.BlockHash {
		t.Fatalf("reloaded snapshot = %+v", reloaded.voteSnap)
	}
}
