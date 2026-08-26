// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
)

const (
	portableEmailKeyType    = "TKM_EMAILVM_MAIL_KEY"
	portableEmailKeyVersion = 1
	portableEmailAlgorithm  = "X25519"
	portableEmailScryptN    = 1 << 17
	portableEmailScryptR    = 8
	portableEmailScryptP    = 1
	portableEmailKeyBytes   = 32
)

var (
	portableMailboxPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	emailVMOutputFlag       = &cli.PathFlag{Name: "out", Aliases: []string{"o"}, Usage: "encrypted portable mail-key output file"}
	emailVMMailPasswordFlag = &cli.PathFlag{Name: "mail-password", Usage: "file containing the portable mail-key password (interactive prompt when omitted)"}
	emailVMExportKeyCommand = &cli.Command{
		Name:      "export-key",
		Usage:     "Export one mailbox's password-encrypted private mail key locally",
		ArgsUsage: "<username@domain>",
		Action:    emailVMExportKey,
		Flags: []cli.Flag{
			utils.DataDirFlag,
			utils.KeyStoreDirFlag,
			utils.PasswordFileFlag,
			utils.HttpHeaderFlag,
			domainRPCFlag,
			emailVMOutputFlag,
			emailVMMailPasswordFlag,
		},
		Description: `Decrypts the canonical mailbox owner's local ML-DSA-87
keystore and writes a web-wallet-compatible encrypted X25519 mail keyfile.
The PQ seed and both passwords remain local and are never sent to RPC.

Example:
  gtkm emailvm export-key info@tkm --keystore ~/.tkmchain/keystore

Use --password and --mail-password only for controlled non-interactive use;
each flag names a local file whose first line contains the corresponding
password. The output is created with mode 0600 and is never overwritten.`,
	}
)

type portableEmailMetadata struct {
	Type      string   `json:"type"`
	Version   int      `json:"version"`
	Algorithm string   `json:"algorithm"`
	PublicKey string   `json:"publicKey"`
	Owner     string   `json:"owner"`
	Mailboxes []string `json:"mailboxes"`
}

type portableEmailCipherParams struct {
	Nonce string `json:"nonce"`
}

type portableEmailKDFParams struct {
	N     int    `json:"n"`
	R     int    `json:"r"`
	P     int    `json:"p"`
	DKLen int    `json:"dklen"`
	Salt  string `json:"salt"`
}

type portableEmailCrypto struct {
	Cipher       string                    `json:"cipher"`
	Ciphertext   string                    `json:"ciphertext"`
	CipherParams portableEmailCipherParams `json:"cipherparams"`
	KDF          string                    `json:"kdf"`
	KDFParams    portableEmailKDFParams    `json:"kdfparams"`
}

type portableEmailKeyfile struct {
	portableEmailMetadata
	Crypto portableEmailCrypto `json:"crypto"`
}

type emailVMExportMailbox struct {
	Address       string         `json:"address"`
	Owner         common.Address `json:"owner"`
	EncryptionKey hexutil.Bytes  `json:"encryptionKey"`
}

