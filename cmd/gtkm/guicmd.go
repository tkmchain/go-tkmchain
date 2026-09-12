// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/ethereum/go-ethereum/internal/gui"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

var (
	guiBrowserFlag = &cli.BoolFlag{
		Name:  "gui.browser",
		Usage: "Open the dashboard in the default browser instead of a desktop window",
	}
	guiPortFlag = &cli.IntFlag{
		Name:  "gui.port",
		Usage: "HTTP port for the dashboard (0 = random free port)",
		Value: 0,
	}
	guiHostFlag = &cli.StringFlag{
		Name:  "gui.host",
		Usage: "Bind address for the dashboard (default 127.0.0.1; use 0.0.0.0 to reach it from your phone on the LAN)",
		Value: "",
	}

	guiFlags = []cli.Flag{guiBrowserFlag, guiPortFlag, guiHostFlag}

	guiCommand = &cli.Command{
		Action: localGUI,
		Name:   "gui",
		Usage:  "Start a desktop GUI for the node with mining, kings, wallet, phone, mail, supply and governance dashboards",
		Flags:  slices.Concat(nodeFlags, rpcFlags, guiFlags),
		Description: `
The gtkm GUI starts a full node and opens a native desktop window (or your
default browser with --gui.browser) showing live dashboards for chain status,
RandomX mining, Rotating Kings, wallet/accounts, TKM Phone, EmailVM, supply
accounting, governance disclosures, and a generic JSON-RPC explorer covering
every exposed API namespace.

All node flags apply, for example:

    gtkm gui --mine --miner.threads=2 --miner.etherbase=0xYourAddress
    gtkm gui --http --http.api eth,net,web3,tkm,tkmphone,emailvm,rk,mainking,miner

Mobile / LAN: the dashboard is also a installable progressive web app. Expose
it with --gui.host 0.0.0.0 and a fixed --gui.port, then open the printed URL
on your phone. Note: anyone on the network can then read the RPC token and
call node methods (including spending from unlocked keystore accounts).

Build the native desktop binary:

    make gtkm-gui        # Linux desktop (webkit2gtk)
    make gtkm-gui-windows # Windows desktop (cross-compile, WebView2)

Windows builds are cross-compiled with mingw-w64 and need the Edge/Chromium
WebView2 runtime installed on the target machine.`,
	}
)

// localGUI starts a full node and opens the dashboard on top of it. It blocks
// until the window/browser is closed or the process receives a signal.
func localGUI(ctx *cli.Context) error {
	prepare(ctx)
	stack, _ := makeFullNode(ctx)
	startNode(ctx, stack, true)
	defer stack.Close()

	proverContext, cancelProver := context.WithCancel(ctx.Context)
	proverCLI := *ctx
	proverCLI.Context = proverContext
	proverDone := make(chan struct{})
	go func() {
		defer close(proverDone)
		prover, err := startTkmProver(&proverCLI)
		if err != nil {
			log.Error("Wallet proof builder setup failed", "err", err)
			return
		}
		<-proverContext.Done()
		prover.stop()
	}()
	defer func() { cancelProver(); <-proverDone }()

	proverConfig := ctx.Path(tkmProverConfigFlag.Name)
	if proverConfig == "" {
		home, _ := os.UserHomeDir()
		proverConfig = filepath.Join(home, ".tkmchain", "tkmprover", "config.json")
	}
	client := stack.Attach()

	g, err := gui.New(client, gui.Options{
		Title:        "TKM Wallet",
		ProverConfig: proverConfig,
		Width:        1280,
		Height:       800,
		Port:         ctx.Int(guiPortFlag.Name),
		Host:         ctx.String(guiHostFlag.Name),
		ForceBrowser: ctx.Bool(guiBrowserFlag.Name),
	})
	if err != nil {
		return err
	}
	defer g.Close()

	return g.Run(context.Background())
}
