// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/ethereum/go-ethereum/node"
	"github.com/urfave/cli/v2"
	xproxy "golang.org/x/net/proxy"
)

const maxBootstrapArchiveSize = int64(512) << 30

var (
	bootstrapURLFlag = &cli.StringFlag{
		Name:     "url",
		Usage:    "HTTPS or HTTP URL of an RLP block archive (all traffic uses Tor SOCKS5)",
		Required: true,
	}
	bootstrapSHA256Flag = &cli.StringFlag{
		Name:  "sha256",
		Usage: "Expected SHA-256 digest of the downloaded archive (recommended)",
	}
	bootstrapSOCKS5Flag = &cli.StringFlag{
		Name:  "tor-socks5",
		Usage: "Tor SOCKS5 proxy used for the archive download",
		Value: "socks5://127.0.0.1:9050",
	}
	bootstrapTimeoutFlag = &cli.StringFlag{
		Name:  "timeout",
		Usage: "Download timeout (for example 12h or 45m)",
		Value: "12h",
	}
	bootstrapNoImportFlag = &cli.BoolFlag{
		Name:  "no-import",
		Usage: "Download and place the archive without importing it",
	}

	bootstrapCommand = &cli.Command{
		Name:      "bootstrap",
		Usage:     "Download and import a verified current-chain block archive",
		ArgsUsage: "",
		Flags: append([]cli.Flag{
			bootstrapURLFlag,
			bootstrapSHA256Flag,
			bootstrapSOCKS5Flag,
			bootstrapTimeoutFlag,
			bootstrapNoImportFlag,
		}, importCommand.Flags...),
		Before: func(ctx *cli.Context) error {
			flags.MigrateGlobalFlags(ctx)
			return debug.Setup(ctx)
		},
		Action: bootstrapChain,
		Description: `
Downloads an RLP-encoded chain archive through the local Tor SOCKS5 proxy,
verifies its optional SHA-256 digest, and stores it under the platform-specific
node directory before importing it. The default locations are:

  Linux:   ~/.tkmchain/gtkm/bootstrap/
  Windows: %LOCALAPPDATA%\\Tkmchain\\gtkm\\bootstrap\\

Stop any running gtkm process before importing. The archive remains on disk so
an interrupted import can be resumed with the regular ` + "`import`" + ` command.
`,
	}
)

func bootstrapChain(ctx *cli.Context) error {
	if ctx.NArg() != 0 {
		return fmt.Errorf("bootstrap does not accept positional arguments")
	}
	timeout, err := time.ParseDuration(ctx.String(bootstrapTimeoutFlag.Name))
	if err != nil || timeout <= 0 {
		return fmt.Errorf("invalid --%s duration %q", bootstrapTimeoutFlag.Name, ctx.String(bootstrapTimeoutFlag.Name))
	}
	archiveURL, err := url.Parse(strings.TrimSpace(ctx.String(bootstrapURLFlag.Name)))
	if err != nil || archiveURL.Scheme == "" || archiveURL.Host == "" || (archiveURL.Scheme != "http" && archiveURL.Scheme != "https") {
		return fmt.Errorf("--%s must be an http(s) URL", bootstrapURLFlag.Name)
	}
	instanceDir, err := bootstrapInstanceDir(ctx)
	if err != nil {
		return err
	}
	archiveDir := filepath.Join(instanceDir, "bootstrap")
	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return fmt.Errorf("create bootstrap directory: %w", err)
	}

	stamp := time.Now().UTC().Format("20060102T150405Z")
	tmp, err := os.CreateTemp(archiveDir, ".chain-"+stamp+"-*.part")
	if err != nil {
		return fmt.Errorf("create bootstrap temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	fmt.Printf("Downloading bootstrap archive through Tor from %s...\n", archiveURL.Redacted())
	digest, gzipArchive, size, err := downloadBootstrapArchive(ctx, archiveURL.String(), ctx.String(bootstrapSOCKS5Flag.Name), timeout, tmp)
	if err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync bootstrap archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close bootstrap archive: %w", err)
	}

	if expected := strings.TrimPrefix(strings.TrimSpace(ctx.String(bootstrapSHA256Flag.Name)), "0x"); expected != "" {
		want, err := hex.DecodeString(expected)
		if err != nil || len(want) != sha256.Size {
			return fmt.Errorf("--%s must be a 64-character hexadecimal SHA-256 digest", bootstrapSHA256Flag.Name)
		}
		if !bytes.Equal(want, digest[:]) {
			return fmt.Errorf("bootstrap SHA-256 mismatch: have %x want %s", digest, expected)
		}
	} else {
		fmt.Printf("Warning: no --%s supplied; HTTPS/Tor transport protects delivery, but a published digest is recommended.\n", bootstrapSHA256Flag.Name)
	}

	ext := ".rlp"
	if gzipArchive {
		ext += ".gz"
	}
	target := filepath.Join(archiveDir, "chain-"+stamp+ext)
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("install bootstrap archive: %w", err)
	}
	fmt.Printf("Bootstrap archive saved to %s (%s, %d bytes, sha256=%x)\n", target, ext, size, digest)

	if ctx.Bool(bootstrapNoImportFlag.Name) {
		return nil
	}
	fmt.Println("Importing bootstrap blocks; the node must be stopped while this runs...")
	if err := importChainFiles(ctx, []string{target}, false); err != nil {
		return fmt.Errorf("bootstrap import failed (archive retained at %s): %w", target, err)
	}
	return nil
}

