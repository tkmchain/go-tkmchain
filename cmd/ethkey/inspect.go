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
	"encoding/hex"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/urfave/cli/v2"
)

type outputInspect struct {
	Address    string
	Algorithm  string `json:"algorithm,omitempty"`
	PublicKey  string
	PrivateKey string `json:"privateKey,omitempty"`
}

var (
	privateFlag = &cli.BoolFlag{
		Name:  "private",
		Usage: "include the private key in the output",
	}
)

var commandInspect = &cli.Command{
	Name:      "inspect",
	Usage:     "inspect a keyfile",
	ArgsUsage: "<keyfile>",
	Description: `
Print various information about the keyfile.

Private key information can be printed by using the --private flag;
make sure to use this feature with great caution!`,
	Flags: []cli.Flag{
		passphraseFlag,
		jsonFlag,
		privateFlag,
	},
	Action: func(ctx *cli.Context) error {
		keyfilepath := ctx.Args().First()

		// Read key from file.
		keyjson, err := os.ReadFile(keyfilepath)
		if err != nil {
			utils.Fatalf("Failed to read the keyfile at '%s': %v", keyfilepath, err)
		}

		// Decrypt key with passphrase.
		passphrase := getPassphrase(ctx, false)
		showPrivate := ctx.Bool(privateFlag.Name)
		if pqKey, err := keystore.DecryptPQKey(keyjson, passphrase); err == nil {
			defer clear(pqKey.Seed)
			out := outputInspect{
				Address:   pqKey.Address.Hex(),
				Algorithm: pqKey.Algorithm,
				PublicKey: hex.EncodeToString(pqKey.PublicKey),
			}
			if showPrivate {
				out.PrivateKey = hex.EncodeToString(pqKey.Seed)
			}
			if ctx.Bool(jsonFlag.Name) {
				mustPrintJSON(out)
			} else {
				fmt.Println("Address:       ", out.Address)
				fmt.Println("Algorithm:     ", out.Algorithm)
				fmt.Println("Public key:    ", out.PublicKey)
				if showPrivate {
					fmt.Println("Private seed:  ", out.PrivateKey)
				}
			}
			return nil
		}
		key, err := keystore.DecryptKey(keyjson, passphrase)
		if err != nil {
			utils.Fatalf("Error decrypting key: %v", err)
		}

		// Output all relevant information we can retrieve.
		out := outputInspect{
			Address:   key.Address.Hex(),
			Algorithm: keystore.AlgorithmECDSA,
			PublicKey: hex.EncodeToString(
				crypto.FromECDSAPub(&key.PrivateKey.PublicKey)),
		}
		if showPrivate {
			out.PrivateKey = hex.EncodeToString(crypto.FromECDSA(key.PrivateKey))
		}

		if ctx.Bool(jsonFlag.Name) {
			mustPrintJSON(out)
		} else {
			fmt.Println("Address:       ", out.Address)
			fmt.Println("Algorithm:     ", out.Algorithm)
			fmt.Println("Public key:    ", out.PublicKey)
			if showPrivate {
				fmt.Println("Private key:   ", out.PrivateKey)
			}
		}
		return nil
	},
}
