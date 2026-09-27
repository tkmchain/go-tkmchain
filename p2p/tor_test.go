package p2p

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestSOCKS5DialerValidation(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1:9050", "socks5://127.0.0.1", "socks5://user:pass@127.0.0.1:9050", "socks5://127.0.0.1:70000"} {
		if _, err := NewSOCKS5Dialer(endpoint); err == nil {
			t.Fatalf("accepted unsafe SOCKS5 endpoint %q", endpoint)
		}
	}
	if dialer, err := NewSOCKS5Dialer("socks5://127.0.0.1:9050"); err != nil || dialer == nil {
		t.Fatalf("valid SOCKS5 endpoint rejected: %v", err)
	}
}

// TestOnionSOCKS5DialerRoutesAndRejects verifies the two properties that keep
// onion-only P2P fail-closed: an onion destination is handed to the SOCKS5
// proxy as a hostname (so Tor resolves it), while a clearnet destination is
// rejected before a socket is opened.
func TestOnionSOCKS5DialerRoutesAndRejects(t *testing.T) {
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	seen := make(chan string, 1)
	proxyErr := make(chan error, 1)
	go func() {
		conn, err := proxy.Accept()
		if err != nil {
			proxyErr <- err
			return
		}
		defer conn.Close()
		var greeting [3]byte
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			proxyErr <- err
			return
		}
		if string(greeting[:2]) != "\x05\x01" || greeting[2] != 0 {
			proxyErr <- errInvalidSOCKS5Handshake
			return
		}
		if _, err := conn.Write([]byte{5, 0}); err != nil {
			proxyErr <- err
			return
		}
		var header [4]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			proxyErr <- err
			return
		}
		if header[0] != 5 || header[1] != 1 || header[3] != 3 {
			proxyErr <- errInvalidSOCKS5Handshake
			return
		}
		var length [1]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			proxyErr <- err
			return
		}
		name := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, name); err != nil {
			proxyErr <- err
			return
		}
		var port [2]byte
		if _, err := io.ReadFull(conn, port[:]); err != nil {
			proxyErr <- err
			return
		}
		seen <- net.JoinHostPort(string(name), strconv.Itoa(int(port[0])<<8|int(port[1])))
		_, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 1})
		proxyErr <- err
	}()

	proxyURL := "socks5://" + proxy.Addr().String()
	dialer, err := NewOnionSOCKS5Dialer(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	onion := enode.NewV4(&key.PublicKey, net.ParseIP("127.0.0.1"), 39001, 0).WithHostname("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.onion")
	conn, err := dialer.Dial(context.Background(), onion)
	if err != nil {
		t.Fatalf("onion dial failed: %v", err)
	}
	_ = conn.Close()
	select {
	case got := <-seen:
		if got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.onion:39001" {
			t.Fatalf("SOCKS destination = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("SOCKS proxy did not receive the onion destination")
	}
	if err := <-proxyErr; err != nil {
		t.Fatal(err)
	}

	clearnet := enode.NewV4(&key.PublicKey, net.ParseIP("127.0.0.1"), 39002, 0)
	if _, err := dialer.Dial(context.Background(), clearnet); err == nil || !strings.Contains(err.Error(), "non-onion") {
		t.Fatalf("clearnet dial error = %v, want onion-only rejection", err)
	}
}

var errInvalidSOCKS5Handshake = &socks5TestError{"invalid SOCKS5 handshake"}

type socks5TestError struct{ message string }

func (e *socks5TestError) Error() string { return e.message }
