package eth

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	transactionBucketWindowSize      = uint64(10)
	transactionBucketMaxTransactions = uint64(10000)
	transactionBucketMaxRawBytes     = uint64(256 * 1024 * 1024)
	transactionBucketMaxRawTxBytes   = 128 * 1024
	transactionBucketActiveTTL       = time.Hour
	transactionBucketRetryInterval   = 10 * time.Second
	transactionBucketTickInterval    = 2 * time.Second
	transactionBucketRetention       = 7 * 24 * time.Hour
)

var (
	transactionBatchPrefix = []byte("tkmprivacy/transaction-batches/v1/meta/")
	transactionEntryPrefix = []byte("tkmprivacy/transaction-batches/v1/tx/")
)

type transactionBucketBackend interface {
	chainConfig() *params.ChainConfig
	currentHeader() *types.Header
	canonicalNonce(common.Address) (uint64, error)
	poolNonce(common.Address) uint64
	poolTransaction(common.Hash) *types.Transaction
	canonicalTransaction(common.Hash) bool
	submitTransaction(context.Context, *types.Transaction) error
}

type ethereumTransactionBucketBackend struct {
	eth *Ethereum
}

func (b ethereumTransactionBucketBackend) chainConfig() *params.ChainConfig {
	return b.eth.blockchain.Config()
}

func (b ethereumTransactionBucketBackend) currentHeader() *types.Header {
	return b.eth.blockchain.CurrentHeader()
}

func (b ethereumTransactionBucketBackend) canonicalNonce(address common.Address) (uint64, error) {
	state, err := b.eth.blockchain.State()
	if err != nil {
		return 0, err
	}
	return state.GetNonce(address), nil
}

func (b ethereumTransactionBucketBackend) poolNonce(address common.Address) uint64 {
	return b.eth.txPool.PoolNonce(address)
}

func (b ethereumTransactionBucketBackend) poolTransaction(hash common.Hash) *types.Transaction {
	return b.eth.txPool.Get(hash)
}

func (b ethereumTransactionBucketBackend) canonicalTransaction(hash common.Hash) bool {
	lookup, tx := b.eth.blockchain.GetCanonicalTransaction(hash)
	return lookup != nil && tx != nil
}

func (b ethereumTransactionBucketBackend) submitTransaction(ctx context.Context, tx *types.Transaction) error {
	_, err := ethapi.SubmitTransaction(ctx, b.eth.APIBackend, tx)
	return err
}

type transactionBucketEntry struct {
	Index       uint64      `json:"index"`
	Nonce       uint64      `json:"nonce"`
	Hash        common.Hash `json:"hash"`
	Raw         []byte      `json:"raw,omitempty"`
	Status      string      `json:"status"`
	Error       string      `json:"error,omitempty"`
	LastAttempt int64       `json:"lastAttempt,omitempty"`
}

type transactionBucketBatch struct {
	ID              string                             `json:"id"`
	CancelTokenHash string                             `json:"cancelTokenHash"`
	From            common.Address                     `json:"from"`
	BaseNonce       uint64                             `json:"baseNonce"`
	ExpectedCount   uint64                             `json:"expectedCount"`
	ReceivedCount   uint64                             `json:"receivedCount"`
	RawBytes        uint64                             `json:"rawBytes"`
	State           string                             `json:"state"`
	Error           string                             `json:"error,omitempty"`
	CreatedAt       int64                              `json:"createdAt"`
	UpdatedAt       int64                              `json:"updatedAt"`
	ExpiresAt       int64                              `json:"expiresAt"`
	CompletedAt     int64                              `json:"completedAt,omitempty"`
	Entries         map[uint64]*transactionBucketEntry `json:"-"`
}

// TransactionBatchBegin acknowledges durable creation of a batch from its
// first locally signed transaction.
type TransactionBatchBegin struct {
	BatchID          string                      `json:"batchId"`
	CancelToken      string                      `json:"cancelToken"`
	From             common.Address              `json:"from"`
	BaseNonce        hexutil.Uint64              `json:"baseNonce"`
	ExpectedCount    hexutil.Uint64              `json:"expectedCount"`
	WindowSize       hexutil.Uint64              `json:"windowSize"`
	ExpiresAt        hexutil.Uint64              `json:"expiresAt"`
	FirstTransaction TransactionBatchQueueResult `json:"firstTransaction"`
}

