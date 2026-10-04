package shield3wallet

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	UsernameVersion        = uint64(1)
	UsernameMinLength      = 3
	UsernameMaxLength      = 32
	UsernameMaxLease       = 365 * 24 * time.Hour
	UsernameReclaimGrace   = 30 * 24 * time.Hour
	UsernameLookupBatchLen = 16
	UsernameDirectoryLimit = 100_000
	UsernameOwnerLimit     = 8
	UsernameClaimWorkBits  = 18
	UsernamePaymentCodeMax = 32 * 1024
	UsernameSignatureMax   = 8 * 1024
	UsernameRecordMax      = 64 * 1024
	usernamePrefix         = "TKM_SHIELD3_USERNAME_V1"
)

var reservedUsernames = map[string]struct{}{
	"admin": {}, "administrator": {}, "api": {}, "support": {}, "security": {},
	"root": {}, "system": {}, "tkm": {}, "tkmchain": {}, "wallet": {},
	"null": {}, "undefined": {}, "postmaster": {}, "abuse": {},
}

var usernameRecordPrefix = []byte("tkm/shield3/usernames/v1/")
var usernameAnchorPrefix = []byte("tkm/shield3/usernames/anchor/v1/")
var usernameHeadKey = []byte("tkm/shield3/usernames/head/v1")

// UsernameBinding is an owner-signed, chain-bound off-chain directory record.
// The directory must not publish these records in EVM state or emit them in
// public logs. LookupBatch hides the selected entry only when its cover names
// are independently sourced from the directory handling the batch request.
type UsernameBinding struct {
	Version     uint64         `json:"version"`
	ChainID     uint64         `json:"chainId"`
	Username    string         `json:"username"`
	Address     common.Address `json:"address"`
	CodeHash    common.Hash    `json:"codeHash"`
	PaymentCode string         `json:"paymentCode"`
	Sequence    uint64         `json:"sequence"`
	ExpiresAt   uint64         `json:"expiresAt"`
	ClaimNonce  uint64         `json:"claimNonce"`
	Signature   []byte         `json:"signature"`
}

// NormalizeUsername accepts ASCII handles only. Restricting the alphabet
// avoids Unicode confusables, bidi controls, alternate normalization forms,
// and case-folding ambiguities in a payment destination.
func NormalizeUsername(input string) (string, error) {
	name := strings.TrimPrefix(strings.TrimSpace(input), "@")
	name = strings.ToLower(name)
	if len(name) < UsernameMinLength || len(name) > UsernameMaxLength {
		return "", fmt.Errorf("username must be %d-%d ASCII characters", UsernameMinLength, UsernameMaxLength)
	}
	if _, reserved := reservedUsernames[name]; reserved {
		return "", errors.New("username is reserved")
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return "", errors.New("username cannot start or end with a hyphen")
	}
	previousHyphen := false
	for _, c := range []byte(name) {
		valid := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if !valid {
			return "", errors.New("username may contain only ASCII letters, digits, underscore, and hyphen")
		}
		if c == '-' && previousHyphen {
			return "", errors.New("username cannot contain consecutive hyphens")
		}
		previousHyphen = c == '-'
	}
	return name, nil
}

// UsernameHandle returns a short chain-specific spelling with a typo-detecting
// checksum. The checksum is not an ownership proof; the signed binding is.
func UsernameHandle(username string, chainID uint64) (string, error) {
	name, err := NormalizeUsername(username)
	if err != nil || chainID == 0 {
		if err != nil {
			return "", err
		}
		return "", errors.New("username handle requires a non-zero chain ID")
	}
	var chain [8]byte
	binary.BigEndian.PutUint64(chain[:], chainID)
	h := sha256.New()
	h.Write([]byte("TKM_SHIELD3_USERNAME_HANDLE_V1"))
	h.Write(chain[:])
	h.Write([]byte(name))
	check := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil)[:4])
	return "@" + name + "#" + strings.ToLower(check), nil
}

