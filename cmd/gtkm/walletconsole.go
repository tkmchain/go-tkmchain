package main

import (
	"bufio"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
)

const walletInteractiveUsage = `
Interactive terminal wallet for local gtkm nodes.

The wallet connects to the node's local IPC endpoint, keeps private keys in the
keystore, and signs locally. It never sends a password or private key over RPC.
Use the Send flow for native TKM or other native account transfers. Shielded
transfers continue to use the dedicated shielded wallet/prover flow.
The TKM Phone panel can purchase numbers, register an ML-DSA-87 device, and
send encrypted messages. Phone purchases require a PQ account and explicit
confirmation; payment is confirmed before ownership is transferred.
`

func interactiveWallet(ctx *cli.Context) error {
	cfg := defaultNodeConfig()
	utils.SetDataDir(ctx, &cfg)
	endpoint := cfg.IPCEndpoint()
	rpcClient, err := utils.DialRPCWithHeaders(endpoint, nil)
	if err != nil {
		return fmt.Errorf("connect to gtkm IPC at %s: %w (start gtkm first)", endpoint, err)
	}
	defer rpcClient.Close()
	client := ethclient.NewClient(rpcClient)
	defer client.Close()

	am := makeAccountManager(ctx)
	backends := am.Backends(keystore.KeyStoreType)
	if len(backends) == 0 {
		return errors.New("keystore backend unavailable")
	}
	ks, ok := backends[0].(*keystore.KeyStore)
	if !ok {
		return errors.New("keystore backend has unexpected type")
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return fmt.Errorf("read chain ID: %w", err)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		clearWalletScreen()
		printWalletHeader(chainID, endpoint)
		accounts := ks.Accounts()
		if len(accounts) == 0 {
			fmt.Println("\n  No accounts found in the configured keystore.")
			fmt.Println("  Create one with: gtkm account new")
			return nil
		}
		fmt.Println("\n  1) Portfolio      View balances and account details")
		fmt.Println("  2) Send          Send native TKM or a native account transfer")
		fmt.Println("  3) Accounts      List local accounts")
		fmt.Println("  4) TKM Phone     Phone status, prices, and registered numbers")
		fmt.Println("  5) Email         EmailVM status and encrypted mailbox views")
		fmt.Println("  6) Kings         Rotating-king schedule and recent rotations")
		fmt.Println("  7) Refresh       Reload node and account data")
		fmt.Println("  0) Exit")
		choice, err := readWalletLine(reader, "\n  Select an option")
		if err != nil {
			return err
		}
		switch strings.TrimSpace(choice) {
		case "1":
			showWalletPortfolio(reader, client, ks, accounts)
		case "2":
			if err := sendFromWallet(reader, client, ks, accounts, chainID); err != nil {
				showWalletError(reader, err)
			}
		case "3":
			showWalletAccounts(reader, ks, accounts)
		case "4":
			showWalletPhone(reader, rpcClient, client, ks, accounts, chainID)
		case "5":
			showWalletEmail(reader, rpcClient)
		case "6":
			showWalletKings(reader, rpcClient)
		case "7":
			continue
		case "0", "q", "Q":
			fmt.Println("\n  Wallet closed.")
			return nil
		default:
			fmt.Println("\n  Choose 1, 2, 3, 4, 5, 6, 7, or 0.")
			pauseWallet(reader)
		}
	}
}

type walletEmailStatusView struct {
	Ready        bool           `json:"ready"`
	IndexedBlock hexutil.Uint64 `json:"indexedBlock"`
	HeadBlock    hexutil.Uint64 `json:"headBlock"`
	Domains      hexutil.Uint64 `json:"domains"`
	Mailboxes    hexutil.Uint64 `json:"mailboxes"`
	Messages     hexutil.Uint64 `json:"messages"`
	Pending      hexutil.Uint64 `json:"pendingPayments"`
	Protocol     string         `json:"protocol"`
	MessageStore string         `json:"messageStore"`
	SuperClaimed bool           `json:"superClaimed"`
	SuperAddress common.Address `json:"superAddress"`
}

type walletKingStatusView struct {
	Address       common.Address `json:"address"`
	Registered    bool           `json:"registered"`
	Current       bool           `json:"current"`
	Next          bool           `json:"next"`
	LockedAmount  *hexutil.Big   `json:"lockedAmount"`
	UnlockHeight  uint64         `json:"unlockHeight,omitempty"`
	TotalReceived *hexutil.Big   `json:"totalReceived"`
}