// TransactionBucketInfo advertises daemon-side batching limits before the
// browser spends time constructing the first proof.
type TransactionBucketInfo struct {
	Available       bool           `json:"available"`
	WindowSize      hexutil.Uint64 `json:"windowSize"`
	MaxTransactions hexutil.Uint64 `json:"maxTransactions"`
}

// TransactionBatchQueueResult acknowledges durable storage of one locally
// signed transaction. Submitted is true when the scheduler has also promoted
// it into the node transaction pool.
type TransactionBatchQueueResult struct {
	BatchID   string         `json:"batchId"`
	Index     hexutil.Uint64 `json:"index"`
	TxHash    common.Hash    `json:"txHash"`
	Nonce     hexutil.Uint64 `json:"nonce"`
	Status    string         `json:"status"`
	Submitted bool           `json:"submitted"`
}

// TransactionBatchStatus is a compact progress view. Raw transactions and the
// cancellation-token hash never leave the daemon store.
type TransactionBatchStatus struct {
	BatchID        string         `json:"batchId"`
	From           common.Address `json:"from"`
	BaseNonce      hexutil.Uint64 `json:"baseNonce"`
	ExpectedCount  hexutil.Uint64 `json:"expectedCount"`
	ReceivedCount  hexutil.Uint64 `json:"receivedCount"`
	QueuedCount    hexutil.Uint64 `json:"queuedCount"`
	SubmittedCount hexutil.Uint64 `json:"submittedCount"`
	ConfirmedCount hexutil.Uint64 `json:"confirmedCount"`
	FailedCount    hexutil.Uint64 `json:"failedCount"`
	CanceledCount  hexutil.Uint64 `json:"canceledCount"`
	WindowSize     hexutil.Uint64 `json:"windowSize"`
	State          string         `json:"state"`
	Error          string         `json:"error,omitempty"`
	CreatedAt      hexutil.Uint64 `json:"createdAt"`
	UpdatedAt      hexutil.Uint64 `json:"updatedAt"`
	ExpiresAt      hexutil.Uint64 `json:"expiresAt"`
}

type transactionBucket struct {
	backend transactionBucketBackend
	db      ethdb.Database

	mu      sync.Mutex
	batches map[string]*transactionBucketBatch
	quit    chan struct{}
	done    chan struct{}
	started bool
}

