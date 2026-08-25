package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestShieldedSpendData(t *testing.T) {
	if got, err := shieldedSpendData("request-123", ""); err != nil || string(got) != "request-123" {
		t.Fatalf("legacy request metadata = %q, %v", got, err)
	}
	want := []byte("TKMEMAILVM1{\"v\":1}")
	encoded := "0x" + strings.ToLower(bytesToHex(want))
	got, err := shieldedSpendData("request-123", encoded)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("application metadata = %x, %v", got, err)
	}
	if _, err := shieldedSpendData("request-123", "not-hex"); err == nil {
		t.Fatal("non-hex application metadata was accepted")
	}
	if _, err := shieldedSpendData("request-123", "0x"+strings.Repeat("00", 12*1024+1)); err == nil {
		t.Fatal("oversized application metadata was accepted")
	}
}

func bytesToHex(input []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(input)*2)
	for i, value := range input {
		out[i*2] = alphabet[value>>4]
		out[i*2+1] = alphabet[value&0x0f]
	}
	return string(out)
}