func bootstrapInstanceDir(ctx *cli.Context) (string, error) {
	base := strings.TrimSpace(ctx.String(utils.DataDirFlag.Name))
	if base == "" {
		base = node.DefaultDataDir()
	} else {
		base = utils.MakeDataDir(ctx)
	}
	if base == "" {
		return "", errors.New("cannot determine the data directory; pass --datadir")
	}
	return filepath.Join(base, clientIdentifier), nil
}

func downloadBootstrapArchive(ctx *cli.Context, rawURL, socks5URL string, timeout time.Duration, out *os.File) ([sha256.Size]byte, bool, int64, error) {
	var digest [sha256.Size]byte
	client, err := bootstrapHTTPClient(socks5URL, timeout)
	if err != nil {
		return digest, false, 0, err
	}
	req, err := http.NewRequestWithContext(ctx.Context, http.MethodGet, rawURL, nil)
	if err != nil {
		return digest, false, 0, fmt.Errorf("create bootstrap request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return digest, false, 0, fmt.Errorf("download bootstrap archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return digest, false, 0, fmt.Errorf("bootstrap server returned HTTP %s", resp.Status)
	}
	if resp.ContentLength > maxBootstrapArchiveSize {
		return digest, false, 0, fmt.Errorf("bootstrap archive is too large (%d bytes; limit %d)", resp.ContentLength, maxBootstrapArchiveSize)
	}

	h := sha256.New()
	reader := io.LimitReader(io.TeeReader(resp.Body, h), maxBootstrapArchiveSize+1)
	size, err := io.Copy(out, reader)
	if err != nil {
		return digest, false, size, fmt.Errorf("write bootstrap archive: %w", err)
	}
	if size == 0 {
		return digest, false, 0, errors.New("bootstrap archive is empty")
	}
	if size > maxBootstrapArchiveSize {
		return digest, false, size, fmt.Errorf("bootstrap archive exceeds %d-byte limit", maxBootstrapArchiveSize)
	}
	copy(digest[:], h.Sum(nil))

	var header [2]byte
	if _, err := out.ReadAt(header[:], 0); err != nil {
		return digest, false, size, fmt.Errorf("inspect bootstrap archive: %w", err)
	}
	return digest, header[0] == 0x1f && header[1] == 0x8b, size, nil
}

func bootstrapHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	u, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil || u.Scheme != "socks5" || u.Hostname() == "" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("--tor-socks5 must be a plain socks5://host:port URL")
	}
	dialer, err := xproxy.SOCKS5("tcp", u.Host, nil, xproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("configure Tor SOCKS5 proxy: %w", err)
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			resultCh := make(chan result, 1)
			go func() {
				conn, err := dialer.Dial(network, address)
				resultCh <- result{conn: conn, err: err}
			}()
			select {
			case result := <-resultCh:
				return result.conn, result.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}