func newTransactionBucket(backend transactionBucketBackend, db ethdb.Database) *transactionBucket {
	bucket := &transactionBucket{
		backend: backend,
		db:      db,
		batches: make(map[string]*transactionBucketBatch),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if err := bucket.load(); err != nil {
		log.Error("Failed to load shielded transaction bucket", "err", err)
	}
	return bucket
}

func (bucket *transactionBucket) start() {
	if bucket == nil {
		return
	}
	bucket.mu.Lock()
	if bucket.started {
		bucket.mu.Unlock()
		return
	}
	bucket.started = true
	bucket.mu.Unlock()
	go bucket.loop()
}

func (bucket *transactionBucket) stop() {
	if bucket == nil {
		return
	}
	bucket.mu.Lock()
	started := bucket.started
	bucket.mu.Unlock()
	if !started {
		return
	}
	select {
	case <-bucket.quit:
	default:
		close(bucket.quit)
	}
	<-bucket.done
}

func (bucket *transactionBucket) loop() {
	ticker := time.NewTicker(transactionBucketTickInterval)
	defer ticker.Stop()
	defer close(bucket.done)
	for {
		select {
		case <-ticker.C:
			bucket.run()
		case <-bucket.quit:
			return
		}
	}
}

func (bucket *transactionBucket) begin(expected uint64, raw []byte) (TransactionBatchBegin, error) {
	if expected == 0 || expected > transactionBucketMaxTransactions {
		return TransactionBatchBegin{}, fmt.Errorf("transaction batch count must be between 1 and %d", transactionBucketMaxTransactions)
	}
	tx, from, err := bucket.validateRawTransaction(raw)
	if err != nil {
		return TransactionBatchBegin{}, err
	}
	baseNonce := tx.Nonce()
	if expected > math.MaxUint64-baseNonce {
		return TransactionBatchBegin{}, errors.New("transaction batch nonce range overflows uint64")
	}
	id, err := randomBucketSecret()
	if err != nil {
		return TransactionBatchBegin{}, err
	}
	token, err := randomBucketSecret()
	if err != nil {
		return TransactionBatchBegin{}, err
	}
	now := time.Now().UTC()
	batch := &transactionBucketBatch{
		ID:              id,
		CancelTokenHash: bucketTokenHash(token),
		From:            from,
		BaseNonce:       baseNonce,
		ExpectedCount:   expected,
		ReceivedCount:   1,
		RawBytes:        uint64(len(raw)),
		State:           "receiving",
		CreatedAt:       now.Unix(),
		UpdatedAt:       now.Unix(),
		ExpiresAt:       now.Add(transactionBucketActiveTTL).Unix(),
		Entries:         make(map[uint64]*transactionBucketEntry),
	}
	entry := &transactionBucketEntry{
		Index:  0,
		Nonce:  tx.Nonce(),
		Hash:   tx.Hash(),
		Raw:    append([]byte(nil), raw...),
		Status: "queued",
	}
	batch.Entries[0] = entry
	if expected == 1 {
		batch.State = "processing"
	}
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if active := bucket.activeBatchForSenderLocked(from, ""); active != nil {
		return TransactionBatchBegin{}, fmt.Errorf("sender already has active transaction batch %s", active.ID)
	}
	if current := bucket.backend.poolNonce(from); current != baseNonce {
		return TransactionBatchBegin{}, fmt.Errorf("first transaction nonce is stale: have %d, want %d; rebuild the first proof", baseNonce, current)
	}
	if bucket.totalRawBytesLocked()+uint64(len(raw)) > transactionBucketMaxRawBytes {
		return TransactionBatchBegin{}, errors.New("transaction bucket storage limit reached; retry after queued batches confirm")
	}
	bucket.batches[id] = batch
	if err := bucket.persistLocked(batch, entry); err != nil {
		delete(bucket.batches, id)
		return TransactionBatchBegin{}, err
	}
	bucket.scheduleBatchLocked(batch, now)
	return TransactionBatchBegin{
		BatchID:          id,
		CancelToken:      token,
		From:             from,
		BaseNonce:        hexutil.Uint64(baseNonce),
		ExpectedCount:    hexutil.Uint64(expected),
		WindowSize:       hexutil.Uint64(transactionBucketWindowSize),
		ExpiresAt:        hexutil.Uint64(batch.ExpiresAt),
		FirstTransaction: queueResult(batch, entry),
	}, nil
}

func (bucket *transactionBucket) validateRawTransaction(raw []byte) (*types.Transaction, common.Address, error) {
	if len(raw) == 0 || len(raw) > transactionBucketMaxRawTxBytes {
		return nil, common.Address{}, fmt.Errorf("raw transaction must contain between 1 and %d bytes", transactionBucketMaxRawTxBytes)
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return nil, common.Address{}, fmt.Errorf("decode raw transaction: %w", err)
	}
	head := bucket.backend.currentHeader()
	if head == nil {
		return nil, common.Address{}, errors.New("canonical chain head is unavailable")
	}
	config := bucket.backend.chainConfig()
	if config == nil || config.ChainID == nil || tx.ChainId().Cmp(config.ChainID) != 0 {
		return nil, common.Address{}, errors.New("raw transaction has the wrong chain ID")
	}
	if tx.Type() != types.PQTkmTxType {
		return nil, common.Address{}, errors.New("transaction bucket accepts only locally signed PQ transactions")
	}
	if _, ok, err := core.DecodeShieldedTransaction(tx.Data()); err != nil || !ok {
		if err != nil {
			return nil, common.Address{}, fmt.Errorf("decode shielded transaction: %w", err)
		}
		return nil, common.Address{}, errors.New("transaction bucket accepts only shielded transactions")
	}
	if err := core.ValidateShieldedTransactionBasics(config, head.Number, head.Time, tx); err != nil {
		return nil, common.Address{}, err
	}
	signer := types.MakeSigner(config, head.Number, head.Time)
	from, err := types.Sender(signer, tx)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("recover PQ transaction sender: %w", err)
	}
	return tx, from, nil
}

