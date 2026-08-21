package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/consensys/gnark/backend/groth16"
	bn254groth16 "github.com/consensys/gnark/backend/groth16/bn254"
	cs "github.com/consensys/gnark/constraint/bn254"
	"github.com/consensys/gnark/frontend"
	r1csbuilder "github.com/consensys/gnark/frontend/cs/r1cs"
)

const (
	defaultListen            = "127.0.0.1:8787"
	defaultGasLimit          = uint64(3_000_000)
	defaultReceiptTimeout    = 20 * time.Second
	maxRequestBodyBytes      = 1 << 20
	outputEncryptedBytesSize = 96
)

type Config struct {
	Listen               string `json:"listen"`
	AllowedOrigin        string `json:"allowedOrigin"`
	BearerToken          string `json:"bearerToken"`
	NodeRPC              string `json:"nodeRPC"`
	KeystoreDir          string `json:"keystoreDir"`
	SignerAddress        string `json:"signerAddress"`
	SignerPassphrase     string `json:"signerPassphrase"`
	SignerPassphraseFile string `json:"signerPassphraseFile"`
	SignMode             string `json:"signMode"`
	ProvingKeyPath       string `json:"provingKeyPath"`
	ProvingKeyV2Path     string `json:"provingKeyV2Path"`
	NotesPath            string `json:"notesPath"`
	RequestsPath         string `json:"requestsPath"`
	GasLimit             uint64 `json:"gasLimit"`
	SubmitSync           bool   `json:"submitSync"`
	ReceiptTimeoutMs     int64  `json:"receiptTimeoutMs"`
}

type PayoutRequest struct {
	RequestID             string    `json:"requestId"`
	PoolWallet            string    `json:"poolWallet"`
	To                    string    `json:"to"`
	AmountAntd            float64   `json:"amountAntd"`
	AmountWei             string    `json:"amountWei"`
	PayoutTxType          string    `json:"payoutTxType,omitempty"`
	Nonce                 string    `json:"nonce,omitempty"`
	GasPriceWei           string    `json:"gasPriceWei,omitempty"`
	RecipientViewKey      string    `json:"recipientViewKey,omitempty"`
	ChangeViewKey         string    `json:"changeViewKey,omitempty"`
	PrivacyCommitmentTime uint64    `json:"privacyCommitmentTime"`
	QuantumResistantTime  uint64    `json:"quantumResistantTime"`
	CreatedAt             time.Time `json:"createdAt"`
}

