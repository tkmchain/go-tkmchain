package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	xproxy "golang.org/x/net/proxy"
)

const emailProxyBodyLimit = 16 << 20

type emailProxy struct {
	remote *url.URL
	socks5 string

	listener net.Listener
	server   *http.Server
	client   *http.Client
	origin   string
	mu       sync.RWMutex
}

func newEmailProxy(rawRemote, rawSOCKS5 string) (*emailProxy, error) {
	remote, err := url.Parse(strings.TrimSpace(rawRemote))
	if err != nil || (remote.Scheme != "https" && remote.Scheme != "http") || remote.Host == "" {
		return nil, errors.New("EmailVM URL must be an absolute http(s) URL")
	}
	// The standalone launcher can never be used to open the full wallet. Force
	// the shared frontend into its email-only mode even for custom endpoints.
	remoteQuery := remote.Query()
	remoteQuery.Set("app", "email")
	remote.RawQuery = remoteQuery.Encode()
	socks, err := url.Parse(strings.TrimSpace(rawSOCKS5))
	if err != nil || socks.Scheme != "socks5" || socks.Hostname() == "" || socks.Port() == "" || socks.User != nil || socks.RawQuery != "" || socks.Fragment != "" {
		return nil, errors.New("Tor proxy must be a plain socks5://host:port URL")
	}
	if port, err := strconv.Atoi(socks.Port()); err != nil || port < 1 || port > 65535 {
		return nil, errors.New("Tor proxy port is invalid")
	}
	return &emailProxy{remote: remote, socks5: socks.String()}, nil
}

func (p *emailProxy) Start() error {
	socks, err := url.Parse(p.socks5)
	if err != nil {
		return err
	}
	dialer, err := xproxy.SOCKS5("tcp", socks.Host, nil, xproxy.Direct)
	if err != nil {
		return fmt.Errorf("configure Tor SOCKS5 proxy: %w", err)
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConnsPerHost:   4,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		ExpectContinueTimeout: 1 * time.Second,
	}
	p.client = &http.Client{Transport: transport, Timeout: 2 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("bind local EmailVM client: %w", err)
	}
	p.listener = listener
	p.origin = "http://" + listener.Addr().String()
	p.server = &http.Server{
		Handler:           http.HandlerFunc(p.handle),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() { _ = p.server.Serve(listener) }()
	return nil
}

func (p *emailProxy) URL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.origin + "/"
}

func (p *emailProxy) Close() error {
	if p.server != nil {
		return p.server.Shutdown(context.Background())
	}
	return nil
}

func (p *emailProxy) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete && r.Method != http.MethodOptions {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.ContentLength > emailProxyBodyLimit {
		http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
		return
	}
	remote, err := p.upstreamRequest(r.URL.RequestURI())
	if err != nil {
		http.Error(w, "invalid upstream request", http.StatusBadRequest)
		return
	}

	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, emailProxyBodyLimit+1)
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, remote.String(), body)
	if err != nil {
		http.Error(w, "invalid upstream request", http.StatusBadRequest)
		return
	}
	for key, values := range r.Header {
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Connection") || strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Accept-Encoding") {
			continue
		}
		for _, value := range values {
			if strings.EqualFold(key, "Origin") && strings.HasPrefix(value, p.origin) {
				value = p.remote.Scheme + "://" + p.remote.Host
			}
			if strings.EqualFold(key, "Referer") && strings.HasPrefix(value, p.origin) {
				value = strings.Replace(value, p.origin, p.remote.Scheme+"://"+p.remote.Host, 1)
			}
			request.Header.Add(key, value)
		}
	}
	// Identity responses let us safely rewrite absolute links and redirects to
	// the loopback origin. The browser never receives a URL that bypasses Tor.
	request.Header.Set("Accept-Encoding", "identity")
	response, err := p.client.Do(request)
	if err != nil {
		http.Error(w, "Tor upstream unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, emailProxyBodyLimit+1))
	if err != nil || len(data) > emailProxyBodyLimit {
		http.Error(w, "upstream response is too large", http.StatusBadGateway)
		return
	}

	contentType := response.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "json") {
		data = replaceBytes(data, []byte(p.remote.String()), []byte(p.origin))
		data = replaceBytes(data, []byte(p.remote.Scheme+"://"+p.remote.Host), []byte(p.origin))
	}
	for key, values := range response.Header {
		if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Content-Encoding") || strings.EqualFold(key, "Transfer-Encoding") || strings.EqualFold(key, "Connection") {
			continue
		}
		for _, value := range values {
			if strings.EqualFold(key, "Location") {
				value = p.localizeLocation(value)
			}
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (p *emailProxy) localizeLocation(raw string) string {
	location, err := url.Parse(raw)
	if err != nil || location.Host != p.remote.Host {
		return raw
	}
	query := ""
	if location.RawQuery != "" {
		query = "?" + location.RawQuery
	}
	return p.origin + "/" + strings.TrimLeft(location.Path, "/") + query
}

func (p *emailProxy) upstreamRequest(target string) (*url.URL, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	path := parsed.Path
	if path == "" {
		path = "/"
	}
	base := *p.remote
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if path == "/" {
		query := parsed.Query()
		if parsed.RawQuery == "" {
			query = p.remote.Query()
		}
		query.Set("app", "email")
		base.RawQuery = query.Encode()
	} else if parsed.RawQuery != "" {
		base.RawQuery = parsed.RawQuery
	} else {
		base.RawQuery = ""
	}
	base.Fragment = ""
	return &base, nil
}

func replaceBytes(input, old, replacement []byte) []byte {
	return []byte(strings.ReplaceAll(string(input), string(old), string(replacement)))
}
