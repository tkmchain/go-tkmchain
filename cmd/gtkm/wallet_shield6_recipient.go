package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

var walletShield6ProverCache struct {
	sync.Mutex
	directory string
	pins      recipient.ArtifactPins
	prover    *recipient.PreparedProver
}

func walletShield6RecipientClient(ctx context.Context, rpc shield3wallet.RPC, codes []recipient.PaymentCode) (*shield3wallet.ShieldedV6RecipientClient, error) {
	var status struct {
		RecipientProofsRequired bool        `json:"recipientProofsRequired"`
		RecipientKeyHash        common.Hash `json:"recipientKeyHash"`
		ProvingKeyHash          common.Hash `json:"recipientProvingKeyHash"`
	}
	if err := rpc.CallContext(ctx, &status, "tkmprivacy_shieldedV6Status"); err != nil {
		return nil, err
	}
	if !status.RecipientProofsRequired {
		if len(codes) > 0 {
			return nil, errors.New("recipient-stamped payment codes require the recipient-proof fork")
		}
		return nil, nil
	}
	directory := os.Getenv("TKM_SHIELD6_RECIPIENT_PARAMETERS")
	if directory == "" {
		return nil, errors.New("set TKM_SHIELD6_RECIPIENT_PARAMETERS to the locally installed, authenticated recipient parameter directory")
	}
	file, err := os.Open(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	var pins recipient.ArtifactPins
	if err := decoder.Decode(&pins); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("invalid recipient artifact manifest trailing data")
	}
	if status.ProvingKeyHash == (common.Hash{}) || common.Hash(pins.ProvingKey) != status.ProvingKeyHash || common.Hash(pins.VerifyingKey) != status.RecipientKeyHash {
		return nil, errors.New("chain recipient key differs from authenticated local release manifest")
	}
	cache := &walletShield6ProverCache
	cache.Lock()
	defer cache.Unlock()
	if cache.prover == nil || cache.directory != directory || cache.pins != pins {
		p, err := recipient.OpenPreparedProver(filepath.Join(directory, "recipient.r1cs"), filepath.Join(directory, "recipient.pk"), filepath.Join(directory, "recipient.vk"), pins)
		if err != nil {
			return nil, err
		}
		cache.directory, cache.pins, cache.prover = directory, pins, p
	}
	return &shield3wallet.ShieldedV6RecipientClient{Prover: cache.prover, Codes: codes}, nil
}

func registerWalletShield6Recipient(reader *bufio.Reader, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) error {
	if client == nil || chainID == nil || !chainID.IsUint64() || chainID.Sign() <= 0 {
		return errors.New("connected Shield6 chain required")
	}
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	confirmation, err := readWalletLine(reader, "Type REGISTER to authorize this account's immutable Shield6 recipient key commitment and pay registration gas")
	if err != nil {
		return err
	}
	if confirmation != "REGISTER" {
		return nil
	}
	password := utils.GetPassPhrase("PQ account password", false)
	defer clearWalletBytes([]byte(password))
	key, err := walletPQKey(ks, account, password)
	if err != nil {
		return err
	}
	defer clearWalletBytes(key.Seed)
	if key.Shield3Stamp == nil {
		return errors.New("stamp the account before registering Shield6 recipient keys")
	}
	identity, err := shield3wallet.NewIdentity(key.Seed, chainID.Uint64(), key.Shield3Stamp)
	if err != nil {
		return err
	}
	defer identity.Clear()
	keys, err := shielded6.DeriveWalletKeys(key.Seed, chainID.Uint64())
	if err != nil {
		return err
	}
	defer keys.Clear()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	nonce, err := client.PendingNonceAt(ctx, account.Address)
	if err != nil {
		return err
	}
	price, err := client.SuggestGasPrice(ctx)
	if err != nil || price == nil || price.Sign() <= 0 {
		return errors.New("network gas price unavailable")
	}
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(key.Seed)
	if err != nil {
		return err
	}
	skeleton := types.NewTx(&types.PQTkmTx{ChainID: chainID, Nonce: nonce, Gas: 5_000_000, GasTipCap: price, GasFeeCap: new(big.Int).Mul(price, big.NewInt(2)), To: &params.ShieldedPoolAddress, Value: new(big.Int), Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(accountKey)})
	registrationProver, err := walletShield6RegistrationProver(ctx, client.Client())
	if err != nil {
		return err
	}
	signed, err := shield3wallet.BuildAndSignShieldedV6Binding(ctx, client.Client(), key.Seed, identity, keys, skeleton, registrationProver)
	if err != nil {
		return err
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return err
	}
	var hash common.Hash
	if err := client.Client().CallContext(ctx, &hash, "eth_sendRawTransaction", hexutil.Encode(raw)); err != nil {
		return err
	}
	if hash != signed.Hash() {
		return errors.New("registration transaction hash mismatch")
	}
	fmt.Printf("Shield6 recipient registration submitted: %s\n", hash)
	receipt, err := waitWalletReceipt(ctx, client, hash)
	if err != nil {
		return err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return errors.New("Shield6 recipient registration failed")
	}
	code, err := shield3wallet.ShieldedV6PaymentCode(ctx, client.Client(), identity, keys)
	if err != nil {
		return fmt.Errorf("registration included; refresh payment code: %w", err)
	}
	encoded, err := recipient.EncodePaymentCode(code)
	if err != nil {
		return err
	}
	fmt.Printf("Shield6 recipient payment code:\n%s\n", encoded)
	pauseWallet(reader)
	return nil
}

func walletShield6RegistrationProver(ctx context.Context, rpc shield3wallet.RPC) (*recipient.PreparedProver, error) {
	var status struct {
		Pin        common.Hash `json:"registrationKeyHash"`
		ProvingPin common.Hash `json:"registrationProvingKeyHash"`
	}
	if err := rpc.CallContext(ctx, &status, "tkmprivacy_shieldedV6Status"); err != nil {
		return nil, err
	}
	directory := os.Getenv("TKM_SHIELD6_RECIPIENT_PARAMETERS")
	if directory == "" {
		return nil, errors.New("install authenticated Shield6 registration parameters first")
	}
	file, err := os.Open(filepath.Join(directory, "registration-manifest.json"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var pins recipient.ArtifactPins
	dec := json.NewDecoder(io.LimitReader(file, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pins); err != nil {
		return nil, err
	}
	if dec.Decode(new(any)) != io.EOF || (status.ProvingPin == (common.Hash{}) || common.Hash(pins.ProvingKey) != status.ProvingPin || common.Hash(pins.VerifyingKey) != status.Pin) {
		return nil, errors.New("registration parameters do not match chain configuration")
	}
	return recipient.OpenRegistrationProver(filepath.Join(directory, "registration.r1cs"), filepath.Join(directory, "registration.pk"), filepath.Join(directory, "registration.vk"), pins)
}