type PayoutResponse struct {
	TxHash string `json:"txHash,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type DepositRequest struct {
	RequestID        string    `json:"requestId"`
	AmountAntd       float64   `json:"amountAntd"`
	AmountWei        string    `json:"amountWei"`
	AssetID          string    `json:"assetId,omitempty"`
	OwnerSecret      string    `json:"ownerSecret,omitempty"`
	Nonce            string    `json:"nonce,omitempty"`
	GasPriceWei      string    `json:"gasPriceWei,omitempty"`
	From             string    `json:"from,omitempty"`
	To               string    `json:"to,omitempty"`
	RecipientViewKey string    `json:"recipientViewKey,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

type DepositResponse struct {
	TxHash     string        `json:"txHash,omitempty"`
	Status     string        `json:"status,omitempty"`
	NoteID     string        `json:"noteId,omitempty"`
	Commitment string        `json:"commitment,omitempty"`
	AmountWei  string        `json:"amountWei,omitempty"`
	AmountAntd float64       `json:"amountAntd,omitempty"`
	Note       *ShieldedNote `json:"note,omitempty"`
	Error      string        `json:"error,omitempty"`
}

type NoteStore struct {
	Notes []ShieldedNote `json:"notes"`
}

type ShieldedNote struct {
	Version         uint64   `json:"version,omitempty"`
	ID              string   `json:"id"`
	Commitment      string   `json:"commitment,omitempty"`
	Nullifier       string   `json:"nullifier,omitempty"`
	OwnerSecret     string   `json:"ownerSecret"`
	NoteRandomness  string   `json:"noteRandomness"`
	NoteValueWei    string   `json:"noteValueWei"`
	AssetID         string   `json:"assetId"`
	MerklePath      []string `json:"merklePath"`
	MerklePathIndex []string `json:"merklePathIndex"`
	MerkleRoot      string   `json:"merkleRoot,omitempty"`
	CreatedTxHash   string   `json:"createdTxHash,omitempty"`
	CreatedAt       string   `json:"createdAt,omitempty"`
	Source          string   `json:"source,omitempty"`
	Status          string   `json:"status,omitempty"`
	SpentRequestID  string   `json:"spentRequestId,omitempty"`
	SpentTxHash     string   `json:"spentTxHash,omitempty"`
}

type commitmentPathRPC struct {
	Commitment      common.Hash      `json:"commitment"`
	Found           bool             `json:"found"`
	Index           hexutil.Uint64   `json:"index"`
	Root            common.Hash      `json:"root"`
	MerklePath      []common.Hash    `json:"merklePath"`
	MerklePathIndex []hexutil.Uint64 `json:"merklePathIndex"`
}

type RequestDB struct {
	Requests map[string]RequestRecord `json:"requests"`
	Deposits map[string]DepositRecord `json:"deposits,omitempty"`
}

type RequestRecord struct {
	Request   PayoutRequest `json:"request"`
	Status    string        `json:"status"`
	TxHash    string        `json:"txHash,omitempty"`
	Error     string        `json:"error,omitempty"`
	NoteID    string        `json:"noteId,omitempty"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type DepositRecord struct {
	Request    DepositRequest `json:"request"`
	Status     string         `json:"status"`
	TxHash     string         `json:"txHash,omitempty"`
	Error      string         `json:"error,omitempty"`
	NoteID     string         `json:"noteId,omitempty"`
	Commitment string         `json:"commitment,omitempty"`
	AmountWei  string         `json:"amountWei,omitempty"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

type Prover struct {
	cfg        Config
	client     *ethclient.Client
	ks         *keystore.KeyStore
	pk         groth16.ProvingKey
	r1cs       *cs.R1CS
	pkV2       groth16.ProvingKey
	r1csV2     *cs.R1CS
	passphrase string
	startupErr string
	mu         sync.Mutex
	buildSlots chan struct{}
}

func main() {
	configPath := flag.String("config", "/home/mike/shielded-prover/config.json", "path to prover config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	prover, err := NewProver(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := prover.Serve(); err != nil {
		log.Fatal(err)
	}
}

func loadConfig(path string) (Config, error) {
	cfg := Config{
		Listen:           defaultListen,
		NodeRPC:          "http://127.0.0.1:8545",
		SignMode:         "pq",
		GasLimit:         defaultGasLimit,
		SubmitSync:       false,
		ReceiptTimeoutMs: int64(defaultReceiptTimeout / time.Millisecond),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	if cfg.Listen == "" {
		cfg.Listen = defaultListen
	}
	cfg.AllowedOrigin = strings.TrimSpace(cfg.AllowedOrigin)
	cfg.BearerToken = strings.TrimSpace(cfg.BearerToken)
	cfg.NodeRPC = strings.TrimSpace(cfg.NodeRPC)
	cfg.KeystoreDir = strings.TrimSpace(cfg.KeystoreDir)
	cfg.SignerAddress = normalizeAddress(cfg.SignerAddress)
	cfg.SignMode = strings.ToLower(strings.TrimSpace(cfg.SignMode))
	if cfg.SignMode == "" {
		cfg.SignMode = "pq"
	}
	cfg.ProvingKeyPath = strings.TrimSpace(cfg.ProvingKeyPath)
	cfg.ProvingKeyV2Path = strings.TrimSpace(cfg.ProvingKeyV2Path)
	if cfg.ProvingKeyV2Path == "" && cfg.ProvingKeyPath != "" {
		cfg.ProvingKeyV2Path = filepath.Join(filepath.Dir(cfg.ProvingKeyPath), "proving-v2.key")
	}
	cfg.NotesPath = strings.TrimSpace(cfg.NotesPath)
	cfg.RequestsPath = strings.TrimSpace(cfg.RequestsPath)
	if cfg.GasLimit == 0 {
		cfg.GasLimit = defaultGasLimit
	}
	if cfg.ReceiptTimeoutMs <= 0 {
		cfg.ReceiptTimeoutMs = int64(defaultReceiptTimeout / time.Millisecond)
	}
	if cfg.SignerPassphrase == "" && cfg.SignerPassphraseFile != "" {
		passphrase, err := os.ReadFile(cfg.SignerPassphraseFile)
		if err != nil {
			return cfg, err
		}
		cfg.SignerPassphrase = strings.TrimRight(string(passphrase), "\r\n")
	}
	if cfg.BearerToken == "" {
		return cfg, errors.New("bearerToken is required")
	}
	if cfg.NodeRPC == "" || cfg.ProvingKeyPath == "" || cfg.ProvingKeyV2Path == "" || cfg.NotesPath == "" || cfg.RequestsPath == "" {
		return cfg, errors.New("nodeRPC, provingKeyPath, provingKeyV2Path, notesPath, and requestsPath are required")
	}
	if cfg.SignMode != "proof-only" && (cfg.KeystoreDir == "" || cfg.SignerAddress == "") {
		return cfg, errors.New("keystoreDir and signerAddress are required unless signMode is proof-only")
	}
	return cfg, nil
}

func NewProver(cfg Config) (*Prover, error) {
	prover := &Prover{cfg: cfg, passphrase: cfg.SignerPassphrase, buildSlots: make(chan struct{}, 1)}
	client, err := ethclient.Dial(cfg.NodeRPC)
	if err != nil {
		return nil, err
	}
	prover.client = client
	r1cs, err := compileCircuit()
	if err != nil {
		return nil, err
	}
	prover.r1cs = r1cs
	pk := groth16.NewProvingKey(ecc.BN254)
	if err := readObject(cfg.ProvingKeyPath, pk); err != nil {
		prover.startupErr = err.Error()
		log.Printf("proving key not loaded yet: %v", err)
	} else {
		prover.pk = pk
	}
	r1csV2, err := compileCircuitV2()
	if err != nil {
		return nil, err
	}
	prover.r1csV2 = r1csV2
	pkV2 := groth16.NewProvingKey(ecc.BN254)
	if err := readObject(cfg.ProvingKeyV2Path, pkV2); err != nil {
		prover.startupErr = strings.TrimSpace(strings.Join([]string{prover.startupErr, "V2: " + err.Error()}, "; "))
		log.Printf("V2 proving key not loaded yet: %v", err)
	} else {
		prover.pkV2 = pkV2
	}
	if cfg.SignMode != "proof-only" {
		prover.ks = keystore.NewKeyStore(cfg.KeystoreDir, keystore.StandardScryptN, keystore.StandardScryptP)
	}
	return prover, nil
}

func (p *Prover) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", p.handleHealth)
	mux.HandleFunc("/payout", p.handlePayout)
	mux.HandleFunc("/deposit", p.handleDeposit)
	mux.HandleFunc("/build-deposit", p.handleBuildDeposit)
	mux.HandleFunc("/build-transfer", p.handleBuildTransfer)
	mux.HandleFunc("/build-withdrawal", p.handleBuildWithdrawal)
	server := &http.Server{Addr: p.cfg.Listen, Handler: p.corsHandler(mux), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("shielded payout prover listening on %s", p.cfg.Listen)
	return server.ListenAndServe()
}

func (p *Prover) corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("origin")
		if origin != "" && p.cfg.AllowedOrigin != "" && origin == p.cfg.AllowedOrigin {
			w.Header().Set("access-control-allow-origin", origin)
			w.Header().Set("access-control-allow-headers", "authorization, content-type")
			w.Header().Set("access-control-allow-methods", "GET, POST, OPTIONS")
			if r.Header.Get("access-control-request-private-network") == "true" {
				w.Header().Set("access-control-allow-private-network", "true")
			}
			w.Header().Set("vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (p *Prover) handleHealth(w http.ResponseWriter, r *http.Request) {
	noteStatus := p.noteInventoryStatus()
	status := map[string]any{
		"ok":                     p.ready(),
		"buildReady":             p.ready(),
		"withdrawalBuildReady":   p.ready() && p.pkV2 != nil && p.r1csV2 != nil,
		"payoutReady":            p.ready() && noteStatus.HasSpendableNotes,
		"listen":                 p.cfg.Listen,
		"nodeRPC":                p.cfg.NodeRPC,
		"signMode":               p.cfg.SignMode,
		"signerAddress":          p.cfg.SignerAddress,
		"provingKeyPath":         p.cfg.ProvingKeyPath,
		"provingKeyV2Path":       p.cfg.ProvingKeyV2Path,
		"notesPath":              p.cfg.NotesPath,
		"requestsPath":           p.cfg.RequestsPath,
		"hasProvingKey":          p.pk != nil,
		"hasProvingKeyV2":        p.pkV2 != nil,
		"hasRPC":                 p.client != nil,
		"hasKeystore":            p.ks != nil,
		"hasSpendableNotes":      noteStatus.HasSpendableNotes,
		"noteCount":              noteStatus.NoteCount,
		"availableNoteCount":     noteStatus.AvailableNoteCount,
		"availableNoteTotalWei":  noteStatus.AvailableNoteTotalWei,
		"availableNoteMaxWei":    noteStatus.AvailableNoteMaxWei,
		"availableNoteTotalAntd": noteStatus.AvailableNoteTotalAntd,
		"availableNoteMaxAntd":   noteStatus.AvailableNoteMaxAntd,
		"noteInventoryError":     noteStatus.Error,
		"startupError":           p.startupErr,
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

type noteInventoryStatus struct {
	HasSpendableNotes      bool
	NoteCount              int
	AvailableNoteCount     int
	AvailableNoteTotalWei  string
	AvailableNoteMaxWei    string
	AvailableNoteTotalAntd float64
	AvailableNoteMaxAntd   float64
	Error                  string
}

func (p *Prover) noteInventoryStatus() noteInventoryStatus {
	status := noteInventoryStatus{
		AvailableNoteTotalWei: "0",
		AvailableNoteMaxWei:   "0",
	}
	if p == nil || strings.TrimSpace(p.cfg.NotesPath) == "" {
		status.Error = "notes path is not configured"
		return status
	}
	store, err := readNoteStore(p.cfg.NotesPath)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.NoteCount = len(store.Notes)
	total := new(big.Int)
	maxValue := new(big.Int)
	firstWitnessError := ""
	for _, note := range store.Notes {
		if strings.TrimSpace(note.Status) != "" && strings.TrimSpace(note.Status) != "available" {
			continue
		}
		value, ok := parseDecimalBig(note.NoteValueWei)
		if !ok || value.Sign() <= 0 || value.BitLen() > 64 {
			continue
		}
		if err := validateMerkleWitness(note.MerklePath, note.MerklePathIndex); err != nil {
			if firstWitnessError == "" {
				firstWitnessError = fmt.Sprintf("note %s has invalid Merkle witness: %v", note.ID, err)
			}
			continue
		}
		status.AvailableNoteCount++
		total.Add(total, value)
		if value.Cmp(maxValue) > 0 {
			maxValue.Set(value)
		}
	}
	status.HasSpendableNotes = status.AvailableNoteCount > 0
	if !status.HasSpendableNotes && firstWitnessError != "" {
		status.Error = firstWitnessError
	}
	status.AvailableNoteTotalWei = total.String()
	status.AvailableNoteMaxWei = maxValue.String()
	status.AvailableNoteTotalAntd = weiToAntdFloat(total)
	status.AvailableNoteMaxAntd = weiToAntdFloat(maxValue)
	return status
}

func (p *Prover) handlePayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if p.cfg.SignMode == "proof-only" {
		writeJSON(w, http.StatusConflict, PayoutResponse{Error: "prover is running in proof-only mode and cannot sign or submit transactions"})
		return
	}
	var req PayoutRequest
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, PayoutResponse{Error: err.Error()})
		return
	}
	ctx, cancel := p.operationContext()
	defer cancel()
	txHash, err := p.ProcessPayout(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, PayoutResponse{Status: "waiting", Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, PayoutResponse{Status: "sent", TxHash: txHash})
}

func (p *Prover) handleDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if p.cfg.SignMode == "proof-only" {
		writeJSON(w, http.StatusConflict, DepositResponse{Error: "prover is running in proof-only mode and cannot sign or submit transactions"})
		return
	}
	var req DepositRequest
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, DepositResponse{Error: err.Error()})
		return
	}
	ctx, cancel := p.operationContext()
	defer cancel()
	resp, err := p.ProcessDeposit(ctx, req)
	if err != nil {
		resp.Status = "waiting"
		resp.Error = err.Error()
		writeJSON(w, http.StatusBadRequest, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (p *Prover) authorized(r *http.Request) bool {
	want := "Bearer " + p.cfg.BearerToken
	got := r.Header.Get("authorization")
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (p *Prover) ready() bool {
	return p != nil && p.client != nil && (p.cfg.SignMode == "proof-only" || p.ks != nil) && p.pk != nil && p.r1cs != nil && p.pkV2 != nil && p.r1csV2 != nil
}

func (p *Prover) operationContext() (context.Context, context.CancelFunc) {
	timeout := time.Duration(p.cfg.ReceiptTimeoutMs) * time.Millisecond
	if timeout < defaultReceiptTimeout {
		timeout = defaultReceiptTimeout
	}
	return context.WithTimeout(context.Background(), timeout+5*time.Minute)
}

func isAcceptedButUnminedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "was added to the transaction pool") && strings.Contains(msg, "wasn't processed")
}

func (p *Prover) ProcessPayout(ctx context.Context, req PayoutRequest) (txHash string, retErr error) {
	if !p.ready() {
		return "", errors.New("prover is not ready; check /healthz")
	}
	if err := validatePayoutRequest(req); err != nil {
		return "", err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	db, err := readRequestDB(p.cfg.RequestsPath)
	if err != nil {
		return "", err
	}
	panicNoteID := ""
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("payout request %s recovered panic: %v", req.RequestID, recovered)
			if panicNoteID == "" {
				p.recordError(db, req, "", "internal prover error before transaction construction")
			} else {
				db.Requests[req.RequestID] = RequestRecord{
					Request:   req,
					Status:    "processing",
					Error:     "internal prover error; inspect node and request state before retrying",
					NoteID:    panicNoteID,
					UpdatedAt: time.Now().UTC(),
				}
				if err := writeRequestDB(p.cfg.RequestsPath, db); err != nil {
					log.Printf("request db error after recovered panic: %v", err)
				}
			}
			txHash = ""
			retErr = errors.New("internal prover error while processing payout")
		}
	}()
	var existing RequestRecord
	replacing := false
	if rec, ok := db.Requests[req.RequestID]; ok {
		if rec.TxHash != "" {
			if !payoutReplacementRequested(req) {
				return rec.TxHash, nil
			}
			existing = rec
			replacing = true
		}
		if rec.Status == "processing" && !replacing {
			return "", errors.New("request is already processing")
		}
	}

	notes, err := readNoteStore(p.cfg.NotesPath)
	if err != nil {
		return "", err
	}
	amountWei, ok := parseBigFlexible(req.AmountWei)
	if !ok || amountWei.Sign() <= 0 || amountWei.BitLen() > 64 {
		p.recordError(db, req, "", "amountWei must be a positive uint64 value")
		return "", errors.New("amountWei must be a positive uint64 value")
	}
	noteIndex := -1
	if replacing {
		noteIndex = selectReplacementNote(notes, existing.NoteID, req.RequestID)
	} else {
		noteIndex = selectSpendableNote(notes, amountWei)
	}
	if noteIndex < 0 {
		p.recordError(db, req, "", "no spendable shielded note is available for this amount")
		return "", errors.New("no spendable shielded note is available for this amount")
	}
	note := notes.Notes[noteIndex]
	panicNoteID = note.ID
	db.Requests[req.RequestID] = RequestRecord{Request: req, Status: "processing", NoteID: note.ID, UpdatedAt: time.Now().UTC()}
	if err := writeRequestDB(p.cfg.RequestsPath, db); err != nil {
		return "", err
	}

	txHash, changeNote, err := p.buildSignSubmit(ctx, req, note, amountWei)
	if err != nil {
		p.recordError(db, req, note.ID, err.Error())
		return "", err
	}
	notes.Notes[noteIndex].Status = "spent"
	notes.Notes[noteIndex].SpentRequestID = req.RequestID
	notes.Notes[noteIndex].SpentTxHash = txHash
	if changeNote != nil {
		notes.Notes = appendOrReplaceNote(notes.Notes, *changeNote)
	}
	if err := writeNoteStore(p.cfg.NotesPath, notes); err != nil {
		return "", err
	}
	db.Requests[req.RequestID] = RequestRecord{Request: req, Status: "sent", TxHash: txHash, NoteID: note.ID, UpdatedAt: time.Now().UTC()}
	if err := writeRequestDB(p.cfg.RequestsPath, db); err != nil {
		return "", err
	}
	return txHash, nil
}

func (p *Prover) ProcessDeposit(ctx context.Context, req DepositRequest) (DepositResponse, error) {
	if !p.ready() {
		return DepositResponse{}, errors.New("prover is not ready; check /healthz")
	}
	if strings.TrimSpace(req.RequestID) == "" {
		req.RequestID = "deposit-" + hexutil.Encode(randomBytes(16))[2:]
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	amountWei, err := depositAmountWei(req)
	if err != nil {
		return DepositResponse{}, err
	}
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		assetID = "1"
	}
	ownerSecret := strings.TrimSpace(req.OwnerSecret)
	if ownerSecret == "" {
		owner := randomElement()
		ownerSecret = owner.BigInt(new(big.Int)).String()
	} else if _, ok := parseDecimalBig(ownerSecret); !ok {
		return DepositResponse{}, fmt.Errorf("ownerSecret must be a decimal BN254 field element")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	db, err := readRequestDB(p.cfg.RequestsPath)
	if err != nil {
		return DepositResponse{}, err
	}
	if rec, ok := db.Deposits[req.RequestID]; ok {
		resp := depositRecordResponse(rec)
		if rec.TxHash != "" {
			return p.refreshDepositRecord(ctx, db, req, rec, amountWei), nil
		}
		if rec.Status == "processing" {
			return resp, errors.New("deposit request is already processing")
		}
	}
	db.Deposits[req.RequestID] = DepositRecord{
		Request:   req,
		Status:    "processing",
		AmountWei: amountWei.String(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := writeRequestDB(p.cfg.RequestsPath, db); err != nil {
		return DepositResponse{}, err
	}

	txHash, note, err := p.buildSignSubmitDeposit(ctx, req, amountWei, assetID, ownerSecret)
	resp := DepositResponse{
		TxHash:     txHash,
		Status:     note.Status,
		NoteID:     note.ID,
		Commitment: note.Commitment,
		AmountWei:  amountWei.String(),
		AmountAntd: weiToAntdFloat(amountWei),
	}
	if err != nil {
		if txHash != "" && note.ID != "" {
			if notes, noteErr := readNoteStore(p.cfg.NotesPath); noteErr == nil {
				notes.Notes = appendOrReplaceNote(notes.Notes, note)
				if writeErr := writeNoteStore(p.cfg.NotesPath, notes); writeErr != nil {
					log.Printf("deposit note store error after %s: %v", txHash, writeErr)
				}
			} else {
				log.Printf("deposit note store read error after %s: %v", txHash, noteErr)
			}
			p.recordDeposit(db, req, note, amountWei, txHash, note.Status, err.Error())
		} else {
			p.recordDeposit(db, req, note, amountWei, "", "waiting", err.Error())
		}
		return resp, err
	}
	notes, err := readNoteStore(p.cfg.NotesPath)
	if err != nil {
		p.recordDeposit(db, req, note, amountWei, txHash, "pending", err.Error())
		return DepositResponse{}, err
	}
	notes.Notes = appendOrReplaceNote(notes.Notes, note)
	if err := writeNoteStore(p.cfg.NotesPath, notes); err != nil {
		p.recordDeposit(db, req, note, amountWei, txHash, "pending", err.Error())
		return DepositResponse{}, err
	}
	p.recordDeposit(db, req, note, amountWei, txHash, note.Status, "")
	return resp, nil
}

func (p *Prover) buildSignSubmitDeposit(ctx context.Context, req DepositRequest, amountWei *big.Int, assetID string, ownerSecret string) (string, ShieldedNote, error) {
	chainID, err := p.client.ChainID(ctx)
	if err != nil {
		return "", ShieldedNote{}, err
	}
	proofBlock := new(big.Int)
	signerAddr := common.HexToAddress(p.cfg.SignerAddress)
	nonce, err := p.client.PendingNonceAt(ctx, signerAddr)
	if err != nil {
		return "", ShieldedNote{}, err
	}
	nonce, err = depositNonce(req, nonce)
	if err != nil {
		return "", ShieldedNote{}, err
	}
	gasPrice, err := p.client.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(params.GWei)
	}
	gasPrice, err = depositGasPriceWei(req, gasPrice)
	if err != nil {
		return "", ShieldedNote{}, err
	}

	draft, assignment, note, commitment, err := p.buildDepositDraft(req, amountWei, assetID, ownerSecret, chainID, proofBlock)
	if err != nil {
		return "", ShieldedNote{}, err
	}
	unsigned := p.unsignedTxWithValue(chainID, nonce, gasPrice, amountWei, draft.Data)
	intentHash, err := core.ShieldedTransactionIntentHash(unsigned, draft.Envelope)
	if err != nil {
		return "", ShieldedNote{}, err
	}
	applyIntentHash(assignment, intentHash)
	binding := fieldHash(shielded.DomainBind,
		mustElement(ownerSecret),
		zeroElement(),
		fieldElementFromHash(draft.OutputRoot),
		fieldElementFromHash(draft.Envelope.BalanceCommitment),
		fieldElementFromBig(chainID),
		fieldElementFromBytes(intentHash[:16]),
		fieldElementFromBytes(intentHash[16:]),
	)
	bindingBytes := binding.Bytes()
	draft.Envelope.BindingSig = bindingBytes[:]
	assignment.BindingSigHash = binding.BigInt(new(big.Int)).String()

	proofBytes, err := p.prove(assignment)
	if err != nil {
		return "", ShieldedNote{}, err
	}
	draft.Envelope.Spends[0].Proof = proofBytes
	finalData, err := core.EncodeShieldedTransaction(draft.Envelope)
	if err != nil {
		return "", ShieldedNote{}, err
	}
	tx := p.unsignedTxWithValue(chainID, nonce, gasPrice, amountWei, finalData)
	account := accounts.Account{Address: signerAddr}
	signed, err := p.ks.SignTxWithPassphrase(account, p.passphrase, tx, chainID)
	if err != nil {
		return "", ShieldedNote{}, err
	}

	timeout := time.Duration(p.cfg.ReceiptTimeoutMs) * time.Millisecond
	receipt, err := p.client.SendTransactionSync(ctx, signed, &timeout)
	if err != nil {
		note.CreatedTxHash = signed.Hash().Hex()
		note.Status = "pending"
		return signed.Hash().Hex(), note, err
	}
	if receipt == nil {
		note.CreatedTxHash = signed.Hash().Hex()
		note.Status = "pending"
		return signed.Hash().Hex(), note, errors.New("nil receipt")
	}
	txHash := receipt.TxHash.Hex()
	note = p.finalizeNoteWitness(ctx, note, commitment, txHash)
	return txHash, note, nil
}

func (p *Prover) buildDepositDraft(req DepositRequest, amountWei *big.Int, assetID string, ownerSecret string, chainID, blockNumber *big.Int) (draftEnvelope, *shielded.SpendCircuit, ShieldedNote, common.Hash, error) {
	asset, err := parseFieldElement(assetID)
	if err != nil {
		return draftEnvelope{}, nil, ShieldedNote{}, common.Hash{}, fmt.Errorf("invalid assetId: %w", err)
	}
	owner, err := parseFieldElement(ownerSecret)
	if err != nil {
		return draftEnvelope{}, nil, ShieldedNote{}, common.Hash{}, fmt.Errorf("invalid ownerSecret: %w", err)
	}
	var viewKey []byte
	if strings.TrimSpace(req.RecipientViewKey) != "" {
		viewKey, err = parseViewPublicKey(req.RecipientViewKey)
		if err != nil {
			return draftEnvelope{}, nil, ShieldedNote{}, common.Hash{}, err
		}
	}
	recipientAddress := common.HexToAddress(p.cfg.SignerAddress)
	if strings.TrimSpace(req.From) != "" {
		if !isValidAddress(req.From) {
			return draftEnvelope{}, nil, ShieldedNote{}, common.Hash{}, fmt.Errorf("invalid from address %q", req.From)
		}
		recipientAddress = common.HexToAddress(req.From)
	}
	noteRandomness := randomElement()
	outputRecipients := [shielded.OutputSlots]fr.Element{
		owner,
		zeroElement(),
		zeroElement(),
		zeroElement(),
	}
	outputValues := [shielded.OutputSlots]fr.Element{
		fieldElementFromBig(amountWei),
		zeroElement(),
		zeroElement(),
		zeroElement(),
	}
	outputRandomness := [shielded.OutputSlots]fr.Element{
		noteRandomness,
		randomElement(),
		randomElement(),
		randomElement(),
	}
	var outputCommitments [shielded.OutputSlots]fr.Element
	var outputs []core.ShieldedOutput
	for i := 0; i < shielded.OutputSlots; i++ {
		outputCommitments[i] = fieldHash(shielded.DomainNote, outputRecipients[i], asset, outputValues[i], outputRandomness[i])
		commitment := hashFromField(outputCommitments[i])
		var output core.ShieldedOutput
		if i == 0 && len(viewKey) != 0 {
			nullifier := hashFromField(fieldHash(shielded.DomainNull, owner, outputRandomness[i]))
			output, err = encryptShieldedNote(commitment, noteOpening(
				recipientAddress,
				owner.BigInt(new(big.Int)),
				asset.BigInt(new(big.Int)),
				outputValues[i].BigInt(new(big.Int)),
				outputRandomness[i].BigInt(new(big.Int)),
				commitment,
				nullifier,
			), viewKey)
		} else if i == 0 {
			output = legacyMetadataOutput(commitment, req.RequestID, i, "deposit")
		} else {
			output, err = decoyShieldedOutput(commitment)
		}
		if err != nil {
			return draftEnvelope{}, nil, ShieldedNote{}, common.Hash{}, err
		}
		outputs = append(outputs, output)
	}
	outputRoot := hashFromField(fieldHash(shielded.DomainOutput, outputCommitments[:]...))
	noteValue := new(big.Int)
	totalOutput := new(big.Int).Set(amountWei)
	balanceCommitment := fieldHash(shielded.DomainBal, fieldElementFromBig(noteValue), fieldElementFromBig(totalOutput), fieldElementFromBig(amountWei), asset, fieldElementFromHash(outputRoot))
	envelope := &core.ShieldedTransaction{
		Version: 1,
		Spends: []core.ShieldedSpend{{
			EncryptedSpendData: []byte(req.RequestID),
		}},
		Outputs:           outputs,
		BalanceCommitment: hashFromField(balanceCommitment),
		BindingSig:        make([]byte, common.HashLength),
	}
	data, err := core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return draftEnvelope{}, nil, ShieldedNote{}, common.Hash{}, err
	}

	zeroPath := make([]string, shielded.MerkleDepth)
	zeroPathIndex := make([]string, shielded.MerkleDepth)
	for i := 0; i < shielded.MerkleDepth; i++ {
		zeroPath[i] = "0"
		zeroPathIndex[i] = "0"
	}
	outputRootElement := fieldElementFromHash(outputRoot)
	assignment := &shielded.SpendCircuit{
		ChainID:           chainID.String(),
		BlockNumber:       blockNumber.String(),
		TxHashHi:          "0",
		TxHashLo:          "0",
		SpendIndex:        "0",
		Nullifier:         "0",
		Anchor:            "0",
		BalanceCommitment: balanceCommitment.BigInt(new(big.Int)).String(),
		PublicValue:       amountWei.String(),
		OutputRoot:        outputRootElement.BigInt(new(big.Int)).String(),
		BindingSigHash:    new(big.Int).SetBytes(envelope.BindingSig).String(),
		OwnerSecret:       ownerSecret,
		NoteRandomness:    noteRandomness.BigInt(new(big.Int)).String(),
		NoteValue:         "0",
		AssetID:           assetID,
	}
	fillCircuitArrays(assignment, zeroPath, zeroPathIndex, outputRecipients, outputValues, outputRandomness, outputCommitments)
	commitment := hashFromField(outputCommitments[0])
	note := ShieldedNote{
		ID:             "deposit-" + req.RequestID,
		Commitment:     commitment.Hex(),
		Nullifier:      hashFromField(fieldHash(shielded.DomainNull, owner, noteRandomness)).Hex(),
		OwnerSecret:    ownerSecret,
		NoteRandomness: noteRandomness.BigInt(new(big.Int)).String(),
		NoteValueWei:   amountWei.String(),
		AssetID:        assetID,
		Status:         "pending",
		Source:         "deposit",
		CreatedAt:      req.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	return draftEnvelope{Envelope: envelope, Data: data, OutputRoot: outputRoot}, assignment, note, commitment, nil
}

func (p *Prover) buildSignSubmit(ctx context.Context, req PayoutRequest, note ShieldedNote, amountWei *big.Int) (string, *ShieldedNote, error) {
	chainID, err := p.client.ChainID(ctx)
	if err != nil {
		return "", nil, err
	}
	proofBlock := new(big.Int)
	signerAddr := common.HexToAddress(p.cfg.SignerAddress)
	nonce, err := p.client.PendingNonceAt(ctx, signerAddr)
	if err != nil {
		return "", nil, err
	}
	nonce, err = payoutNonce(req, nonce)
	if err != nil {
		return "", nil, err
	}
	gasPrice, err := p.client.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(params.GWei)
	}
	gasPrice, err = payoutGasPriceWei(req, gasPrice)
	if err != nil {
		return "", nil, err
	}

	draft, assignment, err := p.buildDraftEnvelope(req, note, amountWei, chainID, proofBlock, nonce, gasPrice)
	if err != nil {
		return "", nil, err
	}
	unsigned := p.unsignedTx(chainID, nonce, gasPrice, draft.Data)
	intentHash, err := core.ShieldedTransactionIntentHash(unsigned, draft.Envelope)
	if err != nil {
		return "", nil, err
	}
	applyIntentHash(assignment, intentHash)
	binding := fieldHash(shielded.DomainBind,
		mustElement(note.OwnerSecret),
		fieldElementFromHash(draft.Envelope.Spends[0].Nullifier),
		fieldElementFromHash(draft.OutputRoot),
		fieldElementFromHash(draft.Envelope.BalanceCommitment),
		fieldElementFromBig(chainID),
		fieldElementFromBytes(intentHash[:16]),
		fieldElementFromBytes(intentHash[16:]),
	)
	bindingBytes := binding.Bytes()
	draft.Envelope.BindingSig = bindingBytes[:]
	assignment.BindingSigHash = binding.BigInt(new(big.Int)).String()

	proofBytes, err := p.prove(assignment)
	if err != nil {
		return "", nil, err
	}
	draft.Envelope.Spends[0].Proof = proofBytes
	finalData, err := core.EncodeShieldedTransaction(draft.Envelope)
	if err != nil {
		return "", nil, err
	}
	tx := p.unsignedTx(chainID, nonce, gasPrice, finalData)
	account := accounts.Account{Address: signerAddr}
	signed, err := p.ks.SignTxWithPassphrase(account, p.passphrase, tx, chainID)
	if err != nil {
		return "", nil, err
	}
	if p.cfg.SubmitSync {
		timeout := time.Duration(p.cfg.ReceiptTimeoutMs) * time.Millisecond
		receipt, err := p.client.SendTransactionSync(ctx, signed, &timeout)
		if err != nil {
			if isAcceptedButUnminedError(err) {
				if draft.ChangeNote != nil {
					draft.ChangeNote.CreatedTxHash = signed.Hash().Hex()
					draft.ChangeNote.Status = "pending"
				}
				return signed.Hash().Hex(), draft.ChangeNote, nil
			}
			return "", nil, err
		}
		if receipt == nil {
			return "", nil, errors.New("nil receipt")
		}
		txHash := receipt.TxHash.Hex()
		if draft.ChangeNote != nil {
			finalized := p.finalizeNoteWitness(ctx, *draft.ChangeNote, draft.ChangeCommitment, txHash)
			draft.ChangeNote = &finalized
		}
		return txHash, draft.ChangeNote, nil
	}
	if err := p.client.SendTransaction(ctx, signed); err != nil {
		return "", nil, err
	}
	if draft.ChangeNote != nil {
		draft.ChangeNote.CreatedTxHash = signed.Hash().Hex()
		draft.ChangeNote.Status = "pending"
	}
	return signed.Hash().Hex(), draft.ChangeNote, nil
}

type draftEnvelope struct {
	Envelope         *core.ShieldedTransaction
	Data             []byte
	OutputRoot       common.Hash
	ChangeCommitment common.Hash
	ChangeNote       *ShieldedNote
}

func (p *Prover) buildDraftEnvelope(req PayoutRequest, note ShieldedNote, amountWei *big.Int, chainID, blockNumber *big.Int, nonce uint64, gasPrice *big.Int) (draftEnvelope, *shielded.SpendCircuit, error) {
	noteValue, ok := parseDecimalBig(note.NoteValueWei)
	if !ok || noteValue.Sign() <= 0 || noteValue.BitLen() > 64 {
		return draftEnvelope{}, nil, fmt.Errorf("note %s has invalid uint64 noteValueWei", note.ID)
	}
	if noteValue.Cmp(amountWei) < 0 {
		return draftEnvelope{}, nil, fmt.Errorf("note %s is smaller than payout amount", note.ID)
	}
	if len(note.MerklePath) != shielded.MerkleDepth || len(note.MerklePathIndex) != shielded.MerkleDepth {
		return draftEnvelope{}, nil, fmt.Errorf("note %s must have %d-deep Merkle path", note.ID, shielded.MerkleDepth)
	}

	assetID := note.AssetID
	if strings.TrimSpace(assetID) == "" {
		assetID = "1"
	}
	owner, err := parseFieldElement(note.OwnerSecret)
	if err != nil {
		return draftEnvelope{}, nil, fmt.Errorf("note %s has invalid ownerSecret: %w", note.ID, err)
	}
	randomness, err := parseFieldElement(note.NoteRandomness)
	if err != nil {
		return draftEnvelope{}, nil, fmt.Errorf("note %s has invalid noteRandomness: %w", note.ID, err)
	}
	asset, err := parseFieldElement(assetID)
	if err != nil {
		return draftEnvelope{}, nil, fmt.Errorf("note %s has invalid assetId: %w", note.ID, err)
	}
	var recipientViewKey []byte
	if strings.TrimSpace(req.RecipientViewKey) != "" {
		recipientViewKey, err = parseViewPublicKey(req.RecipientViewKey)
		if err != nil {
			return draftEnvelope{}, nil, err
		}
	}
	var changeViewKey []byte
	if strings.TrimSpace(req.ChangeViewKey) != "" {
		changeViewKey, err = parseViewPublicKey(req.ChangeViewKey)
		if err != nil {
			return draftEnvelope{}, nil, fmt.Errorf("change %w", err)
		}
	}
	if !isValidAddress(req.PoolWallet) {
		return draftEnvelope{}, nil, fmt.Errorf("invalid poolWallet address %q", req.PoolWallet)
	}
	recipientAddress := common.HexToAddress(req.To)
	changeAddress := common.HexToAddress(req.PoolWallet)
	noteValueElement := fieldElementFromBig(noteValue)
	inputCommitment := fieldHash(shielded.DomainNote, owner, asset, noteValueElement, randomness)
	anchor, err := computeAnchor(inputCommitment, note.MerklePath, note.MerklePathIndex)
	if err != nil {
		return draftEnvelope{}, nil, fmt.Errorf("note %s has invalid Merkle witness: %w", note.ID, err)
	}
	nullifier := fieldHash(shielded.DomainNull, owner, randomness)

	outputRecipients := [shielded.OutputSlots]fr.Element{
		fieldElementFromBig(common.HexToAddress(req.To).Big()),
		owner,
		zeroElement(),
		zeroElement(),
	}
	changeWei := new(big.Int).Sub(noteValue, amountWei)
	outputValues := [shielded.OutputSlots]fr.Element{
		fieldElementFromBig(amountWei),
		fieldElementFromBig(changeWei),
		zeroElement(),
		zeroElement(),
	}
	outputRandomness := [shielded.OutputSlots]fr.Element{
		randomElement(),
		randomElement(),
		randomElement(),
		randomElement(),
	}
	var outputCommitments [shielded.OutputSlots]fr.Element
	var outputs []core.ShieldedOutput
	for i := 0; i < shielded.OutputSlots; i++ {
		outputCommitments[i] = fieldHash(shielded.DomainNote, outputRecipients[i], asset, outputValues[i], outputRandomness[i])
		commitment := hashFromField(outputCommitments[i])
		var output core.ShieldedOutput
		switch {
		case i == 0 && len(recipientViewKey) != 0:
			outputNullifier := hashFromField(fieldHash(shielded.DomainNull, outputRecipients[i], outputRandomness[i]))
			output, err = encryptShieldedNote(commitment, noteOpening(
				recipientAddress,
				outputRecipients[i].BigInt(new(big.Int)),
				asset.BigInt(new(big.Int)),
				outputValues[i].BigInt(new(big.Int)),
				outputRandomness[i].BigInt(new(big.Int)),
				commitment,
				outputNullifier,
			), recipientViewKey)
		case i == 0:
			output = legacyMetadataOutput(commitment, req.RequestID, i, "payout")
		case i == 1 && changeWei.Sign() > 0 && len(changeViewKey) != 0:
			outputNullifier := hashFromField(fieldHash(shielded.DomainNull, outputRecipients[i], outputRandomness[i]))
			output, err = encryptShieldedNote(commitment, noteOpening(
				changeAddress,
				outputRecipients[i].BigInt(new(big.Int)),
				asset.BigInt(new(big.Int)),
				outputValues[i].BigInt(new(big.Int)),
				outputRandomness[i].BigInt(new(big.Int)),
				commitment,
				outputNullifier,
			), changeViewKey)
		case i == 1 && changeWei.Sign() > 0:
			output = legacyMetadataOutput(commitment, req.RequestID, i, "change")
		default:
			output, err = decoyShieldedOutput(commitment)
		}
		if err != nil {
			return draftEnvelope{}, nil, err
		}
		outputs = append(outputs, output)
	}
	outputRoot := hashFromField(fieldHash(shielded.DomainOutput, outputCommitments[:]...))
	totalOutput := new(big.Int).Set(noteValue)
	publicValue := new(big.Int)
	balanceCommitment := fieldHash(shielded.DomainBal, noteValueElement, fieldElementFromBig(totalOutput), fieldElementFromBig(publicValue), asset, fieldElementFromHash(outputRoot))
	envelope := &core.ShieldedTransaction{
		Version: 1,
		Spends: []core.ShieldedSpend{{
			Nullifier:          hashFromField(nullifier),
			Anchor:             hashFromField(anchor),
			EncryptedSpendData: []byte(req.RequestID),
		}},
		Outputs:           outputs,
		BalanceCommitment: hashFromField(balanceCommitment),
		BindingSig:        make([]byte, common.HashLength),
	}
	data, err := core.EncodeShieldedTransaction(envelope)
	if err != nil {
		return draftEnvelope{}, nil, err
	}
	var changeNote *ShieldedNote
	var changeCommitment common.Hash
	if changeWei.Sign() > 0 {
		changeCommitment = hashFromField(outputCommitments[1])
		changeNote = &ShieldedNote{
			ID:             "change-" + req.RequestID,
			Commitment:     changeCommitment.Hex(),
			Nullifier:      hashFromField(fieldHash(shielded.DomainNull, owner, outputRandomness[1])).Hex(),
			OwnerSecret:    note.OwnerSecret,
			NoteRandomness: outputRandomness[1].BigInt(new(big.Int)).String(),
			NoteValueWei:   changeWei.String(),
			AssetID:        assetID,
			Status:         "pending",
			Source:         "change",
			CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		}
	}

	outputRootElement := fieldElementFromHash(outputRoot)
	assignment := &shielded.SpendCircuit{
		ChainID:           chainID.String(),
		BlockNumber:       blockNumber.String(),
		TxHashHi:          "0",
		TxHashLo:          "0",
		SpendIndex:        "0",
		Nullifier:         nullifier.BigInt(new(big.Int)).String(),
		Anchor:            anchor.BigInt(new(big.Int)).String(),
		BalanceCommitment: balanceCommitment.BigInt(new(big.Int)).String(),
		PublicValue:       publicValue.String(),
		OutputRoot:        outputRootElement.BigInt(new(big.Int)).String(),
		BindingSigHash:    new(big.Int).SetBytes(envelope.BindingSig).String(),
		OwnerSecret:       note.OwnerSecret,
		NoteRandomness:    note.NoteRandomness,
		NoteValue:         noteValue.String(),
		AssetID:           assetID,
	}
	fillCircuitArrays(assignment, note.MerklePath, note.MerklePathIndex, outputRecipients, outputValues, outputRandomness, outputCommitments)
	return draftEnvelope{Envelope: envelope, Data: data, OutputRoot: outputRoot, ChangeCommitment: changeCommitment, ChangeNote: changeNote}, assignment, nil
}

func (p *Prover) unsignedTx(chainID *big.Int, nonce uint64, gasPrice *big.Int, data []byte) *types.Transaction {
	return p.unsignedTxWithValue(chainID, nonce, gasPrice, new(big.Int), data)
}

func (p *Prover) unsignedTxWithValue(chainID *big.Int, nonce uint64, gasPrice *big.Int, value *big.Int, data []byte) *types.Transaction {
	to := params.ShieldedPoolAddress
	txValue := new(big.Int)
	if value != nil {
		txValue.Set(value)
	}
	if p.cfg.SignMode == "legacy" {
		return types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			GasTipCap: gasPrice,
			GasFeeCap: gasPrice,
			Gas:       p.cfg.GasLimit,
			To:        &to,
			Value:     txValue,
			Data:      data,
		})
	}
	return types.NewTx(&types.PQTkmTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: gasPrice,
		GasFeeCap: gasPrice,
		Gas:       p.cfg.GasLimit,
		To:        &to,
		Value:     txValue,
		Data:      data,
	})
}

