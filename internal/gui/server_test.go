// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package gui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
)

// testRPCService provides a small set of methods the proxy test can call.
type testRPCService struct{}

func (s *testRPCService) Version() string { return "test/1.0" }

func (s *testRPCService) Echo(value string) (string, error) {
	return value, nil
}

func (s *testRPCService) Fail() (string, error) {
	return "", fmt.Errorf("boom")
}

type testEthService struct{}

func (s *testEthService) BlockNumber() string { return "0x0" }

// newTestServer starts a fake node RPC server plus a GUI bound to it.
func newTestServer(t *testing.T) (*GUI, string) {
	t.Helper()
	node := rpc.NewServer()
	if err := node.RegisterName("test", &testRPCService{}); err != nil {
		t.Fatalf("failed to register test service: %v", err)
	}
	if err := node.RegisterName("eth", &testEthService{}); err != nil {
		t.Fatalf("failed to register test eth service: %v", err)
	}
	client := rpc.DialInProc(node)

	g, err := New(client, Options{ForceBrowser: true, Port: 0})
	if err != nil {
		t.Fatalf("failed to create GUI: %v", err)
	}
	if err := g.listen(); err != nil {
		t.Fatalf("failed to start GUI server: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g, g.Endpoint()
}

func TestHealthRequiresRPCReady(t *testing.T) {
	g, url := newTestServer(t)
	resp, err := http.Get(url + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ready node returned %d", resp.StatusCode)
	}

	// Closing the attached client simulates the chain backend going away while
	// the GUI listener is still alive.  Health must become non-ready so clients
	// wait for a restart instead of showing an endless reconnect loop.
	g.client.Close()
	resp, err = http.Get(url + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz after close failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("closed node returned %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func callRPC(t *testing.T, url, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/rpc", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GUI-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rpc call failed: %v", err)
	}
	return resp
}

func TestIndexServesToken(t *testing.T) {
	g, url := newTestServer(t)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("gtkm")) {
		t.Errorf("index page missing brand text")
	}
	if !bytes.Contains(body, []byte(g.token)) {
		t.Errorf("index page missing injected token")
	}
	if bytes.Contains(body, []byte("__GUI_TOKEN__")) {
		t.Errorf("index page token placeholder was not substituted")
	}
}

func TestRPCSingle(t *testing.T) {
	g, url := newTestServer(t)
	resp := callRPC(t, url, g.token, `{"jsonrpc":"2.0","id":1,"method":"test_version","params":[]}`)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	var envelope struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("unexpected error: %v", envelope.Error.Message)
	}
	if envelope.Result != "test/1.0" {
		t.Fatalf("wrong result: %q", envelope.Result)
	}
}

func TestRPCBatch(t *testing.T) {
	g, url := newTestServer(t)
	body := `[
		{"jsonrpc":"2.0","id":1,"method":"test_version","params":[]},
		{"jsonrpc":"2.0","id":2,"method":"test_echo","params":["hello"]}
	]`
	resp := callRPC(t, url, g.token, body)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	var batch []map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
		t.Fatalf("failed to decode batch: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(batch))
	}
	if strings.TrimSpace(string(batch[0]["result"])) != `"test/1.0"` {
		t.Errorf("bad first result: %s", batch[0]["result"])
	}
	if strings.TrimSpace(string(batch[1]["result"])) != `"hello"` {
		t.Errorf("bad second result: %s", batch[1]["result"])
	}
}

func TestRPCError(t *testing.T) {
	g, url := newTestServer(t)
	resp := callRPC(t, url, g.token, `{"jsonrpc":"2.0","id":1,"method":"test_fail","params":[]}`)
	defer resp.Body.Close()
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if envelope.Error == nil {
		t.Fatal("expected an error envelope")
	}
	if !strings.Contains(envelope.Error.Message, "boom") {
		t.Errorf("unexpected error message: %s", envelope.Error.Message)
	}
}

func TestRPCTokenRequired(t *testing.T) {
	_, url := newTestServer(t)
	resp := callRPC(t, url, "badtoken", `{"jsonrpc":"2.0","id":1,"method":"test_version","params":[]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestAssetServing(t *testing.T) {
	_, url := newTestServer(t)
	for _, path := range []string{"/css/style.css", "/js/app.js", "/js/wallet.js",
		"/manifest.webmanifest", "/sw.js", "/icons/icon-192.png", "/icons/icon.svg"} {
		resp, err := http.Get(url + path)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("GET %s returned %d", path, resp.StatusCode)
		}
	}
}

func TestPWAHeaders(t *testing.T) {
	_, url := newTestServer(t)
	resp, err := http.Get(url + "/manifest.webmanifest")
	if err != nil {
		t.Fatalf("GET manifest failed: %v", err)
	}
	resp.Body.Close()

	index, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	index.Body.Close()
	if cc := index.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("index cache-control = %q, want no-store", cc)
	}
}

func TestWildcardHostURLIsLoopback(t *testing.T) {
	node := rpc.NewServer()
	if err := node.RegisterName("test", &testRPCService{}); err != nil {
		t.Fatalf("failed to register test service: %v", err)
	}
	client := rpc.DialInProc(node)
	g, err := New(client, Options{Host: "0.0.0.0", Port: 0})
	if err != nil {
		t.Fatalf("failed to create GUI: %v", err)
	}
	if err := g.listen(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	if !strings.HasPrefix(g.URL(), "http://127.0.0.1:") {
		t.Fatalf("wildcard bound URL should point at loopback for the local window, got %q", g.URL())
	}
}

func TestNotificationNoContent(t *testing.T) {
	g, url := newTestServer(t)
	resp := callRPC(t, url, g.token, `{"jsonrpc":"2.0","method":"test_version","params":[]}`)
	defer resp.Body.Close()
	// Notification requests (no id) get an empty 204 reply.
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}
