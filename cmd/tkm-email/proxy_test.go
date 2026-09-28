package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewEmailProxyRequiresTorSocks5(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		socks  string
	}{
		{name: "relative remote", remote: "/mail", socks: "socks5://127.0.0.1:9050"},
		{name: "direct proxy", remote: "https://mail.tkmchain.site/", socks: "http://127.0.0.1:9050"},
		{name: "missing port", remote: "https://mail.tkmchain.site/", socks: "socks5://127.0.0.1"},
		{name: "proxy credentials", remote: "https://mail.tkmchain.site/", socks: "socks5://user:pass@127.0.0.1:9050"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newEmailProxy(test.remote, test.socks); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestEmailProxyDoesNotFallBackToDirectNetwork(t *testing.T) {
	proxy, err := newEmailProxy("https://mail.tkmchain.site/", "socks5://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	response, err := http.Get(proxy.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadGateway)
	}
	if !strings.Contains(response.Status, "502") {
		t.Fatalf("unexpected response status %q", response.Status)
	}
}

func TestEmailProxyBindsLoopback(t *testing.T) {
	proxy, err := newEmailProxy("https://mail.tkmchain.site/", "socks5://127.0.0.1:9050")
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	if !strings.HasPrefix(proxy.URL(), "http://127.0.0.1:") {
		t.Fatalf("proxy URL is not loopback-only: %s", proxy.URL())
	}
}
