package shield3wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

// Relay URLs are chosen explicitly by the user. Keys and note openings never
// leave the local wallet. Remote services require TLS; HTTP is loopback-only.
func ValidateRelayURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid relay URL")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return errors.New("remote relays require HTTPS; HTTP is allowed only on loopback")
	}
	return nil
}
func relayHTTP(ctx context.Context, endpoint, path string, request, result any) error {
	if err := ValidateRelayURL(endpoint); err != nil {
		return err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Do not send an authorization to a redirected host or through proxy env vars.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 20<<20+1))
	if err != nil || len(body) > 20<<20 {
		return errors.New("relay response exceeds wallet limit")
	}
	if response.StatusCode != http.StatusOK {
		return errors.New("relay request failed; retry the saved payment or check its status")
	}
	return json.Unmarshal(body, result)
}
func FetchRelayOffer(ctx context.Context, endpoint, requestID string) (RelayOffer, error) {
	var offer RelayOffer
	err := relayHTTP(ctx, endpoint, "/offer", struct {
		RequestID string `json:"requestId"`
	}{requestID}, &offer)
	return offer, err
}

type RelayResponse struct {
	Transaction         hexutil.Bytes `json:"transaction"`
	TransactionHash     common.Hash   `json:"transactionHash"`
	Status              string        `json:"status"`
	SubmissionUncertain bool          `json:"submissionUncertain,omitempty"`
}

func SubmitRelayPacket(ctx context.Context, endpoint, requestID string, raw []byte) (RelayResponse, error) {
	var response RelayResponse
	err := relayHTTP(ctx, endpoint, "/submit", struct {
		Transaction hexutil.Bytes `json:"transaction"`
		RequestID   string        `json:"requestId"`
	}{raw, requestID}, &response)
	if err != nil {
		return response, err
	}
	hash, err := MatchSignedRelay(raw, response.Transaction)
	if err != nil || hash != response.TransactionHash {
		return RelayResponse{}, errors.New("relay returned a different or unauthenticated transaction")
	}
	response.Status = "unconfirmed" // Only the wallet's own node can confirm it.
	return response, nil
}

// MatchSignedRelay checks the operator's PQ signature and every payer-bound
// field. A server-reported hash or confirmation is never sufficient by itself.
func MatchSignedRelay(draftRaw, signedRaw []byte) (common.Hash, error) {
	if uint64(len(draftRaw)) > core.ShieldedV3MaxTxSize || uint64(len(signedRaw)) > core.ShieldedV3MaxTxSize {
		return common.Hash{}, errors.New("relay transaction exceeds limit")
	}
	var draft, signed types.Transaction
	if draft.UnmarshalBinary(draftRaw) != nil || signed.UnmarshalBinary(signedRaw) != nil {
		return common.Hash{}, errors.New("invalid relay encoding")
	}
	algorithm, pub, sig, ok := draft.PQTkmFields()
	signedAlgorithm, signedPub, _, signedOK := signed.PQTkmFields()
	if !ok || !signedOK || len(sig) != 0 || algorithm != pqcrypto.AlgorithmMLDSA87 || signedAlgorithm != algorithm || !bytes.Equal(pub, signedPub) || !bytes.Equal(signed.Data(), draft.Data()) || signed.Nonce() != draft.Nonce() || signed.Gas() != draft.Gas() || signed.GasFeeCap().Cmp(draft.GasFeeCap()) != 0 || signed.GasTipCap().Cmp(draft.GasTipCap()) != 0 || signed.ChainId().Cmp(draft.ChainId()) != 0 || signed.Value().Cmp(draft.Value()) != 0 || signed.To() == nil || draft.To() == nil || *signed.To() != *draft.To() || len(signed.AccessList()) != 0 || len(draft.AccessList()) != 0 {
		return common.Hash{}, errors.New("signed relay does not match the prepared payment")
	}
	e, found, err := core.DecodeShieldedV3Transaction(draft.Data())
	if err != nil || !found || !e.Relayed {
		return common.Hash{}, errors.New("not a relay payment")
	}
	sender, err := types.Sender(types.NewQuantumSigner(signed.ChainId()), &signed)
	address, addressErr := pqcrypto.Address(algorithm, pub)
	if err != nil || addressErr != nil || sender != address {
		return common.Hash{}, errors.New("invalid relay operator signature")
	}
	return signed.Hash(), nil
}
