package shield3relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type operatorRPC struct {
	identity *shield3wallet.Identity
	nonce    uint64
	head     uint64
	lost     bool
	sent     [][]byte
}

func (r *operatorRPC) CallContext(_ context.Context, dest any, method string, args ...any) error {
	var result any
	switch method {
	case "tkmprivacy_shieldedV3Status":
		result = map[string]any{"active": true, "nativeVerifier": true}
	case "tkmprivacy_antarticalStamp":
		result = core.AntarticalStampStatus{Registered: true, Owner: r.identity.Owner, Commitment: r.identity.Stamp.Commitment}
	case "eth_getBlockByNumber":
		result = map[string]any{"number": "0x0", "timestamp": hexutil.EncodeUint64(r.head), "hash": common.HexToHash("0x1234")}
	case "eth_getTransactionCount":
		result = hexutil.EncodeUint64(r.nonce)
	case "eth_gasPrice":
		result = "0x1"
	case "tkmprivacy_shieldedV3Paths":
		outputs := args[0].([]shielded3.Digest)
		paths := make([]core.ShieldedV3Path, len(outputs))
		for i, c := range outputs {
			paths[i].Commitment = c
		}
		result = paths
	case "tkmprivacy_shieldedV3RootsKnown":
		result = true
	case "tkmprivacy_shieldedV3NullifierStatus":
		result = map[string]any{"transactionHash": common.Hash{}, "pending": false}
	case "eth_sendRawTransaction":
		raw := []byte(args[0].(hexutil.Bytes))
		r.sent = append(r.sent, common.CopyBytes(raw))
		if r.lost {
			return errors.New("simulated lost node reply")
		}
		var tx types.Transaction
		if err := tx.UnmarshalBinary(raw); err != nil {
			return err
		}
		result = tx.Hash()
	default:
		return errors.New("unexpected RPC in relay test")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}
func operator(t *testing.T) (*Service, *operatorRPC, []byte, string) {
	t.Helper()
	if !shielded3.NativeAvailable() {
		t.Skip("requires -tags shield3")
	}
	seed := make([]byte, 32)
	stamp, err := pqcrypto.CreateShieldedV3Stamp(seed, 8979, "Operator", "Private Country")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := shield3wallet.NewIdentity(seed, 8979, stamp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(identity.Clear)
	rpc := &operatorRPC{identity: identity, head: params.MainnetAntarticalTime}
	dir := t.TempDir()
	service, err := New(rpc, seed, identity, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	return service, rpc, seed, dir
}
func syntheticDraft(t *testing.T, s *Service) (*types.Transaction, []byte, []byte) {
	t.Helper()
	e := &core.ShieldedV3Transaction{Version: 3, Relayed: true, InputCount: 1, Nullifier: shielded3.Digest{1}, WithdrawalValue: new(big.Int), GasSponsorValue: big.NewInt(7_000_000), ValidUntil: params.MainnetAntarticalTime + 3600}
	data, err := core.EncodeShieldedV3Transaction(e)
	if err != nil {
		t.Fatal(err)
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(s.seed)
	if err != nil {
		t.Fatal(err)
	}
	draft := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Gas: 7_000_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Value: new(big.Int), Data: data, Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(key)})
	signed, err := types.SignPQTkmTx(draft, types.NewQuantumSigner(big.NewInt(8979)), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := draft.MarshalBinary()
	signedRaw, _ := signed.MarshalBinary()
	return draft, raw, signedRaw
}
func TestDurableReplayLostReplyAndRestart(t *testing.T) {
	service, rpc, seed, dir := operator(t)
	draft, raw, signedRaw := syntheticDraft(t, service)
	requestID := "opaque-payment-request-1"
	// Seed an already validated durable record to exercise the retry path without
	// duplicating native proving. The separate vector test exercises first signing.
	if err := service.persist(filepath.Join(service.dir, "nonce-0.json"), record{RequestID: requestID, DraftHash: draft.Hash(), Transaction: signedRaw}); err != nil {
		t.Fatal(err)
	}
	rpc.lost = true
	first, err := service.submit(context.Background(), requestID, raw)
	if err != nil || !first.SubmissionUncertain {
		t.Fatal("lost reply did not return stable hash", err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(rpc, seed, rpc.identity, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	rpc.lost = false
	second, err := restarted.submit(context.Background(), requestID, raw)
	if err != nil || second.TransactionHash != first.TransactionHash || !bytes.Equal(second.Transaction, first.Transaction) {
		t.Fatal("restart changed signed payment", err)
	}
	if len(rpc.sent) != 2 || !bytes.Equal(rpc.sent[0], rpc.sent[1]) {
		t.Fatal("retry broadcast different bytes")
	}
	if _, err := restarted.submit(context.Background(), "different-request-id", raw); err == nil {
		t.Fatal("accepted another request at reserved nonce")
	}
	rpc.nonce = 1
	if _, err := restarted.offer(context.Background(), requestID); err == nil {
		t.Fatal("offered another nonce for an already signed request")
	}
	_, pub, _, _ := draft.PQTkmFields()
	next := types.NewTx(&types.PQTkmTx{ChainID: draft.ChainId(), Nonce: 1, To: draft.To(), Gas: draft.Gas(), GasFeeCap: draft.GasFeeCap(), GasTipCap: draft.GasTipCap(), Value: draft.Value(), Data: draft.Data(), Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pub})
	nextRaw, _ := next.MarshalBinary()
	if _, err := restarted.submit(context.Background(), requestID, nextRaw); err == nil {
		t.Fatal("reused one request ID to sign a second nonce")
	}

	raw[len(raw)-1] ^= 1
	if _, err := restarted.submit(context.Background(), requestID, raw); err == nil {
		t.Fatal("accepted changed payment at reserved nonce")
	}
}
func TestQuoteLeaseAndEndpointBoundary(t *testing.T) {
	service, rpc, seed, dir := operator(t)
	if duplicate, err := New(rpc, seed, rpc.identity, dir); err == nil {
		duplicate.Close()
		t.Fatal("allowed concurrent operator process")
	}
	first, err := service.offer(context.Background(), "opaque-quote-request-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.offer(context.Background(), "opaque-quote-request-1")
	if err != nil || !bytes.Equal(first.Signature, second.Signature) {
		t.Fatal("quote retry did not return exact saved offer", err)
	}
	if _, err := service.offer(context.Background(), "opaque-quote-request-2"); err == nil {
		t.Fatal("leased same nonce to concurrent payers")
	}
	rpc.head = first.ValidUntil
	if _, err := service.offer(context.Background(), "opaque-quote-request-2"); err != nil {
		t.Fatal("expired lease did not release operator", err)
	}
	server := httptest.NewServer(service)
	defer server.Close()
	for _, origin := range []string{"https://attacker.example", "http://localhost"} {
		request, _ := http.NewRequest(http.MethodPost, server.URL+"/offer", bytes.NewBufferString(`{"requestId":"opaque-quote-request-2"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatal("accepted browser-driven operator request")
		}
	}
	if _, err := shield3wallet.FetchRelayOffer(context.Background(), server.URL, "opaque-quote-request-2"); err != nil {
		t.Fatal("wallet could not fetch operator quote", err)
	}
}
func TestRealBatchRelayFirstSigningAndExactRetry(t *testing.T) {
	dir := os.Getenv("TKM_SHIELD3_RELAY_TESTDATA")
	if dir == "" {
		t.Skip("native wallet integration must generate the batch relay vector first")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "draft.bin"))
	if err != nil {
		t.Fatal(err)
	}
	service, rpc, _, _ := operator(t)
	var draft types.Transaction
	if err := draft.UnmarshalBinary(raw); err != nil {
		t.Fatal(err)
	}
	rpc.nonce = draft.Nonce()
	requestID := "native-batch-payment-request"
	if _, err := service.offer(context.Background(), requestID); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service)
	defer server.Close()
	rpc.lost = true
	first, err := shield3wallet.SubmitRelayPacket(context.Background(), server.URL, requestID, raw)
	if err != nil || !first.SubmissionUncertain {
		t.Fatal("first native signing lost stable transaction", err)
	}
	rpc.lost = false
	second, err := shield3wallet.SubmitRelayPacket(context.Background(), server.URL, requestID, raw)
	if err != nil || first.TransactionHash != second.TransactionHash || !bytes.Equal(first.Transaction, second.Transaction) {
		t.Fatal("native retry changed exact payment", err)
	}
	if len(rpc.sent) != 2 || !bytes.Equal(rpc.sent[0], rpc.sent[1]) {
		t.Fatal("native retry changed broadcast bytes")
	}
}
