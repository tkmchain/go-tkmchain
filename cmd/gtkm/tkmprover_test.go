package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagedTkmProverRestartsAfterUnexpectedExit(t *testing.T) {
	var starts atomic.Int32
	launch := func() (*exec.Cmd, error) {
		attempt := starts.Add(1)
		mode := "stay"
		if attempt == 1 {
			mode = "exit"
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestTkmProverSupervisorChild$")
		cmd.Env = append(os.Environ(), "TKM_PROVER_SUPERVISOR_TEST="+mode)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
	initial, err := launch()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &managedTkmProver{cmd: initial, launch: launch, cancel: cancel, done: make(chan struct{})}
	go p.supervise(ctx)
	defer p.stop()

	deadline := time.Now().Add(5 * time.Second)
	for starts.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := starts.Load(); got < 2 {
		t.Fatalf("prover restart attempts = %d, want at least 2", got)
	}
}

func TestTkmProverSupervisorChild(t *testing.T) {
	switch os.Getenv("TKM_PROVER_SUPERVISOR_TEST") {
	case "exit":
		os.Exit(9)
	case "stay":
		select {}
	}
}

func TestTkmProverConfigPreservesAutomaticFunding(t *testing.T) {
	const limit = "1000000000000000000000"
	var cfg tkmProverConfig
	if err := json.Unmarshal([]byte(`{"autoPublicFunding":true,"autoPublicFundingLimitWei":"`+limit+`"}`), &cfg); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted["autoPublicFunding"] != true || persisted["autoPublicFundingLimitWei"] != limit {
		t.Fatalf("automatic funding settings were not preserved: %s", data)
	}
}

func TestDownloadTkmProvingKey(t *testing.T) {
	want := []byte("test proving key")
	digest := sha256.Sum256(want)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(want)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "cache", "proving.key")
	if err := downloadTkmProvingKey(context.Background(), path, server.URL, hex.EncodeToString(digest[:]), 1024); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("downloaded key = %q, want %q", got, want)
	}
}

func TestDownloadTkmProvingKeyRejectsWrongHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("wrong key"))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "proving.key")
	err := downloadTkmProvingKey(context.Background(), path, server.URL, strings.Repeat("0", 64), 1024)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("error = %v, want SHA-256 mismatch", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("invalid key was cached: %v", statErr)
	}
}
