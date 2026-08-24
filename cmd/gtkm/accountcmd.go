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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/urfave/cli/v2"
)

var (
	walletCommand = &cli.Command{
		Name:      "wallet",
		Usage:     "Manage Ethereum presale wallets",
		ArgsUsage: "",
		Description: `
    gtkm wallet import /path/to/my/presale.wallet

will prompt for your password and imports your ether presale account.
It can be used non-interactively with the --password option taking a
passwordfile as argument containing the wallet password in plaintext.`,
		Subcommands: []*cli.Command{
			{

				Name:      "import",
				Usage:     "Import Ethereum presale wallet",
				ArgsUsage: "<keyFile>",
				Action:    importWallet,
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.PasswordFileFlag,
					utils.LightKDFFlag,
				},
				Description: `
	gtkm wallet [options] /path/to/my/presale.wallet

will prompt for your password and imports your ether presale account.
It can be used non-interactively with the --password option taking a
passwordfile as argument containing the wallet password in plaintext.`,
			},
		},
	}

	accountCommand = &cli.Command{
		Name:  "account",
		Usage: "Manage accounts",
		Description: `

Manage accounts, list all existing accounts, import a private key into a new
account, create a new account or update an existing account.

It supports interactive mode, when you are prompted for password as well as
non-interactive mode where passwords are supplied via a given password file.
Non-interactive mode is only meant for scripted use on test networks or known
safe environments.

Make sure you remember the password you gave when creating a new account (with
either new or import). Without it you are not able to unlock your account.

Note that exporting your key in unencrypted format is NOT supported.

Keys are stored under <DATADIR>/keystore.
It is safe to transfer the entire directory or the individual keys therein
between ethereum nodes by simply copying.

Make sure you backup your keys regularly.`,
		Subcommands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "Print summary of existing accounts",
				Action: accountList,
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
				},
				Description: `
Print a short summary of all accounts`,
			},
			{
				Name:   "new",
				Usage:  "Create a new post-quantum account",
				Action: accountCreate,
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.PasswordFileFlag,
					utils.LightKDFFlag,
				},
				Description: `
    gtkm account new

Creates a new ML-DSA-87 post-quantum account and prints its public address and
mainnet tkmshield2 payment code.

The account is saved in encrypted format, you are prompted for a password.

You must remember this password to unlock your account in the future.

For non-interactive use the password can be specified with the --password flag:

Note, this is meant to be used for testing only, it is a bad idea to save your
password to file or expose in any other way.
`,
			},
			{
				Name:      "update",
				Usage:     "Update an existing account",
				Action:    accountUpdate,
				ArgsUsage: "<address>",
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.LightKDFFlag,
				},
				Description: `
    gtkm account update <address>

Update an existing account.

The account is saved in the newest version in encrypted format, you are prompted
for a password to unlock the account and another to save the updated file.

This same command can therefore be used to migrate an account of a deprecated
format to the newest format or change the password for an account.

For non-interactive use the password can be specified with the --password flag:

    gtkm account update [options] <address>

Since only one password can be given, only format update can be performed,
changing your password is only possible interactively.
`,
			},
			{
				Name:      "shielded-view-key",
				Usage:     "Export an account's shielded view-only key offline",
				ArgsUsage: "[address]",
				Action:    accountShieldedViewKey,
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.PasswordFileFlag,
				},
				Description: `
    gtkm account shielded-view-key [address]

Decrypts a local ML-DSA-87 keyfile and prints the matching mainnet tkmshield2
payment code and X25519 view-only private key required by tkmexchange-go.

This command runs entirely offline and does not modify the keyfile. If the
keystore contains exactly one PQ account, the address can be omitted. The view
key can recognize incoming shielded notes but cannot sign or spend funds.`,
			},
			{
				Name:   "import",
				Usage:  "Import a private key into a new account",
				Action: accountImport,
				Flags: []cli.Flag{
					utils.DataDirFlag,
					utils.KeyStoreDirFlag,
					utils.PasswordFileFlag,
					utils.LightKDFFlag,
				},
				ArgsUsage: "<keyFile>",
				Description: `
    gtkm account import <keyfile>

Imports an unencrypted private key from <keyfile> and creates a new account.
Prints the address.

The keyfile is assumed to contain an unencrypted private key in hexadecimal format.

The account is saved in encrypted format, you are prompted for a password.

You must remember this password to unlock your account in the future.

For non-interactive use the password can be specified with the -password flag:

    gtkm account import [options] <keyfile>

Note:
As you can directly copy your encrypted accounts to another ethereum instance,
this import mechanism is not needed when you transfer an account between
nodes.
`,
			},
		},
	}
)

// makeAccountManager creates an account manager with backends
func makeAccountManager(ctx *cli.Context) *accounts.Manager {
	cfg := loadBaseConfig(ctx)
	am := accounts.NewManager(nil)
	keydir, isEphemeral, err := cfg.Node.GetKeyStoreDir()
	if err != nil {
		utils.Fatalf("Failed to get the keystore directory: %v", err)
	}
	if isEphemeral {
		utils.Fatalf("Can't use ephemeral directory as keystore path")
	}

	if err := setAccountManagerBackends(&cfg.Node, am, keydir); err != nil {
		utils.Fatalf("Failed to set account manager backends: %v", err)
	}
	return am
}

