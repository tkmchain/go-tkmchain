package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded"
)

type BuildTransferRequest struct {
	RequestID        string       `json:"requestId"`
	ApplicationData  string       `json:"applicationData,omitempty"`
	From             string       `json:"from"`
	To               string       `json:"to"`
	AmountWei        string       `json:"amountWei"`
	RecipientViewKey string       `json:"recipientViewKey"`
	ChangeViewKey    string       `json:"changeViewKey"`
	Nonce            string       `json:"nonce,omitempty"`
	GasPriceWei      string       `json:"gasPriceWei,omitempty"`
	Note             ShieldedNote `json:"note"`
}

type BuildWithdrawalRequest struct {
	RequestID       string       `json:"requestId"`
	ApplicationData string       `json:"applicationData,omitempty"`
	From            string       `json:"from"`
	To              string       `json:"to"`
	AmountWei       string       `json:"amountWei"`
	ChangeViewKey   string       `json:"changeViewKey"`
	Nonce           string       `json:"nonce,omitempty"`
	GasPriceWei     string       `json:"gasPriceWei,omitempty"`
	Note            ShieldedNote `json:"note"`
}

type unsignedPQTransaction struct {
	ChainID    string `json:"chainId"`
	Nonce      string `json:"nonce"`
	GasTipCap  string `json:"gasTipCap"`
	GasFeeCap  string `json:"gasFeeCap"`
	Gas        string `json:"gas"`
	To         string `json:"to"`
	Value      string `json:"value"`
	Data       string `json:"data"`
	AccessList []any  `json:"accessList"`
}

type BuildShieldedResponse struct {
	Transaction     *unsignedPQTransaction  `json:"transaction,omitempty"`
	IntentHash      string                  `json:"intentHash,omitempty"`
	SpentNullifier  string                  `json:"spentNullifier,omitempty"`
	CreatedNotes    []ShieldedNote          `json:"createdNotes,omitempty"`
	ShieldedVersion uint64                  `json:"shieldedVersion,omitempty"`
	GasSponsorWei   string                  `json:"gasSponsorWei,omitempty"`
	OutputOpenings  []ShieldedOutputOpening `json:"outputOpenings,omitempty"`
	Error           string                  `json:"error,omitempty"`
}

// ShieldedOutputOpening lets a locally signing client independently recompute
// the V2 note commitment before authorizing the transaction. It is returned
// only to the authenticated requestor and never written into the transaction.
type ShieldedOutputOpening struct {
	Index      uint64 `json:"index"`
	Recipient  string `json:"recipient"`
	AssetID    string `json:"assetId"`
	ValueWei   string `json:"valueWei"`
	Randomness string `json:"randomness"`
	Commitment string `json:"commitment"`
}