func (p *Prover) prove(assignment *shielded.SpendCircuit) ([]byte, error) {
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, err
	}
	proof, err := groth16.Prove(p.r1cs, p.pk, witness)
	if err != nil {
		return nil, err
	}
	bnProof, ok := proof.(*bn254groth16.Proof)
	if !ok {
		return nil, fmt.Errorf("unexpected proof type %T", proof)
	}
	a := bnProof.Ar.Bytes()
	b := bnProof.Bs.Bytes()
	c := bnProof.Krs.Bytes()
	return core.EncodeShieldedGroth16Proof(core.ShieldedGroth16Proof{
		A: a[:],
		B: b[:],
		C: c[:],
	})
}

func (p *Prover) proveV2(assignment *shielded.SpendCircuitV2) ([]byte, error) {
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, err
	}
	proof, err := groth16.Prove(p.r1csV2, p.pkV2, witness)
	if err != nil {
		return nil, err
	}
	bnProof, ok := proof.(*bn254groth16.Proof)
	if !ok {
		return nil, fmt.Errorf("unexpected V2 proof type %T", proof)
	}
	a := bnProof.Ar.Bytes()
	b := bnProof.Bs.Bytes()
	c := bnProof.Krs.Bytes()
	return core.EncodeShieldedGroth16Proof(core.ShieldedGroth16Proof{A: a[:], B: b[:], C: c[:]})
}