// ParseUsernameHandle validates the check digits and returns the canonical
// username. A checksum catches typing errors; resolution still authenticates
// the signed directory binding and verifies the Shield3 payment code.
func ParseUsernameHandle(handle string, chainID uint64) (string, error) {
	parts := strings.Split(strings.TrimSpace(handle), "#")
	if len(parts) != 2 {
		return "", errors.New("username handle must include its checksum")
	}
	want, err := UsernameHandle(parts[0], chainID)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(want, strings.TrimSpace(handle)) {
		return "", errors.New("username checksum or chain ID does not match")
	}
	return NormalizeUsername(parts[0])
}

func usernameBindingMessage(binding UsernameBinding) ([]byte, error) {
	name, err := NormalizeUsername(binding.Username)
	if err != nil || name != binding.Username || binding.Version != UsernameVersion || binding.ChainID == 0 || binding.Address == (common.Address{}) || binding.CodeHash == (common.Hash{}) || binding.Sequence == 0 || binding.ExpiresAt == 0 {
		return nil, errors.New("invalid username binding fields")
	}
	unsigned := binding
	unsigned.Signature = nil
	unsigned.PaymentCode = ""
	encoded, err := rlp.EncodeToBytes(unsigned)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte(usernamePrefix))
	h.Write(encoded)
	return h.Sum(nil), nil
}

func usernameClaimDigest(record UsernameBinding, nonce uint64) common.Hash {
	var chain, counter, sequence [8]byte
	binary.BigEndian.PutUint64(chain[:], record.ChainID)
	binary.BigEndian.PutUint64(counter[:], nonce)
	binary.BigEndian.PutUint64(sequence[:], record.Sequence)
	h := sha256.New()
	h.Write([]byte("TKM_SHIELD3_USERNAME_CLAIM_POW_V1"))
	h.Write(chain[:])
	h.Write([]byte(record.Username))
	h.Write(record.Address[:])
	h.Write(record.CodeHash[:])
	h.Write(sequence[:])
	h.Write(counter[:])
	return common.BytesToHash(h.Sum(nil))
}

func validUsernameClaimWork(digest common.Hash) bool {
	for bit := 0; bit < UsernameClaimWorkBits; bit++ {
		if digest[bit/8]&(1<<uint(7-bit%8)) != 0 {
			return false
		}
	}
	return true
}

// CreateUsernameBinding signs an off-chain name-to-Shield3-code record using
// the Shield3 account seed. Records are leases: renewal increments sequence,
// and directories must reject expired or replayed records.
func CreateUsernameBinding(seed []byte, username, paymentCode string, chainID, sequence uint64, expires time.Time, now time.Time) (UsernameBinding, error) {
	var record UsernameBinding
	name, err := NormalizeUsername(username)
	if err != nil {
		return record, err
	}
	if chainID == 0 || sequence == 0 {
		return record, errors.New("username binding requires chain ID and non-zero sequence")
	}
	if len(paymentCode) == 0 || len(paymentCode) > UsernamePaymentCodeMax {
		return record, errors.New("Shield3 payment code exceeds the username binding limit")
	}
	now = now.UTC().Truncate(time.Second)
	expires = expires.UTC().Truncate(time.Second)
	if !expires.After(now) || expires.Sub(now) > UsernameMaxLease {
		return record, errors.New("username lease must expire within one year")
	}
	payload, err := DecodePaymentCode(paymentCode, chainID)
	if err != nil {
		return record, fmt.Errorf("invalid Shield3 payment code: %w", err)
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return record, err
	}
	publicKey := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil || address != payload.Address {
		return record, errors.New("username signer does not own the Shield3 payment code")
	}
	record = UsernameBinding{
		Version: UsernameVersion, ChainID: chainID, Username: name,
		Address: payload.Address, CodeHash: cryptoHash([]byte(paymentCode)),
		PaymentCode: paymentCode, Sequence: sequence, ExpiresAt: uint64(expires.Unix()),
	}
	for nonce := uint64(0); ; nonce++ {
		if validUsernameClaimWork(usernameClaimDigest(record, nonce)) {
			record.ClaimNonce = nonce
			break
		}
		if nonce == ^uint64(0) {
			return UsernameBinding{}, errors.New("username claim work nonce exhausted")
		}
	}
	message, err := usernameBindingMessage(record)
	if err != nil {
		return UsernameBinding{}, err
	}
	record.Signature, err = pqcrypto.SignMLDSA87(key, message)
	if err != nil {
		return UsernameBinding{}, err
	}
	return record, nil
}

