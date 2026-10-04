package shield3wallet

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

func TestUsernameHandleAndPrivateLookupBatch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	seed := make([]byte, pqcrypto.MLDSA87SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	stamp, err := pqcrypto.CreateShieldedV3Stamp(seed, 8979, "Example", "New Zealand")
	if err != nil {
		if strings.Contains(err.Error(), "STARK backend unavailable") {
			t.Skip("native Shield3 STARK artifact is not built in this environment")
		}
		t.Fatal(err)
	}
	identity, err := NewIdentity(seed, 8979, stamp)
	if err != nil {
		if strings.Contains(err.Error(), "STARK backend unavailable") {
			t.Skip("native Shield3 STARK artifact is not built in this environment")
		}
		t.Fatal(err)
	}
	defer identity.Clear()

	handle, err := UsernameHandle("@Alice", 8979)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ParseUsernameHandle(handle, 8979); err != nil || got != "alice" {
		t.Fatalf("ParseUsernameHandle() = %q, %v", got, err)
	}
	if _, err := ParseUsernameHandle(handle, 8980); err == nil {
		t.Fatal("accepted a handle on the wrong chain")
	}
	mutated := handle[:len(handle)-1] + "0"
	if _, err := ParseUsernameHandle(mutated, 8979); err == nil {
		t.Fatal("accepted a handle with a damaged checksum")
	}
	for _, input := range []string{"admin", "ab", "aаlice", "alice..", "-alice", "alice-"} {
		if _, err := NormalizeUsername(input); err == nil {
			t.Errorf("NormalizeUsername(%q) unexpectedly succeeded", input)
		}
	}

	expires := now.Add(24 * time.Hour)
	primary, err := CreateUsernameBinding(seed, "alice", identity.Code, 8979, 1, expires, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyUsernameBinding(primary, 8979, now); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
	tampered := primary
	tampered.PaymentCode = tampered.PaymentCode[:len(tampered.PaymentCode)-1] + "A"
	if err := VerifyUsernameBinding(tampered, 8979, now); err == nil {
		t.Fatal("accepted a modified recipient payment code")
	}
	if err := VerifyUsernameBinding(primary, 8980, now); err == nil {
		t.Fatal("accepted cross-chain binding")
	}
	if err := VerifyUsernameBinding(primary, 8979, expires); err == nil {
		t.Fatal("accepted expired binding")
	}

	directory, err := NewUsernameDirectory(8979)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.Put(primary, now); err != nil {
		t.Fatal(err)
	}
	resolved, err := directory.Resolve(handle, now)
	if err != nil || resolved.Address != primary.Address || resolved.CodeHash != primary.CodeHash {
		t.Fatalf("Resolve() returned wrong record: %+v, %v", resolved, err)
	}
	if err := directory.Put(primary, now); err == nil || !strings.Contains(err.Error(), "replayed") {
		t.Fatalf("replayed update error = %v", err)
	}
	db := memorydb.New()
	persistent, err := NewPersistentUsernameDirectory(8979, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistent.Put(primary, now); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewPersistentUsernameDirectory(8979, db)
	if err != nil {
		t.Fatalf("reopen persisted directory: %v", err)
	}
	restored, err := reopened.Resolve(handle, now)
	if err != nil || restored.CodeHash != primary.CodeHash || restored.Sequence != primary.Sequence {
		t.Fatalf("restored directory entry = %+v, %v", restored, err)
	}
	if _, err := reopened.LookupBatch([]string{"alice"}, now); err == nil {
		t.Fatal("lookup API accepted a variable-size batch")
	}

	// Canonical block anchors survive normal head advancement and are removed
	// if a reorg replaces the registration block.
	anchoredDB := memorydb.New()
	anchored, err := NewPersistentUsernameDirectory(8979, anchoredDB)
	if err != nil {
		t.Fatal(err)
	}
	registrationHash := common.HexToHash("0x1234")
	if err := anchored.PutAt(primary, now, 10, registrationHash); err != nil {
		t.Fatal(err)
	}
	canonical := func(number uint64) common.Hash {
		if number == 10 {
			return registrationHash
		}
		return common.HexToHash("0x9876")
	}
	if err := anchored.SyncCanonical(UsernameChainHead{BlockNumber: 12, BlockHash: common.HexToHash("0x12")}, canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := anchored.Resolve(handle, now); err != nil {
		t.Fatalf("canonical anchored name was removed: %v", err)
	}
	if err := anchored.SyncCanonical(UsernameChainHead{BlockNumber: 12, BlockHash: common.HexToHash("0x13")}, func(uint64) common.Hash {
		return common.HexToHash("0xabcd")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := anchored.Resolve(handle, now); err == nil {
		t.Fatal("name from orphaned registration block remained resolvable")
	}
	if anchored.ChainHead().BlockNumber != 12 || anchored.Count() != 0 {
		t.Fatal("directory did not persist its synchronized chain head or reorg pruning")
	}

	decoys := make([]UsernameBinding, 0, UsernameLookupBatchLen-1)
	for i := 0; i < UsernameLookupBatchLen-1; i++ {
		binding, err := CreateUsernameBinding(seed, "cover"+string(rune('a'+i)), identity.Code, 8979, 1, expires, now)
		if err != nil {
			t.Fatal(err)
		}
		decoys = append(decoys, binding)
	}
	batch, err := LookupBatch(handle, decoys, 8979, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != UsernameLookupBatchLen {
		t.Fatalf("batch length = %d, want %d", len(batch), UsernameLookupBatchLen)
	}
	seen := make(map[string]bool, len(batch))
	containsTarget := false
	for _, name := range batch {
		if seen[name] {
			t.Fatalf("duplicate cover name %q", name)
		}
		seen[name] = true
		if name == "alice" {
			containsTarget = true
		}
	}
	if !containsTarget {
		t.Fatal("private lookup batch omitted its target")
	}
	if _, err := LookupBatch(handle, nil, 8979, now); err == nil {
		t.Fatal("private lookup fell back to a single-name query without covers")
	}
}

func TestUsernameValidationAndFailClosedLookup(t *testing.T) {
	for _, input := range []string{"admin", "ab", "aаlice", "alice..", "-alice", "alice-"} {
		if _, err := NormalizeUsername(input); err == nil {
			t.Errorf("NormalizeUsername(%q) unexpectedly succeeded", input)
		}
	}
	handle, err := UsernameHandle("reader-1", 8979)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseUsernameHandle(handle, 8980); err == nil {
		t.Fatal("accepted a handle on the wrong chain")
	}
	if _, err := ParseUsernameHandle(handle[:len(handle)-1]+"0", 8979); err == nil {
		t.Fatal("accepted a handle with a damaged checksum")
	}
	if _, err := LookupBatch(handle, nil, 8979, time.Now()); err == nil {
		t.Fatal("private lookup fell back to a single-name query without covers")
	}
	bad := UsernameBinding{Version: UsernameVersion, ChainID: 8979, Username: "alice", Sequence: 1, ExpiresAt: 1}
	if _, err := usernameBindingMessage(bad); err == nil {
		t.Fatal("accepted malformed binding")
	}
}