func compileCircuit() (*cs.R1CS, error) {
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1csbuilder.NewBuilder, &shielded.SpendCircuit{})
	if err != nil {
		return nil, err
	}
	r1cs, ok := ccs.(*cs.R1CS)
	if !ok {
		return nil, fmt.Errorf("unexpected constraint system type %T", ccs)
	}
	return r1cs, nil
}

func compileCircuitV2() (*cs.R1CS, error) {
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1csbuilder.NewBuilder, &shielded.SpendCircuitV2{})
	if err != nil {
		return nil, err
	}
	r1cs, ok := ccs.(*cs.R1CS)
	if !ok {
		return nil, fmt.Errorf("unexpected V2 constraint system type %T", ccs)
	}
	return r1cs, nil
}

func fillCircuitArrays(c *shielded.SpendCircuit, path, pathIndex []string, recipients, values, randomness, commitments [shielded.OutputSlots]fr.Element) {
	for i := 0; i < shielded.MerkleDepth; i++ {
		c.MerklePath[i] = path[i]
		c.MerklePathIndex[i] = pathIndex[i]
	}
	for i := 0; i < shielded.OutputSlots; i++ {
		c.OutputRecipient[i] = recipients[i].BigInt(new(big.Int)).String()
		c.OutputValue[i] = values[i].BigInt(new(big.Int)).String()
		c.OutputRandomness[i] = randomness[i].BigInt(new(big.Int)).String()
		c.OutputCommitment[i] = commitments[i].BigInt(new(big.Int)).String()
	}
}