func cryptoHash(data []byte) common.Hash {
	digest := sha256.Sum256(data)
	return common.BytesToHash(digest[:])
}

// VerifyUsernameBinding rejects cross-chain records, expired leases, invalid
// recipient codes, altered code bytes, and signatures from a different PQ key.
func VerifyUsernameBinding(record UsernameBinding, chainID uint64, now time.Time) error {
	if len(record.PaymentCode) == 0 || len(record.PaymentCode) > UsernamePaymentCodeMax || len(record.Signature) == 0 || len(record.Signature) > UsernameSignatureMax {
		return errors.New("username binding exceeds its size limits")
	}
	message, err := usernameBindingMessage(record)
	if err != nil {
		return err
	}
	if record.ChainID != chainID || chainID == 0 {
		return errors.New("username binding belongs to a different chain")
	}
	if !validUsernameClaimWork(usernameClaimDigest(record, record.ClaimNonce)) {
		return errors.New("username claim proof of work is invalid")
	}
	if now.Unix() >= int64(record.ExpiresAt) || int64(record.ExpiresAt)-now.Unix() > int64(UsernameMaxLease.Seconds()) {
		return errors.New("username binding is expired or has an invalid lease")
	}
	if cryptoHash([]byte(record.PaymentCode)) != record.CodeHash {
		return errors.New("username binding payment code checksum mismatch")
	}
	payload, err := DecodePaymentCode(record.PaymentCode, chainID)
	if err != nil || payload.Address != record.Address {
		return errors.New("username binding does not match a valid Shield3 payment code")
	}
	prefix := "tkmshield3."
	if !strings.HasPrefix(record.PaymentCode, prefix) {
		return errors.New("invalid Shield3 payment code")
	}
	encoded, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(record.PaymentCode, prefix))
	if err != nil {
		return errors.New("invalid Shield3 payment code")
	}
	var code PaymentCode
	if err := rlp.DecodeBytes(encoded, &code); err != nil || !pqcrypto.VerifyMLDSA87(code.PublicKey, message, record.Signature) {
		return errors.New("username binding signature is invalid")
	}
	return nil
}

// UsernameDirectory is a concurrency-safe local directory. When constructed
// with a database it persists records locally; it does not gossip or replicate
// them across nodes.
type UsernameDirectory struct {
	mu      sync.RWMutex
	chainID uint64
	records map[string]UsernameBinding
	anchors map[string]UsernameChainAnchor
	head    UsernameChainHead
	db      ethdb.KeyValueStore
}

// UsernameChainAnchor ties an off-chain signed binding to the canonical block
// at which the node accepted it. The binding itself stays private from EVM
// state; this anchor lets each node invalidate local records on a reorg.
type UsernameChainAnchor struct {
	BlockNumber uint64
	BlockHash   common.Hash
}

// UsernameChainHead records how far the node has reconciled its directory
// against canonical chain history.
type UsernameChainHead struct {
	BlockNumber uint64
	BlockHash   common.Hash
}

func NewUsernameDirectory(chainID uint64) (*UsernameDirectory, error) {
	if chainID == 0 {
		return nil, errors.New("username directory requires a chain ID")
	}
	return &UsernameDirectory{chainID: chainID, records: make(map[string]UsernameBinding), anchors: make(map[string]UsernameChainAnchor)}, nil
}

