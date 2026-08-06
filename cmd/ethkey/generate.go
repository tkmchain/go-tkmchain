// Copyright 2017 The go-ethereum Authors
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
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
)

type outputGenerate struct {
	Address      string
	AddressEIP55 string
	Algorithm    string `json:"algorithm,omitempty"`
	PublicKey    string `json:"publicKey,omitempty"`
}

var (
	privateKeyFlag = &cli.StringFlag{
		Name:  "privatekey",
		Usage: "file containing a raw private key to encrypt",
	}
	lightKDFFlag = &cli.BoolFlag{
		Name:  "lightkdf",
		Usage: "use less secure scrypt parameters",
	}
	pqFlag = &cli.BoolFlag{
		Name:  "pq",
		Usage: "generate an ML-DSA-87 post-quantum keyfile",
	}
	pqSeedFlag = &cli.StringFlag{
		Name:  "pqseed",
		Usage: "hex encoded 32-byte ML-DSA-87 seed, or a file containing it",
	}
)

var commandGenerate = &cli.Command{
	Name:      "generate",
	Usage:     "generate new keyfile",
	ArgsUsage: "[ <keyfile> ]",
	Description: `
Generate a new keyfile.

If you want to encrypt an existing private key, it can be specified by setting
--privatekey with the location of the file containing the private key.
`,
	Flags: []cli.Flag{
		passphraseFlag,
		jsonFlag,
		privateKeyFlag,
		lightKDFFlag,
		pqFlag,
		pqSeedFlag,
	},
	Action: func(ctx *cli.Context) error {
		// Check if keyfile path given and make sure it doesn't already exist.
		keyfilepath := ctx.Args().First()
		if keyfilepath == "" {
			keyfilepath = defaultKeyfileName
		}
		if _, err := os.Stat(keyfilepath); err == nil {
			utils.Fatalf("Keyfile already exists at %s.", keyfilepath)
		} else if !os.IsNotExist(err) {
			utils.Fatalf("Error checking if keyfile exists: %v", err)
		}

		passphrase := getPassphrase(ctx, true)
		scryptN, scryptP := keystore.StandardScryptN, keystore.StandardScryptP
		if ctx.Bool(lightKDFFlag.Name) {
			scryptN, scryptP = keystore.LightScryptN, keystore.LightScryptP
		}

		if ctx.Bool(pqFlag.Name) || ctx.String(pqSeedFlag.Name) != "" {
			if ctx.String(privateKeyFlag.Name) != "" {
				utils.Fatalf("Can't use --privatekey with --pq or --pqseed.")
			}
			var key *keystore.PQKey
			var err error
			if seedSpec := ctx.String(pqSeedFlag.Name); seedSpec != "" {
				seed, err := loadPQSeed(seedSpec)
				if err != nil {
					utils.Fatalf("Can't load PQ seed: %v", err)
				}
				key, err = keystore.NewPQKeyFromSeed(seed)
				clear(seed)
			} else {
				key, err = keystore.NewPQKey()
			}
			if err != nil {
				utils.Fatalf("Failed to create PQ key: %v", err)
			}
			defer zeroPQKeySeed(key)
			keyjson, err := keystore.EncryptPQKey(key, passphrase, scryptN, scryptP)
			if err != nil {
				utils.Fatalf("Error encrypting PQ key: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(keyfilepath), 0700); err != nil {
				utils.Fatalf("Could not create directory %s", filepath.Dir(keyfilepath))
			}
			if err := os.WriteFile(keyfilepath, keyjson, 0600); err != nil {
				utils.Fatalf("Failed to write keyfile to %s: %v", keyfilepath, err)
			}
			out := outputGenerate{
				Address:   key.Address.Hex(),
				Algorithm: key.Algorithm,
				PublicKey: hex.EncodeToString(key.PublicKey),
			}
			if ctx.Bool(jsonFlag.Name) {
				mustPrintJSON(out)
			} else {
				fmt.Println("Address:", out.Address)
				fmt.Println("Algorithm:", out.Algorithm)
				fmt.Println("Public key:", out.PublicKey)
			}
			return nil
		}

		var privateKey *ecdsa.PrivateKey
		var err error
		if file := ctx.String(privateKeyFlag.Name); file != "" {
			// Load private key from file.
			privateKey, err = crypto.LoadECDSA(file)
			if err != nil {
				utils.Fatalf("Can't load private key: %v", err)
			}
		} else {
			// If not loaded, generate random.
			privateKey, err = crypto.GenerateKey()
			if err != nil {
				utils.Fatalf("Failed to generate random private key: %v", err)
			}
		}

		// Create the keyfile object with a random UUID.
		UUID, err := uuid.NewRandom()
		if err != nil {
			utils.Fatalf("Failed to generate random uuid: %v", err)
		}
		key := &keystore.Key{
			Id:         UUID,
			Address:    crypto.PubkeyToAddress(privateKey.PublicKey),
			PrivateKey: privateKey,
		}

		keyjson, err := keystore.EncryptKey(key, passphrase, scryptN, scryptP)
		if err != nil {
			utils.Fatalf("Error encrypting key: %v", err)
		}

		// Store the file to disk.
		if err := os.MkdirAll(filepath.Dir(keyfilepath), 0700); err != nil {
			utils.Fatalf("Could not create directory %s", filepath.Dir(keyfilepath))
		}
		if err := os.WriteFile(keyfilepath, keyjson, 0600); err != nil {
			utils.Fatalf("Failed to write keyfile to %s: %v", keyfilepath, err)
		}

		// Output some information.
		out := outputGenerate{
			Address: key.Address.Hex(),
		}
		if ctx.Bool(jsonFlag.Name) {
			mustPrintJSON(out)
		} else {
			fmt.Println("Address:", out.Address)
		}
		return nil
	},
}

func loadPQSeed(seedSpec string) ([]byte, error) {
	seedHex := strings.TrimSpace(seedSpec)
	if content, err := os.ReadFile(seedSpec); err == nil {
		seedHex = strings.TrimSpace(string(content))
	}
	seedHex = strings.TrimPrefix(seedHex, "0x")
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, err
	}
	if len(seed) != 32 {
		return nil, fmt.Errorf("invalid ML-DSA-87 seed length %d, want 32", len(seed))
	}
	return seed, nil
}

func zeroPQKeySeed(key *keystore.PQKey) {
	if key != nil {
		clear(key.Seed)
	}
}