func applyIntentHash(c *shielded.SpendCircuit, hash common.Hash) {
	c.TxHashHi = new(big.Int).SetBytes(hash[:16]).String()
	c.TxHashLo = new(big.Int).SetBytes(hash[16:]).String()
}

func computeAnchor(commitment fr.Element, path, index []string) (fr.Element, error) {
	if err := validateMerkleWitness(path, index); err != nil {
		return fr.Element{}, err
	}
	root := commitment
	for i := 0; i < shielded.MerkleDepth; i++ {
		sibling, _ := parseFieldElement(path[i])
		idx, _ := parseBigFlexible(index[i])
		if idx.Sign() == 1 {
			root = fieldHash(shielded.DomainNode, sibling, root)
		} else {
			root = fieldHash(shielded.DomainNode, root, sibling)
		}
	}
	return root, nil
}

func validateMerkleWitness(path, index []string) error {
	if len(path) != shielded.MerkleDepth || len(index) != shielded.MerkleDepth {
		return fmt.Errorf("expected %d path elements and indexes", shielded.MerkleDepth)
	}
	for i := 0; i < shielded.MerkleDepth; i++ {
		if _, err := parseFieldElement(path[i]); err != nil {
			return fmt.Errorf("path[%d]: %w", i, err)
		}
		idx, ok := parseBigFlexible(index[i])
		if !ok || idx.Sign() < 0 || idx.BitLen() > 1 {
			return fmt.Errorf("pathIndex[%d] must be 0 or 1", i)
		}
	}
	return nil
}