type walletKingStatsView struct {
	CurrentKing         common.Address         `json:"currentKing"`
	NextKing            common.Address         `json:"nextKing"`
	TotalKings          int                    `json:"totalKings"`
	RegisteredKings     int                    `json:"registeredKings"`
	RotationInterval    uint64                 `json:"rotationInterval"`
	CurrentBlock        uint64                 `json:"currentBlock"`
	NextRotationHeight  uint64                 `json:"nextRotationHeight"`
	BlocksUntilRotation uint64                 `json:"blocksUntilRotation"`
	Kings               []walletKingStatusView `json:"kings"`
}

type walletRotationHistoryView struct {
	BlockHeight  uint64         `json:"blockHeight"`
	PreviousKing common.Address `json:"previousKing"`
	NewKing      common.Address `json:"newKing"`
}

func walletRPCContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func printWalletRPCJSON(raw json.RawMessage) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		fmt.Println(string(raw))
		return
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Println(string(raw))
		return
	}
	fmt.Println(string(formatted))
}

func showWalletPhone(reader *bufio.Reader, rpcClient *rpc.Client, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) {
	clearWalletScreen()
	fmt.Println("TKM PHONE")
	fmt.Println("─────────")
	ctx, cancel := walletRPCContext()
	defer cancel()

	var status tkmPhoneStatusView
	if err := rpcClient.CallContext(ctx, &status, "tkmphone_status"); err != nil {
		fmt.Printf("  Phone service unavailable: %v\n", err)
	} else {
		state := "inactive"
		if status.Active {
			state = "active"
		}
		dataSource := "local clock"
		if status.UsingChainHead {
			dataSource = "chain head"
		}
		fmt.Printf("  Status:       %s\n", state)
		fmt.Printf("  Chain head:   #%d\n", uint64(status.HeadNumber))
		fmt.Printf("  Data source:  %s\n", dataSource)
	}

	var bucket, mainKing, sale hexutil.Big
	if err := rpcClient.CallContext(ctx, &bucket, "tkmphone_bucketPrice"); err == nil &&
		rpcClient.CallContext(ctx, &mainKing, "tkmphone_mainKingNumberPrice") == nil &&
		rpcClient.CallContext(ctx, &sale, "tkmphone_numberSalePrice") == nil {
		fmt.Println("\n  Prices:")
		fmt.Printf("    Bucket:       %s\n", formatTKM((*big.Int)(&bucket)))
		fmt.Printf("    Main King no: %s\n", formatTKM((*big.Int)(&mainKing)))
		fmt.Printf("    Operator no:  %s\n", formatTKM((*big.Int)(&sale)))
	} else {
		fmt.Println("\n  Prices: unavailable (enable the tkmphone RPC namespace)")
	}

	var numbers []json.RawMessage
	if err := rpcClient.CallContext(ctx, &numbers, "tkmphone_registeredNumbers"); err == nil {
		fmt.Printf("\n  Registered numbers: %d\n", len(numbers))
	} else {
		fmt.Printf("\n  Registered numbers: unavailable (%v)\n", err)
	}

	fmt.Println("\n  Actions")
	fmt.Println("    1) Buy a phone number       Pay the operator and complete ownership transfer")
	fmt.Println("    2) Register this device     Bind your PQ wallet/device to an owned number")
	fmt.Println("    3) Send encrypted message   Send a private number-to-number message")
	fmt.Println("    4) Inspect a number          View ownership and device metadata")
	fmt.Println("    0) Back")
	action, err := readWalletLine(reader, "Select a phone action")
	if err != nil {
		return
	}
	switch strings.TrimSpace(action) {
	case "1":
		if err := buyWalletPhoneNumber(reader, rpcClient, client, ks, walletAccounts, chainID); err != nil {
			showWalletError(reader, err)
		}
	case "2":
		if err := registerWalletPhoneDevice(reader, rpcClient, ks, walletAccounts); err != nil {
			showWalletError(reader, err)
		}
	case "3":
		if err := sendWalletPhoneMessage(reader, rpcClient, ks, walletAccounts); err != nil {
			showWalletError(reader, err)
		}
	case "4":
		inspectWalletPhoneNumber(reader, rpcClient)
	case "0", "":
		return
	default:
		fmt.Println("\n  Choose 1, 2, 3, 4, or 0.")
		pauseWallet(reader)
	}
}

