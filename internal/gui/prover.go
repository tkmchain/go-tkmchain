package gui

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// handleProver authenticates the GUI and forwards only proof operations to the
// loopback prover. Its credential never enters the browser or a log.
func (g *GUI) handleProver(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("X-GUI-Token") != g.token {
		http.Error(w, `{"error":"forbidden"}`, 403)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/prover")
	if !(path == "/healthz" && r.Method == "GET" || (path == "/build-deposit" || path == "/build-transfer" || path == "/build-withdrawal") && r.Method == "POST") {
		http.Error(w, `{"error":"unsupported proof operation"}`, 400)
		return
	}
	data, err := os.ReadFile(g.opts.ProverConfig)
	var cfg struct {
		Listen      string `json:"listen"`
		BearerToken string `json:"bearerToken"`
	}
	if err != nil || json.Unmarshal(data, &cfg) != nil {
		http.Error(w, `{"error":"Local proof builder is still being set up. Try again shortly."}`, 503)
		return
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost") {
		http.Error(w, `{"error":"Proof builder must listen on loopback"}`, 503)
		return
	}
	body := http.MaxBytesReader(w, r.Body, 4<<20)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://"+cfg.Listen+path, body)
	if err != nil {
		http.Error(w, `{"error":"invalid proof request"}`, 400)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
	client := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		http.Error(w, `{"error":"Local proof builder is unavailable or still loading. Try again shortly."}`, 503)
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	io.Copy(w, io.LimitReader(res.Body, 8<<20))
}