func emailVMExportKey(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return errors.New("usage: gtkm emailvm export-key <username@domain>")
	}
	mailbox := strings.ToLower(strings.TrimSpace(ctx.Args().First()))
	if !portableMailboxPattern.MatchString(mailbox) {
		return errors.New("mailbox must be a canonical username@domain address")
	}
	client, err := domainDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var record emailVMExportMailbox
	if err := client.CallContext(context.Background(), &record, "tkmdomain_mailbox", mailbox); err != nil {
		return fmt.Errorf("resolve canonical mailbox: %w", err)
	}
	if record.Owner == (common.Address{}) || !strings.EqualFold(record.Address, mailbox) {
		return errors.New("RPC returned an invalid canonical mailbox owner")
	}

	cfg := loadBaseConfig(ctx)
	keydir, isEphemeral, err := cfg.Node.GetKeyStoreDir()
	if err != nil {
		return fmt.Errorf("get keystore directory: %w", err)
	}
	if isEphemeral {
		return errors.New("can't use ephemeral directory as keystore path")
	}
	keyfile, err := findPQKeyfile(keydir, record.Owner.Hex())
	if err != nil {
		return fmt.Errorf("find PQ keyfile for %s owner %s: %w", mailbox, record.Owner.Hex(), err)
	}
	keyJSON, err := os.ReadFile(keyfile)
	if err != nil {
		return fmt.Errorf("read PQ keyfile: %w", err)
	}
	password, fromFile := readPasswordFromFile(ctx.Path(utils.PasswordFileFlag.Name))
	var key *keystore.PQKey
	for attempt := 0; attempt < 3; attempt++ {
		if !fromFile {
			password = utils.GetPassPhrase(fmt.Sprintf("Unlock PQ owner of %s | Attempt %d/3", mailbox, attempt+1), false)
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
	if key.Address != record.Owner {
		return errors.New("decrypted PQ key does not own the canonical mailbox")
	}
	mailPrivateKey, err := pqcrypto.EmailVMPrivateKey(key.Seed)
	if err != nil {
		return fmt.Errorf("derive EmailVM mail key: %w", err)
	}
	defer clear(mailPrivateKey)
	mailPublicKey, err := pqcrypto.EmailVMPublicKey(key.Seed)
	if err != nil {
		return fmt.Errorf("derive EmailVM public key: %w", err)
	}
	if len(record.EncryptionKey) > 0 && !bytes.Equal(record.EncryptionKey, mailPublicKey) {
		return errors.New("canonical mailbox publishes a different mail key; import its existing backup instead of replacing it")
	}

	mailPassword, passwordFromFile := readPasswordFromFile(ctx.Path(emailVMMailPasswordFlag.Name))
	if !passwordFromFile {
		mailPassword = utils.GetPassPhrase("Password for exported portable mail key", true)
	}
	if len(mailPassword) < 8 {
		return errors.New("portable mail-key password must be at least 8 characters")
	}
	portable, err := encryptPortableEmailKey(mailPrivateKey, mailPublicKey, mailPassword, record.Owner, mailbox, crand.Reader)
	if err != nil {
		return err
	}
	output := ctx.Path(emailVMOutputFlag.Name)
	if output == "" {
		output = "tkm-mail-key-" + strings.ReplaceAll(mailbox, "@", "-at-") + ".json"
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(portable, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(absOutput, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		return fmt.Errorf("write output file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}
	fmt.Printf("Mailbox:             %s\n", mailbox)
	fmt.Printf("Owner:               %s\n", record.Owner.Hex())
	fmt.Printf("Mail public key:     %s\n", hexutil.Encode(mailPublicKey))
	fmt.Printf("Encrypted keyfile:   %s\n", absOutput)
	if len(record.EncryptionKey) == 0 {
		fmt.Println("Network publication: not published yet; publish this public key before receiving mail")
	} else {
		fmt.Println("Network publication: canonical public key matches")
	}
	fmt.Println("The file contains no PQ seed, shielded viewing key, notes, or spending key.")
	return nil
}

func encryptPortableEmailKey(privateKey, publicKey []byte, password string, owner common.Address, mailbox string, random io.Reader) (portableEmailKeyfile, error) {
	if len(privateKey) != 32 || len(publicKey) != 32 {
		return portableEmailKeyfile{}, errors.New("EmailVM X25519 keys must be exactly 32 bytes")
	}
	if len(password) < 8 {
		return portableEmailKeyfile{}, errors.New("portable mail-key password must be at least 8 characters")
	}
	salt := make([]byte, 32)
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := io.ReadFull(random, salt); err != nil {
		return portableEmailKeyfile{}, fmt.Errorf("generate mail-key salt: %w", err)
	}
	if _, err := io.ReadFull(random, nonce); err != nil {
		return portableEmailKeyfile{}, fmt.Errorf("generate mail-key nonce: %w", err)
	}
	metadata := portableEmailMetadata{
		Type: portableEmailKeyType, Version: portableEmailKeyVersion, Algorithm: portableEmailAlgorithm,
		PublicKey: strings.ToLower(hexutil.Encode(publicKey)), Owner: owner.Hex(), Mailboxes: []string{mailbox},
	}
	aad, err := json.Marshal(metadata)
	if err != nil {
		return portableEmailKeyfile{}, err
	}
	derivedKey, err := scrypt.Key([]byte(password), salt, portableEmailScryptN, portableEmailScryptR, portableEmailScryptP, portableEmailKeyBytes)
	if err != nil {
		return portableEmailKeyfile{}, fmt.Errorf("derive portable mail-key encryption key: %w", err)
	}
	defer clear(derivedKey)
	aead, err := chacha20poly1305.NewX(derivedKey)
	if err != nil {
		return portableEmailKeyfile{}, err
	}
	ciphertext := aead.Seal(nil, nonce, privateKey, aad)
	return portableEmailKeyfile{
		portableEmailMetadata: metadata,
		Crypto: portableEmailCrypto{
			Cipher: "xchacha20-poly1305", Ciphertext: hexutil.Encode(ciphertext), CipherParams: portableEmailCipherParams{Nonce: hexutil.Encode(nonce)}, KDF: "scrypt",
			KDFParams: portableEmailKDFParams{N: portableEmailScryptN, R: portableEmailScryptR, P: portableEmailScryptP, DKLen: portableEmailKeyBytes, Salt: hexutil.Encode(salt)},
		},
	}, nil
}
