// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testPQSeedHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	testPQAddress = "0x803e6EE61B7Ecba64eDF13ce0c4a8a65C495e5A5"
)

func TestPQGenerateAndInspect(t *testing.T) {
	t.Parallel()
	tmpdir := t.TempDir()
	keyfile := filepath.Join(tmpdir, "pq-keyfile.json")
	passfile := filepath.Join(tmpdir, "password.txt")
	if err := os.WriteFile(passfile, []byte("test-pass\n"), 0600); err != nil {
		t.Fatal(err)
	}

	generate := runEthkey(t, "generate", "--pq", "--pqseed", testPQSeedHex, "--passwordfile", passfile, "--lightkdf", "--json", keyfile)
	generate.ExpectRegexp(`"Address": "` + testPQAddress + `"`)
	generate.ExpectRegexp(`"algorithm": "ML-DSA-87"`)
	generate.ExpectRegexp(`"publicKey": "[0-9a-f]+"\n}`)
	generate.ExpectExit()

	inspect := runEthkey(t, "inspect", "--passwordfile", passfile, "--json", keyfile)
	inspect.ExpectRegexp(`"Address": "` + testPQAddress + `"`)
	inspect.ExpectRegexp(`"algorithm": "ML-DSA-87"`)
	inspect.ExpectRegexp(`"PublicKey": "[0-9a-f]+"\n}`)
	inspect.ExpectExit()
}
