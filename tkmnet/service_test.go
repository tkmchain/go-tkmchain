package tkmnet

import (
	"context"
	"crypto/mlkem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestServiceOpensFinalPayload(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "relay", "key")
	opened := make(chan []byte, 1)
	svc, err := NewService(ServiceConfig{
		Enabled:        true,
		ListenAddr:     "127.0.0.1:0",
		OnionOnly:      true,
		HopIndex:       2,
		PrivateKeyPath: keyPath,
		Handler: func(_ context.Context, route Route, packet []byte) ([]byte, error) {
			payload, err := OpenPayload(packet, route)
			if err == nil {
				opened <- payload
			}
			return nil, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(); err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()

	hops := make([]Hop, MaxHops)
	keys := make([]*mlkem.DecapsulationKey1024, 2)
	for i := 0; i < 2; i++ {
		key, err := mlkem.GenerateKey1024()
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		hops[i].PublicKey = key.EncapsulationKey().Bytes()
		hops[i].ID[0] = byte(i + 1)
	}
	hops[2] = Hop{PublicKey: svc.PublicKey()}
	hops[2].ID = svc.RelayID()
	packet, err := Build(BuildOptions{Service: ServicePhone, Hops: hops}, []byte("encrypted phone message"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		route, err := OpenLayer(packet, keys[i], uint8(i))
		if err != nil {
			t.Fatal(err)
		}
		packet, err = Forward(packet, route)
		if err != nil {
			t.Fatal(err)
		}
	}
	conn, err := net.DialTimeout("tcp", svc.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePacket(conn, packet); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	_ = conn.Close()
	select {
	case got := <-opened:
		if string(got) != "encrypted phone message" {
			t.Fatalf("payload = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("service handler did not receive the packet")
	}
}

func TestServiceRejectsPublicListener(t *testing.T) {
	_, err := NewService(ServiceConfig{Enabled: true, ListenAddr: "0.0.0.0:39000", OnionOnly: true, PrivateKeyPath: filepath.Join(t.TempDir(), "key")})
	if err == nil {
		t.Fatal("public tkmnet listener accepted")
	}
}

func TestRelayForwardingRequiresTorAndPort(t *testing.T) {
	now := time.Now()
	peer := pinnedDirectoryPeers(t, 1, now)[0].Descriptor
	base := ServiceConfig{Enabled: true, ListenAddr: "127.0.0.1:0", PrivateKeyPath: filepath.Join(t.TempDir(), "key"), RelayPeers: []Descriptor{peer}}
	if _, err := NewService(base); err == nil {
		t.Fatal("relay forwarding accepted without Tor settings")
	}
	base.SOCKS5Proxy = "socks5://127.0.0.1:9050"
	if _, err := NewService(base); err == nil {
		t.Fatal("relay forwarding accepted without an onion port")
	}
	base.RelayPort = "39000"
	if _, err := NewService(base); err != nil {
		t.Fatalf("valid onion forwarding config rejected: %v", err)
	}
}

func TestThreeRelayUsernameRequestRoundTripThroughSOCKS(t *testing.T) {
	type relayFixture struct {
		service    *Service
		descriptor Descriptor
	}
	fixtures := make([]relayFixture, MaxHops)
	now := time.Now()
	for i := range fixtures {
		kem, err := mlkem.GenerateKey1024()
		if err != nil {
			t.Fatal(err)
		}
		signer, err := pqcrypto.GenerateMLDSA87()
		if err != nil {
			t.Fatal(err)
		}
		descriptor := Descriptor{
			Version:          Version,
			Onion:            strings.Repeat(string(rune('a'+i)), 56) + ".onion",
			PublicKey:        kem.EncapsulationKey().Bytes(),
			SigningPublicKey: pqcrypto.PublicKeyBytes(signer),
			Expires:          uint64(now.Add(time.Hour).Unix()),
		}
		copy(descriptor.ID[:], relayIDDigest(descriptor.PublicKey))
		if err := SignDescriptor(&descriptor, func(message []byte) ([]byte, error) {
			return pqcrypto.SignMLDSA87(signer, message)
		}); err != nil {
			t.Fatal(err)
		}
		keyPath := filepath.Join(t.TempDir(), "relay-key")
		if err := os.WriteFile(keyPath, kem.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		fixtures[i].descriptor = descriptor
		fixtures[i].service, err = NewService(ServiceConfig{
			Enabled:        true,
			ListenAddr:     "127.0.0.1:0",
			PrivateKeyPath: keyPath,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	proxy, proxyAddr := startMappedSOCKSProxy(t)
	defer proxy.Close()
	for i := range fixtures {
		service := fixtures[i].service
		if i+1 < len(fixtures) {
			service.cfg.SOCKS5Proxy = "socks5://" + proxyAddr
			service.cfg.RelayPort = "39000"
			service.cfg.RelayPeers = []Descriptor{fixtures[i+1].descriptor, fixtures[MaxHops-1].descriptor}
			service.peers = map[[LayerNextIDSize]byte]Descriptor{
				fixtures[i+1].descriptor.ID:       fixtures[i+1].descriptor,
				fixtures[MaxHops-1].descriptor.ID: fixtures[MaxHops-1].descriptor,
			}
			service.dialer, _ = NewSOCKS5Dialer(service.cfg.SOCKS5Proxy)
		}
		if i == MaxHops-1 {
			service.cfg.PayloadHandler = func(_ context.Context, service ServiceID, payload []byte) ([]byte, error) {
				if service != ServiceUsername {
					return nil, errors.New("wrong service")
				}
				if _, err := DecodeUsernameQuery(payload); err != nil {
					return nil, err
				}
				return []byte("username response"), nil
			}
		}
		if err := service.Start(); err != nil {
			t.Fatal(err)
		}
		defer service.Stop()
		proxy.add(fixtures[i].descriptor.Onion, service.Addr().String())
	}

	response, err := ExchangeService(context.Background(), "socks5://"+proxyAddr, "39000", []Descriptor{fixtures[0].descriptor, fixtures[1].descriptor, fixtures[2].descriptor}, ServiceUsername, mustUsernameQuery(t, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "username response" {
		t.Fatalf("three-relay route response = %q", response)
	}
}

func mustUsernameQuery(t *testing.T, name string) []byte {
	t.Helper()
	query, err := EncodeUsernameQuery(name)
	if err != nil {
		t.Fatal(err)
	}
	return query
}

type mappedSOCKSProxy struct {
	listener net.Listener
	mu       sync.RWMutex
	backends map[string]string
}

func startMappedSOCKSProxy(t *testing.T) (*mappedSOCKSProxy, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &mappedSOCKSProxy{listener: listener, backends: make(map[string]string)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go proxy.handle(conn)
		}
	}()
	return proxy, listener.Addr().String()
}

func (p *mappedSOCKSProxy) add(onion, backend string) {
	p.mu.Lock()
	p.backends[onion] = backend
	p.mu.Unlock()
}

func (p *mappedSOCKSProxy) Close() error { return p.listener.Close() }

func (p *mappedSOCKSProxy) handle(client net.Conn) {
	defer client.Close()
	var greeting [2]byte
	if _, err := io.ReadFull(client, greeting[:]); err != nil || greeting[0] != 5 {
		return
	}
	methods := make([]byte, greeting[1])
	if _, err := io.ReadFull(client, methods); err != nil {
		return
	}
	if _, err := client.Write([]byte{5, 0}); err != nil {
		return
	}
	var request [4]byte
	if _, err := io.ReadFull(client, request[:]); err != nil || request[0] != 5 || request[1] != 1 || request[3] != 3 {
		return
	}
	var size [1]byte
	if _, err := io.ReadFull(client, size[:]); err != nil {
		return
	}
	host := make([]byte, size[0])
	if _, err := io.ReadFull(client, host); err != nil {
		return
	}
	var port [2]byte
	if _, err := io.ReadFull(client, port[:]); err != nil {
		return
	}
	p.mu.RLock()
	backend := p.backends[string(host)]
	p.mu.RUnlock()
	if backend == "" {
		_, _ = client.Write([]byte{5, 4, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	upstream, err := net.Dial("tcp", backend)
	if err != nil {
		_, _ = client.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	if _, err := client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	closed := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); closed <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); closed <- struct{}{} }()
	<-closed
}