func (bucket *transactionBucket) queue(batchID string, index uint64, raw []byte) (TransactionBatchQueueResult, error) {
	tx, from, err := bucket.validateRawTransaction(raw)
	if err != nil {
		return TransactionBatchQueueResult{}, err
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	batch := bucket.batches[batchID]
	if batch == nil {
		return TransactionBatchQueueResult{}, errors.New("transaction batch not found")
	}
	now := time.Now().UTC()
	if batch.ExpiresAt <= now.Unix() {
		return TransactionBatchQueueResult{}, errors.New("transaction batch has expired")
	}
	if batch.State == "completed" || batch.State == "canceled" || batch.State == "expired" || batch.State == "failed" {
		return TransactionBatchQueueResult{}, fmt.Errorf("transaction batch is %s", batch.State)
	}
	if from != batch.From {
		return TransactionBatchQueueResult{}, errors.New("raw transaction sender does not match the batch sender")
	}
	if index >= batch.ExpectedCount {
		return TransactionBatchQueueResult{}, errors.New("transaction batch index is out of range")
	}
	expectedNonce := batch.BaseNonce + index
	if tx.Nonce() != expectedNonce {
		return TransactionBatchQueueResult{}, fmt.Errorf("raw transaction nonce %d does not match reserved nonce %d", tx.Nonce(), expectedNonce)
	}
	if existing := batch.Entries[index]; existing != nil {
		if existing.Hash != tx.Hash() {
			return TransactionBatchQueueResult{}, errors.New("transaction batch index already contains a different transaction")
		}
		return queueResult(batch, existing), nil
	}
	if index != batch.ReceivedCount {
		return TransactionBatchQueueResult{}, fmt.Errorf("transaction batch requires index %d next", batch.ReceivedCount)
	}
	previousState, previousExpiresAt := batch.State, batch.ExpiresAt
	if bucket.totalRawBytesLocked()+uint64(len(raw)) > transactionBucketMaxRawBytes {
		return TransactionBatchQueueResult{}, errors.New("transaction bucket storage limit reached; retry after queued batches confirm")
	}
	entry := &transactionBucketEntry{
		Index:  index,
		Nonce:  tx.Nonce(),
		Hash:   tx.Hash(),
		Raw:    append([]byte(nil), raw...),
		Status: "queued",
	}
	batch.Entries[index] = entry
	batch.ReceivedCount++
	batch.RawBytes += uint64(len(raw))
	batch.UpdatedAt = now.Unix()
	batch.ExpiresAt = now.Add(transactionBucketActiveTTL).Unix()
	if batch.ReceivedCount == batch.ExpectedCount {
		batch.State = "processing"
	}
	if err := bucket.persistLocked(batch, entry); err != nil {
		delete(batch.Entries, index)
		batch.ReceivedCount--
		batch.RawBytes -= uint64(len(raw))
		batch.State = previousState
		batch.ExpiresAt = previousExpiresAt
		return TransactionBatchQueueResult{}, err
	}
	bucket.scheduleBatchLocked(batch, now)
	return queueResult(batch, entry), nil
}

func (bucket *transactionBucket) status(batchID string) (TransactionBatchStatus, error) {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	batch := bucket.batches[batchID]
	if batch == nil {
		return TransactionBatchStatus{}, errors.New("transaction batch not found")
	}
	bucket.scheduleBatchLocked(batch, time.Now().UTC())
	return batchStatus(batch), nil
}

func (bucket *transactionBucket) cancel(batchID, token string) (TransactionBatchStatus, error) {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	batch := bucket.batches[batchID]
	if batch == nil {
		return TransactionBatchStatus{}, errors.New("transaction batch not found")
	}
	if bucketTokenHash(token) != batch.CancelTokenHash {
		return TransactionBatchStatus{}, errors.New("invalid transaction batch cancellation token")
	}
	if batch.State == "completed" {
		return TransactionBatchStatus{}, errors.New("completed transaction batch cannot be canceled")
	}
	now := time.Now().UTC().Unix()
	batch.State = "canceled"
	batch.UpdatedAt = now
	batch.CompletedAt = now
	var changed []*transactionBucketEntry
	for _, entry := range batch.Entries {
		if entry.Status == "queued" {
			entry.Status = "canceled"
			batch.RawBytes -= uint64(len(entry.Raw))
			entry.Raw = nil
			entry.Error = "canceled before txpool submission"
			changed = append(changed, entry)
		}
	}
	if err := bucket.persistLocked(batch, changed...); err != nil {
		return TransactionBatchStatus{}, err
	}
	return batchStatus(batch), nil
}

func (bucket *transactionBucket) reservedNonce(address common.Address) uint64 {
	if bucket == nil {
		return 0
	}
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	var reserved uint64
	for _, batch := range bucket.batches {
		if batch.From != address || (batch.State != "receiving" && batch.State != "processing") {
			continue
		}
		end := batch.BaseNonce + batch.ExpectedCount
		if end > reserved {
			reserved = end
		}
	}
	return reserved
}

func (bucket *transactionBucket) run() {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	now := time.Now().UTC()
	ids := make([]string, 0, len(bucket.batches))
	for id := range bucket.batches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		batch := bucket.batches[id]
		bucket.scheduleBatchLocked(batch, now)
		if batch.CompletedAt > 0 && now.Sub(time.Unix(batch.CompletedAt, 0)) > transactionBucketRetention {
			if err := bucket.deleteLocked(batch); err != nil {
				log.Warn("Failed to prune shielded transaction batch", "batch", id, "err", err)
			} else {
				delete(bucket.batches, id)
			}
		}
	}
}