type walletPhoneNumberView struct {
	Number    string         `json:"number"`
	Owner     common.Address `json:"owner"`
	Operator  common.Address `json:"operator"`
	SalePrice *hexutil.Big   `json:"salePrice"`
	Active    bool           `json:"active"`
	SoldAt    hexutil.Uint64 `json:"soldAt"`
	InUse     bool           `json:"inUse"`
}

type walletPhoneDeviceView struct {
	Device    string        `json:"device"`
	PublicKey hexutil.Bytes `json:"publicKey"`
	Active    bool          `json:"active"`
}

type walletRegisteredPhoneView struct {
	Number      walletPhoneNumberView   `json:"number"`
	Registered  bool                    `json:"registered"`
	DeviceCount hexutil.Uint64          `json:"deviceCount"`
	Devices     []walletPhoneDeviceView `json:"devices"`
}

type walletPhoneCipherView struct {
	Ciphertext hexutil.Bytes `json:"ciphertext"`
	Nonce      hexutil.Bytes `json:"nonce"`
}

var walletPhonePQPrefix = []byte("TKMPHONE_PQ_V1")

func chooseWalletAccount(reader *bufio.Reader, walletAccounts []accounts.Account) (accounts.Account, error) {
	for i, account := range walletAccounts {
		fmt.Printf("  %d) %s\n", i+1, account.Address.Hex())
	}
	choice, err := readWalletLine(reader, "Wallet account number")
	if err != nil {
		return accounts.Account{}, err
	}
	var index int
	if _, err := fmt.Sscanf(choice, "%d", &index); err != nil || index < 1 || index > len(walletAccounts) {
		return accounts.Account{}, errors.New("invalid account selection")
	}
	return walletAccounts[index-1], nil
}

func walletPhoneKey(ks *keystore.KeyStore, account accounts.Account, password string) (*keystore.PQKey, error) {
	algorithm, err := ks.AccountAlgorithm(account)
	if err != nil {
		return nil, err
	}
	if algorithm != pqcrypto.AlgorithmMLDSA87 {
		return nil, errors.New("TKM Phone actions require an ML-DSA-87 account; migrate a legacy account first")
	}
	if account.URL.Path == "" {
		return nil, errors.New("PQ account keyfile path is unavailable")
	}
	keyJSON, err := os.ReadFile(account.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("read PQ account key: %w", err)
	}
	key, err := keystore.DecryptPQKey(keyJSON, password)
	if err != nil {
		return nil, fmt.Errorf("unlock PQ account: %w", err)
	}
	if key.Address != account.Address {
		clear(key.Seed)
		return nil, errors.New("PQ key address does not match the selected account")
	}
	return key, nil
}

func walletPhonePublicKey(ks *keystore.KeyStore, account accounts.Account, password string) ([]byte, error) {
	key, err := walletPhoneKey(ks, account, password)
	if err != nil {
		return nil, err
	}
	defer clear(key.Seed)
	return common.CopyBytes(key.PublicKey), nil
}

func signWalletPhoneDigest(ks *keystore.KeyStore, account accounts.Account, password string, digest common.Hash) (hexutil.Bytes, error) {
	key, err := walletPhoneKey(ks, account, password)
	if err != nil {
		return nil, err
	}
	defer clear(key.Seed)
	mldsaKey, err := pqcrypto.NewMLDSA87FromSeed(key.Seed)
	if err != nil {
		return nil, err
	}
	message := append(common.CopyBytes(walletPhonePQPrefix), digest.Bytes()...)
	signature, err := pqcrypto.SignMLDSA87(mldsaKey, message)
	if err != nil {
		return nil, fmt.Errorf("sign phone action: %w", err)
	}
	envelope := append(common.CopyBytes(walletPhonePQPrefix), key.PublicKey...)
	envelope = append(envelope, signature...)
	return hexutil.Bytes(envelope), nil
}

func inspectWalletPhoneNumber(reader *bufio.Reader, client *rpc.Client) {
	fmt.Println("\n  Enter a phone number to inspect its ownership and device metadata.")
	number, err := readWalletLine(reader, "Phone number")
	if err != nil {
		return
	}
	if number == "" {
		return
	}
	ctx, cancel := walletRPCContext()
	defer cancel()
	var record json.RawMessage
	if err := client.CallContext(ctx, &record, "tkmphone_registeredNumber", number); err != nil {
		fmt.Printf("\n  Lookup failed: %v\n", err)
	} else {
		fmt.Println("\n  Registered number")
		printWalletRPCJSON(record)
	}
	pauseWallet(reader)
}

