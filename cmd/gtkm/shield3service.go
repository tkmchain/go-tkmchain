// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.

package main

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/internal/gui"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
)

// embeddedShield3 owns the Shield3 listener started by the main gtkm process.
// Keeping the listener and the node in one process prevents a second gtkm
// instance from racing over the datadir or exposing a stale RPC client.
type embeddedShield3 struct {
	service *gui.GUI
	client  *rpc.Client
	cancel  context.CancelFunc
	done    <-chan error
}

func startEmbeddedShield3(ctx *cli.Context, stack *node.Node) (*embeddedShield3, error) {
	if !ctx.Bool(shield3ServiceEnabledFlag.Name) {
		return nil, nil
	}

	host := ctx.String(shield3WalletHostFlag.Name)
	if host == "" {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("%s must be a loopback address; the embedded Shield3 service never binds publicly", shield3WalletHostFlag.Name)
	}

	dataDir := utils.MakeDataDir(ctx)
	tokenFile := ctx.Path(shield3WalletTokenFlag.Name)
	if tokenFile == "" {
		tokenFile = filepath.Join(dataDir, "shield3-wallet.token")
	}
	stateDir := ctx.Path(shield3WalletStateFlag.Name)
	if stateDir == "" {
		stateDir = filepath.Join(dataDir, "shield3-submissions")
	}
	origin := ctx.String(shield3WalletOriginFlag.Name)
	if origin == "" {
		origin = "https://wallet.tkmchain.site"
	}
	socks5 := ctx.String(utils.P2PSOCKS5ProxyFlag.Name)
	if socks5 == "" {
		socks5 = "socks5://127.0.0.1:9050"
	}

	client := stack.Attach()
	service, err := gui.New(client, gui.Options{
		Title:          "TKM Shield3 wallet service",
		Shield3Only:    true,
		WalletStateDir: stateDir,
		Host:           host,
		Port:           ctx.Int(shield3WalletPortFlag.Name),
		AllowedOrigin:  origin,
		TokenFile:      tokenFile,
		RelayTransport: shield3wallet.RelayTransportConfig{SOCKS5Proxy: socks5, OnionOnly: true},
	})
	if err != nil {
		client.Close()
		return nil, err
	}
	if err := service.StartHeadless(); err != nil {
		service.Close()
		client.Close()
		return nil, fmt.Errorf("start embedded Shield3 service: %w (stop any standalone gtkm shield3-wallet process first)", err)
	}

	serviceCtx, cancel := context.WithCancel(ctx.Context)
	done := make(chan error, 1)
	go func() {
		done <- service.Wait(serviceCtx)
	}()
	log.Info("Shield3 service started with gtkm", "url", service.URL(), "origin", origin, "tokenFile", tokenFile)
	return &embeddedShield3{service: service, client: client, cancel: cancel, done: done}, nil
}

func (s *embeddedShield3) Close() {
	if s == nil {
		return
	}
	s.cancel()
	_ = s.service.Close()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		log.Warn("Shield3 service did not stop within the shutdown timeout")
	}
	s.client.Close()
}
