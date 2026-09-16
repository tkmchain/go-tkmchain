// Package shield3relay implements a shared operator without custody of payer
// funds or access to payer viewing/spending keys.
package shield3relay

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/gofrs/flock"
)

type Service struct {
	rpc      shield3wallet.RPC
	seed     []byte
	identity *shield3wallet.Identity
	dir      string
	lock     *flock.Flock
	mu       sync.Mutex
}
type record struct {
	RequestID   string        `json:"requestId"`
	DraftHash   common.Hash   `json:"draftHash"`
	Transaction hexutil.Bytes `json:"transaction"`
}

func New(rpc shield3wallet.RPC, seed []byte, identity *shield3wallet.Identity, dir string) (*Service, error) {
	if rpc == nil || identity == nil || dir == "" {
		return nil, errors.New("relay requires a node, stamped identity and persistent state directory")
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(key))
	if err != nil || address != identity.Address {
		return nil, errors.New("relay seed does not match operator")
	}
	dir = filepath.Join(dir, fmt.Sprintf("%d-%s", identity.ChainID, identity.Address.Hex()))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(dir, "operator.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, errors.New("another relay is using this operator state")
	}
	return &Service{rpc: rpc, seed: common.CopyBytes(seed), identity: identity, dir: dir, lock: lock}, nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.seed)
	return s.lock.Unlock()
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	fail := func(code int, message string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	if r.Method != http.MethodPost || r.Header.Get("Origin") != "" || r.Header.Get("Content-Type") != "application/json" {
		fail(403, "relay accepts wallet JSON requests only")
		return
	}
	if r.URL.Path != "/offer" && r.URL.Path != "/submit" {
		fail(404, "unknown relay operation")
		return
	}
	var request struct {
		RequestID   string        `json:"requestId"`
		Transaction hexutil.Bytes `json:"transaction,omitempty"`
	}
	limit := int64(1024)
	if r.URL.Path == "/submit" {
		limit = 20 << 20
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		fail(400, "invalid relay request")
		return
	}
	if len(request.RequestID) < 16 || len(request.RequestID) > 128 {
		fail(400, "stable opaque request ID required")
		return
	}

	// A single operator nonce is serialized; separate operators can scale out.
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.URL.Path == "/offer" {
		if len(request.Transaction) != 0 {
			fail(400, "fee offer does not accept a transaction")
			return
		}
		offer, err := s.offer(r.Context(), request.RequestID)
		if err != nil {
			fail(503, "operator busy or quote unavailable; retry later and check node activation and stamp")
			return
		}
		json.NewEncoder(w).Encode(offer)
		return
	}
	response, err := s.submit(r.Context(), request.RequestID, request.Transaction)
	if err != nil {
		fail(400, "relay payment rejected; check its expiry, operator nonce and canonical status")
		return
	}
	json.NewEncoder(w).Encode(response)
}
func (s *Service) submit(ctx context.Context, requestID string, raw []byte) (shield3wallet.RelayResponse, error) {
	if uint64(len(raw)) > core.ShieldedV3MaxTxSize {
		return shield3wallet.RelayResponse{}, errors.New("relay packet exceeds limit")
	}
	var draft types.Transaction
	if err := draft.UnmarshalBinary(raw); err != nil {
		return shield3wallet.RelayResponse{}, err
	}
	indexPath := s.requestPath(requestID)
	indexData, indexErr := readBounded(indexPath, 4096)
	if indexErr == nil {
		var index requestIndex
		if json.Unmarshal(indexData, &index) != nil || index.RequestID != requestID || index.Nonce != draft.Nonce() || index.DraftHash != draft.Hash() {
			return shield3wallet.RelayResponse{}, errors.New("request ID already authorizes a different payment")
		}
	} else if !errors.Is(indexErr, os.ErrNotExist) {
		return shield3wallet.RelayResponse{}, indexErr
	}

	path := filepath.Join(s.dir, fmt.Sprintf("nonce-%d.json", draft.Nonce()))
	var saved record
	data, err := readBounded(path, int64(core.ShieldedV3MaxTxSize)*2)
	if err == nil {
		if uint64(len(data)) > core.ShieldedV3MaxTxSize*2 || json.Unmarshal(data, &saved) != nil || saved.DraftHash != draft.Hash() || saved.RequestID != requestID {
			return shield3wallet.RelayResponse{}, errors.New("operator nonce already reserved for another payment")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		lease, err := s.readLease()
		if err != nil || lease == nil || lease.RequestID != requestID || draft.Nonce() != lease.Offer.Nonce || draft.GasFeeCap().Cmp((*big.Int)(lease.Offer.GasFeeCap)) != 0 || draft.GasTipCap().Cmp((*big.Int)(lease.Offer.GasTipCap)) != 0 {
			return shield3wallet.RelayResponse{}, errors.New("payment does not match its operator fee lease")
		}

		envelope, found, envelopeErr := core.DecodeShieldedV3Transaction(draft.Data())
		if envelopeErr != nil || !found || envelope.ValidUntil != lease.Offer.ValidUntil {
			return shield3wallet.RelayResponse{}, errors.New("payment expiry does not match fee lease")
		}
		if _, err := shield3wallet.ReviewRelayOffer(ctx, s.rpc, &lease.Offer, s.identity.ChainID); err != nil {
			return shield3wallet.RelayResponse{}, err
		}

		unsigned, err := shield3wallet.BuildRelaySubmission(ctx, s.rpc, s.seed, s.identity, raw)
		if err != nil {
			return shield3wallet.RelayResponse{}, err
		}
		key, err := pqcrypto.NewMLDSA87FromSeed(s.seed)
		if err != nil {
			return shield3wallet.RelayResponse{}, err
		}
		signed, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(unsigned.ChainId()), key)
		if err != nil {
			return shield3wallet.RelayResponse{}, err
		}
		saved.DraftHash = draft.Hash()
		saved.RequestID = requestID
		saved.Transaction, err = signed.MarshalBinary()
		if err != nil {
			return shield3wallet.RelayResponse{}, err
		}
		if err = s.persist(path, saved); err != nil {
			return shield3wallet.RelayResponse{}, err
		}
	} else {
		return shield3wallet.RelayResponse{}, err
	}
	hash, err := shield3wallet.MatchSignedRelay(raw, saved.Transaction)
	if err != nil {
		return shield3wallet.RelayResponse{}, err
	}
	// The nonce reservation must be durable first. Persist the global request
	// index before broadcast, so one ID cannot sign at another nonce after a
	// restart or after the daemon's pending nonce advances.
	if errors.Is(indexErr, os.ErrNotExist) {
		if err := s.persist(indexPath, requestIndex{requestID, draft.Nonce(), draft.Hash()}); err != nil {
			return shield3wallet.RelayResponse{}, err
		}
	}

	response := shield3wallet.RelayResponse{Transaction: saved.Transaction, TransactionHash: hash, Status: "unconfirmed"}
	// The node serves raw transactions from its canonical chain or transaction
	// pool. An exact match means this payment is already known: do not turn a
	// successful retry into uncertainty by rebroadcasting a mined nonce. If a
	// reorg removes it or lookup fails, retry only the same durable signed bytes.
	var known hexutil.Bytes
	if err := s.rpc.CallContext(ctx, &known, "eth_getRawTransactionByHash", hash); err == nil && bytes.Equal(known, saved.Transaction) {
		return response, nil
	}
	var returned common.Hash
	// Every retry uses the exact durable signed bytes, even after a lost reply or
	// restart. Never re-sign a different payment at a reserved operator nonce.
	if err := s.rpc.CallContext(ctx, &returned, "eth_sendRawTransaction", saved.Transaction); err != nil || returned != hash {
		response.SubmissionUncertain = true
	}
	return response, nil
}
func (s *Service) persist(path string, saved any) error {
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dir, ".relay-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		dir, err := os.Open(s.dir)
		if err != nil {
			return err
		}
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}

