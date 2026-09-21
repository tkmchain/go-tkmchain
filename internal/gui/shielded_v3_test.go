package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/zk/shielded3"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestShield3PrivateEndpointBoundary(t *testing.T) {
	g := &GUI{token: "private-token"}
	for _, test := range []struct{ remote, host, origin, token string }{
		{"203.0.113.7:1234", "localhost:8080", "", "private-token"},
		{"127.0.0.1:1234", "attacker.example:8080", "", "private-token"},
		{"127.0.0.1:1234", "localhost:8080", "https://attacker.example", "private-token"},
		{"127.0.0.1:1234", "localhost:8080", "", "wrong-token"},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://"+test.host+"/shield3/viewkeys", nil)
		request.RemoteAddr = test.remote
		request.Host = test.host
		request.Header.Set("Origin", test.origin)
		request.Header.Set("X-GUI-Token", test.token)
		response := httptest.NewRecorder()
		g.handleShield3(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("private endpoint exposed: %+v status %d", test, response.Code)
		}
	}
}
func TestShield3SubmissionSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	g := &GUI{opts: Options{WalletStateDir: dir}}
	key, err := pqcrypto.NewMLDSA87FromSeed(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	signed, err := types.SignNewPQTkmTx(key, types.NewQuantumSigner(big.NewInt(8979)), &types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Gas: 7000000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Value: new(big.Int), Data: []byte(core.ShieldedV3Magic)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		t.Fatal(err)
	}
	record := &shield3RequestRecord{Digest: [64]byte{1}, Raw: raw, Hash: tx.Hash()}
	id := "stable-request-id"
	if err := g.saveShield3Submission(id, record); err != nil {
		t.Fatal(err)
	}
	restarted := &GUI{opts: Options{WalletStateDir: dir}}
	restored, err := restarted.loadShield3Submission(id)
	if err != nil || restored == nil || restored.Hash != record.Hash || restored.Digest != record.Digest {
		t.Fatalf("restart lost submission: %v", err)
	}
	restored.Raw[len(restored.Raw)-1] ^= 1
	if err := g.saveShield3Submission(id, restored); err != nil {
		t.Fatal(err)
	}
	saved, err := g.loadShield3Submission(id)
	if err != nil || saved.Hash != record.Hash || string(saved.Raw) != string(record.Raw) {
		t.Fatal("replaced exact signed bytes")
	}
	if err := os.WriteFile(g.shield3SubmissionPath(id), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.loadShield3Submission(id); err == nil {
		t.Fatal("accepted corrupt saved submission")
	}
}

// Existing durable requests must keep their pre-sponsorship canonical digest,
// otherwise an upgrade would prevent retrying the exact saved signed bytes.
func TestShield3ExistingRequestDigestUnchanged(t *testing.T) {
	request := shield3Request{Seed: []byte{1}, RequestID: "stable-request-id"}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte(`{"seed":"0x01","stamp":null,"account":"0x0000000000000000000000000000000000000000","recipient":"","amountWei":"","requestId":"stable-request-id","view":null}`)
	if !bytes.Equal(encoded, expected) {
		t.Fatalf("existing request digest changed: %s", encoded)
	}
}

func TestShield3UnsignedRelayDraftSurvivesRestart(t *testing.T) {
	key, err := pqcrypto.NewMLDSA87FromSeed(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	envelope := &core.ShieldedV3Transaction{Version: 3, Nullifier: shielded3.Digest{1}, InputCount: 1, WithdrawalValue: new(big.Int), GasSponsorValue: big.NewInt(7000000), Relayed: true, ValidUntil: 200}
	data, err := core.EncodeShieldedV3Transaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Gas: 7000000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Value: new(big.Int), Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(key), Data: data})
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	g := &GUI{opts: Options{WalletStateDir: t.TempDir()}}
	record := &shield3RequestRecord{Digest: [64]byte{1}, Raw: raw, Hash: tx.Hash(), Unsigned: true, DraftAccount: common.HexToAddress("0x1")}
	if err = g.saveShield3Submission("prepared-relay-request", record); err != nil {
		t.Fatal(err)
	}
	restored, err := g.loadShield3Submission("prepared-relay-request")
	if err != nil || restored == nil || !restored.Unsigned || restored.DraftAccount != record.DraftAccount || !bytes.Equal(restored.Raw, raw) {
		t.Fatal("lost relay authorization", err)
	}
	server := rpc.NewServer()
	defer server.Stop()
	eth := &relayTestEth{Time: 100}
	privacy := &relayTestPrivacy{}
	if err = server.RegisterName("eth", eth); err != nil {
		t.Fatal(err)
	}
	if err = server.RegisterName("tkmprivacy", privacy); err != nil {
		t.Fatal(err)
	}
	client := rpc.DialInProc(server)
	defer client.Close()
	restarted := &GUI{client: client, opts: g.opts}
	reserved, err := restarted.shield3ReservedRPC(context.Background(), record.DraftAccount)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		TransactionHash common.Hash `json:"transactionHash"`
		Pending         bool        `json:"pending"`
	}
	if err = reserved.CallContext(context.Background(), &state, "tkmprivacy_shieldedV3NullifierStatus", envelope.Nullifier); err != nil || !state.Pending || state.TransactionHash != record.Hash {
		t.Fatal("restart lost note reservation", err)
	}
	privacy.Hash = common.HexToHash("0xbeef")
	privacy.Pending = true
	if err = reserved.CallContext(context.Background(), &state, "tkmprivacy_shieldedV3NullifierStatus", envelope.Nullifier); err != nil || state.TransactionHash != privacy.Hash {
		t.Fatal("draft masked actual transaction hash", err)
	}
	privacy.Hash = common.Hash{}
	privacy.Pending = false
	eth.Time = 200
	released, err := restarted.shield3ReservedRPC(context.Background(), record.DraftAccount)
	if err != nil {
		t.Fatal(err)
	}
	if err = released.CallContext(context.Background(), &state, "tkmprivacy_shieldedV3NullifierStatus", envelope.Nullifier); err != nil || state.Pending {
		t.Fatal("expired draft kept note reserved", err)
	}
	if !shield3DraftExpired(restored, 200) || shield3DraftExpired(restored, 199) {
		t.Fatal("wrong draft expiry boundary")
	}
	record.Unsigned = false
	encoded, _ := json.Marshal(record)
	if err = os.WriteFile(g.shield3SubmissionPath("prepared-relay-request"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = g.loadShield3Submission("prepared-relay-request"); err == nil {
		t.Fatal("accepted unsigned transaction as signed submission")
	}
}

type relayTestEth struct{ Time uint64 }

func (s *relayTestEth) GetBlockByNumber(string, bool) map[string]hexutil.Uint64 {
	return map[string]hexutil.Uint64{"timestamp": hexutil.Uint64(s.Time)}
}

type relayTestPrivacy struct {
	Hash    common.Hash
	Pending bool
}

func (s *relayTestPrivacy) ShieldedV3NullifierStatus(shielded3.Digest) map[string]any {
	return map[string]any{"transactionHash": s.Hash, "pending": s.Pending}
}
