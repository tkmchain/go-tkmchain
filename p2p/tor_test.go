package p2p

import "testing"

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