func selectSpendableNote(store NoteStore, amount *big.Int) int {
	for i, note := range store.Notes {
		if strings.TrimSpace(note.Status) != "" && strings.TrimSpace(note.Status) != "available" {
			continue
		}
		value, ok := parseDecimalBig(note.NoteValueWei)
		if ok && value.Cmp(amount) >= 0 && value.BitLen() <= 64 && validateMerkleWitness(note.MerklePath, note.MerklePathIndex) == nil {
			return i
		}
	}
	return -1
}

func selectReplacementNote(store NoteStore, noteID string, requestID string) int {
	noteID = strings.TrimSpace(noteID)
	requestID = strings.TrimSpace(requestID)
	if noteID == "" || requestID == "" {
		return -1
	}
	for i, note := range store.Notes {
		if note.ID != noteID {
			continue
		}
		if strings.TrimSpace(note.Status) != "spent" {
			continue
		}
		if strings.TrimSpace(note.SpentRequestID) != requestID {
			continue
		}
		if validateMerkleWitness(note.MerklePath, note.MerklePathIndex) != nil {
			continue
		}
		return i
	}
	return -1
}

func weiToAntdFloat(wei *big.Int) float64 {
	if wei == nil || wei.Sign() <= 0 {
		return 0
	}
	value, _ := new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(1e18)).Float64()
	return value
}

