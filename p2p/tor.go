package p2p

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/p2p/enode"
	xproxy "golang.org/x/net/proxy"
)

// SOCKS5Dialer routes outbound devp2p connections through an explicit SOCKS5
// proxy. It never falls back to a direct socket, which makes it suitable for
// strict privacy mode. Incoming connections still require an onion service or
// a separately protected listener.
type SOCKS5Dialer struct {
	proxy     xproxy.Dialer
	onionOnly bool
}

func NewSOCKS5Dialer(proxyURL string) (*SOCKS5Dialer, error) {
	return newSOCKS5Dialer(proxyURL, false)
}

// NewOnionSOCKS5Dialer creates a SOCKS5 dialer which only accepts onion
// service hostnames. IP literals and clearnet DNS names are rejected before
// they reach Tor, preventing accidental clearnet peer connections.
func NewOnionSOCKS5Dialer(proxyURL string) (*SOCKS5Dialer, error) {
	return newSOCKS5Dialer(proxyURL, true)
}

func newSOCKS5Dialer(proxyURL string, onionOnly bool) (*SOCKS5Dialer, error) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Scheme != "socks5" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("P2P SOCKS5 proxy must be a plain socks5://host:port URL")
	}
	if _, err := strconv.ParseUint(u.Port(), 10, 16); err != nil {
		return nil, errors.New("P2P SOCKS5 proxy requires a valid port")
	}
	dialer, err := xproxy.SOCKS5("tcp", u.Host, nil, xproxy.Direct)
	if err != nil {
		return nil, err
	}
	return &SOCKS5Dialer{proxy: dialer, onionOnly: onionOnly}, nil
}

func (d *SOCKS5Dialer) Dial(ctx context.Context, dest *enode.Node) (net.Conn, error) {
	if d == nil || d.proxy == nil {
		return nil, errors.New("P2P SOCKS5 dialer is not configured")
	}
	if dest == nil {
		return nil, errors.New("P2P destination is missing")
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(dest.Hostname())), ".")
	if d.onionOnly {
		if !strings.HasSuffix(host, ".onion") {
			return nil, errors.New("onion-only P2P mode rejects non-onion peer")
		}
		if dest.TCP() == 0 {
			return nil, errNoPort
		}
	} else if host == "" {
		endpoint, ok := dest.TCPEndpoint()
		if !ok {
			return nil, errNoPort
		}
		host = endpoint.Addr().String()
	}
	if dest.TCP() == 0 {
		return nil, errNoPort
	}
	addr := net.JoinHostPort(host, strconv.Itoa(dest.TCP()))
	result := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		conn, err := d.proxy.Dial("tcp", addr)
		result <- struct {
			conn net.Conn
			err  error
		}{conn, err}
	}()
	select {
	case r := <-result:
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
