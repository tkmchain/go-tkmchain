package eth

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	tkmGovernanceStateKey = []byte("tkm-governance-disclosures-v1")
	tkmGovDomain          = []byte("TKMCHAIN_GOV_DISCLOSURE_V1")
	tkmGovAnchorPrefix    = []byte("TKMGOV_DISCLOSURE_V1")
)

type GovernanceAPI struct {
	service *GovernanceService
}

type GovernanceService struct {
	lock     sync.RWMutex
	eth      *Ethereum
	mainKing common.Address
	chainID  *big.Int
	db       ethdb.KeyValueStore
	govDir   string
	nextID   uint64
	records  map[uint64]GovernanceDisclosure
	byHash   map[common.Hash]uint64
}

type governanceSnapshot struct {
	NextID  uint64
	Records map[uint64]GovernanceDisclosure
}

type GovernanceDisclosure struct {
	ID             hexutil.Uint64 `json:"id"`
	Kind           string         `json:"kind"`
	Title          string         `json:"title"`
	Version        hexutil.Uint64 `json:"version"`
	ContentHash    common.Hash    `json:"contentHash"`
	URI            string         `json:"uri"`
	PreviousHash   common.Hash    `json:"previousHash"`
	Timestamp      hexutil.Uint64 `json:"timestamp"`
	DisclosureHash common.Hash    `json:"disclosureHash"`
	AnchorTx       common.Hash    `json:"anchorTx"`
	MainKing       common.Address `json:"mainKing"`
	Signature      hexutil.Bytes  `json:"signature"`
	CreatedAt      hexutil.Uint64 `json:"createdAt"`
}

func NewGovernanceAPI(e *Ethereum) *GovernanceAPI {
	return &GovernanceAPI{service: e.governanceService()}
}

func NewGovernanceService(e *Ethereum, mainKing common.Address, chainID *big.Int, db ethdb.KeyValueStore, govDir string) *GovernanceService {
	if chainID == nil {
		chainID = params.RandomXChainConfig.ChainID
		if chainID == nil {
			chainID = big.NewInt(8979)
		}
	}
	svc := &GovernanceService{
		eth:      e,
		mainKing: mainKing,
		chainID:  new(big.Int).Set(chainID),
		db:       db,
		govDir:   govDir,
		records:  make(map[uint64]GovernanceDisclosure),
		byHash:   make(map[common.Hash]uint64),
	}
	if err := svc.load(); err != nil {
		log.Warn("Failed to load TKM governance disclosure state", "err", err)
	}
	return svc
}

func (s *Ethereum) governanceService() *GovernanceService {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.governanceSvc == nil {
		chainID := params.RandomXChainConfig.ChainID
		if s.blockchain != nil && s.blockchain.Config() != nil && s.blockchain.Config().ChainID != nil {
			chainID = s.blockchain.Config().ChainID
		}
		s.governanceSvc = NewGovernanceService(s, s.GetMainKingAddress(), chainID, s.chainDb, s.governanceDir)
	}
	return s.governanceSvc
}

func (api *GovernanceAPI) DisclosureHash(kind string, title string, version hexutil.Uint64, contentHash common.Hash, uri string, previousHash common.Hash, timestamp hexutil.Uint64) common.Hash {
	return api.service.disclosureHash(kind, title, uint64(version), contentHash, uri, previousHash, uint64(timestamp))
}

func (api *GovernanceAPI) PublishDisclosure(kind string, title string, version hexutil.Uint64, contentHash common.Hash, uri string, previousHash common.Hash, timestamp hexutil.Uint64, anchorTx common.Hash, signature hexutil.Bytes) (GovernanceDisclosure, error) {
	return api.service.PublishDisclosure(kind, title, uint64(version), contentHash, uri, previousHash, uint64(timestamp), anchorTx, []byte(signature))
}

func (api *GovernanceAPI) GetDisclosure(id hexutil.Uint64) (GovernanceDisclosure, error) {
	return api.service.GetDisclosure(uint64(id))
}

func (api *GovernanceAPI) ListDisclosures(kind string, from hexutil.Uint64, limit hexutil.Uint64) []GovernanceDisclosure {
	return api.service.ListDisclosures(kind, uint64(from), uint64(limit))
}

func (api *GovernanceAPI) LatestDisclosure(kind string) (GovernanceDisclosure, error) {
	return api.service.LatestDisclosure(kind)
}

func (api *GovernanceAPI) VerifyDisclosure(id hexutil.Uint64) (bool, error) {
	return api.service.VerifyDisclosure(uint64(id))
}