func registerWalletPhoneDevice(reader *bufio.Reader, client *rpc.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account) error {
	clearWalletScreen()
	fmt.Println("REGISTER PHONE DEVICE")
	fmt.Println("────────────────────")
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	number, err := readWalletLine(reader, "Owned phone number")
	if err != nil {
		return err
	}
	device, err := readWalletLine(reader, "Device name")
	if err != nil {
		return err
	}
	if number == "" || device == "" {
		return errors.New("phone number and device name are required")
	}
	password := utils.GetPassPhrase("PQ account password", false)
	defer clearWalletBytes([]byte(password))
	publicKey, err := walletPhonePublicKey(ks, account, password)
	if err != nil {
		return err
	}
	ctx, cancel := walletRPCContext()
	defer cancel()
	var record walletRegisteredPhoneView
	if err := client.CallContext(ctx, &record, "tkmphone_registeredNumber", number); err != nil {
		return fmt.Errorf("read phone ownership: %w", err)
	}
	if record.Number.Owner != account.Address {
		return fmt.Errorf("selected account %s does not own %s", account.Address.Hex(), number)
	}
	var digest common.Hash
	if err := client.CallContext(ctx, &digest, "tkmphone_deviceKeySigningHash", number, device, hexutil.Bytes(publicKey)); err != nil {
		return fmt.Errorf("create device registration hash: %w", err)
	}
	signature, err := signWalletPhoneDigest(ks, account, password, digest)
	if err != nil {
		return err
	}
	var result json.RawMessage
	if err := client.CallContext(ctx, &result, "tkmphone_registerDeviceKey", number, device, hexutil.Bytes(publicKey), signature); err != nil {
		return fmt.Errorf("register phone device: %w", err)
	}
	fmt.Println("\n  Device registered successfully.")
	printWalletRPCJSON(result)
	pauseWallet(reader)
	return nil
}

func sendWalletPhoneMessage(reader *bufio.Reader, client *rpc.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account) error {
	clearWalletScreen()
	fmt.Println("SEND ENCRYPTED PHONE MESSAGE")
	fmt.Println("───────────────────────────")
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	from, err := readWalletLine(reader, "Your phone number")
	if err != nil {
		return err
	}
	to, err := readWalletLine(reader, "Recipient phone number")
	if err != nil {
		return err
	}
	message, err := readWalletLine(reader, "Message")
	if err != nil {
		return err
	}
	if from == "" || to == "" || message == "" {
		return errors.New("sender number, recipient number, and message are required")
	}
	if len([]byte(message)) > 64*1024 {
		return errors.New("message exceeds the 64 KiB phone payload limit")
	}
	ctx, cancel := walletRPCContext()
	defer cancel()
	var owner walletRegisteredPhoneView
	if err := client.CallContext(ctx, &owner, "tkmphone_registeredNumber", from); err != nil {
		return fmt.Errorf("read sender phone: %w", err)
	}
	if owner.Number.Owner != account.Address {
		return fmt.Errorf("selected account %s does not own %s", account.Address.Hex(), from)
	}
	if !owner.Registered {
		return errors.New("sender phone has no active device; register this device first")
	}
	nonce := make([]byte, 16)
	if _, err := crand.Read(nonce); err != nil {
		return fmt.Errorf("generate message nonce: %w", err)
	}
	var cipher walletPhoneCipherView
	if err := client.CallContext(ctx, &cipher, "tkmphone_encryptPayload", from, to, hexutil.Bytes(nonce), hexutil.Bytes([]byte(message))); err != nil {
		return fmt.Errorf("encrypt phone message: %w", err)
	}
	fmt.Println("\n  Review encrypted message")
	fmt.Printf("    From: %s\n    To:   %s\n    Size: %d bytes\n", from, to, len([]byte(message)))
	confirm, err := readWalletLine(reader, "Type SEND to confirm")
	if err != nil {
		return err
	}
	if confirm != "SEND" {
		fmt.Println("  Cancelled. Nothing was signed or sent.")
		pauseWallet(reader)
		return nil
	}
	submitCtx, submitCancel := walletRPCContext()
	defer submitCancel()
	var digest common.Hash
	if err := client.CallContext(submitCtx, &digest, "tkmphone_sendMessageSigningHash", from, to, hexutil.Bytes(nonce), cipher.Ciphertext); err != nil {
		return fmt.Errorf("create message signing hash: %w", err)
	}
	password := utils.GetPassPhrase("PQ account password", false)
	defer clearWalletBytes([]byte(password))
	signature, err := signWalletPhoneDigest(ks, account, password, digest)
	if err != nil {
		return err
	}
	var result json.RawMessage
	if err := client.CallContext(submitCtx, &result, "tkmphone_sendEncryptedMessage", from, to, cipher.Ciphertext, hexutil.Bytes(nonce), signature); err != nil {
		return fmt.Errorf("send encrypted phone message: %w", err)
	}
	fmt.Println("\n  Encrypted message sent successfully.")
	printWalletRPCJSON(result)
	pauseWallet(reader)
	return nil
}

