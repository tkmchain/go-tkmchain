package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadShield3RecipientCodeFromWrappedFile(t *testing.T) {
	want := "tkmshield3." + strings.Repeat("a", 28000)
	path := filepath.Join(t.TempDir(), "recipient.txt")
	if err := os.WriteFile(path, []byte(want[:14000]+"\n"+want[14000:]+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadShield3RecipientCode("file:" + path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loaded recipient code length = %d, want %d", len(got), len(want))
	}
}

func TestLoadShield3RecipientCodePassesDirectCode(t *testing.T) {
	want := "tkmshield3." + strings.Repeat("a", 28000)
	got, err := loadShield3RecipientCode(want)
	if err != nil || got != want {
		t.Fatalf("direct recipient code changed: err=%v", err)
	}
}

func TestReadWalletRawLineAcceptsLongShield3Code(t *testing.T) {
	want := "tkmshield3." + strings.Repeat("a", 28000)
	var progress bytes.Buffer
	got, err := readWalletRawLine(strings.NewReader(want+"\nnext prompt input\n"), &progress, 65536)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("raw input length = %d, want %d", len(got), len(want))
	}
}
