package gui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

const maxGUIBootstrapArchiveSize = int64(512) << 30

type bootstrapDownloadResult struct {
	Filename string `json:"filename"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Gzip     bool   `json:"gzip"`
}

// handleBootstrap downloads the verified chain archive into the node's
// bootstrap directory. Authentication uses the same per-window token as RPC.
func (g *GUI) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "{\"error\":\"method not allowed\"}", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-GUI-Token") != g.token {
		http.Error(w, "{\"error\":\"forbidden\"}", http.StatusForbidden)
		return
	}
	if g.opts.BootstrapDir == "" || g.opts.BootstrapURL == "" {
		http.Error(w, "{\"error\":\"bootstrap download is not configured\"}", http.StatusServiceUnavailable)
		return
	}
	result, err := downloadGUIBootstrap(r.Context(), guiBootstrapConfig{
		URL:       g.opts.BootstrapURL,
		SHA256:    g.opts.BootstrapSHA256,
		SOCKS5:    g.opts.BootstrapSOCKS5,
		Directory: g.opts.BootstrapDir,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("{\"error\":%q}", err.Error()), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

type guiBootstrapConfig struct {
	URL       string
	SHA256    string
	SOCKS5    string
	Directory string
}

func downloadGUIBootstrap(ctx context.Context, cfg guiBootstrapConfig) (bootstrapDownloadResult, error) {
	var result bootstrapDownloadResult
	rawURL := strings.TrimSpace(cfg.URL)
	archiveURL, err := url.Parse(rawURL)
	if err != nil || archiveURL.Host == "" || (archiveURL.Scheme != "http" && archiveURL.Scheme != "https") {
		return result, errors.New("bootstrap URL must be an http(s) URL")
	}
	socks5 := strings.TrimSpace(cfg.SOCKS5)
	if socks5 == "" {
		socks5 = "socks5://127.0.0.1:9050"
	}
	client, err := guiBootstrapHTTPClient(socks5)
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(cfg.Directory, 0700); err != nil {
		return result, fmt.Errorf("create bootstrap directory: %w", err)
	}
	tmp, err := os.CreateTemp(cfg.Directory, ".chain-*.part")
	if err != nil {
		return result, fmt.Errorf("create bootstrap temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		tmp.Close()
		return result, fmt.Errorf("create bootstrap request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		tmp.Close()
		return result, fmt.Errorf("download bootstrap archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		tmp.Close()
		return result, fmt.Errorf("bootstrap server returned HTTP %s", resp.Status)
	}
	if resp.ContentLength > maxGUIBootstrapArchiveSize {
		tmp.Close()
		return result, fmt.Errorf("bootstrap archive is too large (%d bytes)", resp.ContentLength)
	}

	hash := sha256.New()
	reader := io.LimitReader(io.TeeReader(resp.Body, hash), maxGUIBootstrapArchiveSize+1)
	size, err := io.Copy(tmp, reader)
	if err != nil {
		tmp.Close()
		return result, fmt.Errorf("write bootstrap archive: %w", err)
	}
	if size == 0 {
		tmp.Close()
		return result, errors.New("bootstrap archive is empty")
	}
	if size > maxGUIBootstrapArchiveSize {
		tmp.Close()
		return result, fmt.Errorf("bootstrap archive exceeds %d-byte limit", maxGUIBootstrapArchiveSize)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return result, fmt.Errorf("sync bootstrap archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return result, fmt.Errorf("close bootstrap archive: %w", err)
	}

	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	if expected := strings.TrimPrefix(strings.TrimSpace(cfg.SHA256), "0x"); expected != "" {
		want, err := hex.DecodeString(expected)
		if err != nil || len(want) != sha256.Size {
			return result, errors.New("bootstrap SHA-256 must be a 64-character hexadecimal digest")
		}
		if !bytes.Equal(want, digest[:]) {
			return result, fmt.Errorf("bootstrap SHA-256 mismatch: have %x want %s", digest, expected)
		}
	}

	header := make([]byte, 2)
	file, err := os.Open(tmpName)
	if err != nil {
		return result, fmt.Errorf("inspect bootstrap archive: %w", err)
	}
	_, readErr := io.ReadFull(file, header)
	file.Close()
	if readErr != nil {
		return result, fmt.Errorf("inspect bootstrap archive: %w", readErr)
	}
	gzipArchive := header[0] == 0x1f && header[1] == 0x8b
	ext := ".rlp"
	if gzipArchive {
		ext += ".gz"
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	target := filepath.Join(cfg.Directory, "chain-"+stamp+ext)
	if err := os.Rename(tmpName, target); err != nil {
		return result, fmt.Errorf("install bootstrap archive: %w", err)
	}
	return bootstrapDownloadResult{
		Filename: filepath.Base(target),
		Bytes:    size,
		SHA256:   hex.EncodeToString(digest[:]),
		Gzip:     gzipArchive,
	}, nil
}

func guiBootstrapHTTPClient(proxyURL string) (*http.Client, error) {
	u, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil || u.Scheme != "socks5" || u.Hostname() == "" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("bootstrap Tor proxy must be a plain socks5://host:port URL")
	}
	dialer, err := xproxy.SOCKS5("tcp", u.Host, nil, xproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("configure bootstrap Tor proxy: %w", err)
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			resultCh := make(chan struct {
				conn net.Conn
				err  error
			}, 1)
			go func() {
				conn, err := dialer.Dial(network, address)
				resultCh <- struct {
					conn net.Conn
					err  error
				}{conn: conn, err: err}
			}()
			select {
			case result := <-resultCh:
				return result.conn, result.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   12 * time.Hour,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}