func antdToWeiInt(amount float64) *big.Int {
	wei := new(big.Float).Mul(big.NewFloat(amount), big.NewFloat(1e18))
	i, _ := wei.Int(nil)
	return i
}

func validatePayoutRequest(req PayoutRequest) error {
	if len(req.RequestID) < 8 {
		return errors.New("requestId is required")
	}
	if !isValidAddress(req.To) {
		return fmt.Errorf("invalid to address %q", req.To)
	}
	if !isValidAddress(req.PoolWallet) {
		return fmt.Errorf("invalid poolWallet address %q", req.PoolWallet)
	}
	if req.AmountAntd <= 0 || math.IsNaN(req.AmountAntd) || math.IsInf(req.AmountAntd, 0) {
		return errors.New("amountAntd must be positive")
	}
	return nil
}

func payoutReplacementRequested(req PayoutRequest) bool {
	return strings.TrimSpace(req.Nonce) != "" || strings.TrimSpace(req.GasPriceWei) != ""
}

func depositAmountWei(req DepositRequest) (*big.Int, error) {
	if strings.TrimSpace(req.AmountWei) != "" {
		amount, ok := parseBigFlexible(req.AmountWei)
		if !ok || amount.Sign() <= 0 || amount.BitLen() > 64 {
			return nil, errors.New("amountWei must be a positive uint64 value")
		}
		return amount, nil
	}
	if req.AmountAntd <= 0 || math.IsNaN(req.AmountAntd) || math.IsInf(req.AmountAntd, 0) {
		return nil, errors.New("amountAntd must be positive when amountWei is not set")
	}
	amount := antdToWeiInt(req.AmountAntd)
	if amount.Sign() <= 0 || amount.BitLen() > 64 {
		return nil, errors.New("deposit amount must fit in the uint64 shielded circuit limit")
	}
	return amount, nil
}

func payoutNonce(req PayoutRequest, fallback uint64) (uint64, error) {
	raw := strings.TrimSpace(req.Nonce)
	if raw == "" {
		return fallback, nil
	}
	nonce, ok := parseBigFlexible(raw)
	if !ok || nonce.Sign() < 0 || nonce.BitLen() > 64 {
		return 0, errors.New("nonce must be a uint64 value")
	}
	return nonce.Uint64(), nil
}

func payoutGasPriceWei(req PayoutRequest, fallback *big.Int) (*big.Int, error) {
	raw := strings.TrimSpace(req.GasPriceWei)
	if raw == "" {
		return new(big.Int).Set(fallback), nil
	}
	gasPrice, ok := parseBigFlexible(raw)
	if !ok || gasPrice.Sign() <= 0 || gasPrice.BitLen() > 256 {
		return nil, errors.New("gasPriceWei must be a positive uint256 value")
	}
	return gasPrice, nil
}

func depositNonce(req DepositRequest, fallback uint64) (uint64, error) {
	raw := strings.TrimSpace(req.Nonce)
	if raw == "" {
		return fallback, nil
	}
	nonce, ok := parseBigFlexible(raw)
	if !ok || nonce.Sign() < 0 || nonce.BitLen() > 64 {
		return 0, errors.New("nonce must be a uint64 value")
	}
	return nonce.Uint64(), nil
}

func depositGasPriceWei(req DepositRequest, fallback *big.Int) (*big.Int, error) {
	raw := strings.TrimSpace(req.GasPriceWei)
	if raw == "" {
		return new(big.Int).Set(fallback), nil
	}
	gasPrice, ok := parseBigFlexible(raw)
	if !ok || gasPrice.Sign() <= 0 || gasPrice.BitLen() > 256 {
		return nil, errors.New("gasPriceWei must be a positive uint256 value")
	}
	return gasPrice, nil
}

func (p *Prover) finalizeNoteWitness(ctx context.Context, note ShieldedNote, commitment common.Hash, txHash string) ShieldedNote {
	note.CreatedTxHash = txHash
	var path commitmentPathRPC
	if err := p.client.Client().CallContext(ctx, &path, "tkmprivacy_commitmentPath", commitment); err != nil {
		note.Status = "pending"
		return note
	}
	if !path.Found || len(path.MerklePath) != shielded.MerkleDepth || len(path.MerklePathIndex) != shielded.MerkleDepth {
		note.Status = "pending"
		return note
	}
	note.MerkleRoot = path.Root.Hex()
	note.MerklePath = make([]string, shielded.MerkleDepth)
	note.MerklePathIndex = make([]string, shielded.MerkleDepth)
	for i := 0; i < shielded.MerkleDepth; i++ {
		sibling := fieldElementFromHash(path.MerklePath[i])
		note.MerklePath[i] = sibling.BigInt(new(big.Int)).String()
		note.MerklePathIndex[i] = fmt.Sprintf("%d", uint64(path.MerklePathIndex[i]))
	}
	note.Status = "available"
	return note
}

