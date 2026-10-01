// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/internal/gui"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/urfave/cli/v2"
)

var (
	shield3WalletHostFlag = &cli.StringFlag{
		Name:  "shield3.host",
		Usage: "loopback address for the headless Shield3 service",
		Value: "127.0.0.1",
	}
	shield3WalletPortFlag = &cli.IntFlag{
		Name:  "shield3.port",
		Usage: "HTTP port for the headless Shield3 service",
		Value: 8788,
	}
	shield3WalletOriginFlag = &cli.StringFlag{
		Name:  "shield3.allowed-origin",
		Usage: "exact HTTPS origin allowed to call the local service (for example https://wallet.tkmchain.site)",
	}
	shield3WalletTokenFlag = &cli.PathFlag{
		Name:  "shield3.token-file",
		Usage: "0600 file receiving the bearer token used by the web wallet",
	}
	shield3WalletStateFlag = &cli.PathFlag{
		Name:  "shield3.state-dir",
		Usage: "durable local Shield3 request state directory",
	}

	shield3WalletCommand = &cli.Command{
		Name:      "shield3-wallet",
		Usage:     "Run the headless local Shield3 wallet service for a web wallet",
		ArgsUsage: "[IPC or RPC endpoint]",
		Flags: slicesConcat(
			[]cli.Flag{utils.DataDirFlag, utils.HttpHeaderFlag, shield3WalletHostFlag, shield3WalletPortFlag, shield3WalletOriginFlag, shield3WalletTokenFlag, shield3WalletStateFlag},
		),
		Description: `
Runs only the authenticated local Shield3 wallet API. It attaches to an
already-running gtkm node and does not start a desktop GUI or expose a public
RPC proxy. The spending seed is sent only to this loopback process, where the
canonical native prover builds, signs, and submits the transaction.

For the hosted wallet, use:

    gtkm shield3-wallet --datadir ~/.tkmchain \
      --shield3.allowed-origin https://wallet.tkmchain.site \
      --shield3.token-file ~/.tkmchain/shield3-wallet.token

Enter the token from the 0600 token file in the web wallet. Keep this service
bound to loopback; it deliberately refuses non-loopback requests. An IPC path
or local RPC URL may be supplied as the positional endpoint.
`,
		Action: runHeadlessShield3Wallet,
	}
)

// slicesConcat keeps this command independent from the large node flag set.
// urfave/cli accepts a plain slice and the helper avoids importing slices in
// older downstream build environments.
func slicesConcat(groups ...[]cli.Flag) []cli.Flag {
	var out []cli.Flag
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func runHeadlessShield3Wallet(ctx *cli.Context) error {
	host := ctx.String(shield3WalletHostFlag.Name)
	if host == "" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%s must be a loopback address; the headless Shield3 service never binds publicly", shield3WalletHostFlag.Name)
	}
	if origin := ctx.String(shield3WalletOriginFlag.Name); origin == "" {
		return fmt.Errorf("%s is required so the hosted wallet is explicitly authorized", shield3WalletOriginFlag.Name)
	}

	cfg := defaultNodeConfig()
	utils.SetDataDir(ctx, &cfg)
	endpoint := ctx.Args().First()
	if endpoint == "" {
		endpoint = cfg.IPCEndpoint()
	}
	client, err := utils.DialRPCWithHeaders(endpoint, ctx.StringSlice(utils.HttpHeaderFlag.Name))
	if err != nil {
		return fmt.Errorf("connect to gtkm at %s: %w (start gtkm first)", endpoint, err)
	}
	defer client.Close()

	dataDir := utils.MakeDataDir(ctx)
	tokenFile := ctx.Path(shield3WalletTokenFlag.Name)
	if tokenFile == "" {
		tokenFile = filepath.Join(dataDir, "shield3-wallet.token")
	}
	stateDir := ctx.Path(shield3WalletStateFlag.Name)
	if stateDir == "" {
		stateDir = filepath.Join(dataDir, "shield3-submissions")
	}
	socks5 := ctx.String("p2p.tor-socks5")
	if socks5 == "" {
		socks5 = "socks5://127.0.0.1:9050"
	}

	service, err := gui.New(client, gui.Options{
		Title:          "TKM Shield3 wallet service",
		Shield3Only:    true,
		WalletStateDir: stateDir,
		Host:           host,
		Port:           ctx.Int(shield3WalletPortFlag.Name),
		AllowedOrigin:  ctx.String(shield3WalletOriginFlag.Name),
		TokenFile:      tokenFile,
		RelayTransport: shield3wallet.RelayTransportConfig{SOCKS5Proxy: socks5, OnionOnly: true},
	})
	if err != nil {
		return err
	}
	defer service.Close()

	ctxSignal, stop := signal.NotifyContext(ctx.Context, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return service.RunHeadless(ctxSignal)
}
