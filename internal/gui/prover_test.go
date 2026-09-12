package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProverProxyAuthenticationAndBoundary(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Header.Get("Authorization") != "Bearer fixture-only" {
			t.Error("missing private authorization")
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	config := filepath.Join(t.TempDir(), "prover.json")
	data, _ := json.Marshal(map[string]string{"listen": strings.TrimPrefix(upstream.URL, "http://"), "bearerToken": "fixture-only"})
	os.WriteFile(config, data, 0600)
	g := &GUI{token: "gui-fixture", opts: Options{ProverConfig: config}}
	for _, tc := range []struct {
		path, token string
		status      int
	}{{"/prover/healthz", "", 403}, {"/prover/private", "gui-fixture", 400}, {"/prover/healthz", "gui-fixture", 200}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("X-GUI-Token", tc.token)
		w := httptest.NewRecorder()
		g.handleProver(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if strings.Contains(w.Body.String(), "fixture-only") {
			t.Fatal("leaked prover credential")
		}
	}
	if !called {
		t.Fatal("did not forward health request")
	}
	for _, token := range []string{"", "gui-fixture"} {
		r := httptest.NewRequest("POST", "/prover/build-withdrawal", strings.NewReader(`{"applicationData":"0x1234"}`))
		r.Header.Set("X-GUI-Token", token)
		w := httptest.NewRecorder()
		g.handleProver(w, r)
		want := 200
		if token == "" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("withdrawal proxy: got %d want %d", w.Code, want)
		}
	}
	os.WriteFile(config, []byte(`{"listen":"example.com:8787"}`), 0600)
	r := httptest.NewRequest("GET", "/prover/healthz", nil)
	r.Header.Set("X-GUI-Token", "gui-fixture")
	w := httptest.NewRecorder()
	g.handleProver(w, r)
	if w.Code != 503 {
		t.Fatal("accepted nonlocal prover")
	}
}
