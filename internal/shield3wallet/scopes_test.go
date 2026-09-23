package shield3wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type receiptOnlyRPC struct {
	RPC
	spendCalls int
}

func (r *receiptOnlyRPC) CallContext(ctx context.Context, dest any, method string, args ...any) error {
	if method == "tkmprivacy_shieldedV3NullifierStatus" {
		r.spendCalls++
	}
	return r.RPC.CallContext(ctx, dest, method, args...)
}
func TestScopedViewKeyIsolation(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Shield3")
	}
	identity := testIdentity(t, make([]byte, 32))
	incoming, err := identity.ScopedViewKey("incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer incoming.Clear()
	if incoming.NullifierKey != nil || len(incoming.Outgoing) != 0 {
		t.Fatal("receive-only key leaked spend/outgoing permission")
	}
	full := identity.ViewKey()
	defer full.Clear()
	if full.NullifierKey == nil || *full.NullifierKey != identity.NullifierKey || full.validate() != nil {
		t.Fatal("incomplete full permission")
	}
	invalid := incoming
	invalid.NullifierKey = full.NullifierKey
	if invalid.validate() == nil {
		t.Fatal("accepted a misleading incoming scope with spend tracking")
	}
	invalid = full
	invalid.Version = 1
	if invalid.validate() == nil {
		t.Fatal("accepted obsolete key lacking private nullifier semantics")
	}
	original := common.CopyBytes(identity.IncomingSeed)
	defer clear(original)
	clear(incoming.Incoming)
	if !bytes.Equal(identity.IncomingSeed, original) {
		t.Fatal("exported key shared identity memory")
	}
}
func TestBatchAggregateLimit(t *testing.T) {
	identity := &Identity{ChainID: 8979, Stamp: &pqcrypto.ShieldedV3StampRecord{}}
	receiver := PaymentPayload{ChainID: 8979}
	half := new(big.Int).Div(shielded3.MaxSendWei(), big.NewInt(2))
	valid := []Payment{{receiver, half}, {receiver, half}}
	total, err := validatePayments(identity, valid)
	if err != nil || total.Cmp(shielded3.MaxSendWei()) != 0 {
		t.Fatal("rejected exact aggregate limit", err)
	}
	if _, err := validatePayments(identity, append(valid, Payment{receiver, big.NewInt(1)})); err == nil {
		t.Fatal("allowed aggregate above maximum")
	}
	for _, payments := range [][]Payment{nil, {{receiver, nil}}, {{receiver, new(big.Int)}}, {{receiver, big.NewInt(-1)}}, {{receiver, big.NewInt(1)}, {receiver, big.NewInt(1)}, {receiver, big.NewInt(1)}, {receiver, big.NewInt(1)}}} {
		if _, err := validatePayments(identity, payments); err == nil {
			t.Fatal("accepted invalid batch")
		}
	}
	crossChain := receiver
	crossChain.ChainID++
	if _, err := validatePayments(identity, []Payment{{crossChain, big.NewInt(1)}}); err == nil {
		t.Fatal("accepted cross-chain batch")
	}
	if _, err := BuildBatch(context.Background(), nil, nil, identity, []Payment{{receiver, new(big.Int).Add(shielded3.MaxSendWei(), common.Big1)}}); err == nil {
		t.Fatal("builder failed to enforce aggregate cap before RPC")
	}
}
func TestRelayTransportPrivacyPolicy(t *testing.T) {
	for _, proxyURL := range []string{"socks5://127.0.0.1:9050", "socks5://[::1]:9050"} {
		if err := (RelayTransportConfig{SOCKS5Proxy: proxyURL}).validate(); err != nil {
			t.Fatal(proxyURL, err)
		}
	}
	for _, proxyURL := range []string{"http://127.0.0.1:9050", "socks5://user:pass@127.0.0.1:9050", "socks5://127.0.0.1", "socks5://127.0.0.1:70000"} {
		if err := (RelayTransportConfig{SOCKS5Proxy: proxyURL}).validate(); err == nil {
			t.Fatal("accepted unsafe SOCKS5 proxy", proxyURL)
		}
	}
	body, err := paddedJSON(struct {
		RequestID string `json:"requestId"`
	}{"opaque-request-123456"}, DefaultRelayRequestBytes)
	if err != nil || len(body) != DefaultRelayRequestBytes {
		t.Fatalf("fixed-size padding failed: len=%d err=%v", len(body), err)
	}
	if !bytes.Contains(body, []byte(`"requestId":"opaque-request-123456"`)) || !bytes.Contains(body, []byte(`"padding":"`)) {
		t.Fatal("fixed-size request lost fields or padding")
	}
	largeRequest := struct {
		Transaction string `json:"transaction"`
	}{strings.Repeat("x", 40<<10)}
	target, err := relayRequestSize(largeRequest, DefaultRelayRequestBytes)
	if err != nil || target != 64<<10 {
		t.Fatalf("large relay request did not select the next fixed class: target=%d err=%v", target, err)
	}
	largeBody, err := paddedJSON(largeRequest, target)
	if err != nil || len(largeBody) != target {
		t.Fatalf("large relay request padding failed: len=%d target=%d err=%v", len(largeBody), target, err)
	}
	var decoded struct {
		Transaction string `json:"transaction"`
		Padding     string `json:"padding"`
	}
	if err := json.Unmarshal(largeBody, &decoded); err != nil || decoded.Transaction != largeRequest.Transaction || len(decoded.Padding) == 0 {
		t.Fatal("large relay request lost fields or padding", err)
	}
	if err := relayHTTPWithConfig(context.Background(), "https://example.onion", "/offer", struct{}{}, &struct{}{}, RelayTransportConfig{}); err == nil {
		t.Fatal("allowed onion relay without an explicit Tor proxy")
	}
}