type pqKeyfileMetadata struct {
	Address   string `json:"address"`
	Algorithm string `json:"algorithm"`
	Version   int    `json:"version"`
}

func findPQKeyfile(keydir, requestedAddress string) (string, error) {
	entries, err := os.ReadDir(keydir)
	if err != nil {
		return "", fmt.Errorf("read keystore directory: %w", err)
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(keydir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var metadata pqKeyfileMetadata
		if json.Unmarshal(raw, &metadata) != nil || metadata.Version != 4 || metadata.Algorithm != pqcrypto.AlgorithmMLDSA87 || !common.IsHexAddress(metadata.Address) {
			continue
		}
		if requestedAddress != "" && common.HexToAddress(metadata.Address) != common.HexToAddress(requestedAddress) {
			continue
		}
		matches = append(matches, path)
	}
	if len(matches) == 0 {
		if requestedAddress != "" {
			return "", fmt.Errorf("no ML-DSA-87 keyfile for %s in %s", common.HexToAddress(requestedAddress).Hex(), keydir)
		}
		return "", fmt.Errorf("no ML-DSA-87 keyfiles found in %s", keydir)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("keystore contains %d ML-DSA-87 accounts; specify one public address as the command argument", len(matches))
	}
	return matches[0], nil
}

func accountShieldedViewKey(ctx *cli.Context) error {
	if ctx.Args().Len() > 1 {
		return errors.New("at most one account address may be specified")
	}
	requestedAddress := strings.TrimSpace(ctx.Args().First())
	if requestedAddress != "" && !common.IsHexAddress(requestedAddress) {
		return errors.New("address must be specified in hexadecimal form")
	}
	cfg := loadBaseConfig(ctx)
	keydir, isEphemeral, err := cfg.Node.GetKeyStoreDir()
	if err != nil {
		return fmt.Errorf("get keystore directory: %w", err)
	}
	if isEphemeral {
		return errors.New("can't use ephemeral directory as keystore path")
	}
	keyfile, err := findPQKeyfile(keydir, requestedAddress)
	if err != nil {
		return err
	}
	keyJSON, err := os.ReadFile(keyfile)
	if err != nil {
		return fmt.Errorf("read keyfile: %w", err)
	}
	password, fromFile := readPasswordFromFile(ctx.Path(utils.PasswordFileFlag.Name))
	var key *keystore.PQKey
	for attempt := 0; attempt < 3; attempt++ {
		if !fromFile {
			password = utils.GetPassPhrase(fmt.Sprintf("Unlock PQ account to export its view-only key | Attempt %d/3", attempt+1), false)
		}
		key, err = keystore.DecryptPQKey(keyJSON, password)
		if err == nil || fromFile || !errors.Is(err, keystore.ErrDecrypt) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("decrypt PQ keyfile: %w", err)
	}
	defer clear(key.Seed)
	viewPrivateKey, err := pqcrypto.ShieldedViewPrivateKey(key.Seed)
	if err != nil {
		return fmt.Errorf("derive shielded view key: %w", err)
	}
	defer clear(viewPrivateKey)
	paymentCode, err := pqcrypto.ShieldedPaymentCode(key.Seed, params.MainnetChainConfig.ChainID, key.Address)
	if err != nil {
		return fmt.Errorf("derive shielded payment code: %w", err)
	}
	fmt.Printf("Public address:                    %s\n", key.Address.Hex())
	fmt.Printf("TKM_SHIELDED_SETTLEMENT_ADDRESS:  %s\n", paymentCode)
	fmt.Printf("TKM_SHIELDED_VIEW_PRIVATE_KEY:     %s\n", hex.EncodeToString(viewPrivateKey))
	fmt.Println("\nKeep the view-only key private. It can read incoming shielded payment details but cannot sign or spend funds.")
	return nil
}

func accountList(ctx *cli.Context) error {
	am := makeAccountManager(ctx)
	var index int
	for _, wallet := range am.Wallets() {
		for _, account := range wallet.Accounts() {
			fmt.Printf("Account #%d: {%x} %s\n", index, account.Address, &account.URL)
			index++
		}
	}

	return nil
}

// readPasswordFromFile reads the first line of the given file, trims line endings,
// and returns the password and whether the reading was successful.
func readPasswordFromFile(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	text, err := os.ReadFile(path)
	if err != nil {
		utils.Fatalf("Failed to read password file: %v", err)
	}
	lines := strings.Split(string(text), "\n")
	if len(lines) == 0 {
		return "", false
	}
	// Sanitise DOS line endings.
	return strings.TrimRight(lines[0], "\r"), true
}

// accountCreate creates a new post-quantum account into the keystore defined by the CLI flags.
func accountCreate(ctx *cli.Context) error {
	cfg := loadBaseConfig(ctx)
	keydir, isEphemeral, err := cfg.Node.GetKeyStoreDir()
	if err != nil {
		utils.Fatalf("Failed to get the keystore directory: %v", err)
	}
	if isEphemeral {
		utils.Fatalf("Can't use ephemeral directory as keystore path")
	}
	scryptN := keystore.StandardScryptN
	scryptP := keystore.StandardScryptP
	if cfg.Node.UseLightweightKDF {
		scryptN = keystore.LightScryptN
		scryptP = keystore.LightScryptP
	}

	password, ok := readPasswordFromFile(ctx.Path(utils.PasswordFileFlag.Name))
	if !ok {
		password = utils.GetPassPhrase("Your new account is locked with a password. Please give a password. Do not forget this password.", true)
	}
	ks := keystore.NewKeyStore(keydir, scryptN, scryptP)
	account, paymentCode, err := ks.NewPQAccountWithShieldedPaymentCode(password, params.MainnetChainConfig.ChainID)

	if err != nil {
		utils.Fatalf("Failed to create account: %v", err)
	}
	fmt.Printf("\nYour new post-quantum key was generated\n\n")
	fmt.Printf("Key algorithm:           ML-DSA-87\n")
	fmt.Printf("Public address:          %s\n", account.Address.Hex())
	fmt.Printf("Shielded payment code:   %s\n", paymentCode)
	fmt.Printf("Path of secret key file: %s\n\n", account.URL.Path)
	fmt.Printf("- You can share your public address with anyone. Others need it to interact with you.\n")
	fmt.Printf("- You must NEVER share the secret key with anyone! The key controls access to your funds!\n")
	fmt.Printf("- You must BACKUP your key file! Without the key, it's impossible to access account funds!\n")
	fmt.Printf("- You must REMEMBER your password! Without the password, it's impossible to decrypt the key!\n\n")
	return nil
}

// accountUpdate transitions an account from a previous format to the current
// one, also providing the possibility to change the pass-phrase.
func accountUpdate(ctx *cli.Context) error {
	if ctx.Args().Len() == 0 {
		utils.Fatalf("No accounts specified to update")
	}
	am := makeAccountManager(ctx)
	backends := am.Backends(keystore.KeyStoreType)
	if len(backends) == 0 {
		utils.Fatalf("Keystore is not available")
	}
	ks := backends[0].(*keystore.KeyStore)

	for _, addr := range ctx.Args().Slice() {
		if !common.IsHexAddress(addr) {
			return errors.New("address must be specified in hexadecimal form")
		}
		account := accounts.Account{Address: common.HexToAddress(addr)}
		newPassword := utils.GetPassPhrase("Please give a NEW password. Do not forget this password.", true)
		updateFn := func(attempt int) error {
			prompt := fmt.Sprintf("Please provide the OLD password for account %s | Attempt %d/%d", addr, attempt+1, 3)
			password := utils.GetPassPhrase(prompt, false)
			return ks.Update(account, password, newPassword)
		}
		// let user attempt unlock thrice.
		err := updateFn(0)
		for attempts := 1; attempts < 3 && errors.Is(err, keystore.ErrDecrypt); attempts++ {
			err = updateFn(attempts)
		}
		if err != nil {
			return fmt.Errorf("could not update account: %w", err)
		}
	}
	return nil
}

func importWallet(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		utils.Fatalf("keyfile must be given as the only argument")
	}
	keyfile := ctx.Args().First()
	keyJSON, err := os.ReadFile(keyfile)
	if err != nil {
		utils.Fatalf("Could not read wallet file: %v", err)
	}

	am := makeAccountManager(ctx)
	backends := am.Backends(keystore.KeyStoreType)
	if len(backends) == 0 {
		utils.Fatalf("Keystore is not available")
	}
	password, ok := readPasswordFromFile(ctx.Path(utils.PasswordFileFlag.Name))
	if !ok {
		password = utils.GetPassPhrase("", false)
	}
	ks := backends[0].(*keystore.KeyStore)
	acct, err := ks.ImportPreSaleKey(keyJSON, password)
	if err != nil {
		utils.Fatalf("%v", err)
	}
	fmt.Printf("Address: {%x}\n", acct.Address)
	return nil
}

func accountImport(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		utils.Fatalf("keyfile must be given as the only argument")
	}
	keyfile := ctx.Args().First()
	key, err := crypto.LoadECDSA(keyfile)
	if err != nil {
		utils.Fatalf("Failed to load the private key: %v", err)
	}
	am := makeAccountManager(ctx)
	backends := am.Backends(keystore.KeyStoreType)
	if len(backends) == 0 {
		utils.Fatalf("Keystore is not available")
	}
	ks := backends[0].(*keystore.KeyStore)
	password, ok := readPasswordFromFile(ctx.Path(utils.PasswordFileFlag.Name))
	if !ok {
		password = utils.GetPassPhrase("Your new account is locked with a password. Please give a password. Do not forget this password.", true)
	}
	acct, err := ks.ImportECDSA(key, password)
	if err != nil {
		utils.Fatalf("Could not create the account: %v", err)
	}
	fmt.Printf("Address: {%x}\n", acct.Address)
	return nil
}
