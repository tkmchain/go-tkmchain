// tkmnet-relay runs one onion-published TKMNet transit relay. It is an
// operator utility, not a directory server or a consensus participant.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/tkmnet"
)

func main() {
	stateDir := env("TKMNET_STATE_DIR", "/var/lib/tkmnet")
	relayKeyPath := env("TKMNET_RELAY_KEY", filepath.Join(stateDir, "relay-kem.key"))
	exportOnly := os.Getenv("TKMNET_EXPORT_ONLY") == "1"
	addr := env("TKMNET_LISTEN", "127.0.0.1:39000")
	proxy := env("TKMNET_SOCKS5", "socks5://127.0.0.1:9050")
	port := env("TKMNET_RELAY_PORT", "39000")
	onionFile := env("TKMNET_ONION_FILE", "/var/lib/tor/hidden/hostname")
	peersFile := env("TKMNET_PEERS_FILE", "/etc/tkmnet/peers.json")
	healthAddr := env("TKMNET_HEALTH_ADDR", "127.0.0.1:39080")

	if err := os.MkdirAll(stateDir, 0700); err != nil {
		log.Fatalf("create state directory: %v", err)
	}
	onionBytes, err := os.ReadFile(onionFile)
	if err != nil {
		log.Fatalf("read Tor onion hostname: %v", err)
	}
	onion := string(bytes.TrimSpace(onionBytes))
	peers, err := readPeers(peersFile)
	if err != nil {
		log.Fatalf("read relay peer descriptors: %v", err)
	}

	service, err := tkmnet.NewService(tkmnet.ServiceConfig{
		Enabled: true, ListenAddr: addr, OnionOnly: true,
		PrivateKeyPath: relayKeyPath,
		SOCKS5Proxy:    proxy, RelayPort: port, RelayPeers: peers,
		Logger: func(message string, args ...any) { log.Printf("%s %v", message, args) },
	})
	if err != nil {
		log.Fatalf("configure relay: %v", err)
	}
	descriptor, err := makeDescriptor(service, onion, stateDir)
	if err != nil {
		log.Fatalf("create signed relay descriptor: %v", err)
	}
	if err := writeJSON(filepath.Join(stateDir, "descriptor.json"), descriptor, 0644); err != nil {
		log.Fatalf("write relay descriptor: %v", err)
	}
	if exportOnly {
		log.Printf("exported signed relay descriptor onion=%s relayID=%s descriptor=%s", onion, hex.EncodeToString(descriptor.ID[:]), filepath.Join(stateDir, "descriptor.json"))
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := service.Start(); err != nil {
		log.Fatalf("start relay: %v", err)
	}
	defer service.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if service.Addr() == nil {
			http.Error(w, "relay stopped", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if len(peers) == 0 {
			http.Error(w, "relay listener is up; forwarding peers are not configured", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "forwarding peers: %d\n", len(peers))
	})
	server := &http.Server{Addr: healthAddr, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("health listener failed: %v", err)
		}
	}()
	log.Printf("TKMNet transit relay started onion=%s relayID=%s peers=%d descriptor=%s", onion, hex.EncodeToString(descriptor.ID[:]), len(peers), filepath.Join(stateDir, "descriptor.json"))
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func makeDescriptor(service *tkmnet.Service, onion, stateDir string) (tkmnet.Descriptor, error) {
	seedPath := filepath.Join(stateDir, "descriptor-signing.seed")
	seed, err := os.ReadFile(seedPath)
	if errors.Is(err, os.ErrNotExist) {
		seed = make([]byte, pqcrypto.MLDSA87SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return tkmnet.Descriptor{}, err
		}
		if err := os.WriteFile(seedPath, seed, 0600); err != nil {
			return tkmnet.Descriptor{}, err
		}
	} else if err != nil {
		return tkmnet.Descriptor{}, err
	}
	defer clear(seed)
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return tkmnet.Descriptor{}, fmt.Errorf("load descriptor signing seed: %w", err)
	}
	d := tkmnet.Descriptor{
		Version: tkmnet.Version, ID: service.RelayID(), Onion: onion,
		PublicKey: service.PublicKey(), SigningPublicKey: pqcrypto.PublicKeyBytes(key),
		Expires: uint64(time.Now().Add(23 * time.Hour).Unix()),
	}
	if err := tkmnet.SignDescriptor(&d, func(message []byte) ([]byte, error) {
		return pqcrypto.SignMLDSA87(key, message)
	}); err != nil {
		return tkmnet.Descriptor{}, err
	}
	if err := d.Verify(time.Now()); err != nil {
		return tkmnet.Descriptor{}, err
	}
	return d, nil
}

func readPeers(path string) ([]tkmnet.Descriptor, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var peers []tkmnet.Descriptor
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&peers); err != nil {
		return nil, err
	}
	if len(peers) > 0 {
		// NewService verifies signatures, expiry and duplicate relay IDs.
	}
	return peers, nil
}

func writeJSON(path string, value any, mode os.FileMode) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