func appendOrReplaceNote(notes []ShieldedNote, note ShieldedNote) []ShieldedNote {
	for i := range notes {
		if note.ID != "" && notes[i].ID == note.ID {
			notes[i] = note
			return notes
		}
		if note.Commitment != "" && strings.EqualFold(notes[i].Commitment, note.Commitment) {
			notes[i] = note
			return notes
		}
	}
	return append(notes, note)
}

func (p *Prover) recordError(db RequestDB, req PayoutRequest, noteID string, msg string) {
	db.Requests[req.RequestID] = RequestRecord{Request: req, Status: "waiting", Error: msg, NoteID: noteID, UpdatedAt: time.Now().UTC()}
	if err := writeRequestDB(p.cfg.RequestsPath, db); err != nil {
		log.Printf("request db error: %v", err)
	}
}

func (p *Prover) recordDeposit(db RequestDB, req DepositRequest, note ShieldedNote, amountWei *big.Int, txHash string, status string, msg string) {
	if db.Deposits == nil {
		db.Deposits = make(map[string]DepositRecord)
	}
	if status == "" {
		status = "waiting"
	}
	amount := ""
	if amountWei != nil {
		amount = amountWei.String()
	}
	db.Deposits[req.RequestID] = DepositRecord{
		Request:    req,
		Status:     status,
		TxHash:     txHash,
		Error:      msg,
		NoteID:     note.ID,
		Commitment: note.Commitment,
		AmountWei:  amount,
		UpdatedAt:  time.Now().UTC(),
	}
	if err := writeRequestDB(p.cfg.RequestsPath, db); err != nil {
		log.Printf("deposit request db error: %v", err)
	}
}

func (p *Prover) refreshDepositRecord(ctx context.Context, db RequestDB, req DepositRequest, rec DepositRecord, amountWei *big.Int) DepositResponse {
	resp := depositRecordResponse(rec)
	if rec.Commitment == "" || rec.Status == "available" {
		return resp
	}
	notes, err := readNoteStore(p.cfg.NotesPath)
	if err != nil {
		return resp
	}
	commitment := common.HexToHash(rec.Commitment)
	for i, note := range notes.Notes {
		if rec.NoteID != "" && note.ID != rec.NoteID {
			continue
		}
		if rec.NoteID == "" && !strings.EqualFold(note.Commitment, rec.Commitment) {
			continue
		}
		finalized := p.finalizeNoteWitness(ctx, note, commitment, rec.TxHash)
		notes.Notes[i] = finalized
		if finalized.Status != note.Status || finalized.MerkleRoot != note.MerkleRoot {
			if err := writeNoteStore(p.cfg.NotesPath, notes); err != nil {
				log.Printf("deposit note finalize write error: %v", err)
				return resp
			}
			p.recordDeposit(db, req, finalized, amountWei, rec.TxHash, finalized.Status, "")
			return DepositResponse{
				TxHash:     rec.TxHash,
				Status:     finalized.Status,
				NoteID:     finalized.ID,
				Commitment: finalized.Commitment,
				AmountWei:  resp.AmountWei,
				AmountAntd: resp.AmountAntd,
			}
		}
		return resp
	}
	return resp
}

func depositRecordResponse(rec DepositRecord) DepositResponse {
	amountWei := strings.TrimSpace(rec.AmountWei)
	var amountAntd float64
	if amount, ok := parseDecimalBig(amountWei); ok {
		amountAntd = weiToAntdFloat(amount)
	}
	return DepositResponse{
		TxHash:     rec.TxHash,
		Status:     rec.Status,
		NoteID:     rec.NoteID,
		Commitment: rec.Commitment,
		AmountWei:  amountWei,
		AmountAntd: amountAntd,
		Error:      rec.Error,
	}
}

func readNoteStore(path string) (NoteStore, error) {
	var store NoteStore
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return store, nil
	}
	return store, json.Unmarshal(data, &store)
}

func writeNoteStore(path string, store NoteStore) error {
	return writeJSONFile(path, store, 0600)
}

func readRequestDB(path string) (RequestDB, error) {
	db := RequestDB{Requests: make(map[string]RequestRecord), Deposits: make(map[string]DepositRecord)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return db, nil
	}
	if err != nil {
		return db, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return db, nil
	}
	if err := json.Unmarshal(data, &db); err != nil {
		return db, err
	}
	if db.Requests == nil {
		db.Requests = make(map[string]RequestRecord)
	}
	if db.Deposits == nil {
		db.Deposits = make(map[string]DepositRecord)
	}
	return db, nil
}

func writeRequestDB(path string, db RequestDB) error {
	return writeJSONFile(path, db, 0600)
}

func writeJSONFile(path string, v any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, perm)
}

func readObject(path string, object io.ReaderFrom) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	n, err := object.ReadFrom(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if int(n) != len(data) {
		return fmt.Errorf("%s: read %d bytes, file has %d", path, n, len(data))
	}
	return nil
}

func fieldHash(domain uint64, inputs ...fr.Element) fr.Element {
	hasher := mimc.NewMiMC()
	domainElement := fieldElementFromUint64(domain)
	domainBytes := domainElement.Bytes()
	hasher.Write(domainBytes[:])
	for _, input := range inputs {
		inputBytes := input.Bytes()
		hasher.Write(inputBytes[:])
	}
	sum := hasher.Sum(nil)
	var out fr.Element
	if err := out.SetBytesCanonical(sum); err != nil {
		panic(err)
	}
	return out
}

func fieldElementFromUint64(v uint64) fr.Element {
	var out fr.Element
	out.SetUint64(v)
	return out
}

func fieldElementFromBig(v *big.Int) fr.Element {
	var out fr.Element
	if v != nil {
		out.SetBigInt(v)
	}
	return out
}

func fieldElementFromBytes(v []byte) fr.Element {
	var out fr.Element
	out.SetBytes(v)
	return out
}

func fieldElementFromHash(hash common.Hash) fr.Element {
	var out fr.Element
	if err := out.SetBytesCanonical(hash.Bytes()); err != nil {
		panic(err)
	}
	return out
}

func hashFromField(v fr.Element) common.Hash {
	var out common.Hash
	b := v.Bytes()
	copy(out[:], b[:])
	return out
}

func mustElement(input string) fr.Element {
	v, err := parseFieldElement(input)
	if err != nil {
		panic(err)
	}
	return v
}

func parseFieldElement(input string) (fr.Element, error) {
	v, ok := parseBigFlexible(input)
	if !ok || v.Sign() < 0 {
		return fr.Element{}, fmt.Errorf("invalid field element %q", input)
	}
	return fieldElementFromBig(v), nil
}

func zeroElement() fr.Element {
	var out fr.Element
	return out
}

func randomElement() fr.Element {
	var out fr.Element
	for {
		b := randomBytes(32)
		if err := out.SetBytesCanonical(b); err == nil {
			return out
		}
	}
}

func randomBytes(n int) []byte {
	out := make([]byte, n)
	if _, err := rand.Read(out); err != nil {
		panic(err)
	}
	return out
}

func normalizeAddress(s string) string {
	s = strings.TrimSpace(s)
	for len(s) >= 2 && strings.EqualFold(s[:2], "0x") {
		s = s[2:]
	}
	if len(s) == 40 && isHexString(s) {
		return "0x" + strings.ToLower(s)
	}
	return strings.TrimSpace(s)
}

func isValidAddress(s string) bool {
	s = normalizeAddress(s)
	return len(s) == 42 && strings.HasPrefix(strings.ToLower(s), "0x") && isHexString(s[2:])
}

func isHexString(s string) bool {
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func parseBigFlexible(s string) (*big.Int, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		return new(big.Int).SetString(s[2:], 16)
	}
	return new(big.Int).SetString(s, 10)
}

func parseDecimalBig(s string) (*big.Int, bool) {
	return new(big.Int).SetString(strings.TrimSpace(s), 10)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
