package gui

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
)

type shield3RequestRecord struct {
	Digest    [64]byte
	Raw       hexutil.Bytes
	Hash      common.Hash
	Submitted bool
}
type shield3Request struct {
	Seed      hexutil.Bytes                   `json:"seed"`
	Stamp     *pqcrypto.ShieldedV3StampRecord `json:"stamp"`
	Account   common.Address                  `json:"account"`
	Recipient string                          `json:"recipient"`
	AmountWei string                          `json:"amountWei"`
	RequestID string                          `json:"requestId"`
	View      *shield3wallet.ViewKey          `json:"view"`
}

func shield3LocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return false
	}
	requestHost := r.Host
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = h
	}
	if requestHost != "localhost" && (net.ParseIP(requestHost) == nil || !net.ParseIP(requestHost).IsLoopback()) {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host
}

// All private wallet operations stay on authenticated loopback HTTP. They are
// deliberately absent from public JSON-RPC and never log seeds/viewing keys.
func (g *GUI) handleShield3(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	fail := func(code int, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
	}
	if r.Method != "POST" || r.Header.Get("X-GUI-Token") != g.token || !shield3LocalRequest(r) {
		fail(403, errors.New("private wallet operations require authenticated loopback access"))
		return
	}
	operation := strings.TrimPrefix(r.URL.Path, "/shield3/")
	switch operation {
	case "identity", "validate", "scan", "viewkeys", "view-scan", "send", "shield":
	default:
		fail(404, errors.New("unsupported Shield3 operation"))
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
	if err != nil {
		fail(400, errors.New("invalid Shield3 request size"))
		return
	}
	defer clear(data)
	var req shield3Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&req); err != nil {
		fail(400, errors.New("invalid Shield3 request"))
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		fail(400, errors.New("trailing Shield3 request data"))
		return
	}
	defer clear(req.Seed)
	if req.View != nil {
		defer clear(req.View.Incoming)
		defer clear(req.View.Outgoing)
	}
	reply := func(value any) { w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode(value) }
	var chain hexutil.Big
	if err = g.client.CallContext(r.Context(), &chain, "eth_chainId"); err != nil {
		fail(503, err)
		return
	}
	chainID := (*big.Int)(&chain)
	if !chainID.IsUint64() {
		fail(400, errors.New("invalid Shield3 chain ID"))
		return
	}
	if operation == "validate" {
		p, err := shield3wallet.DecodePaymentCode(req.Recipient, chainID.Uint64())
		if err != nil {
			fail(400, err)
			return
		}
		reply(map[string]any{"address": p.Address, "chainId": p.ChainID})
		return
	}
	if operation == "view-scan" {
		if req.View == nil || req.View.ChainID != chainID.Uint64() {
			fail(400, errors.New("invalid viewing key"))
			return
		}
		defer clear(req.View.Incoming)
		defer clear(req.View.Outgoing)
		scan, err := shield3wallet.Scan(r.Context(), g.client, *req.View)
		if err != nil {
			fail(400, err)
			return
		}
		reply(scan)
		return
	}
	identity, err := shield3wallet.NewIdentity(req.Seed, chainID.Uint64(), req.Stamp)
	if err != nil {
		fail(400, err)
		return
	}
	defer identity.Clear()
	if req.Account != (common.Address{}) && req.Account != identity.Address {
		fail(400, errors.New("wallet account does not match its private seed"))
		return
	}
	switch operation {
	case "identity":
		reply(map[string]any{"address": identity.Address, "paymentCode": identity.Code})
		return
	case "viewkeys":
		view := identity.ViewKey()
		defer clear(view.Incoming)
		defer clear(view.Outgoing)
		reply(map[string]any{"viewKey": view, "stampKey": hexutil.Bytes(identity.StampSeed)})
		return
	case "scan":
		view := identity.ViewKey()
		defer clear(view.Incoming)
		defer clear(view.Outgoing)
		scan, err := shield3wallet.Scan(r.Context(), g.client, view)
		if err != nil {
			fail(400, err)
			return
		}
		reply(scan)
		return
	}
	if len(req.RequestID) < 16 || len(req.RequestID) > 128 {
		fail(400, errors.New("Shield3 send requires a stable request ID"))
		return
	}
	amount, ok := new(big.Int).SetString(req.AmountWei, 10)
	if !ok {
		fail(400, errors.New("invalid amount in smallest chain units"))
		return
	}
	recipientCode := req.Recipient
	if operation == "shield" {
		recipientCode = identity.Code
	}
	recipient, err := shield3wallet.DecodePaymentCode(recipientCode, identity.ChainID)
	if err != nil {
		fail(400, err)
		return
	}
	requestHash := sha512.New()
	requestHash.Write([]byte(operation))
	canonicalRequest, err := json.Marshal(req)
	if err != nil {
		fail(400, err)
		return
	}
	defer clear(canonicalRequest)
	requestHash.Write(canonicalRequest)
	var digest [64]byte
	copy(digest[:], requestHash.Sum(nil))
	g.shield3Mu.Lock()
	defer g.shield3Mu.Unlock()
	if g.shield3Requests == nil {
		g.shield3Requests = make(map[string]*shield3RequestRecord)
	}
	record := g.shield3Requests[req.RequestID]
	if record == nil && g.opts.WalletStateDir != "" {
		record, err = g.loadShield3Submission(req.RequestID)
		if err != nil {
			fail(500, err)
			return
		}
	}
	if record != nil && record.Digest != digest {
		fail(409, errors.New("request ID already belongs to another wallet operation"))
		return
	}
	if record == nil {
		if len(g.shield3Requests) >= 256 {
			for id, cached := range g.shield3Requests {
				if cached.Submitted && g.opts.WalletStateDir != "" {
					delete(g.shield3Requests, id)
					break
				}
			}
			if len(g.shield3Requests) >= 256 {
				fail(503, errors.New("too many unresolved wallet submissions; check their transaction hashes"))
				return
			}
		}
		unsigned, err := shield3wallet.Build(r.Context(), g.client, req.Seed, identity, recipient, amount, operation == "shield")
		if err != nil {
			fail(400, err)
			return
		}
		key, err := pqcrypto.NewMLDSA87FromSeed(req.Seed)
		if err != nil {
			fail(400, err)
			return
		}
		signed, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(chainID), key)
		if err != nil {
			fail(400, err)
			return
		}
		raw, err := signed.MarshalBinary()
		if err != nil {
			fail(400, err)
			return
		}
		record = &shield3RequestRecord{Digest: digest, Raw: raw, Hash: signed.Hash()}
		if err = g.saveShield3Submission(req.RequestID, record); err != nil {
			fail(500, err)
			return
		}
		g.shield3Requests[req.RequestID] = record
	}
	if !record.Submitted {
		var hash common.Hash
		if err = g.client.CallContext(r.Context(), &hash, "eth_sendRawTransaction", record.Raw); err != nil {
			// A lost RPC reply cannot cause a new proof/spend. Keep the exact signed
			// transaction and return its locally computed hash for confirmation checks.
			reply(map[string]any{"transactionHash": record.Hash, "status": "unconfirmed", "submissionError": fmt.Sprint(err)})
			return
		}
		if hash != record.Hash {
			fail(502, errors.New("node returned an unexpected transaction hash"))
			return
		}
		record.Submitted = true
		if err = g.saveShield3Submission(req.RequestID, record); err != nil {
			reply(map[string]any{"transactionHash": record.Hash, "status": "unconfirmed"})
			return
		}
	}
	reply(map[string]any{"transactionHash": record.Hash, "status": "unconfirmed"})
}
