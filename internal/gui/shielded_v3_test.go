package gui

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"

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