func (bucket *transactionBucket) scheduleBatchLocked(batch *transactionBucketBatch, now time.Time) {
	if batch == nil {
		return
	}
	canonicalNonce, err := bucket.backend.canonicalNonce(batch.From)
	if err != nil {
		batch.Error = err.Error()
		return
	}
	changed := make([]*transactionBucketEntry, 0)
	for index := uint64(0); index < batch.ReceivedCount; index++ {
		entry := batch.Entries[index]
		if entry == nil || entry.Status == "canceled" || entry.Status == "failed" {
			continue
		}
		if bucket.backend.canonicalTransaction(entry.Hash) {
			if entry.Status != "confirmed" {
				entry.Status = "confirmed"
				entry.Error = ""
				changed = append(changed, entry)
			}
			continue
		}
		if entry.Status == "confirmed" {
			entry.Status = "queued" // canonical reorg; safely resubmit below
			changed = append(changed, entry)
		}
		if entry.Nonce < canonicalNonce {
			entry.Status = "failed"
			entry.Error = "reserved nonce was consumed by a different canonical transaction"
			batch.State = "failed"
			batch.Error = entry.Error
			batch.CompletedAt = now.Unix()
			changed = append(changed, entry)
			continue
		}
		if bucket.backend.poolTransaction(entry.Hash) != nil && entry.Status != "submitted" {
			entry.Status = "submitted"
			entry.Error = ""
			changed = append(changed, entry)
		} else if entry.Status == "submitted" && bucket.backend.poolTransaction(entry.Hash) == nil {
			entry.Status = "queued"
			changed = append(changed, entry)
		}
	}
	if batch.State == "canceled" || batch.State == "expired" || batch.State == "failed" {
		if len(changed) > 0 {
			batch.UpdatedAt = now.Unix()
			_ = bucket.persistLocked(batch, changed...)
		}
		return
	}
	if batch.ExpiresAt <= now.Unix() && batch.ReceivedCount < batch.ExpectedCount {
		batch.State = "expired"
		batch.Error = "transaction batch expired before every signed transaction was received"
		batch.CompletedAt = now.Unix()
		for _, entry := range batch.Entries {
			if entry.Status == "queued" {
				entry.Status = "canceled"
				entry.Error = batch.Error
				batch.RawBytes -= uint64(len(entry.Raw))
				entry.Raw = nil
				changed = append(changed, entry)
			}
		}
		batch.UpdatedAt = now.Unix()
		_ = bucket.persistLocked(batch, changed...)
		return
	}
	inflight := uint64(0)
	for _, entry := range batch.Entries {
		if entry.Status == "submitted" {
			inflight++
		}
	}
	for index := uint64(0); index < batch.ReceivedCount && inflight < transactionBucketWindowSize; index++ {
		entry := batch.Entries[index]
		if entry == nil || entry.Status != "queued" || len(entry.Raw) == 0 {
			continue
		}
		if entry.LastAttempt > 0 && now.Sub(time.Unix(entry.LastAttempt, 0)) < transactionBucketRetryInterval {
			break
		}
		tx := new(types.Transaction)
		if err := tx.UnmarshalBinary(entry.Raw); err != nil {
			entry.Status = "failed"
			entry.Error = fmt.Sprintf("decode persisted transaction: %v", err)
			batch.State = "failed"
			batch.Error = entry.Error
			batch.CompletedAt = now.Unix()
			changed = append(changed, entry)
			break
		}
		entry.LastAttempt = now.Unix()
		submitErr := bucket.backend.submitTransaction(context.Background(), tx)
		if bucket.backend.poolTransaction(entry.Hash) == nil {
			if submitErr != nil {
				entry.Error = submitErr.Error()
			} else {
				entry.Error = "transaction is waiting in the node local tracker"
			}
			batch.Error = fmt.Sprintf("transaction %d is waiting for txpool admission: %s", index, entry.Error)
			changed = append(changed, entry)
			break
		}
		entry.Status = "submitted"
		entry.Error = ""
		batch.Error = ""
		inflight++
		changed = append(changed, entry)
	}
	status := batchStatus(batch)
	if uint64(status.ConfirmedCount) == batch.ExpectedCount {
		batch.State = "completed"
		batch.Error = ""
		if batch.CompletedAt == 0 {
			batch.CompletedAt = now.Unix()
		}
	} else if batch.ReceivedCount == batch.ExpectedCount {
		batch.State = "processing"
	} else {
		batch.State = "receiving"
	}
	if len(changed) > 0 {
		batch.UpdatedAt = now.Unix()
		if err := bucket.persistLocked(batch, changed...); err != nil {
			log.Error("Failed to persist shielded transaction batch progress", "batch", batch.ID, "err", err)
		}
	}
}