// NewPersistentUsernameDirectory loads signed bindings from the supplied node
// database and persists subsequent verified updates there. This database is
// local service data; it is not EVM state and does not synchronize by itself.
func NewPersistentUsernameDirectory(chainID uint64, db ethdb.KeyValueStore) (*UsernameDirectory, error) {
	if db == nil {
		return nil, errors.New("persistent username directory requires a database")
	}
	directory, err := NewUsernameDirectory(chainID)
	if err != nil {
		return nil, err
	}
	directory.db = db
	iterator := db.NewIterator(usernameRecordPrefix, nil)
	defer iterator.Release()
	for iterator.Next() {
		if len(directory.records) >= UsernameDirectoryLimit {
			return nil, errors.New("persisted username directory exceeds its record limit")
		}
		if len(iterator.Value()) > UsernameRecordMax {
			return nil, errors.New("persisted username binding exceeds its size limit")
		}
		var record UsernameBinding
		if err := rlp.DecodeBytes(iterator.Value(), &record); err != nil {
			return nil, fmt.Errorf("decode persisted username binding: %w", err)
		}
		name, err := NormalizeUsername(record.Username)
		if err != nil || name != record.Username || record.ChainID != chainID || !bytes.Equal(iterator.Key(), usernameRecordKey(chainID, name)) || VerifyUsernameBinding(record, chainID, time.Unix(int64(record.ExpiresAt)-1, 0)) != nil {
			return nil, errors.New("persisted username directory contains an invalid record")
		}
		if _, duplicate := directory.records[name]; duplicate {
			return nil, errors.New("persisted username directory contains duplicate names")
		}
		directory.records[name] = record
		anchorExists, anchorErr := db.Has(usernameAnchorKey(chainID, name))
		if anchorErr != nil {
			return nil, fmt.Errorf("check username chain anchor: %w", anchorErr)
		}
		if anchorExists {
			anchorBytes, anchorErr := db.Get(usernameAnchorKey(chainID, name))
			if anchorErr != nil {
				return nil, fmt.Errorf("read username chain anchor: %w", anchorErr)
			}
			var anchor UsernameChainAnchor
			if err := rlp.DecodeBytes(anchorBytes, &anchor); err != nil {
				return nil, fmt.Errorf("decode username chain anchor: %w", err)
			}
			directory.anchors[name] = anchor
		}
	}
	if err := iterator.Error(); err != nil {
		return nil, fmt.Errorf("read persisted username directory: %w", err)
	}
	headExists, err := db.Has(usernameHeadKey)
	if err != nil {
		return nil, fmt.Errorf("check username directory head: %w", err)
	}
	if headExists {
		headBytes, err := db.Get(usernameHeadKey)
		if err != nil {
			return nil, fmt.Errorf("read username directory head: %w", err)
		}
		if err := rlp.DecodeBytes(headBytes, &directory.head); err != nil {
			return nil, fmt.Errorf("decode username directory head: %w", err)
		}
	}
	return directory, nil
}

func usernameRecordKey(chainID uint64, name string) []byte {
	var chain [8]byte
	binary.BigEndian.PutUint64(chain[:], chainID)
	h := sha256.New()
	h.Write([]byte("TKM_SHIELD3_USERNAME_RECORD_KEY_V1"))
	h.Write(chain[:])
	h.Write([]byte(name))
	return append(append([]byte(nil), usernameRecordPrefix...), h.Sum(nil)...)
}

func usernameAnchorKey(chainID uint64, name string) []byte {
	key := usernameRecordKey(chainID, name)
	return append(append([]byte(nil), usernameAnchorPrefix...), key[len(usernameRecordPrefix):]...)
}

// Put verifies the signature and lease before applying an update. Names cannot
// be hijacked by another key, and stale/replayed sequence numbers are refused.
func (d *UsernameDirectory) Put(record UsernameBinding, now time.Time) error {
	if d == nil {
		return errors.New("username directory is unavailable")
	}
	if err := VerifyUsernameBinding(record, d.chainID, now); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if old, exists := d.records[record.Username]; exists {
		if old.Address != record.Address && now.Unix() < int64(old.ExpiresAt)+int64(UsernameReclaimGrace.Seconds()) {
			return errors.New("username is already owned by another Shield3 identity")
		}
		if old.Address == record.Address && record.Sequence == old.Sequence {
			oldBytes, oldErr := rlp.EncodeToBytes(old)
			newBytes, newErr := rlp.EncodeToBytes(record)
			if oldErr == nil && newErr == nil && bytes.Equal(oldBytes, newBytes) {
				return nil // Exact retry is idempotent across interrupted network writes.
			}
		}
		if old.Address == record.Address && record.Sequence <= old.Sequence {
			return errors.New("username binding sequence is stale or replayed")
		}
	} else if len(d.records) >= UsernameDirectoryLimit {
		return errors.New("username directory is full")
	} else {
		owned := 0
		for _, binding := range d.records {
			if binding.Address == record.Address && now.Unix() < int64(binding.ExpiresAt) {
				owned++
			}
		}
		if owned >= UsernameOwnerLimit {
			return errors.New("Shield3 identity has reached the username limit")
		}
	}
	if d.db != nil {
		encoded, err := rlp.EncodeToBytes(record)
		if err != nil {
			return fmt.Errorf("encode username binding: %w", err)
		}
		if err := d.db.Put(usernameRecordKey(d.chainID, record.Username), encoded); err != nil {
			return fmt.Errorf("persist username binding: %w", err)
		}
	}
	d.records[record.Username] = record
	return nil
}