func TestRelayURLPolicy(t *testing.T) {
	for _, endpoint := range []string{"https://relay.example", "https://relay.example/shield3", "http://127.0.0.1:8790", "http://[::1]:8790", "http://localhost:8790"} {
		if err := ValidateRelayURL(endpoint); err != nil {
			t.Fatal(endpoint, err)
		}
	}
	for _, endpoint := range []string{"http://relay.example", "ftp://relay.example", "https://user:pass@relay.example", "https://relay.example?token=secret", "https://relay.example#fragment", ""} {
		if ValidateRelayURL(endpoint) == nil {
			t.Fatal("accepted unsafe relay endpoint", endpoint)
		}
	}
}

func TestStampOnlyDisclosureCannotRecognizeOutput(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Shield3")
	}
	identity := testIdentity(t, make([]byte, 32))
	disclosure, err := ExportStampDisclosure(make([]byte, 32), identity)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(disclosure.RecordKey)
	result, err := VerifyStampDisclosure(disclosure)
	if err != nil || result.Name != "Private Name" || result.Country != "Private Country" || result.Address != identity.Address {
		t.Fatal("stamp record did not open", err)
	}
	context := pqcrypto.ShieldedV3Context{ChainID: 8979, Purpose: pqcrypto.ShieldedV3Stamp, Commitment: identity.Stamp.Commitment}
	other, err := pqcrypto.SealShieldedV3(identity.StampPublicKey, []byte("another output stamp"), context)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pqcrypto.OpenShieldedV3RecordKey(disclosure.RecordKey, other, context); err == nil {
		t.Fatal("one-record stamp key opened another stamp encryption")
	}
	if len(disclosure.RecordKey) != 32 {
		t.Fatal("exported master stamp seed")
	}
	changed := disclosure
	changed.Record = disclosure.Record
	changed.Record.ChainID++
	if _, err := VerifyStampDisclosure(changed); err == nil {
		t.Fatal("accepted altered signed stamp chain")
	}
}

func TestOnionOnlyRelayPolicy(t *testing.T) {
	cfg := RelayTransportConfig{OnionOnly: true, SOCKS5Proxy: "socks5://127.0.0.1:9050"}
	if err := relayHTTPWithConfig(context.Background(), "https://relay.example", "/offer", struct{}{}, &struct{}{}, cfg); err == nil {
		t.Fatal("onion-only relay policy accepted a clearnet endpoint")
	}
}
