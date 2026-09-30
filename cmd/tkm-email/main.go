// Copyright 2026 The TKMChain Authors
//
// TKM Email is a small desktop launcher for the wallet's EmailVM interface.
// It deliberately exposes only email actions: it does not start a node, open
// a wallet UI, or provide phone, send, or receive controls.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var Version = "dev"

func main() {
	remoteURL := flag.String("url", "https://wallet.tkmchain.site/?app=email", "EmailVM client URL (all traffic is routed through Tor)")
	socks5 := flag.String("tor-socks5", "socks5://127.0.0.1:9050", "mandatory Tor SOCKS5 proxy")
	noOpen := flag.Bool("no-open", false, "serve the Tor-only client and print its local URL without opening a window")
	showVersion := flag.Bool("version", false, "print the launcher version")
	flag.Parse()
	if *showVersion {
		fmt.Println(Version)
		return
	}
	proxy, err := newEmailProxy(*remoteURL, *socks5)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tkm-email:", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := proxy.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "tkm-email:", err)
		os.Exit(1)
	}
	defer proxy.Close()

	localURL := proxy.URL()
	fmt.Printf("TKM Email (Tor-only, email-only): %s\n", localURL)
	fmt.Printf("Remote EmailVM endpoint: %s\n", *remoteURL)
	if !*noOpen {
		if err := runWindow(localURL); err != nil {
			fmt.Fprintln(os.Stderr, "tkm-email:", err)
			fmt.Println("Open this local URL manually:", localURL)
		}
	}
	<-ctx.Done()
}
