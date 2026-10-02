// Copyright 2016 The go-ethereum Authors
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
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/cespare/cp"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// These tests are 'smoke tests' for the account related
// subcommands and flags.
//
// For most tests, the test files from package accounts
// are copied into a temporary keystore directory.

func tmpDatadirWithKeystore(t *testing.T) string {
	datadir := t.TempDir()
	keystore := filepath.Join(datadir, "keystore")
	source := filepath.Join("..", "..", "accounts", "keystore", "testdata", "keystore")
	if err := cp.CopyAll(keystore, source); err != nil {
		t.Fatal(err)
	}
	return datadir
}

func TestAccountListEmpty(t *testing.T) {
	t.Parallel()
	geth := runGeth(t, "account", "list")
	geth.ExpectExit()
}

func TestAccountList(t *testing.T) {
	t.Parallel()
	datadir := tmpDatadirWithKeystore(t)
	var want = `
Account #0: {7ef5a6135f1fd6a02593eedc869c6d41d934aef8} keystore://{{.Datadir}}/keystore/UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8
Account #1: {f466859ead1932d743d622cb74fc058882e8648a} keystore://{{.Datadir}}/keystore/aaa
Account #2: {289d485d9771714cce91d3393d764e1311907acc} keystore://{{.Datadir}}/keystore/zzz
`
	if runtime.GOOS == "windows" {
		want = `
Account #0: {7ef5a6135f1fd6a02593eedc869c6d41d934aef8} keystore://{{.Datadir}}\keystore\UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8
Account #1: {f466859ead1932d743d622cb74fc058882e8648a} keystore://{{.Datadir}}\keystore\aaa
Account #2: {289d485d9771714cce91d3393d764e1311907acc} keystore://{{.Datadir}}\keystore\zzz
`
	}
	{
		geth := runGeth(t, "account", "list", "--datadir", datadir)
		geth.Expect(want)
		geth.ExpectExit()
	}
	{
		geth := runGeth(t, "--datadir", datadir, "account", "list")
		geth.Expect(want)
		geth.ExpectExit()
	}
}

func TestAccountNew(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Shield3")
	}
	t.Parallel()
	geth := runGeth(t, "account", "new", "--lightkdf")
	defer geth.ExpectExit()
	geth.Expect(`
Your new account is locked with a password. Please give a password. Do not forget this password.
!! Unsupported terminal, password will be echoed.
Password: {{.InputLine "foobar"}}
Repeat password: {{.InputLine "foobar"}}
Private stamp name: {{.InputLine "Private Test Name"}}
Private stamp country: {{.InputLine "Private Test Country"}}

Your new post-quantum key was generated
`)
	geth.ExpectRegexp(`
Key algorithm:           ML-DSA-87
Public address:          0x[0-9a-fA-F]{40}
Shield3 address:         tkmshield3\.[A-Za-z0-9_-]+
Path of secret key file: .*UTC--.+--[0-9a-f]{40}

- Share your Shield3 address to receive private payments.
- Your stamp is saved locally. Before payments, register it with gtkm wallet interactive -> Stamp address.
- You must NEVER share the secret key with anyone! The key controls access to your funds!
- You must BACKUP your key file! Without the key, it's impossible to access account funds!
- You must REMEMBER your password! Without the password, it's impossible to decrypt the key!
`)
}