// PutAt stores a verified binding with the canonical block that accepted it.
// Nodes reject or prune bindings whose anchor is absent from their canonical
// chain, so a record accepted on an orphaned branch cannot remain resolvable.
func (d *UsernameDirectory) PutAt(record UsernameBinding, now time.Time, number uint64, hash common.Hash) error {
	if hash == (common.Hash{}) {
		return errors.New("username registration requires a canonical block hash")
	}
	if err := d.Put(record, now); err != nil {
		return err
	}
	anchor := UsernameChainAnchor{BlockNumber: number, BlockHash: hash}
	encoded, err := rlp.EncodeToBytes(anchor)
	if err != nil {
		return fmt.Errorf("encode username chain anchor: %w", err)
	}
	if d.db != nil {
		if err := d.db.Put(usernameAnchorKey(d.chainID, record.Username), encoded); err != nil {
			return fmt.Errorf("persist username chain anchor: %w", err)
		}
	}
	d.mu.Lock()
	d.anchors[record.Username] = anchor
	d.mu.Unlock()
	return nil
}

// SyncCanonical reconciles stored names with the current canonical chain. A
// missing anchor is treated as legacy/unverified data and removed. The head
// marker advances only after all records have been checked successfully.
func (d *UsernameDirectory) SyncCanonical(head UsernameChainHead, canonicalHash func(uint64) common.Hash) error {
	if d == nil || canonicalHash == nil || head.BlockHash == (common.Hash{}) {
		return errors.New("username directory requires a canonical chain head")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.head.BlockNumber == head.BlockNumber && d.head.BlockHash == head.BlockHash {
		return nil
	}
	for name, anchor := range d.anchors {
		canonical := anchor.BlockNumber <= head.BlockNumber && canonicalHash(anchor.BlockNumber) == anchor.BlockHash
		if canonical {
			continue
		}
		if d.db != nil {
			if err := d.db.Delete(usernameRecordKey(d.chainID, name)); err != nil {
				return fmt.Errorf("remove non-canonical username record: %w", err)
			}
			if err := d.db.Delete(usernameAnchorKey(d.chainID, name)); err != nil {
				return fmt.Errorf("remove non-canonical username anchor: %w", err)
			}
		}
		delete(d.records, name)
		delete(d.anchors, name)
	}
	// Prune persisted records written by older versions without a block anchor.
	for name := range d.records {
		if _, ok := d.anchors[name]; ok {
			continue
		}
		if d.db != nil {
			if err := d.db.Delete(usernameRecordKey(d.chainID, name)); err != nil {
				return fmt.Errorf("remove unanchored username record: %w", err)
			}
		}
		delete(d.records, name)
	}
	encoded, err := rlp.EncodeToBytes(head)
	if err != nil {
		return fmt.Errorf("encode username directory head: %w", err)
	}
	if d.db != nil {
		if err := d.db.Put(usernameHeadKey, encoded); err != nil {
			return fmt.Errorf("persist username directory head: %w", err)
		}
	}
	d.head = head
	return nil
}

func (d *UsernameDirectory) ChainHead() UsernameChainHead {
	if d == nil {
		return UsernameChainHead{}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.head
}

func (d *UsernameDirectory) Count() int {
	if d == nil {
		return 0
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.records)
}

// Names returns live canonical names in lexical order for clients constructing
// cover lookups. It does not reveal which entry a caller intends to resolve.
func (d *UsernameDirectory) Names(now time.Time) []string {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	names := make([]string, 0, len(d.records))
	for name, record := range d.records {
		if VerifyUsernameBinding(record, d.chainID, now) == nil {
			names = append(names, name)
		}
	}
	d.mu.RUnlock()
	sort.Strings(names)
	return names
}

// Records returns verified live bindings in lexical order for directory
// replication. The returned slice owns its entries and cannot mutate storage.
func (d *UsernameDirectory) Records(now time.Time) []UsernameBinding {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	records := make([]UsernameBinding, 0, len(d.records))
	for _, record := range d.records {
		if VerifyUsernameBinding(record, d.chainID, now) == nil {
			record.Signature = append([]byte(nil), record.Signature...)
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Username < records[j].Username })
	return records
}

func (d *UsernameDirectory) Resolve(handle string, now time.Time) (UsernameBinding, error) {
	if d == nil {
		return UsernameBinding{}, errors.New("username directory is unavailable")
	}
	name, err := ParseUsernameHandle(handle, d.chainID)
	if err != nil {
		return UsernameBinding{}, err
	}
	d.mu.RLock()
	record, ok := d.records[name]
	d.mu.RUnlock()
	if !ok {
		return UsernameBinding{}, errors.New("username was not found")
	}
	if err := VerifyUsernameBinding(record, d.chainID, now); err != nil {
		return UsernameBinding{}, err
	}
	return record, nil
}

// LookupBatch resolves a fixed-size set. It returns the exact count and order
// on success; it refuses duplicates, malformed names, absent entries, and
// expired records without returning partial data.
func (d *UsernameDirectory) LookupBatch(names []string, now time.Time) ([]UsernameBinding, error) {
	if d == nil || len(names) != UsernameLookupBatchLen {
		return nil, errors.New("private username lookup requires exactly 16 entries")
	}
	canonical := make([]string, len(names))
	seen := make(map[string]struct{}, len(names))
	for i, input := range names {
		name, err := NormalizeUsername(input)
		if err != nil || name != input {
			return nil, errors.New("private username lookup contains a malformed name")
		}
		if _, exists := seen[name]; exists {
			return nil, errors.New("private username lookup contains duplicate names")
		}
		seen[name] = struct{}{}
		canonical[i] = name
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]UsernameBinding, len(canonical))
	for i, name := range canonical {
		record, ok := d.records[name]
		if !ok || VerifyUsernameBinding(record, d.chainID, now) != nil {
			return nil, errors.New("one or more username lookup entries are unavailable")
		}
		result[i] = record
	}
	return result, nil
}

// LookupBatch creates a fixed-size query set. It hides the target from a
// resolver only if all decoys were obtained independently; callers must fail
// closed if fewer than UsernameLookupBatchLen-1 trusted decoys are available.
func LookupBatch(target string, liveDecoys []UsernameBinding, chainID uint64, now time.Time) ([]string, error) {
	targetName, err := ParseUsernameHandle(target, chainID)
	if err != nil {
		return nil, err
	}
	if len(liveDecoys) < UsernameLookupBatchLen-1 {
		return nil, fmt.Errorf("private lookup requires %d verified cover names", UsernameLookupBatchLen-1)
	}
	if len(liveDecoys) > UsernameDirectoryLimit {
		return nil, errors.New("private lookup cover set exceeds the directory limit")
	}
	seen := map[string]struct{}{targetName: {}}
	names := make([]string, 0, len(liveDecoys))
	for _, decoy := range liveDecoys {
		if err := VerifyUsernameBinding(decoy, chainID, now); err != nil {
			continue
		}
		if _, duplicate := seen[decoy.Username]; duplicate {
			continue
		}
		seen[decoy.Username] = struct{}{}
		names = append(names, decoy.Username)
	}
	if len(names) < UsernameLookupBatchLen-1 {
		return nil, fmt.Errorf("private lookup has only %d valid cover names; need %d", len(names), UsernameLookupBatchLen-1)
	}
	// Uniformly sample cover records and shuffle the final batch. No field marks
	// which requested name is the target.
	selected := make([]string, UsernameLookupBatchLen)
	selected[0] = targetName
	for i := 0; i < UsernameLookupBatchLen-1; i++ {
		pick, err := rand.Int(rand.Reader, big.NewInt(int64(len(names)-i)))
		if err != nil {
			return nil, fmt.Errorf("select private lookup covers: %w", err)
		}
		index := int(pick.Int64())
		selected[i+1] = names[index]
		names[index], names[len(names)-1-i] = names[len(names)-1-i], names[index]
	}
	for i := len(selected) - 1; i > 0; i-- {
		pick, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return nil, fmt.Errorf("shuffle private lookup covers: %w", err)
		}
		j := int(pick.Int64())
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected, nil
}
