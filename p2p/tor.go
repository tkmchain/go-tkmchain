package p2p

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/ethereum/go-ethereum/p2p/enode"
	xproxy "golang.org/x/net/proxy"
)

// SOCKS5Dialer routes outbound devp2p connections through an explicit SOCKS5
// proxy. It never falls back to a direct socket, which makes it suitable for
// strict privacy mode. Incoming connections still require an onion service or
// a separately protected listener.
type SOCKS5Dialer struct {
	proxy xproxy.Dialer
}

func NewSOCKS5Dialer(proxyURL string) (*SOCKS5Dialer, error) {
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
	return &SOCKS5Dialer{proxy: dialer}, nil
}

func (d *SOCKS5Dialer) Dial(ctx context.Context, dest *enode.Node) (net.Conn, error) {
	if d == nil || d.proxy == nil {
		return nil, errors.New("P2P SOCKS5 dialer is not configured")
	}
	addr, ok := dest.TCPEndpoint()
	if !ok {
		return nil, errNoPort
	}
	result := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		conn, err := d.proxy.Dial("tcp", addr.String())
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