func (bucket *transactionBucket) activeBatchForSenderLocked(address common.Address, except string) *transactionBucketBatch {
	for _, batch := range bucket.batches {
		if batch.ID != except && batch.From == address && (batch.State == "receiving" || batch.State == "processing") {
			return batch
		}
	}
	return nil
}

func (bucket *transactionBucket) totalRawBytesLocked() uint64 {
	var total uint64
	for _, batch := range bucket.batches {
		total += batch.RawBytes
	}
	return total
}

func queueResult(batch *transactionBucketBatch, entry *transactionBucketEntry) TransactionBatchQueueResult {
	return TransactionBatchQueueResult{
		BatchID:   batch.ID,
		Index:     hexutil.Uint64(entry.Index),
		TxHash:    entry.Hash,
		Nonce:     hexutil.Uint64(entry.Nonce),
		Status:    entry.Status,
		Submitted: entry.Status == "submitted" || entry.Status == "confirmed",
	}
}

func batchStatus(batch *transactionBucketBatch) TransactionBatchStatus {
	var queued, submitted, confirmed, failed, canceled uint64
	for _, entry := range batch.Entries {
		switch entry.Status {
		case "queued":
			queued++
		case "submitted":
			submitted++
		case "confirmed":
			confirmed++
		case "failed":
			failed++
		case "canceled":
			canceled++
		}
	}
	return TransactionBatchStatus{
		BatchID:        batch.ID,
		From:           batch.From,
		BaseNonce:      hexutil.Uint64(batch.BaseNonce),
		ExpectedCount:  hexutil.Uint64(batch.ExpectedCount),
		ReceivedCount:  hexutil.Uint64(batch.ReceivedCount),
		QueuedCount:    hexutil.Uint64(queued),
		SubmittedCount: hexutil.Uint64(submitted),
		ConfirmedCount: hexutil.Uint64(confirmed),
		FailedCount:    hexutil.Uint64(failed),
		CanceledCount:  hexutil.Uint64(canceled),
		WindowSize:     hexutil.Uint64(transactionBucketWindowSize),
		State:          batch.State,
		Error:          batch.Error,
		CreatedAt:      hexutil.Uint64(batch.CreatedAt),
		UpdatedAt:      hexutil.Uint64(batch.UpdatedAt),
		ExpiresAt:      hexutil.Uint64(batch.ExpiresAt),
	}
}

func (bucket *transactionBucket) persistLocked(batch *transactionBucketBatch, entries ...*transactionBucketEntry) error {
	if bucket.db == nil {
		return errors.New("transaction bucket database is unavailable")
	}
	metadata, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	dbBatch := bucket.db.NewBatch()
	if err := dbBatch.Put(transactionBatchKey(batch.ID), metadata); err != nil {
		return err
	}
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := dbBatch.Put(transactionEntryKey(batch.ID, entry.Index), encoded); err != nil {
			return err
		}
	}
	return dbBatch.Write()
}

