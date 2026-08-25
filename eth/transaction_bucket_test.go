package eth

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type testTransactionBucketBackend struct {
	config        *params.ChainConfig
	header        *types.Header
	stateNonce    uint64
	nextPoolNonce uint64
	pool          map[common.Hash]*types.Transaction
	canonical     map[common.Hash]bool
	submitted     []common.Hash
}

func newTestTransactionBucketBackend() *testTransactionBucketBackend {
	return &testTransactionBucketBackend{
		config:    params.TestChainConfig,
		header:    &types.Header{Number: big.NewInt(1)},
		pool:      make(map[common.Hash]*types.Transaction),
		canonical: make(map[common.Hash]bool),
	}
}

func (backend *testTransactionBucketBackend) chainConfig() *params.ChainConfig {
	return backend.config
}

func (backend *testTransactionBucketBackend) currentHeader() *types.Header {
	return backend.header
}

func (backend *testTransactionBucketBackend) canonicalNonce(common.Address) (uint64, error) {
	return backend.stateNonce, nil
}

func (backend *testTransactionBucketBackend) poolNonce(common.Address) uint64 {
	return backend.nextPoolNonce
}

func (backend *testTransactionBucketBackend) poolTransaction(hash common.Hash) *types.Transaction {
	return backend.pool[hash]
}

func (backend *testTransactionBucketBackend) canonicalTransaction(hash common.Hash) bool {
	return backend.canonical[hash]
}

func (backend *testTransactionBucketBackend) submitTransaction(_ context.Context, tx *types.Transaction) error {
	backend.pool[tx.Hash()] = tx
	backend.submitted = append(backend.submitted, tx.Hash())
	if tx.Nonce() >= backend.nextPoolNonce {
		backend.nextPoolNonce = tx.Nonce() + 1
	}
	return nil
}

func testBucketBatch(t *testing.T, bucket *transactionBucket, count uint64) *transactionBucketBatch {
	t.Helper()
	now := time.Now().UTC()
	batch := &transactionBucketBatch{
		ID:              "test-batch",
		CancelTokenHash: bucketTokenHash("cancel-token"),
		From:            common.HexToAddress("0x0000000000000000000000000000000000001234"),
		ExpectedCount:   count,
		ReceivedCount:   count,
		State:           "processing",
		CreatedAt:       now.Unix(),
		UpdatedAt:       now.Unix(),
		ExpiresAt:       now.Add(time.Hour).Unix(),
		Entries:         make(map[uint64]*transactionBucketEntry),
	}
	for index := uint64(0); index < count; index++ {
		tx := types.NewTransaction(index, common.Address{0x01}, big.NewInt(1), params.TxGas, big.NewInt(1), nil)
		raw, err := tx.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		entry := &transactionBucketEntry{Index: index, Nonce: index, Hash: tx.Hash(), Raw: raw, Status: "queued"}
		batch.Entries[index] = entry
		batch.RawBytes += uint64(len(raw))
	}
	bucket.batches[batch.ID] = batch
	entries := make([]*transactionBucketEntry, 0, len(batch.Entries))
	for index := uint64(0); index < count; index++ {
		entries = append(entries, batch.Entries[index])
	}
	if err := bucket.persistLocked(batch, entries...); err != nil {
		t.Fatal(err)
	}
	return batch
}

func TestTransactionBucketMaintainsTenTransactionWindow(t *testing.T) {
	backend := newTestTransactionBucketBackend()
	db := rawdb.NewMemoryDatabase()
	bucket := newTransactionBucket(backend, db)
	batch := testBucketBatch(t, bucket, 12)

	bucket.run()
	if len(backend.submitted) != 10 {
		t.Fatalf("submitted transaction count = %d, want 10", len(backend.submitted))
	}
	status, err := bucket.status(batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.SubmittedCount != 10 || status.QueuedCount != 2 {
		t.Fatalf("initial status = %+v, want 10 submitted and 2 queued", status)
	}

	for index := uint64(0); index < 5; index++ {
		entry := batch.Entries[index]
		delete(backend.pool, entry.Hash)
		backend.canonical[entry.Hash] = true
	}
	backend.stateNonce = 5
	bucket.run()
	status, err = bucket.status(batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.ConfirmedCount != 5 || status.SubmittedCount != 7 || status.QueuedCount != 0 {
		t.Fatalf("refilled status = %+v, want 5 confirmed and 7 submitted", status)
	}
	if len(backend.submitted) != 12 {
		t.Fatalf("total submitted transaction count = %d, want 12", len(backend.submitted))
	}
}

func TestTransactionBucketPersistsAndCancelsOnlyQueuedTransactions(t *testing.T) {
	backend := newTestTransactionBucketBackend()
	db := rawdb.NewMemoryDatabase()
	bucket := newTransactionBucket(backend, db)
	batch := testBucketBatch(t, bucket, 12)
	bucket.run()

	reloaded := newTransactionBucket(backend, db)
	status, err := reloaded.cancel(batch.ID, "cancel-token")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "canceled" || status.SubmittedCount != 10 || status.CanceledCount != 2 {
		t.Fatalf("canceled status = %+v, want 10 submitted and 2 canceled", status)
	}
	for index := uint64(10); index < 12; index++ {
		if entry := reloaded.batches[batch.ID].Entries[index]; len(entry.Raw) != 0 {
			t.Fatalf("canceled transaction %d retained raw bytes", index)
		}
	}
}

func TestTransactionBucketBeginRequiresSignedShieldedPQTransaction(t *testing.T) {
	backend := newTestTransactionBucketBackend()
	bucket := newTransactionBucket(backend, rawdb.NewMemoryDatabase())
	legacy := types.NewTransaction(0, common.Address{0x01}, big.NewInt(1), params.TxGas, big.NewInt(1), nil)
	raw, err := legacy.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bucket.begin(2, raw); err == nil {
		t.Fatal("transaction bucket accepted an unsigned non-PQ transaction")
	}
	if len(bucket.batches) != 0 {
		t.Fatalf("invalid begin created %d batches", len(bucket.batches))
	}
}