func (svc *GovernanceService) PublishDisclosure(kind string, title string, version uint64, contentHash common.Hash, uri string, previousHash common.Hash, timestamp uint64, anchorTx common.Hash, signature []byte) (GovernanceDisclosure, error) {
	if svc == nil {
		return GovernanceDisclosure{}, errors.New("governance disclosure service is unavailable")
	}
	if kind == "" {
		return GovernanceDisclosure{}, errors.New("disclosure kind is required")
	}
	if title == "" {
		return GovernanceDisclosure{}, errors.New("disclosure title is required")
	}
	if version == 0 {
		return GovernanceDisclosure{}, errors.New("disclosure version must be greater than zero")
	}
	if contentHash == (common.Hash{}) {
		return GovernanceDisclosure{}, errors.New("content hash is required")
	}
	if timestamp == 0 {
		timestamp = uint64(time.Now().Unix())
	}
	digest := svc.disclosureHash(kind, title, version, contentHash, uri, previousHash, timestamp)
	if err := svc.verifyMainKingSignature(digest, signature); err != nil {
		return GovernanceDisclosure{}, err
	}
	if err := svc.validateAnchorTx(anchorTx, digest); err != nil {
		return GovernanceDisclosure{}, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if _, exists := svc.byHash[digest]; exists {
		return GovernanceDisclosure{}, errors.New("governance disclosure hash is already published")
	}
	if previousHash != (common.Hash{}) {
		if _, exists := svc.byHash[previousHash]; !exists {
			return GovernanceDisclosure{}, errors.New("previous governance disclosure hash is unknown")
		}
	}
	svc.nextID++
	record := GovernanceDisclosure{
		ID:             hexutil.Uint64(svc.nextID),
		Kind:           kind,
		Title:          title,
		Version:        hexutil.Uint64(version),
		ContentHash:    contentHash,
		URI:            uri,
		PreviousHash:   previousHash,
		Timestamp:      hexutil.Uint64(timestamp),
		DisclosureHash: digest,
		AnchorTx:       anchorTx,
		MainKing:       svc.mainKing,
		Signature:      append([]byte(nil), signature...),
		CreatedAt:      hexutil.Uint64(now),
	}
	svc.records[svc.nextID] = record
	svc.byHash[digest] = svc.nextID
	if err := svc.saveLocked(); err != nil {
		return GovernanceDisclosure{}, err
	}
	return record, nil
}

func (svc *GovernanceService) GetDisclosure(id uint64) (GovernanceDisclosure, error) {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	record, ok := svc.records[id]
	if !ok {
		return GovernanceDisclosure{}, errors.New("governance disclosure not found")
	}
	return record, nil
}

func (svc *GovernanceService) ListDisclosures(kind string, from uint64, limit uint64) []GovernanceDisclosure {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	if limit == 0 || limit > 500 {
		limit = 100
	}
	ids := make([]uint64, 0, len(svc.records))
	for id := range svc.records {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	out := make([]GovernanceDisclosure, 0, limit)
	for _, id := range ids {
		if from != 0 && id >= from {
			continue
		}
		record := svc.records[id]
		if kind != "" && record.Kind != kind {
			continue
		}
		out = append(out, record)
		if uint64(len(out)) >= limit {
			break
		}
	}
	return out
}

func (svc *GovernanceService) LatestDisclosure(kind string) (GovernanceDisclosure, error) {
	records := svc.ListDisclosures(kind, 0, 1)
	if len(records) == 0 {
		return GovernanceDisclosure{}, errors.New("governance disclosure not found")
	}
	return records[0], nil
}

func (svc *GovernanceService) VerifyDisclosure(id uint64) (bool, error) {
	record, err := svc.GetDisclosure(id)
	if err != nil {
		return false, err
	}
	digest := svc.disclosureHash(record.Kind, record.Title, uint64(record.Version), record.ContentHash, record.URI, record.PreviousHash, uint64(record.Timestamp))
	if digest != record.DisclosureHash {
		return false, errors.New("governance disclosure hash mismatch")
	}
	if err := svc.verifyMainKingSignature(digest, []byte(record.Signature)); err != nil {
		return false, err
	}
	if err := svc.validateAnchorTx(record.AnchorTx, digest); err != nil {
		return false, err
	}
	return true, nil
}

func (svc *GovernanceService) disclosureHash(kind string, title string, version uint64, contentHash common.Hash, uri string, previousHash common.Hash, timestamp uint64) common.Hash {
	parts := [][]byte{
		tkmGovDomain,
		svc.chainID.Bytes(),
		[]byte(kind),
		[]byte(title),
		tkmGovUint64Bytes(version),
		contentHash.Bytes(),
		[]byte(uri),
		previousHash.Bytes(),
		tkmGovUint64Bytes(timestamp),
	}
	return stableGovernanceHash(parts...)
}

func stableGovernanceHash(parts ...[]byte) common.Hash {
	var buf bytes.Buffer
	for _, part := range parts {
		var lenbuf [8]byte
		binary.BigEndian.PutUint64(lenbuf[:], uint64(len(part)))
		buf.Write(lenbuf[:])
		buf.Write(part)
	}
	return crypto.Keccak256Hash(buf.Bytes())
}

func tkmGovUint64Bytes(v uint64) []byte {
	var out [8]byte
	binary.BigEndian.PutUint64(out[:], v)
	return out[:]
}

func (svc *GovernanceService) verifyMainKingSignature(digest common.Hash, signature []byte) error {
	if svc.mainKing == (common.Address{}) {
		return errors.New("main king address is not configured")
	}
	if len(signature) != crypto.SignatureLength {
		return fmt.Errorf("governance disclosure signature must be %d bytes", crypto.SignatureLength)
	}
	sig := append([]byte(nil), signature...)
	if sig[crypto.RecoveryIDOffset] >= 27 {
		sig[crypto.RecoveryIDOffset] -= 27
	}
	if sig[crypto.RecoveryIDOffset] > 1 {
		return fmt.Errorf("invalid governance disclosure signature recovery id %d", sig[crypto.RecoveryIDOffset])
	}
	if signer, err := tkmGovRecoverSigner(digest, sig); err == nil && signer == svc.mainKing {
		return nil
	}
	prefixed := crypto.Keccak256Hash([]byte("\x19Ethereum Signed Message:\n32"), digest.Bytes())
	signer, err := tkmGovRecoverSigner(prefixed, sig)
	if err != nil {
		return fmt.Errorf("invalid governance disclosure signature: %w", err)
	}
	if signer != svc.mainKing {
		return fmt.Errorf("governance disclosure signed by %s, want main king %s", signer.Hex(), svc.mainKing.Hex())
	}
	return nil
}

func tkmGovRecoverSigner(digest common.Hash, signature []byte) (common.Address, error) {
	pub, err := crypto.SigToPub(digest.Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pub), nil
}

func (svc *GovernanceService) validateAnchorTx(anchorTx common.Hash, digest common.Hash) error {
	if anchorTx == (common.Hash{}) || svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	_, tx := svc.eth.blockchain.GetCanonicalTransaction(anchorTx)
	if tx == nil {
		return errors.New("governance anchor transaction is not canonical or indexed")
	}
	to := tx.To()
	if to == nil || *to != svc.mainKing {
		return errors.New("governance anchor transaction must be sent to main king")
	}
	signer, err := types.Sender(types.LatestSigner(svc.eth.blockchain.Config()), tx)
	if err != nil {
		return fmt.Errorf("governance anchor transaction sender unavailable: %w", err)
	}
	if signer != svc.mainKing {
		return fmt.Errorf("governance anchor transaction sent by %s, want main king %s", signer.Hex(), svc.mainKing.Hex())
	}
	data := tx.Data()
	if !bytes.HasPrefix(data, tkmGovAnchorPrefix) || !bytes.Contains(data, digest.Bytes()) {
		return errors.New("governance anchor transaction data does not contain disclosure hash")
	}
	return nil
}

func (svc *GovernanceService) load() error {
	if svc.db == nil {
		return nil
	}
	data, err := svc.db.Get(tkmGovernanceStateKey)
	if err != nil || len(data) == 0 {
		return nil
	}
	var snap governanceSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.nextID = snap.NextID
	if snap.Records != nil {
		svc.records = snap.Records
	}
	svc.rebuildHashIndexLocked()
	return nil
}

func (svc *GovernanceService) saveLocked() error {
	if svc.db == nil {
		return svc.writeFileSnapshotLocked(&governanceSnapshot{NextID: svc.nextID, Records: svc.records})
	}
	snap := governanceSnapshot{NextID: svc.nextID, Records: svc.records}
	data, err := json.Marshal(&snap)
	if err != nil {
		return err
	}
	if err := svc.db.Put(tkmGovernanceStateKey, data); err != nil {
		return err
	}
	if err := svc.db.SyncKeyValue(); err != nil {
		return err
	}
	return svc.writeFileSnapshotLocked(&snap)
}

func (svc *GovernanceService) writeFileSnapshotLocked(snap *governanceSnapshot) error {
	if svc.govDir == "" {
		return nil
	}
	if err := os.MkdirAll(svc.govDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(svc.govDir, "state.json.tmp")
	path := filepath.Join(svc.govDir, "state.json")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (svc *GovernanceService) rebuildHashIndexLocked() {
	svc.byHash = make(map[common.Hash]uint64, len(svc.records))
	for id, record := range svc.records {
		if record.DisclosureHash != (common.Hash{}) {
			svc.byHash[record.DisclosureHash] = id
		}
		if id > svc.nextID {
			svc.nextID = id
		}
	}
}
