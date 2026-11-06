package gui

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShield6GUIPrivateBoundary(t *testing.T) {
	g := &GUI{token: "private-token"}
	body := []byte(`{"proofVersion":6,"requestId":"stable-request-id"}`)
	for _, remote := range []string{"203.0.113.7:1234", "127.0.0.1:1234"} {
		r := httptest.NewRequest(http.MethodPost, "http://localhost:8080/shield3/relay-status", bytes.NewReader(body))
		r.RemoteAddr = remote
		r.Header.Set("X-GUI-Token", g.token)
		w := httptest.NewRecorder()
		g.handleShield3(w, r)
		if remote == "127.0.0.1:1234" {
			if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"found":false`)) {
				t.Fatal(w.Code, w.Body.String())
			}
		} else if w.Code != 403 {
			t.Fatal("Shield6 endpoint exposed remotely")
		}
	}
}

func TestShield6GUISubmissionPersistsAndIsAccountBound(t *testing.T) {
	g := &GUI{token: "private-token", opts: Options{WalletStateDir: t.TempDir()}}
	key, err := pqcrypto.NewMLDSA87FromSeed(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	data, err := core.EncodeShieldedV6Transaction(&core.ShieldedV6Transaction{Version: core.ShieldedV6BindingVersion, AssetID: 1, Binding: &core.ShieldedV6BindingRegistration{Commitment: shielded3.Digest{1}}})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := types.SignNewPQTkmTx(key, types.NewQuantumSigner(big.NewInt(8979)), &types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: 5_000_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Data: data})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	id := "shield6/stable-request-id"
	record := &shield3RequestRecord{Digest: [64]byte{1}, Raw: raw, Hash: signed.Hash()}
	if err := g.saveShield3Submission(id, record); err != nil {
		t.Fatal(err)
	}
	restored, err := g.loadShield3Submission(id)
	if err != nil || restored == nil || !bytes.Equal(restored.Raw, raw) {
		t.Fatal("lost exact Shield6 signed transaction", err)
	}
	body, err := json.Marshal(shield3Request{ProofVersion: 6, RequestID: "stable-request-id", Account: common.Address{99}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://localhost:8080/shield3/relay-status", bytes.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-GUI-Token", g.token)
	w := httptest.NewRecorder()
	g.handleShield3(w, r)
	if w.Code != 400 || !bytes.Contains(w.Body.Bytes(), []byte("account mismatch")) {
		t.Fatal("status crossed account boundary", w.Code, w.Body.String())
	}
}