func (bucket *transactionBucket) load() error {
	if bucket.db == nil {
		return nil
	}
	iterator := bucket.db.NewIterator(transactionBatchPrefix, nil)
	defer iterator.Release()
	for iterator.Next() {
		var batch transactionBucketBatch
		if err := json.Unmarshal(iterator.Value(), &batch); err != nil {
			return fmt.Errorf("decode transaction batch metadata: %w", err)
		}
		batch.Entries = make(map[uint64]*transactionBucketEntry)
		entryIterator := bucket.db.NewIterator(transactionEntryBatchPrefix(batch.ID), nil)
		for entryIterator.Next() {
			var entry transactionBucketEntry
			if err := json.Unmarshal(entryIterator.Value(), &entry); err != nil {
				entryIterator.Release()
				return fmt.Errorf("decode transaction batch entry: %w", err)
			}
			entry.Raw = append([]byte(nil), entry.Raw...)
			batch.Entries[entry.Index] = &entry
		}
		if err := entryIterator.Error(); err != nil {
			entryIterator.Release()
			return err
		}
		entryIterator.Release()
		bucket.batches[batch.ID] = &batch
	}
	return iterator.Error()
}

func (bucket *transactionBucket) deleteLocked(batch *transactionBucketBatch) error {
	dbBatch := bucket.db.NewBatch()
	if err := dbBatch.Delete(transactionBatchKey(batch.ID)); err != nil {
		return err
	}
	for index := range batch.Entries {
		if err := dbBatch.Delete(transactionEntryKey(batch.ID, index)); err != nil {
			return err
		}
	}
	return dbBatch.Write()
}

func transactionBatchKey(batchID string) []byte {
	return append(append([]byte(nil), transactionBatchPrefix...), batchID...)
}

func transactionEntryBatchPrefix(batchID string) []byte {
	prefix := append(append([]byte(nil), transactionEntryPrefix...), batchID...)
	return append(prefix, '/')
}

func transactionEntryKey(batchID string, index uint64) []byte {
	return append(transactionEntryBatchPrefix(batchID), fmt.Sprintf("%020d", index)...)
}

func randomBucketSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := crand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func bucketTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// TransactionBucket returns daemon batching capabilities.
func (api *PrivacyAPI) TransactionBucket() TransactionBucketInfo {
	return TransactionBucketInfo{
		Available:       api != nil && api.e != nil && api.e.transactionBucket != nil,
		WindowSize:      hexutil.Uint64(transactionBucketWindowSize),
		MaxTransactions: hexutil.Uint64(transactionBucketMaxTransactions),
	}
}

// BeginTransactionBatch validates transaction zero's local PQ signature and
// atomically creates a durable sender-owned batch.
func (api *PrivacyAPI) BeginTransactionBatch(expectedCount hexutil.Uint64, firstRaw hexutil.Bytes) (TransactionBatchBegin, error) {
	if api == nil || api.e == nil || api.e.transactionBucket == nil {
		return TransactionBatchBegin{}, errors.New("transaction bucket is unavailable")
	}
	return api.e.transactionBucket.begin(uint64(expectedCount), firstRaw)
}

// QueueRawTransaction durably stores one locally signed shielded transaction
// and lets the daemon scheduler maintain the bounded txpool window.
func (api *PrivacyAPI) QueueRawTransaction(batchID string, index hexutil.Uint64, raw hexutil.Bytes) (TransactionBatchQueueResult, error) {
	if api == nil || api.e == nil || api.e.transactionBucket == nil {
		return TransactionBatchQueueResult{}, errors.New("transaction bucket is unavailable")
	}
	return api.e.transactionBucket.queue(batchID, uint64(index), raw)
}

// TransactionBatchStatus returns durable queue and confirmation progress.
func (api *PrivacyAPI) TransactionBatchStatus(batchID string) (TransactionBatchStatus, error) {
	if api == nil || api.e == nil || api.e.transactionBucket == nil {
		return TransactionBatchStatus{}, errors.New("transaction bucket is unavailable")
	}
	return api.e.transactionBucket.status(batchID)
}

// CancelTransactionBatch stops promotion of transactions that have not yet
// entered the txpool. Already submitted transactions remain valid.
func (api *PrivacyAPI) CancelTransactionBatch(batchID, cancelToken string) (TransactionBatchStatus, error) {
	if api == nil || api.e == nil || api.e.transactionBucket == nil {
		return TransactionBatchStatus{}, errors.New("transaction bucket is unavailable")
	}
	return api.e.transactionBucket.cancel(batchID, cancelToken)
}
