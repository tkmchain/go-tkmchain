package shield3wallet

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"golang.org/x/net/proxy"
)

// RelayTransportConfig controls the network path used to contact a relay.
// A SOCKS5 proxy is opt-in and is normally a local Tor daemon. We deliberately
// do not read HTTP_PROXY/HTTPS_PROXY: an ambient proxy can silently reveal the
// wallet's network origin.
type RelayTransportConfig struct {
	SOCKS5Proxy       string
	FixedRequestBytes int
	RequestDelay      time.Duration
	OnionOnly         bool
}

const DefaultRelayRequestBytes = 32 * 1024

var DirectRelayTransport = RelayTransportConfig{FixedRequestBytes: DefaultRelayRequestBytes}

func (c RelayTransportConfig) validate() error {
	if c.FixedRequestBytes == 0 {
		c.FixedRequestBytes = DefaultRelayRequestBytes
	}
	if c.FixedRequestBytes < 1024 || c.FixedRequestBytes > 256<<10 {
		return errors.New("relay fixed request size is outside the safe range")
	}
	if c.OnionOnly && c.SOCKS5Proxy == "" {
		return errors.New("onion-only relay mode requires an explicit SOCKS5 proxy")
	}
	if c.SOCKS5Proxy == "" {
		return nil
	}
	u, err := url.Parse(c.SOCKS5Proxy)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "socks5" || u.Hostname() == "" {
		return errors.New("relay SOCKS5 proxy must be a plain socks5://host:port URL")
	}
	if _, err := strconv.ParseUint(u.Port(), 10, 16); err != nil {
		return errors.New("relay SOCKS5 proxy requires a valid port")
	}
	return nil
}

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
func paddedJSON(request any, target int) ([]byte, error) {
	// Padding is an ignored top-level field inside the authenticated TLS request.
	// Keep the actual request fields at the top level so the relay can validate
	// them normally while observers see one fixed body size.
	base, err := json.Marshal(request)
	if err != nil || len(base) < 2 || base[0] != '{' || base[len(base)-1] != '}' {
		return nil, errors.New("relay request must be a JSON object")
	}
	for n := 0; n <= target; n++ {
		padding, _ := json.Marshal(strings.Repeat("0", n))
		body := make([]byte, 0, len(base)+len(padding)+12)
		body = append(body, base[:len(base)-1]...)
		if len(base) > 2 {
			body = append(body, ',')
		}
		body = append(body, []byte(`"padding":`)...)
		body = append(body, padding...)
		body = append(body, '}')
		if len(body) == target {
			return body, nil
		}
		if len(body) > target {
			break
		}
		if n == 0 {
			n = target - len(body) - 1
		}
	}
	return nil, errors.New("unable to construct fixed-size relay request")
}

func relayHTTP(ctx context.Context, endpoint, path string, request, result any) error {
	return relayHTTPWithConfig(ctx, endpoint, path, request, result, DirectRelayTransport)
}

func relayHTTPWithConfig(ctx context.Context, endpoint, path string, request, result any, config RelayTransportConfig) error {
	if err := ValidateRelayURL(endpoint); err != nil {
		return err
	}
	if config.FixedRequestBytes == 0 {
		config.FixedRequestBytes = DefaultRelayRequestBytes
	}
	if err := config.validate(); err != nil {
		return err
	}
	u, _ := url.Parse(endpoint)
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(u.Hostname()), "."))
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if strings.HasSuffix(host, ".onion") && config.SOCKS5Proxy == "" {
		return errors.New(".onion relays require an explicit SOCKS5 proxy")
	}
	if config.OnionOnly && !loopback && !strings.HasSuffix(host, ".onion") {
		return errors.New("onion-only relay mode rejects non-onion relay")
	}
	data, err := paddedJSON(request, config.FixedRequestBytes)
	if err != nil {
		return err
	}
	if config.RequestDelay > 0 {
		timer := time.NewTimer(config.RequestDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Do not send an authorization to a redirected host or through proxy env vars.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if config.SOCKS5Proxy != "" {
		proxyURL, _ := url.Parse(config.SOCKS5Proxy)
		dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, nil, proxy.Direct)
		if err != nil {
			return err
		}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialContext(ctx, dialer, network, address)
		}
	}
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

func dialContext(ctx context.Context, dialer proxy.Dialer, network, address string) (net.Conn, error) {
	result := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		conn, err := dialer.Dial(network, address)
		result <- struct {
			conn net.Conn
			err  error
		}{conn, err}
	}()
	select {
	case r := <-result:
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func FetchRelayOffer(ctx context.Context, endpoint, requestID string) (RelayOffer, error) {
	return FetchRelayOfferWithConfig(ctx, endpoint, requestID, DirectRelayTransport)
}

func FetchRelayOfferWithConfig(ctx context.Context, endpoint, requestID string, config RelayTransportConfig) (RelayOffer, error) {
	var offer RelayOffer
	err := relayHTTPWithConfig(ctx, endpoint, "/offer", struct {
		RequestID string `json:"requestId"`
	}{requestID}, &offer, config)
	return offer, err
}

// FetchRelayOfferAny selects the first usable relay. Selection is intentionally
// sequential: an offer reserves a relay nonce, so probing every relay in
// parallel would create unnecessary leases and link the wallet to all of them.
func FetchRelayOfferAny(ctx context.Context, endpoints []string, requestID string, config RelayTransportConfig) (RelayOffer, string, error) {
	order := append([]string(nil), endpoints...)
	for i := len(order) - 1; i > 0; i-- {
		bound := big.NewInt(int64(i + 1))
		choice, err := crand.Int(crand.Reader, bound)
		if err != nil {
			break
		}
		j := int(choice.Int64())
		order[i], order[j] = order[j], order[i]
	}
	var last error
	for _, endpoint := range order {
		offer, err := FetchRelayOfferWithConfig(ctx, endpoint, requestID, config)
		if err == nil {
			return offer, endpoint, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("no relay endpoints configured")
	}
	return RelayOffer{}, "", last
}

type RelayResponse struct {
	Transaction         hexutil.Bytes `json:"transaction"`
	TransactionHash     common.Hash   `json:"transactionHash"`
	Status              string        `json:"status"`
	SubmissionUncertain bool          `json:"submissionUncertain,omitempty"`
}

func SubmitRelayPacket(ctx context.Context, endpoint, requestID string, raw []byte) (RelayResponse, error) {
	return SubmitRelayPacketWithConfig(ctx, endpoint, requestID, raw, DirectRelayTransport)
}

func SubmitRelayPacketWithConfig(ctx context.Context, endpoint, requestID string, raw []byte, config RelayTransportConfig) (RelayResponse, error) {
	var response RelayResponse
	err := relayHTTPWithConfig(ctx, endpoint, "/submit", struct {
		Transaction hexutil.Bytes `json:"transaction"`
		RequestID   string        `json:"requestId"`
	}{raw, requestID}, &response, config)
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