// One opaque quote lease per operator avoids issuing the same nonce to several
// wallets that are concurrently proving. Lost replies can retry the same ID.
type quoteLease struct {
	RequestID string                   `json:"requestId"`
	Offer     shield3wallet.RelayOffer `json:"offer"`
}

func (s *Service) readLease() (*quoteLease, error) {
	data, err := readBounded(filepath.Join(s.dir, "quote.json"), 65536)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lease quoteLease
	if len(data) > 65536 || json.Unmarshal(data, &lease) != nil || lease.Offer.GasFeeCap == nil || lease.Offer.GasTipCap == nil {
		return nil, errors.New("invalid saved operator quote")
	}
	return &lease, nil
}
func (s *Service) offer(ctx context.Context, requestID string) (shield3wallet.RelayOffer, error) {
	if _, err := readBounded(s.requestPath(requestID), 4096); err == nil {
		return shield3wallet.RelayOffer{}, errors.New("request already signed; retry its saved transaction")
	} else if !errors.Is(err, os.ErrNotExist) {
		return shield3wallet.RelayOffer{}, err
	}

	candidate, err := shield3wallet.BuildRelayOffer(ctx, s.rpc, s.seed, s.identity)
	if err != nil {
		return candidate, err
	}
	lease, err := s.readLease()
	if err != nil {
		return candidate, err
	}
	if lease != nil {
		now := candidate.ValidUntil - core.AntarticalStampSponsorshipLifetime
		if lease.Offer.Nonce == candidate.Nonce && lease.Offer.ValidUntil > now {
			if _, err := shield3wallet.ReviewRelayOffer(ctx, s.rpc, &lease.Offer, s.identity.ChainID); err != nil {
				return candidate, err
			}
			if lease.RequestID != requestID {
				return candidate, errors.New("operator is busy proving another payment")
			}
			return lease.Offer, nil
		}
	}
	if _, err := os.Stat(filepath.Join(s.dir, fmt.Sprintf("nonce-%d.json", candidate.Nonce))); err == nil {
		return candidate, errors.New("operator must broadcast its already signed payment before accepting a new quote")
	} else if !errors.Is(err, os.ErrNotExist) {
		return candidate, err
	}
	quotePath := filepath.Join(s.dir, "quote.json")
	if err := os.Remove(quotePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return candidate, err
	}
	if err := s.persist(quotePath, quoteLease{requestID, candidate}); err != nil {
		return candidate, err
	}
	return candidate, nil
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("saved relay state exceeds limit")
	}
	return data, nil
}

type requestIndex struct {
	RequestID string      `json:"requestId"`
	Nonce     uint64      `json:"nonce"`
	DraftHash common.Hash `json:"draftHash"`
}

func (s *Service) requestPath(requestID string) string {
	digest := sha512.Sum512([]byte(requestID))
	return filepath.Join(s.dir, fmt.Sprintf("request-%x.json", digest))
}
