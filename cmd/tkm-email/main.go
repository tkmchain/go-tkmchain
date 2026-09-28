// Copyright 2026 The TKMChain Authors
//
// TKM Email is a small desktop launcher for the standalone EmailVM client.
// It deliberately does not start a node or open a keystore: mail keys and
// shielded payment signing stay in the wallet or in the hosted mail client.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"runtime"
)

var Version = "dev"

func openURL(url string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{url}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		command, args = "xdg-open", []string{url}
	}
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("cannot open a browser (%s is not installed): %w", command, err)
	}
	return exec.Command(command, args...).Start()
}

func main() {
	url := flag.String("url", "https://mail.tkmchain.site/", "standalone TKM Email URL")
	noOpen := flag.Bool("no-open", false, "print the URL without opening a browser")
	showVersion := flag.Bool("version", false, "print the launcher version")
	flag.Parse()
	if *showVersion {
		fmt.Println(Version)
		return
	}
	if *url == "" {
		fmt.Println("tkm-email: --url cannot be empty")
		return
	}
	fmt.Printf("TKM Email: %s\n", *url)
	if *noOpen {
		return
	}
	if err := openURL(*url); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Println("Open this URL in a browser:", *url)
			return
		}
		fmt.Println("Unable to open the browser:", err)
		fmt.Println("Open this URL manually:", *url)
	}
}
