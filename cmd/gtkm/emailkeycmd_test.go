package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
)

func TestPortableEmailKeyfileMatchesWebFormat(t *testing.T) {
	privateKey := bytes.Repeat([]byte{0x11}, 32)
	publicKey := bytes.Repeat([]byte{0x22}, 32)
	owner := common.HexToAddress("0x1111111111111111111111111111111111111111")
	random := bytes.NewReader(bytes.Repeat([]byte{0x33}, 32+chacha20poly1305.NonceSizeX))
	file, err := encryptPortableEmailKey(privateKey, publicKey, "mail-password", owner, "info@tkm", random)
	if err != nil {
		t.Fatal(err)
	}
	if file.Type != portableEmailKeyType || file.Version != portableEmailKeyVersion || file.Algorithm != portableEmailAlgorithm {
		t.Fatalf("unexpected portable metadata: %+v", file.portableEmailMetadata)
	}
	if file.PublicKey != hexutil.Encode(publicKey) || file.Owner != owner.Hex() || len(file.Mailboxes) != 1 || file.Mailboxes[0] != "info@tkm" {
		t.Fatalf("unexpected portable identity: %+v", file.portableEmailMetadata)
	}
	if file.Crypto.Cipher != "xchacha20-poly1305" || file.Crypto.KDF != "scrypt" {
		t.Fatalf("unexpected portable crypto: %+v", file.Crypto)
	}
	salt, err := hexutil.Decode(file.Crypto.KDFParams.Salt)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := hexutil.Decode(file.Crypto.CipherParams.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := hexutil.Decode(file.Crypto.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	derivedKey, err := scrypt.Key([]byte("mail-password"), salt, file.Crypto.KDFParams.N, file.Crypto.KDFParams.R, file.Crypto.KDFParams.P, file.Crypto.KDFParams.DKLen)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(derivedKey)
	aead, err := chacha20poly1305.NewX(derivedKey)
	if err != nil {
		t.Fatal(err)
	}
	aad, err := json.Marshal(file.portableEmailMetadata)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plaintext)
	if !bytes.Equal(plaintext, privateKey) {
		t.Fatal("portable keyfile did not decrypt to the EmailVM private key")
	}
}