func (p *Prover) handleBuildDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !p.acquireBuildSlot() {
		writeJSON(w, http.StatusTooManyRequests, BuildShieldedResponse{Error: "proof builder is busy; retry later"})
		return
	}
	defer p.releaseBuildSlot()
	var req DepositRequest
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, BuildShieldedResponse{Error: err.Error()})
		return
	}
	ctx, cancel := p.operationContext()
	defer cancel()
	resp, err := p.BuildDeposit(ctx, req)
	if err != nil {
		resp.Error = err.Error()
		writeJSON(w, http.StatusBadRequest, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (p *Prover) handleBuildTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !p.acquireBuildSlot() {
		writeJSON(w, http.StatusTooManyRequests, BuildShieldedResponse{Error: "proof builder is busy; retry later"})
		return
	}
	defer p.releaseBuildSlot()
	var req BuildTransferRequest
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, BuildShieldedResponse{Error: err.Error()})
		return
	}
	ctx, cancel := p.operationContext()
	defer cancel()
	resp, err := p.BuildTransfer(ctx, req)
	if err != nil {
		resp.Error = err.Error()
		writeJSON(w, http.StatusBadRequest, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (p *Prover) handleBuildWithdrawal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !p.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !p.acquireBuildSlot() {
		writeJSON(w, http.StatusTooManyRequests, BuildShieldedResponse{Error: "proof builder is busy; retry later"})
		return
	}
	defer p.releaseBuildSlot()
	var req BuildWithdrawalRequest
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, BuildShieldedResponse{Error: err.Error()})
		return
	}
	ctx, cancel := p.operationContext()
	defer cancel()
	resp, err := p.BuildWithdrawal(ctx, req)
	if err != nil {
		resp.Error = err.Error()
		writeJSON(w, http.StatusBadRequest, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (p *Prover) acquireBuildSlot() bool {
	if p == nil || p.buildSlots == nil {
		return false
	}
	select {
	case p.buildSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (p *Prover) releaseBuildSlot() {
	if p == nil || p.buildSlots == nil {
		return
	}
	select {
	case <-p.buildSlots:
	default:
	}
}

func (p *Prover) BuildDeposit(ctx context.Context, req DepositRequest) (BuildShieldedResponse, error) {
	if !p.ready() {
		return BuildShieldedResponse{}, errors.New("prover is not ready; check /healthz")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !isValidAddress(req.From) {
		return BuildShieldedResponse{}, fmt.Errorf("invalid from address %q", req.From)
	}
	if _, err := parseViewPublicKey(req.RecipientViewKey); err != nil {
		return BuildShieldedResponse{}, err
	}
	amountWei, err := depositAmountWei(req)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	assetID := strings.TrimSpace(req.AssetID)
	if assetID == "" {
		assetID = "1"
	}
	ownerSecret := strings.TrimSpace(req.OwnerSecret)
	if ownerSecret == "" {
		ownerSecret = common.HexToAddress(req.From).Big().String()
	}
	chainID, nonce, gasPrice, err := p.proofTransactionContext(ctx, req.From, req.Nonce, req.GasPriceWei)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	if p.activeShieldedVersion(ctx, chainID) == core.ShieldedTxVersionV2 {
		return p.buildDepositV2(ctx, req, amountWei, assetID, chainID, nonce, gasPrice)
	}
	req.CreatedAt = time.Now().UTC()
	draft, assignment, note, _, err := p.buildDepositDraft(req, amountWei, assetID, ownerSecret, chainID, new(big.Int))
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	unsigned := p.unsignedTxWithValue(chainID, nonce, gasPrice, amountWei, draft.Data)
	intentHash, err := core.ShieldedTransactionIntentHash(unsigned, draft.Envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
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
	proof, err := p.prove(assignment)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	draft.Envelope.Spends[0].Proof = proof
	data, err := core.EncodeShieldedTransaction(draft.Envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	note.Status = "pending"
	return BuildShieldedResponse{
		Transaction:  makeUnsignedPQTransaction(chainID, nonce, gasPrice, p.cfg.GasLimit, amountWei, data),
		IntentHash:   intentHash.Hex(),
		CreatedNotes: []ShieldedNote{note},
	}, nil
}

func (p *Prover) BuildTransfer(ctx context.Context, req BuildTransferRequest) (BuildShieldedResponse, error) {
	if !p.ready() {
		return BuildShieldedResponse{}, errors.New("prover is not ready; check /healthz")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(strings.TrimSpace(req.RequestID)) < 8 {
		return BuildShieldedResponse{}, errors.New("requestId must contain at least 8 characters")
	}
	if _, err := shieldedSpendData(req.RequestID, req.ApplicationData); err != nil {
		return BuildShieldedResponse{}, err
	}
	if !isValidAddress(req.From) || !isValidAddress(req.To) {
		return BuildShieldedResponse{}, errors.New("valid from and to addresses are required")
	}
	if _, err := parseViewPublicKey(req.RecipientViewKey); err != nil {
		return BuildShieldedResponse{}, err
	}
	if _, err := parseViewPublicKey(req.ChangeViewKey); err != nil {
		return BuildShieldedResponse{}, fmt.Errorf("change %w", err)
	}
	amountWei, ok := parseBigFlexible(req.AmountWei)
	if !ok || amountWei.Sign() <= 0 || amountWei.BitLen() > 64 {
		return BuildShieldedResponse{}, errors.New("amountWei must be a positive uint64 value")
	}
	chainID, nonce, gasPrice, err := p.proofTransactionContext(ctx, req.From, req.Nonce, req.GasPriceWei)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	if p.activeShieldedVersion(ctx, chainID) == core.ShieldedTxVersionV2 {
		return p.buildTransferV2(ctx, req, amountWei, chainID, nonce, gasPrice)
	}
	if !strings.EqualFold(common.HexToAddress(req.From).Hex(), common.HexToAddress(req.To).Hex()) {
		return BuildShieldedResponse{}, errors.New("V1 proof-only transfers are limited to self-shielding; third-party payments activate with the recipient-bound V2 circuit")
	}
	payout := PayoutRequest{
		RequestID:        req.RequestID,
		ApplicationData:  req.ApplicationData,
		PoolWallet:       req.From,
		To:               req.To,
		AmountWei:        amountWei.String(),
		AmountAntd:       weiToAntdFloat(amountWei),
		RecipientViewKey: req.RecipientViewKey,
		ChangeViewKey:    req.ChangeViewKey,
		Nonce:            req.Nonce,
		GasPriceWei:      req.GasPriceWei,
		CreatedAt:        time.Now().UTC(),
	}
	draft, assignment, err := p.buildDraftEnvelope(payout, req.Note, amountWei, chainID, new(big.Int), nonce, gasPrice)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	unsigned := p.unsignedTx(chainID, nonce, gasPrice, draft.Data)
	intentHash, err := core.ShieldedTransactionIntentHash(unsigned, draft.Envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	applyIntentHash(assignment, intentHash)
	binding := fieldHash(shielded.DomainBind,
		mustElement(req.Note.OwnerSecret),
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
	proof, err := p.prove(assignment)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	draft.Envelope.Spends[0].Proof = proof
	data, err := core.EncodeShieldedTransaction(draft.Envelope)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	created := make([]ShieldedNote, 0, 1)
	if draft.ChangeNote != nil {
		created = append(created, *draft.ChangeNote)
	}
	return BuildShieldedResponse{
		Transaction:    makeUnsignedPQTransaction(chainID, nonce, gasPrice, p.cfg.GasLimit, new(big.Int), data),
		IntentHash:     intentHash.Hex(),
		SpentNullifier: draft.Envelope.Spends[0].Nullifier.Hex(),
		CreatedNotes:   created,
	}, nil
}

func (p *Prover) BuildWithdrawal(ctx context.Context, req BuildWithdrawalRequest) (BuildShieldedResponse, error) {
	if !p.ready() {
		return BuildShieldedResponse{}, errors.New("prover is not ready; check /healthz")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(strings.TrimSpace(req.RequestID)) < 8 {
		return BuildShieldedResponse{}, errors.New("requestId must contain at least 8 characters")
	}
	if _, err := shieldedSpendData(req.RequestID, req.ApplicationData); err != nil {
		return BuildShieldedResponse{}, err
	}
	if !isValidAddress(req.From) || !isValidAddress(req.To) {
		return BuildShieldedResponse{}, errors.New("valid from and withdrawal recipient addresses are required")
	}
	if _, err := parseViewPublicKey(req.ChangeViewKey); err != nil {
		return BuildShieldedResponse{}, fmt.Errorf("change %w", err)
	}
	amountWei, ok := parseBigFlexible(req.AmountWei)
	if !ok || amountWei.Sign() <= 0 || amountWei.BitLen() > 64 {
		return BuildShieldedResponse{}, errors.New("amountWei must be a positive uint64 value")
	}
	chainID, nonce, gasPrice, err := p.proofTransactionContext(ctx, req.From, req.Nonce, req.GasPriceWei)
	if err != nil {
		return BuildShieldedResponse{}, err
	}
	if p.activeShieldedVersion(ctx, chainID) != core.ShieldedTxVersionV2 {
		return BuildShieldedResponse{}, errors.New("shielded withdrawals require the recipient-bound V2 circuit")
	}
	return p.buildWithdrawalV2(ctx, req, amountWei, chainID, nonce, gasPrice)
}

func shieldedSpendData(requestID string, applicationData string) ([]byte, error) {
	if strings.TrimSpace(applicationData) == "" {
		return []byte(requestID), nil
	}
	data, err := hexutil.Decode(strings.TrimSpace(applicationData))
	if err != nil {
		return nil, errors.New("applicationData must be 0x-prefixed hexadecimal data")
	}
	if len(data) == 0 || len(data) > 12*1024 {
		return nil, errors.New("applicationData must contain between 1 and 12288 bytes")
	}
	return data, nil
}

func (p *Prover) proofTransactionContext(ctx context.Context, from, nonceRaw, gasPriceRaw string) (*big.Int, uint64, *big.Int, error) {
	chainID, err := p.client.ChainID(ctx)
	if err != nil {
		return nil, 0, nil, err
	}
	nonce, err := p.client.PendingNonceAt(ctx, common.HexToAddress(from))
	if err != nil {
		return nil, 0, nil, err
	}
	nonceReq := DepositRequest{Nonce: nonceRaw}
	nonce, err = depositNonce(nonceReq, nonce)
	if err != nil {
		return nil, 0, nil, err
	}
	gasPrice, err := p.client.SuggestGasPrice(ctx)
	if err != nil || gasPrice == nil || gasPrice.Sign() <= 0 {
		gasPrice = big.NewInt(params.GWei)
	}
	gasPrice, err = depositGasPriceWei(DepositRequest{GasPriceWei: gasPriceRaw}, gasPrice)
	if err != nil {
		return nil, 0, nil, err
	}
	return chainID, nonce, gasPrice, nil
}

func (p *Prover) shieldedGasSponsorValue(ctx context.Context, chainID *big.Int, sender common.Address, gasPrice *big.Int) (*big.Int, error) {
	if !p.shieldedGasSponsorshipActive(ctx, chainID) {
		return new(big.Int), nil
	}
	maxGasCost := new(big.Int).Mul(new(big.Int).SetUint64(p.cfg.GasLimit), gasPrice)
	balance, err := p.client.BalanceAt(ctx, sender, nil)
	if err != nil {
		return nil, fmt.Errorf("read sender balance for shielded gas sponsorship: %w", err)
	}
	if balance.Cmp(maxGasCost) >= 0 {
		return new(big.Int), nil
	}
	return new(big.Int).Sub(maxGasCost, balance), nil
}

func makeUnsignedPQTransaction(chainID *big.Int, nonce uint64, gasPrice *big.Int, gas uint64, value *big.Int, data []byte) *unsignedPQTransaction {
	return &unsignedPQTransaction{
		ChainID:    hexutil.EncodeBig(chainID),
		Nonce:      hexutil.EncodeUint64(nonce),
		GasTipCap:  hexutil.EncodeBig(gasPrice),
		GasFeeCap:  hexutil.EncodeBig(gasPrice),
		Gas:        hexutil.EncodeUint64(gas),
		To:         params.ShieldedPoolAddress.Hex(),
		Value:      hexutil.EncodeBig(value),
		Data:       hexutil.Encode(data),
		AccessList: []any{},
	}
}