func TestAccountShieldedViewKey(t *testing.T) {
	t.Parallel()
	keydir := filepath.Join(t.TempDir(), "keystore")
	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	if err := os.WriteFile(passwordFile, []byte("view-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ks := keystore.NewKeyStore(keydir, keystore.LightScryptN, keystore.LightScryptP)
	account, paymentCode, err := ks.NewPQAccountWithShieldedPaymentCode("view-password", params.MainnetChainConfig.ChainID)
	if err != nil {
		t.Fatal(err)
	}
	keyJSON, err := os.ReadFile(account.URL.Path)
	if err != nil {
		t.Fatal(err)
	}
	key, err := keystore.DecryptPQKey(keyJSON, "view-password")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key.Seed)
	viewPrivateKey, err := pqcrypto.ShieldedViewPrivateKey(key.Seed)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(viewPrivateKey)

	geth := runGeth(t, "account", "shielded-view-key", "--keystore", keydir, "--password", passwordFile)
	defer geth.ExpectExit()
	geth.Expect(`
Public address:                    ` + account.Address.Hex() + `
TKM_SHIELDED_SETTLEMENT_ADDRESS:  ` + paymentCode + `
TKM_SHIELDED_VIEW_PRIVATE_KEY:     ` + hex.EncodeToString(viewPrivateKey) + `

Keep the view-only key private. It can read incoming shielded payment details but cannot sign or spend funds.
`)
}

func TestAccountImport(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, key, output string }{
		{
			name:   "correct account",
			key:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			output: "Address: {fcad0b19bb29d4674531d6f115237e16afce377c}\n",
		},
		{
			name:   "invalid character",
			key:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef1",
			output: "Fatal: Failed to load the private key: invalid character '1' at end of key file\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			importAccountWithExpect(t, test.key, test.output)
		})
	}
}

func TestAccountHelp(t *testing.T) {
	t.Parallel()
	geth := runGeth(t, "account", "-h")
	geth.WaitExit()
	if have, want := geth.ExitStatus(), 0; have != want {
		t.Errorf("exit error, have %d want %d", have, want)
	}

	geth = runGeth(t, "account", "import", "-h")
	geth.WaitExit()
	if have, want := geth.ExitStatus(), 0; have != want {
		t.Errorf("exit error, have %d want %d", have, want)
	}
}

func importAccountWithExpect(t *testing.T, key string, expected string) {
	dir := t.TempDir()
	keyfile := filepath.Join(dir, "key.prv")
	if err := os.WriteFile(keyfile, []byte(key), 0600); err != nil {
		t.Error(err)
	}
	passwordFile := filepath.Join(dir, "password.txt")
	if err := os.WriteFile(passwordFile, []byte("foobar"), 0600); err != nil {
		t.Error(err)
	}
	geth := runGeth(t, "--lightkdf", "account", "import", "-password", passwordFile, keyfile)
	defer geth.ExpectExit()
	geth.Expect(expected)
}

func TestAccountNewBadRepeat(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Shield3")
	}
	t.Parallel()
	geth := runGeth(t, "account", "new", "--lightkdf")
	defer geth.ExpectExit()
	geth.Expect(`
Your new account is locked with a password. Please give a password. Do not forget this password.
!! Unsupported terminal, password will be echoed.
Password: {{.InputLine "something"}}
Repeat password: {{.InputLine "something else"}}
Fatal: Passwords do not match
`)
}

func TestAccountUpdate(t *testing.T) {
	t.Parallel()
	datadir := tmpDatadirWithKeystore(t)
	geth := runGeth(t, "account", "update",
		"--datadir", datadir, "--lightkdf",
		"f466859ead1932d743d622cb74fc058882e8648a")
	defer geth.ExpectExit()
	geth.Expect(`
Please give a NEW password. Do not forget this password.
!! Unsupported terminal, password will be echoed.
Password: {{.InputLine "foobar2"}}
Repeat password: {{.InputLine "foobar2"}}
Please provide the OLD password for account f466859ead1932d743d622cb74fc058882e8648a | Attempt 1/3
Password: {{.InputLine "foobar"}}
`)
}

func TestWalletImport(t *testing.T) {
	t.Parallel()
	geth := runGeth(t, "wallet", "import", "--lightkdf", "testdata/guswallet.json")
	defer geth.ExpectExit()
	geth.Expect(`
!! Unsupported terminal, password will be echoed.
Password: {{.InputLine "foo"}}
Address: {d4584b5f6229b7be90727b0fc8c6b91bb427821f}
`)

	files, err := os.ReadDir(filepath.Join(geth.Datadir, "keystore"))
	if len(files) != 1 {
		t.Errorf("expected one key file in keystore directory, found %d files (error: %v)", len(files), err)
	}
}

func TestWalletImportBadPassword(t *testing.T) {
	t.Parallel()
	geth := runGeth(t, "wallet", "import", "--lightkdf", "testdata/guswallet.json")
	defer geth.ExpectExit()
	geth.Expect(`
!! Unsupported terminal, password will be echoed.
Password: {{.InputLine "wrong"}}
Fatal: could not decrypt key with given password
`)
}

