package eth

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestGovernanceDisclosureMainKingSignatureAppendOnlyAndPersistence(t *testing.T) {
	mainKingKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrongKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	mainKing := crypto.PubkeyToAddress(mainKingKey.PublicKey)
	db := rawdb.NewMemoryDatabase()
	dir := t.TempDir()
	svc := NewGovernanceService(nil, mainKing, big.NewInt(8979), db, dir)

	contentHash := crypto.Keccak256Hash([]byte("rotating king selection: address A selected because it met stake and uptime requirements"))
	timestamp := uint64(time.Now().Unix())
	digest := svc.disclosureHash("rotating-king-selection", "Rotating King A", 1, contentHash, "docs/governance/2026-07-27-rotating-king-a.md", common.Hash{}, timestamp)
	wrongSig, err := crypto.Sign(digest.Bytes(), wrongKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishDisclosure("rotating-king-selection", "Rotating King A", 1, contentHash, "docs/governance/2026-07-27-rotating-king-a.md", common.Hash{}, timestamp, common.Hash{}, wrongSig); err == nil || !strings.Contains(err.Error(), "want main king") {
		t.Fatalf("wrong signer error = %v, want main king rejection", err)
	}

	sig, err := crypto.Sign(digest.Bytes(), mainKingKey)
	if err != nil {
		t.Fatal(err)
	}
	record, err := svc.PublishDisclosure("rotating-king-selection", "Rotating King A", 1, contentHash, "docs/governance/2026-07-27-rotating-king-a.md", common.Hash{}, timestamp, common.Hash{}, sig)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != 1 || record.DisclosureHash != digest || record.MainKing != mainKing || record.ContentHash != contentHash {
		t.Fatalf("bad disclosure record: %#v", record)
	}
	if ok, err := svc.VerifyDisclosure(uint64(record.ID)); !ok || err != nil {
		t.Fatalf("verify record = %v err=%v", ok, err)
	}
	if _, err := svc.PublishDisclosure("rotating-king-selection", "Rotating King A", 1, contentHash, "docs/governance/2026-07-27-rotating-king-a.md", common.Hash{}, timestamp, common.Hash{}, sig); err == nil || !strings.Contains(err.Error(), "already published") {
		t.Fatalf("duplicate publish error = %v, want already published", err)
	}

	changedContentHash := crypto.Keccak256Hash([]byte("edited wording"))
	changedDigest := svc.disclosureHash("rotating-king-selection", "Rotating King A", 2, changedContentHash, "docs/governance/2026-07-27-rotating-king-a-v2.md", record.DisclosureHash, timestamp+1)
	if changedDigest == digest {
		t.Fatal("changed disclosure body produced same disclosure hash")
	}
	changedSig, err := crypto.Sign(changedDigest.Bytes(), mainKingKey)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.PublishDisclosure("rotating-king-selection", "Rotating King A", 2, changedContentHash, "docs/governance/2026-07-27-rotating-king-a-v2.md", record.DisclosureHash, timestamp+1, common.Hash{}, changedSig)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PreviousHash != record.DisclosureHash || updated.ID != 2 {
		t.Fatalf("bad append-only link: %#v", updated)
	}

	reloaded := NewGovernanceService(nil, mainKing, big.NewInt(8979), db, dir)
	latest, err := reloaded.LatestDisclosure("rotating-king-selection")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != updated.ID || latest.PreviousHash != record.DisclosureHash {
		t.Fatalf("reloaded latest = %#v, want %#v", latest, updated)
	}
	listed := reloaded.ListDisclosures("rotating-king-selection", 0, 10)
	if len(listed) != 2 || listed[0].ID != 2 || listed[1].ID != 1 {
		t.Fatalf("listed disclosures = %#v", listed)
	}
}

func TestGovernanceAPIDisclosureHashAndUnknownPrevious(t *testing.T) {
	mainKingKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	mainKing := crypto.PubkeyToAddress(mainKingKey.PublicKey)
	svc := NewGovernanceService(nil, mainKing, big.NewInt(8979), nil, "")
	api := &GovernanceAPI{service: svc}
	contentHash := crypto.Keccak256Hash([]byte("checkpoint explanation"))
	timestamp := hexutil.Uint64(1785157200)
	digest := api.DisclosureHash("checkpoint", "Checkpoint 7165", 1, contentHash, "docs/governance/checkpoint-7165.md", common.Hash{}, timestamp)
	if digest == (common.Hash{}) {
		t.Fatal("empty governance disclosure hash")
	}
	unknownPrevious := crypto.Keccak256Hash([]byte("unknown previous"))
	linkedDigest := api.DisclosureHash("checkpoint", "Checkpoint 7165", 1, contentHash, "docs/governance/checkpoint-7165.md", unknownPrevious, timestamp)
	sig, err := crypto.Sign(linkedDigest.Bytes(), mainKingKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.PublishDisclosure("checkpoint", "Checkpoint 7165", 1, contentHash, "docs/governance/checkpoint-7165.md", unknownPrevious, timestamp, common.Hash{}, sig); err == nil || !strings.Contains(err.Error(), "previous governance disclosure hash is unknown") {
		t.Fatalf("unknown previous error = %v", err)
	}
}
