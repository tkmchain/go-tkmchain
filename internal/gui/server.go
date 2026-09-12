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

// Package gui provides an embedded web dashboard for the gtkm node. It serves
// a bundled HTML/JS application over loopback HTTP (configurable host) and
// proxies JSON-RPC calls to an attached RPC client. The same web UI can be
// rendered in a native desktop window (built with the "gtkmgui" build tag), in
// a regular browser, or installed as a progressive web app (PWA) on mobile.
package gui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

//go:embed web
var webAssets embed.FS

// Options controls how the GUI is launched.
type Options struct {
	ProverConfig string // private local proof-only prover configuration

	Title         string // window title
	Width, Height int    // initial desktop window size
	Port          int    // HTTP port (0 = random free port)
	Host          string // bind address for the dashboard (default 127.0.0.1)
	ForceBrowser  bool   // always open the dashboard in the default browser
}

// GUI is an embedded web dashboard bound to a local JSON-RPC client.
type GUI struct {
	client *rpc.Client
	opts   Options

	server   *http.Server
	listener net.Listener
	token    string

	closed chan struct{}
	once   sync.Once
}

// New creates a GUI that proxies RPC to the given client. The dashboard is
// not started until Run is called.
func New(client *rpc.Client, opts Options) (*GUI, error) {
	if client == nil {
		return nil, fmt.Errorf("gui: nil RPC client")
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	if opts.Title == "" {
		opts.Title = "gtkm"
	}
	if opts.Width <= 0 {
		opts.Width = 1280
	}
	if opts.Height <= 0 {
		opts.Height = 800
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	return &GUI{
		client: client,
		opts:   opts,
		token:  token,
		closed: make(chan struct{}),
	}, nil
}

// Run starts the HTTP server, then opens the dashboard either in a native
// desktop window or in the default browser. It blocks until the dashboard is
// closed or ctx is cancelled.
func (g *GUI) Run(ctx context.Context) error {
	if err := g.listen(); err != nil {
		return err
	}
	g.logStartup()

	if g.opts.ForceBrowser {
		openExternal(g.URL())
		return g.wait(ctx)
	}
	return openDesktopWindow(g.URL(), g.opts)
}

// logStartup prints the dashboard URL once the server is listening, plus a
// warning when the dashboard is reachable beyond the loopback interface.
func (g *GUI) logStartup() {
	log.Info("GUI dashboard", "url", g.URL())
	if g.opts.Host != "127.0.0.1" && g.opts.Host != "::1" && g.opts.Host != "localhost" {
		log.Warn("GUI dashboard is exposed on the network; anyone on it can read the RPC token and call node methods", "host", g.opts.Host)
	}
}

// Close stops the HTTP server and releases resources.
func (g *GUI) Close() error {
	var err error
	g.once.Do(func() {
		close(g.closed)
		if g.server != nil {
			if err2 := g.server.Shutdown(context.Background()); err2 != nil {
				err = err2
			}
		}
		if g.listener != nil {
			g.listener.Close()
		}
	})
	return err
}

// URL returns the dashboard URL once the server is running. When the dashboard
// is bound to a wildcard address, the URL points at loopback for the local
// desktop window/browser.
func (g *GUI) URL() string {
	if g.listener == nil {
		return ""
	}
	host, port, err := net.SplitHostPort(g.listener.Addr().String())
	if err != nil {
		return fmt.Sprintf("http://%s/", g.listener.Addr())
	}
	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s/", net.JoinHostPort(host, port))
}

// Endpoint returns the dashboard URL once the server is running.
func (g *GUI) Endpoint() string {
	return g.URL()
}

// listen binds the HTTP listener (loopback by default) and starts serving.
func (g *GUI) listen() error {
	host := g.opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := "0"
	if g.opts.Port > 0 {
		port = strconv.Itoa(g.opts.Port)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("gui: failed to bind listener: %v", err)
	}
	g.listener = ln

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", g.handleRPC)
	mux.HandleFunc("/prover/", g.handleProver)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.Handle("/", g.handleAssets())

	g.server = &http.Server{Handler: mux}
	go g.server.Serve(ln)
	return nil
}

// handleAssets serves the embedded dashboard files. The token is injected into
// every HTML response so the frontend can authenticate RPC calls.
func (g *GUI) handleAssets() http.Handler {
	sub, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.webmanifest":
			w.Header().Set("Content-Type", "application/manifest+json")
		case "/sw.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		if strings.HasSuffix(r.URL.Path, "/") || r.URL.Path == "/index.html" || r.URL.Path == "/" {
			data, err := fs.ReadFile(sub, "index.html")
			if err == nil {
				page := strings.ReplaceAll(string(data), "__GUI_TOKEN__", g.token)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Write([]byte(page))
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fileServer.ServeHTTP(w, r)
	})
}

// jsonRPCRequest mirrors a single JSON-RPC request envelope.
type jsonRPCRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      json.RawMessage   `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

// jsonRPCResponse is the envelope returned to the frontend.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// handleRPC proxies JSON-RPC calls to the attached node client. It supports
// both single requests and batch arrays, mirroring the geth transport rules.
func (g *GUI) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-GUI-Token") != g.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var responses []jsonRPCResponse
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		var batch []jsonRPCRequest
		if err := json.Unmarshal(body, &batch); err != nil {
			http.Error(w, "invalid batch", http.StatusBadRequest)
			return
		}
		for _, req := range batch {
			responses = append(responses, g.dispatch(ctx, req))
		}
	} else {
		var req jsonRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		responses = append(responses, g.dispatch(ctx, req))
	}

	w.Header().Set("Content-Type", "application/json")
	single := len(responses) == 1 && !strings.HasPrefix(trimmed, "[")
	if single {
		if responses[0].ID == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		b, _ := json.Marshal(responses[0])
		w.Write(b)
		return
	}
	json.NewEncoder(w).Encode(responses)
}

// dispatch forwards a single request to the node client.
func (g *GUI) dispatch(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		// Notification: no reply expected.
	}
	if req.Method == "" {
		resp.Error = &rpcError{Code: -32600, Message: "invalid request"}
		return resp
	}
	args := make([]interface{}, len(req.Params))
	for i, p := range req.Params {
		args[i] = p
	}
	var out json.RawMessage
	err := g.client.CallContext(ctx, &out, req.Method, args...)
	if err != nil {
		resp.Error = rpcErrorFrom(err)
		return resp
	}
	if out == nil {
		out = json.RawMessage("null")
	}
	resp.Result = out
	return resp
}

// rpcErrorFrom converts a client error into a JSON-RPC error envelope.
func rpcErrorFrom(err error) *rpcError {
	if err == nil {
		return nil
	}
	rpcerr := rpc.Error(nil)
	if ok := errors.As(err, &rpcerr); ok {
		return &rpcError{Code: rpcerr.ErrorCode(), Message: rpcerr.Error()}
	}
	if errors.Is(err, context.Canceled) {
		return &rpcError{Code: -32603, Message: "request cancelled"}
	}
	return &rpcError{Code: -32603, Message: err.Error()}
}

// wait blocks until the GUI is closed or the context is cancelled.
func (g *GUI) wait(ctx context.Context) error {
	select {
	case <-g.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// randomToken returns a random hex token used to authenticate RPC calls.
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("gui: failed to generate token: %v", err)
	}
	return hex.EncodeToString(buf), nil
}
