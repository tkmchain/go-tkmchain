package tkmnet

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSOCKS5DialContextCancelsHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			// Read the SOCKS greeting, then leave method selection unanswered
			// until the caller cancels.
			_, _ = io.ReadFull(conn, make([]byte, 3))
			_, _ = conn.Read(make([]byte, 1))
		}
	}()

	dialer, err := NewSOCKS5Dialer("socks5://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = dialer.DialContext(ctx, strings.Repeat("a", 56)+".onion", "39000")
	var netErr net.Error
	if !errors.Is(err, context.DeadlineExceeded) && (!errors.As(err, &netErr) || !netErr.Timeout()) {
		t.Fatalf("SOCKS dial error = %v, want a context-driven timeout", err)
	}
}

func TestRelayIOTimeoutCoversTorCircuitBuild(t *testing.T) {
	if relayIOTimeout <= 120*time.Second {
		t.Fatalf("relay I/O timeout = %s, want more than Tor's 120 second circuit-build window", relayIOTimeout)
	}
}
