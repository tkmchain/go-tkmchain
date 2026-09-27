package gui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	endpoint, err := loopbackProverEndpoint(cfg.Listen)
	if err != nil {
		http.Error(w, `{"error":"Proof builder must listen on loopback"}`, 503)
		return
	}
	body := http.MaxBytesReader(w, r.Body, 4<<20)
	// Keep the request URL constant. The configured endpoint is used only by
	// the pinned dialer below, so it cannot flow into URL parsing or redirects.
	reqURL := url.URL{Scheme: "http", Host: "127.0.0.1", Path: path}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, reqURL.String(), body)
	if err != nil {
		http.Error(w, `{"error":"invalid proof request"}`, 400)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
	// Pin the connection to the validated loopback IP. Resolving a hostname
	// during the request would reintroduce DNS rebinding and turn this proxy
	// into an SSRF primitive even though the configuration was checked above.
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, endpoint)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req) // #nosec G704 -- transport is pinned to a validated numeric loopback endpoint
	if err != nil {
		http.Error(w, `{"error":"Local proof builder is unavailable or still loading. Try again shortly."}`, 503)
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	if _, err := io.Copy(w, io.LimitReader(res.Body, 8<<20)); err != nil {
		return
	}
}

// loopbackProverEndpoint accepts only numeric loopback addresses and a valid
// TCP port. In particular, localhost is intentionally rejected because its
// DNS answer can change between validation and connection.
func loopbackProverEndpoint(listen string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil || host == "" || strings.Contains(host, "%") {
		return "", errors.New("invalid loopback prover endpoint")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("prover endpoint is not loopback")
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return "", errors.New("invalid prover endpoint port")
	}
	return net.JoinHostPort(ip.String(), strconv.FormatUint(n, 10)), nil
}
