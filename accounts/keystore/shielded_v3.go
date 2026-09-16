package keystore

import (
	"errors"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func (ks *KeyStore) ImportStampedPQSeed(seed []byte, passphrase string, chainID uint64, name, country string) (accounts.Account, error) {
	stamp, err := pqcrypto.CreateShieldedV3Stamp(seed, chainID, name, country)
	if err != nil {
		return accounts.Account{}, err
	}
	key, err := NewPQKeyFromSeed(seed)
	if err != nil {
		return accounts.Account{}, err
	}
	defer zeroPQKey(key)
	key.Shield3Stamp = stamp
	ks.importMu.Lock()
	defer ks.importMu.Unlock()
	if ks.cache.hasAddress(key.Address) {
		return accounts.Account{}, ErrAccountAlreadyExists
	}
	return ks.importPQKey(key, passphrase)
}
func (ks *KeyStore) StampPQAccount(a accounts.Account, passphrase string, chainID uint64, name, country string) error {
	ks.importMu.Lock()
	defer ks.importMu.Unlock()
	a, key, err := ks.getDecryptedPQKey(a, passphrase)
	if err != nil {
		return err
	}
	defer zeroPQKey(key)
	if key.Shield3Stamp != nil {
		return errors.New("account already has a stamp; its original stamp is preserved")
	}
	key.Shield3Stamp, err = pqcrypto.CreateShieldedV3Stamp(key.Seed, chainID, name, country)
	if err != nil {
		return err
	}
	n, p := ks.scryptParams()
	data, err := EncryptPQKey(key, passphrase, n, p)
	if err != nil {
		return err
	}
	return writeKeyFile(a.URL.Path, data)
}

// ImportStampedPQBackup preserves an authenticated original stamp, and rejects
// unstamped or cross-chain backups before adding an account to the keystore.
func (ks *KeyStore) ImportStampedPQBackup(blob []byte, passphrase, newPassphrase string, chainID uint64) (accounts.Account, error) {
	key, err := DecryptPQKey(blob, passphrase)
	if key != nil {
		defer zeroPQKey(key)
	}
	if err != nil {
		return accounts.Account{}, err
	}
	if key.Shield3Stamp == nil || key.Shield3Stamp.ChainID != chainID {
		return accounts.Account{}, errors.New("backup requires its original Shield3 stamp for this chain; restore an older wallet with its recovery words and add a stamp")
	}
	ks.importMu.Lock()
	defer ks.importMu.Unlock()
	if ks.cache.hasAddress(key.Address) {
		return accounts.Account{Address: key.Address}, ErrAccountAlreadyExists
	}
	return ks.importPQKey(key, newPassphrase)
}
