// Copyright 2026 The TKMChain Authors.
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/tkmnet"
	"github.com/urfave/cli/v2"
)

func TestUsernameFlagEnablesTKMNetWithoutForkGate(t *testing.T) {
	set := flag.NewFlagSet("gtkm-test", flag.ContinueOnError)
	set.Bool(utils.TkmnameEnabledFlag.Name, false, "")
	set.Bool(utils.TkmnetEnabledFlag.Name, false, "")
	set.String(utils.TkmnetListenFlag.Name, "127.0.0.1:0", "")
	ctx := cli.NewContext(cli.NewApp(), set, nil)
	if err := ctx.Set(utils.TkmnameEnabledFlag.Name, "true"); err != nil {
		t.Fatal(err)
	}
	cfg := &gethConfig{}
	applyTkmnetConfig(ctx, cfg)
	if !cfg.Tkmnet.Enabled {
		t.Fatal("--tkmname.enable did not enable the embedded TKMNet service")
	}
	if cfg.Tkmnet.ListenAddr != "127.0.0.1:39000" {
		t.Fatalf("--tkmname.enable listener = %q, want fixed loopback relay port", cfg.Tkmnet.ListenAddr)
	}
	if cfg.Tkmnet.SOCKS5Proxy != "socks5://127.0.0.1:9050" || cfg.Tkmnet.RelayPort != "39000" {
		t.Fatalf("--tkmname.enable Tor defaults = (%q, %q), want local SOCKS and onion port 39000", cfg.Tkmnet.SOCKS5Proxy, cfg.Tkmnet.RelayPort)
	}
}

func TestUsernameRPCIncludedInDefaultHTTPAndWSModules(t *testing.T) {
	cfg := defaultNodeConfig()
	for name, modules := range map[string][]string{"HTTP": cfg.HTTPModules, "WebSocket": cfg.WSModules} {
		found := false
		for _, module := range modules {
			if module == "tkmname" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s default modules %v do not include tkmname", name, modules)
		}
	}
}

func TestTkmnetRelayPeerSetIncludesTransitAndDirectories(t *testing.T) {
	var first, second, dir [32]byte
	first[0], second[0], dir[0] = 1, 2, 3
	got := mergeTkmnetRelayPeers(
		[]tkmnet.Descriptor{{ID: first}},
		[]tkmnet.Descriptor{{ID: first}, {ID: second}},
		[]tkmnet.PinnedDirectoryPeer{{Descriptor: tkmnet.Descriptor{ID: dir}}},
	)
	if len(got) != 3 || got[0].ID != first || got[1].ID != second || got[2].ID != dir {
		t.Fatalf("merged relay set = %#v, want unique configured relay, transit, and directory peers", got)
	}
}

func TestPersistTkmnetDescriptorCreatesStableLocalIdentity(t *testing.T) {
	stateDir := t.TempDir()
	relay, err := tkmnet.NewService(tkmnet.ServiceConfig{
		Enabled: true, ListenAddr: "127.0.0.1:0", PrivateKeyPath: filepath.Join(stateDir, "relay-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	onion := strings.Repeat("a", 56) + ".onion"
	first, err := persistTkmnetDescriptor(relay, onion, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Verify(time.Now()); err != nil {
		t.Fatalf("generated descriptor does not verify: %v", err)
	}
	gossip := tkmnet.NewDescriptorGossip()
	if added, err := gossip.Add(first); err != nil || !added {
		t.Fatalf("valid local descriptor was not accepted for gossip: added=%t err=%v", added, err)
	}
	if added, err := gossip.Add(first); err != nil || added {
		t.Fatalf("duplicate descriptor gossip result = added:%t err:%v, want ignored", added, err)
	}
	if got := gossip.Descriptors(); len(got) != 1 || got[0].ID != first.ID {
		t.Fatalf("verified gossip cache = %v, want local descriptor", got)
	}
	second, err := persistTkmnetDescriptor(relay, onion, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.SigningPublicKey) != string(second.SigningPublicKey) {
		t.Fatal("descriptor signing identity changed across restarts")
	}
	pin, err := os.ReadFile(filepath.Join(stateDir, "signing-key.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	wantPin := sha256.Sum256(second.SigningPublicKey)
	if strings.TrimSpace(string(pin)) != hex.EncodeToString(wantPin[:]) {
		t.Fatal("persisted signing-key pin does not match the descriptor")
	}
	info, err := os.Stat(filepath.Join(stateDir, "descriptor-signing.seed"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("descriptor signing seed permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestTkmnetOnionHostnamePrefersServiceHiddenService(t *testing.T) {
	stack, err := node.New(&node.Config{DataDir: t.TempDir(), Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer stack.Close()
	stateDir := filepath.Join(stack.InstanceDir(), "tkmnet")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	serviceOnion := strings.Repeat("s", 56) + ".onion\n"
	if err := os.WriteFile(filepath.Join(stateDir, "onion.hostname"), []byte(serviceOnion), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &gethConfig{}
	cfg.Node.P2P.OnionHostname = strings.Repeat("p", 56) + ".onion"
	if got := tkmnetOnionHostname(cfg, stack); got != strings.TrimSpace(serviceOnion) {
		t.Fatalf("TKMNet descriptor onion = %q, want service onion %q", got, strings.TrimSpace(serviceOnion))
	}
	cfg.Tkmnet.OnionHostname = strings.Repeat("c", 56) + ".onion"
	if got := tkmnetOnionHostname(cfg, stack); got != cfg.Tkmnet.OnionHostname {
		t.Fatalf("explicit TKMNet onion = %q, want %q", got, cfg.Tkmnet.OnionHostname)
	}
}

func TestTkmnetActivationFollowsCanonicalHead(t *testing.T) {
	before := big.NewInt(0)
	if tkmnetRequiredAt(params.MainnetChainConfig, before, params.MainnetAntarticalTime-1) {
		t.Fatal("TKMNet became required before the mainnet Antartical timestamp")
	}

	atFork := big.NewInt(0)
	if !tkmnetRequiredAt(params.MainnetChainConfig, atFork, params.MainnetAntarticalTime) {
		t.Fatal("TKMNet is not required at the mainnet Antartical timestamp")
	}

	config := &tkmnetConfig{}
	if !enableTkmnetAtFork(config, true) || !config.Enabled {
		t.Fatal("active Antartical fork did not enable TKMNet")
	}
	if enableTkmnetAtFork(config, true) {
		t.Fatal("already-enabled TKMNet was reported as newly enabled")
	}

	egypt := big.NewInt(0)
	if !tkmnetRequiredAt(params.EgyptChainConfig, egypt, 0) {
		t.Fatal("Egypt rehearsal network did not require TKMNet at genesis")
	}

	preFork := &tkmnetConfig{}
	if enableTkmnetAtFork(preFork, false) || preFork.Enabled {
		t.Fatal("pre-fork startup enabled TKMNet without the consensus gate")
	}
}
