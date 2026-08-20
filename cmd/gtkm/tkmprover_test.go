package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