func buyWalletPhoneNumber(reader *bufio.Reader, rpcClient *rpc.Client, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) error {
	clearWalletScreen()
	fmt.Println("BUY PHONE NUMBER")
	fmt.Println("────────────────")
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	if algorithm, err := ks.AccountAlgorithm(account); err != nil || algorithm != pqcrypto.AlgorithmMLDSA87 {
		return errors.New("buying a phone number requires an ML-DSA-87 account so it can register a device")
	}
	number, err := readWalletLine(reader, "Phone number to buy")
	if err != nil {
		return err
	}
	if number == "" {
		return errors.New("phone number is required")
	}
	ctx, cancel := walletRPCContext()
	defer cancel()
	var record walletPhoneNumberView
	if err := rpcClient.CallContext(ctx, &record, "tkmphone_number", number); err != nil {
		return fmt.Errorf("look up phone number: %w", err)
	}
	if !record.Active || record.Operator == (common.Address{}) {
		return errors.New("phone number is not available for sale")
	}
	if record.Owner != record.Operator || record.SoldAt != 0 || record.InUse {
		return errors.New("phone number is already sold or in use")
	}
	price := new(big.Int).Mul(big.NewInt(10000), big.NewInt(params.Ether))
	if record.SalePrice != nil && (*big.Int)(record.SalePrice).Sign() != 0 && (*big.Int)(record.SalePrice).Cmp(price) != 0 {
		return errors.New("phone number price is not the canonical 10000 TKM sale price")
	}
	nonce, err := client.PendingNonceAt(ctx, account.Address)
	if err != nil {
		return fmt.Errorf("read pending nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("read network fee: %w", err)
	}
	gas := uint64(params.TxGas)
	if estimated, estimateErr := client.EstimateGas(ctx, ethereum.CallMsg{From: account.Address, To: &record.Operator, Value: price, GasPrice: gasPrice}); estimateErr == nil {
		gas = estimated
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), gasPrice)
	balance, err := client.BalanceAt(ctx, account.Address, nil)
	if err != nil {
		return fmt.Errorf("read balance: %w", err)
	}
	if balance.Cmp(new(big.Int).Add(price, fee)) < 0 {
		return fmt.Errorf("insufficient balance: need %s including fee, have %s", formatTKM(new(big.Int).Add(price, fee)), formatTKM(balance))
	}
	fmt.Println("\nReview phone purchase")
	fmt.Printf("  Buyer:    %s\n  Number:   %s\n  Operator: %s\n  Price:    %s\n  Fee:      %s (maximum estimate)\n", account.Address.Hex(), record.Number, record.Operator.Hex(), formatTKM(price), formatTKM(fee))
	confirm, err := readWalletLine(reader, "Type BUY to confirm")
	if err != nil {
		return err
	}
	if confirm != "BUY" {
		fmt.Println("  Cancelled. Nothing was signed.")
		pauseWallet(reader)
		return nil
	}
	password := utils.GetPassPhrase("Account password", false)
	unsigned := walletTransferTransaction(chainID, nonce, record.Operator, price, gas, gasPrice, pqcrypto.AlgorithmMLDSA87)
	tx, err := ks.SignTxWithPassphrase(account, password, unsigned, chainID)
	clearWalletBytes([]byte(password))
	if err != nil {
		return fmt.Errorf("sign phone purchase: %w", err)
	}
	txCtx, txCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer txCancel()
	if err := client.SendTransaction(txCtx, tx); err != nil {
		return fmt.Errorf("submit phone purchase payment: %w", err)
	}
	fmt.Printf("\n  Payment submitted: %s\n  Waiting for canonical confirmation...\n", tx.Hash().Hex())
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer waitCancel()
	receipt, err := waitWalletReceipt(waitCtx, client, tx.Hash())
	if err != nil {
		return fmt.Errorf("phone payment %s was submitted but confirmation failed: %w", tx.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("phone payment %s was mined but failed", tx.Hash().Hex())
	}
	var result json.RawMessage
	if err := rpcClient.CallContext(waitCtx, &result, "tkmphone_sellNumber", record.Operator, record.Number, account.Address, hexutil.Big(*price), tx.Hash()); err != nil {
		return fmt.Errorf("payment confirmed, but ownership transfer needs operator finalization: %w (payment %s)", err, tx.Hash().Hex())
	}
	fmt.Println("  Phone number ownership transferred successfully.")
	printWalletRPCJSON(result)
	pauseWallet(reader)
	return nil
}

func waitWalletReceipt(ctx context.Context, client *ethclient.Client, hash common.Hash) (*types.Receipt, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		receipt, err := client.TransactionReceipt(ctx, hash)
		if err == nil {
			return receipt, nil
		}
		if err != ethereum.NotFound {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func showWalletEmail(reader *bufio.Reader, client *rpc.Client) {
	clearWalletScreen()
	fmt.Println("EMAILVM")
	fmt.Println("───────")
	fmt.Println("Email data remains encrypted; this view never asks for a mail private key.")
	ctx, cancel := walletRPCContext()
	defer cancel()

	var status walletEmailStatusView
	if err := client.CallContext(ctx, &status, "emailvm_status"); err != nil {
		fmt.Printf("\n  Email service unavailable: %v\n", err)
	} else {
		state := "not ready"
		if status.Ready {
			state = "ready"
		}
		fmt.Printf("\n  Status:       %s\n", state)
		fmt.Printf("  Indexed:      #%d\n", uint64(status.IndexedBlock))
		fmt.Printf("  Domains:      %d    Mailboxes: %d\n", uint64(status.Domains), uint64(status.Mailboxes))
		fmt.Printf("  Messages:     %d    Pending payments: %d\n", uint64(status.Messages), uint64(status.Pending))
		if status.Protocol != "" {
			fmt.Printf("  Protocol:     %s\n", status.Protocol)
		}
	}

	fmt.Println("\n  Enter a mailbox to inspect its public record and encrypted inbox/outbox.")
	mailbox, err := readWalletLine(reader, "Mailbox (user@domain)")
	if err != nil {
		return
	}
	if mailbox != "" {
		for _, item := range []struct {
			label  string
			method string
		}{
			{"Mailbox record", "tkmdomain_mailbox"},
			{"Encrypted inbox", "emailvm_inbox"},
			{"Encrypted outbox", "emailvm_outbox"},
		} {
			var record json.RawMessage
			if err := client.CallContext(ctx, &record, item.method, mailbox); err != nil {
				fmt.Printf("\n  %s unavailable: %v\n", item.label, err)
				continue
			}
			fmt.Printf("\n  %s\n", item.label)
			printWalletRPCJSON(record)
		}
	}
	pauseWallet(reader)
}

func showWalletKings(reader *bufio.Reader, client *rpc.Client) {
	clearWalletScreen()
	fmt.Println("ROTATING KINGS")
	fmt.Println("──────────────")
	ctx, cancel := walletRPCContext()
	defer cancel()

	var stats walletKingStatsView
	if err := client.CallContext(ctx, &stats, "rk_getKingStats", nil); err != nil {
		fmt.Printf("  Rotating-king service unavailable: %v\n", err)
		pauseWallet(reader)
		return
	}
	fmt.Printf("  Current king:       %s\n", stats.CurrentKing.Hex())
	fmt.Printf("  Next king:          %s\n", stats.NextKing.Hex())
	fmt.Printf("  Rotation interval:  %d blocks\n", stats.RotationInterval)
	fmt.Printf("  Current block:      #%d\n", stats.CurrentBlock)
	fmt.Printf("  Next rotation:      #%d (%d blocks)\n", stats.NextRotationHeight, stats.BlocksUntilRotation)
	fmt.Printf("  Registered kings:   %d/%d\n", stats.RegisteredKings, stats.TotalKings)

	var kings []walletKingStatusView
	if err := client.CallContext(ctx, &kings, "rk_list"); err == nil && len(kings) > 0 {
		fmt.Println("\n  Registered schedule:")
		for _, king := range kings {
			slot := "standby"
			if king.Current {
				slot = "current"
			} else if king.Next {
				slot = "next"
			}
			line := fmt.Sprintf("    %-8s %s", slot, king.Address.Hex())
			if king.LockedAmount != nil {
				line += "  stake " + formatTKM((*big.Int)(king.LockedAmount))
			}
			fmt.Println(line)
		}
	}

	var history []walletRotationHistoryView
	if err := client.CallContext(ctx, &history, "rotatingking_getRotationHistory", hexutil.Uint64(5)); err == nil && len(history) > 0 {
		fmt.Println("\n  Recent rotations:")
		for _, entry := range history {
			fmt.Printf("    #%d  %s → %s\n", entry.BlockHeight, entry.PreviousKing.Hex(), entry.NewKing.Hex())
		}
	}
	pauseWallet(reader)
}

func printWalletHeader(chainID *big.Int, endpoint string) {
	fmt.Println("╭────────────────────────────────────────────────────────────╮")
	fmt.Println("│                         TKM WALLET                          │")
	fmt.Println("│              Secure local signing console                  │")
	fmt.Println("╰────────────────────────────────────────────────────────────╯")
	fmt.Printf("  Network: chain %s    IPC: %s\n", chainID.String(), endpoint)
	fmt.Println("  Keys stay local. Review every address, amount, and fee before signing.")
}

func clearWalletScreen() {
	fmt.Print("\033[H\033[2J")
}

func readWalletLine(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}

func showWalletAccounts(reader *bufio.Reader, ks *keystore.KeyStore, accounts []accounts.Account) {
	clearWalletScreen()
	fmt.Println("LOCAL ACCOUNTS")
	fmt.Println("──────────────")
	for i, account := range accounts {
		algorithm, err := ks.AccountAlgorithm(account)
		if err != nil {
			algorithm = "unknown"
		}
		fmt.Printf("  %d  %s  %s\n", i+1, account.Address.Hex(), algorithm)
	}
	pauseWallet(reader)
}

func showWalletPortfolio(reader *bufio.Reader, client *ethclient.Client, ks *keystore.KeyStore, accounts []accounts.Account) {
	clearWalletScreen()
	fmt.Println("PORTFOLIO")
	fmt.Println("─────────")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i, account := range accounts {
		balance, err := client.BalanceAt(ctx, account.Address, nil)
		if err != nil {
			fmt.Printf("  %d  %s  balance unavailable: %v\n", i+1, account.Address.Hex(), err)
			continue
		}
		algorithm, _ := ks.AccountAlgorithm(account)
		fmt.Printf("  %d  %s\n      %s %s\n", i+1, account.Address.Hex(), formatTKM(balance), algorithm)
	}
	pauseWallet(reader)
}

func sendFromWallet(reader *bufio.Reader, client *ethclient.Client, ks *keystore.KeyStore, accounts []accounts.Account, chainID *big.Int) error {
	clearWalletScreen()
	fmt.Println("SEND FUNDS")
	fmt.Println("──────────")
	fmt.Println("This flow signs locally and submits one native transfer.")
	fmt.Println("For shielded TKM, use the shielded wallet/prover flow instead.")
	fmt.Println()
	for i, account := range accounts {
		fmt.Printf("  %d) %s\n", i+1, account.Address.Hex())
	}
	fromText, err := readWalletLine(reader, "From account number")
	if err != nil {
		return err
	}
	var index int
	if _, err := fmt.Sscanf(fromText, "%d", &index); err != nil || index < 1 || index > len(accounts) {
		return errors.New("invalid account selection")
	}
	from := accounts[index-1]
	toText, err := readWalletLine(reader, "Recipient 0x address")
	if err != nil {
		return err
	}
	if !common.IsHexAddress(toText) {
		return errors.New("recipient must be a valid hexadecimal address")
	}
	to := common.HexToAddress(toText)
	amountText, err := readWalletLine(reader, "Amount in TKM")
	if err != nil {
		return err
	}
	value, err := parseWalletAmount(amountText, 18)
	if err != nil || value.Sign() <= 0 {
		return errors.New("amount must be a positive decimal value")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	nonce, err := client.PendingNonceAt(ctx, from.Address)
	if err != nil {
		return fmt.Errorf("read pending nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("read network fee: %w", err)
	}
	gas := uint64(params.TxGas)
	if estimated, estimateErr := client.EstimateGas(ctx, ethereum.CallMsg{From: from.Address, To: &to, Value: value, GasPrice: gasPrice}); estimateErr == nil {
		gas = estimated
	}
	algorithm, err := ks.AccountAlgorithm(from)
	if err != nil {
		return fmt.Errorf("read account algorithm: %w", err)
	}
	tx := walletTransferTransaction(chainID, nonce, to, value, gas, gasPrice, algorithm)
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), gasPrice)
	balance, err := client.BalanceAt(ctx, from.Address, nil)
	if err != nil {
		return fmt.Errorf("read balance: %w", err)
	}
	if balance.Cmp(new(big.Int).Add(value, fee)) < 0 {
		return fmt.Errorf("insufficient balance: need %s including fee, have %s", formatTKM(new(big.Int).Add(value, fee)), formatTKM(balance))
	}

	fmt.Println("\nReview transfer")
	fmt.Printf("  From:  %s\n  To:    %s\n  Amount: %s\n  Fee:    %s (max estimate)\n  Type:   %s\n", from.Address.Hex(), to.Hex(), formatTKM(value), formatTKM(fee), algorithm)
	confirm, err := readWalletLine(reader, "Type SEND to confirm")
	if err != nil {
		return err
	}
	if confirm != "SEND" {
		fmt.Println("  Cancelled. Nothing was signed.")
		pauseWallet(reader)
		return nil
	}
	password := utils.GetPassPhrase("Account password", false)
	defer clearWalletBytes([]byte(password))
	signed, err := ks.SignTxWithPassphrase(from, password, tx, chainID)
	if err != nil {
		return fmt.Errorf("sign transaction: %w", err)
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return fmt.Errorf("submit transaction: %w", err)
	}
	fmt.Printf("\n  Submitted successfully.\n  Transaction hash: %s\n", signed.Hash().Hex())
	pauseWallet(reader)
	return nil
}

func walletTransferTransaction(chainID *big.Int, nonce uint64, to common.Address, value *big.Int, gas uint64, gasPrice *big.Int, algorithm string) *types.Transaction {
	if algorithm == pqcrypto.AlgorithmMLDSA87 {
		return types.NewTx(&types.PQTkmTx{ChainID: chainID, Nonce: nonce, GasTipCap: new(big.Int).Set(gasPrice), GasFeeCap: new(big.Int).Set(gasPrice), Gas: gas, To: &to, Value: new(big.Int).Set(value)})
	}
	return types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: nonce, GasTipCap: new(big.Int).Set(gasPrice), GasFeeCap: new(big.Int).Set(gasPrice), Gas: gas, To: &to, Value: new(big.Int).Set(value)})
}

func parseWalletAmount(input string, decimals int) (*big.Int, error) {
	input = strings.TrimSpace(input)
	if input == "" || strings.HasPrefix(input, "-") || strings.Count(input, ".") > 1 {
		return nil, errors.New("invalid amount")
	}
	parts := strings.SplitN(input, ".", 2)
	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > decimals {
		return nil, fmt.Errorf("amount has more than %d decimal places", decimals)
	}
	fraction += strings.Repeat("0", decimals-len(fraction))
	combined := strings.TrimLeft(whole+fraction, "0")
	if combined == "" {
		combined = "0"
	}
	value := new(big.Int)
	if _, ok := value.SetString(combined, 10); !ok {
		return nil, errors.New("invalid amount")
	}
	return value, nil
}

func formatTKM(value *big.Int) string {
	if value == nil {
		return "0 TKM"
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole := new(big.Int).Quo(new(big.Int).Set(value), base)
	fraction := new(big.Int).Mod(new(big.Int).Set(value), base)
	if fraction.Sign() == 0 {
		return whole.String() + " TKM"
	}
	frac := fmt.Sprintf("%018s", fraction.String())
	frac = strings.TrimRight(frac, "0")
	return whole.String() + "." + frac + " TKM"
}

func pauseWallet(reader *bufio.Reader) {
	_, _ = readWalletLine(reader, "Press Enter to continue")
}

func showWalletError(reader *bufio.Reader, err error) {
	fmt.Printf("\n  Error: %v\n", err)
	pauseWallet(reader)
}

func clearWalletBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