// Exercise the real command, then restore its encrypted keyfile and verify that
// the printed receiving identity retained the exact stamp and chain binding.
func TestAccountNewShield3Backup(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Shield3")
	}
	for _, chainID := range []uint64{8979, 8980} {
		t.Run(fmt.Sprint(chainID), func(t *testing.T) {
			dir := t.TempDir()
			passwordFile := filepath.Join(dir, "password")
			stampFile := filepath.Join(dir, "stamp.json")
			if err := os.WriteFile(passwordFile, []byte("shield3-password\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stampFile, []byte(`{"name":"Private Test Name","country":"Private Test Country"}`), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"account", "new", "--lightkdf", "--password", passwordFile, "--stamp-file", stampFile}
			if chainID == 8980 {
				args = append(args, "--egypt")
			}
			geth := runGeth(t, args...)
			_, matches := geth.ExpectRegexp(`Shield3 address:         (tkmshield3\.[A-Za-z0-9_-]+)
Path of secret key file: ([^\r\n]+)`)
			geth.ExpectRegexp(`- You must REMEMBER your password! Without the password, it's impossible to decrypt the key!

`)
			geth.ExpectExit()
			payload, err := shield3wallet.DecodePaymentCode(matches[1], chainID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := shield3wallet.DecodePaymentCode(matches[1], chainID+1); err == nil {
				t.Fatal("address accepted on another chain")
			}
			blob, err := os.ReadFile(matches[2])
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(blob, []byte("Private Test")) {
				t.Fatal("plaintext stamp leaked into keyfile")
			}
			key, err := keystore.DecryptPQKey(blob, "shield3-password")
			if err != nil {
				t.Fatal(err)
			}
			defer clear(key.Seed)
			identity, err := shield3wallet.NewIdentity(key.Seed, chainID, key.Shield3Stamp)
			if err != nil {
				t.Fatal(err)
			}
			defer identity.Clear()
			restored, err := shield3wallet.DecodePaymentCode(identity.Code, chainID)
			if err != nil || !reflect.DeepEqual(payload, restored) || payload.Address != key.Address {
				t.Fatalf("keyfile did not restore receiving identity: %v", err)
			}
			text, err := pqcrypto.OpenShieldedV3Stamp(identity.StampSeed, key.Shield3Stamp)
			if err != nil || text.Name != "Private Test Name" || text.Country != "Private Test Country" {
				t.Fatalf("stamp did not round-trip: %v", err)
			}
		})
	}
}

func TestAccountNewRejectsInvalidStamp(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Shield3")
	}
	for _, stamp := range []string{"", `{`, `{"name":"","country":"Country"}`} {
		t.Run(stamp, func(t *testing.T) {
			dir := t.TempDir()
			passwordFile := filepath.Join(dir, "password")
			if err := os.WriteFile(passwordFile, []byte("pass"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"account", "new", "--lightkdf", "--password", passwordFile}
			if stamp != "" {
				file := filepath.Join(dir, "stamp.json")
				if err := os.WriteFile(file, []byte(stamp), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--stamp-file", file)
			}
			geth := runGeth(t, args...)
			geth.WaitExit()
			if geth.ExitStatus() == 0 {
				t.Fatal("invalid stamp succeeded")
			}
			files, err := filepath.Glob(filepath.Join(geth.Datadir, "keystore", "*"))
			if err != nil || len(files) != 0 {
				t.Fatalf("failed command wrote a keyfile: %v %v", files, err)
			}
		})
	}
}

func TestAccountNewRequiresShield3Build(t *testing.T) {
	if shielded3.NativeAvailable() {
		t.Skip("tests build without native Shield3")
	}
	geth := runGeth(t, "account", "new")
	geth.WaitExit()
	if geth.ExitStatus() == 0 {
		t.Fatal("silently created a legacy account without Shield3")
	}
	files, err := filepath.Glob(filepath.Join(geth.Datadir, "keystore", "*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("unavailable Shield3 wrote a keyfile: %v %v", files, err)
	}
}
