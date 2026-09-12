//go:build windows && gtkmgui

package main

import (
	"os"
	"path/filepath"
)

// A double-clicked GUI executable opens the wallet; explicit CLI commands keep
// their existing behavior.
func init() {
	if len(os.Args) == 1 {
		executable, _ := os.Executable()
		os.Args = append(os.Args, "gui", "--http", "--http.addr", "127.0.0.1", "--http.api", "eth,net,web3,tkm,tkmprivacy", "--tkmprover", "--tkmprover.bin", filepath.Join(filepath.Dir(executable), "shielded-payout-prover.exe"))
	}
}
