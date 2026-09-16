package gui

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type scopeTestChainAPI struct{}

func (*scopeTestChainAPI) ChainId() *hexutil.Big { return (*hexutil.Big)(big.NewInt(8979)) }
func TestShield3EndpointScopesDoNotExportMasterStampKey(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Shield3")
	}
	seed := make([]byte, 32)
	stamp, err := pqcrypto.CreateShieldedV3Stamp(seed, 8979, "Private Name", "Private Country")
	if err != nil {
		t.Fatal(err)
	}
	server := rpc.NewServer()
	defer server.Stop()
	if err := server.RegisterName("eth", new(scopeTestChainAPI)); err != nil {
		t.Fatal(err)
	}
	client := rpc.DialInProc(server)
	defer client.Close()
	gui := &GUI{client: client, token: "private-token"}
	call := func(operation string, request shield3Request) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		httpRequest := httptest.NewRequest(http.MethodPost, "http://localhost:8080/shield3/"+operation, bytes.NewReader(data))
		httpRequest.RemoteAddr = "127.0.0.1:1234"
		httpRequest.Header.Set("X-GUI-Token", gui.token)
		result := httptest.NewRecorder()
		gui.handleShield3(result, httpRequest)
		return result
	}
	for _, scope := range []string{"incoming", "full", "stamp"} {
		response := call("viewkeys", shield3Request{Seed: seed, Stamp: stamp, Scope: scope})
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", scope, response.Code, response.Body.String())
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		if fields["stampKey"] != nil || fields["seed"] != nil {
			t.Fatal("exported a master stamp/spending seed")
		}
		if scope == "stamp" {
			var disclosure shield3wallet.StampDisclosure
			if err := json.Unmarshal(response.Body.Bytes(), &disclosure); err != nil {
				t.Fatal(err)
			}
			if len(disclosure.RecordKey) != 32 || disclosure.Scope != "stamp" {
				t.Fatal("stamp-only permission exposed wrong key")
			}
			opened := call("view-stamp", shield3Request{StampDisclosure: &disclosure})
			if opened.Code != 200 || !bytes.Contains(opened.Body.Bytes(), []byte("Private Name")) {
				t.Fatal("could not verify one-record stamp permission")
			}
			continue
		}
		var exported struct {
			ViewKey shield3wallet.ViewKey `json:"viewKey"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &exported); err != nil {
			t.Fatal(err)
		}
		defer exported.ViewKey.Clear()
		if exported.ViewKey.Scope != scope {
			t.Fatal("wrong exported scope")
		}
		if scope == "incoming" && (len(exported.ViewKey.Outgoing) != 0 || exported.ViewKey.NullifierKey != nil) {
			t.Fatal("incoming export leaked outgoing/spend permission")
		}
		if scope == "full" && exported.ViewKey.NullifierKey == nil {
			t.Fatal("full export omitted spend tracking")
		}
	}
	if response := call("viewkeys", shield3Request{Seed: seed, Stamp: stamp, Scope: "unknown"}); response.Code != 400 {
		t.Fatal("accepted unknown scope")
	}
}
