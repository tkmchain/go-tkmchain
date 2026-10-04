package tkmnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

// SOCKS5Dialer is a fail-closed dialer for tkmnet onion services. It never
// falls back to a direct socket and rejects IP literals and clearnet DNS names.
type SOCKS5Dialer struct {
	dialer xproxy.Dialer
}

func NewSOCKS5Dialer(proxyURL string) (*SOCKS5Dialer, error) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Scheme != "socks5" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("tkmnet: SOCKS5 proxy must be socks5://host:port")
	}
	if _, err := strconv.ParseUint(u.Port(), 10, 16); err != nil {
		return nil, errors.New("tkmnet: SOCKS5 proxy port is invalid")
	}
	d, err := xproxy.SOCKS5("tcp", u.Host, nil, xproxy.Direct)
	if err != nil {
		return nil, err
	}
	return &SOCKS5Dialer{dialer: d}, nil
}

func (d *SOCKS5Dialer) DialContext(ctx context.Context, onionHost string, port string) (net.Conn, error) {
	if d == nil || d.dialer == nil {
		return nil, errors.New("tkmnet: SOCKS5 dialer is not configured")
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(onionHost), "."))
	if !strings.HasSuffix(host, ".onion") {
		return nil, errors.New("tkmnet: only .onion destinations are allowed")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return nil, errors.New("tkmnet: destination port is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		conn, err := d.dialer.Dial("tcp", net.JoinHostPort(host, port))
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

// ExchangePacket sends one fixed-size request to the first onion relay and
// reads one fixed-size response. Each invocation opens a fresh Tor stream;
// callers doing private username lookup must invoke it once per independently
// assigned directory peer and must encrypt/authenticate the application reply.
func ExchangePacket(ctx context.Context, proxyURL string, firstRelay Descriptor, relayPort string, packet []byte) ([]byte, error) {
	if err := firstRelay.Verify(time.Now()); err != nil {
		return nil, fmt.Errorf("tkmnet: invalid first relay descriptor: %w", err)
	}
	dialer, err := NewSOCKS5Dialer(proxyURL)
	if err != nil {
		return nil, err
	}
	conn, err := dialer.DialContext(ctx, firstRelay.Onion, relayPort)
	if err != nil {
		return nil, fmt.Errorf("tkmnet: onion relay connection failed: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := WritePacket(conn, packet); err != nil {
		return nil, err
	}
	return ReadPacket(conn)
}

// ExchangeService sends one payload through the supplied onion route and
// opens the final relay's authenticated response.
func ExchangeService(ctx context.Context, proxyURL, relayPort string, route []Descriptor, service ServiceID, payload []byte) ([]byte, error) {
	if len(route) != MaxHops {
		return nil, errors.New("tkmnet: service exchange requires a complete three-relay route")
	}
	hops := make([]Hop, MaxHops)
	for i, descriptor := range route {
		if err := descriptor.Verify(time.Now()); err != nil {
			return nil, fmt.Errorf("tkmnet: invalid route descriptor %d: %w", i, err)
		}
		hops[i] = Hop{ID: descriptor.ID, PublicKey: descriptor.PublicKey}
	}
	options := BuildOptions{Service: service, Hops: hops}
	packet, key, err := BuildWithSecret(options, payload)
	if err != nil {
		return nil, err
	}
	defer clearBytes(key[:])
	response, err := ExchangePacket(ctx, proxyURL, route[0], relayPort, packet)
	if err != nil {
		return nil, err
	}
	return OpenResponse(response, packet, key)
}

// WritePacket writes exactly one fixed-size packet. There is no length field
// on the wire, which prevents packet-size metadata from exposing the service
// payload size.
func WritePacket(w io.Writer, packet []byte) error {
	version := byte(0)
	if len(packet) >= HeaderSize && string(packet[:4]) == Magic {
		version = packet[4]
	} else if len(packet) >= 4+HeaderSize && string(packet[:4]) == "TKMR" {
		version = packet[8]
	}
	if version == 0 || len(packet) != packetSizeForVersion(version) {
		return errors.New("tkmnet: invalid packet size")
	}
	for len(packet) > 0 {
		written, err := w.Write(packet)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(packet) {
			return io.ErrShortWrite
		}
		packet = packet[written:]
	}
	return nil
}

func ReadPacket(r io.Reader) ([]byte, error) {
	preamble := make([]byte, 4)
	if _, err := io.ReadFull(r, preamble); err != nil {
		return nil, err
	}
	if string(preamble) == "TKMR" {
		header := make([]byte, HeaderSize)
		if _, err := io.ReadFull(r, header); err != nil {
			return nil, err
		}
		version := header[4]
		if version != Version && version != ExtendedVersion {
			return nil, errors.New("tkmnet: invalid response version")
		}
		packet := make([]byte, packetSizeForVersion(version))
		copy(packet[:4], preamble)
		copy(packet[4:4+HeaderSize], header)
		if _, err := io.ReadFull(r, packet[4+HeaderSize:]); err != nil {
			return nil, err
		}
		return packet, nil
	}
	if string(preamble) != Magic {
		return nil, errors.New("tkmnet: invalid packet preamble")
	}
	header := make([]byte, HeaderSize)
	copy(header[:4], preamble)
	if _, err := io.ReadFull(r, header[4:5]); err != nil {
		return nil, err
	}
	if header[4] != Version && header[4] != ExtendedVersion {
		return nil, errors.New("tkmnet: invalid packet version")
	}
	if _, err := io.ReadFull(r, header[5:]); err != nil {
		return nil, err
	}
	packet := make([]byte, packetSizeForVersion(header[4]))
	copy(packet, header)
	if _, err := io.ReadFull(r, packet[HeaderSize:]); err != nil {
		return nil, err
	}
	return packet, nil
}
