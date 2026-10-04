package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/internal/shield3wallet"
)

func TestWalletSponsorCodeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "code.txt")
	code := "tkmstamp1." + strings.Repeat("A", 40000)
	if err := writeWalletSponsorFile(path, code); err != nil {
		t.Fatal(err)
	}
	got, err := loadWalletSponsorCode("@" + path)
	if err != nil || got != code {
		t.Fatalf("long code truncated: %v", err)
	}
	if err := writeWalletSponsorFile(path, "replacement"); err == nil {
		t.Fatal("overwrote existing file")
	}
	got, err = loadWalletSponsorCode("@" + path)
	if err != nil || got != code {
		t.Fatal("existing code changed")
	}
	info, err := os.Stat(path)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		t.Fatal("unsafe code file permissions")
	}
	if _, err := loadWalletSponsorCode(strings.Repeat("A", 2*shield3wallet.MaxStampSponsorshipCodeSize+1)); err == nil {
		t.Fatal("oversize code accepted")
	}
	if _, err := loadWalletSponsorCode("   "); err == nil {
		t.Fatal("empty code accepted")
	}
	if _, err := loadWalletSponsorCode("@" + filepath.Dir(path)); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestWalletSponsorshipNeedsExplicitConfirmation(t *testing.T) {
	for _, input := range []string{"\n", "yes\n", "AUTHORIZE\n", "pay\n"} {
		if err := confirmWalletSponsorship(bufio.NewReader(strings.NewReader(input)), "PAY"); err == nil {
			t.Fatal("signed without explicit PAY confirmation")
		}
	}
	if err := confirmWalletSponsorship(bufio.NewReader(strings.NewReader("PAY\n")), "PAY"); err != nil {
		t.Fatal(err)
	}
}
