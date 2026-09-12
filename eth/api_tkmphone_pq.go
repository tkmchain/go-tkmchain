package eth

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

// Phone signatures carry the public key because ML-DSA signatures cannot recover it.
// The signed domain prevents a phone signature from authorizing a chain transaction.
var phonePQPrefix = []byte("TKMPHONE_PQ_V1")

const phonePQSignatureSize = 4627

func phonePQMessage(digest common.Hash) []byte {
	return append(append([]byte(nil), phonePQPrefix...), digest.Bytes()...)
}

func recoverPhonePQSigner(digest common.Hash, signature []byte) (common.Address, error) {
	size := len(phonePQPrefix) + pqcrypto.MLDSA87PublicKeySize + phonePQSignatureSize
	if len(signature) != size || !bytes.HasPrefix(signature, phonePQPrefix) {
		return common.Address{}, errors.New("invalid ML-DSA-87 phone signature envelope")
	}
	publicKey := signature[len(phonePQPrefix) : len(phonePQPrefix)+pqcrypto.MLDSA87PublicKeySize]
	sig := signature[len(phonePQPrefix)+pqcrypto.MLDSA87PublicKeySize:]
	if !pqcrypto.VerifyMLDSA87(publicKey, phonePQMessage(digest), sig) {
		return common.Address{}, errors.New("invalid ML-DSA-87 phone signature")
	}
	return pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
}

func (api *TkmPhoneAPI) SignatureAlgorithms() []string {
	return []string{pqcrypto.AlgorithmMLDSA87, "ECDSA-secp256k1"}
}
