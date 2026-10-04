package main

import (
	"bufio"
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/tyler-smith/go-bip39"
	"github.com/urfave/cli/v2"
)

const walletInteractiveUsage = `
Interactive terminal wallet for local gtkm nodes.

The wallet connects to the node's local IPC endpoint, keeps private keys in the
keystore, and signs locally. It never sends a password or private key over RPC.
Use the Send flow for native TKM or other native account transfers. Shielded
transfers continue to use the dedicated shielded wallet/prover flow.
Use Stamp address before a Shield3 send. The flow encrypts the name and
country locally, persists the exact stamp in the PQ keyfile, submits the
owner-proof registration, and waits for immutable on-chain confirmation.
Use Shield3 username to register a checksummed @name for a stamped receiving
address. The wallet signs the binding locally and shows whether the node has
replicated it to its configured directory operators.
Use Stamp sponsorship for an unfunded recipient in another wallet. Exchange
an offer code and recipient authorization code, then let the funded sponsor
review and pay the registration gas. Private keys stay in their own wallets.
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
	languagePreference, language, languageOverride, languageErr := walletLanguageFromFlag(ctx)
	if languageErr != nil {
		return languageErr
	}
	if !languageOverride {
		languagePreference, language, languageErr = configureWalletLanguage(reader, cfg.DataDir)
		if languageErr != nil {
			return fmt.Errorf("choose wallet language: %w", languageErr)
		}
	}
	walletActiveLanguage = language
	for {
		clearWalletScreen()
		printWalletHeader(chainID, endpoint)
		accounts := ks.Accounts()
		if len(accounts) == 0 {
			fmt.Println("\n  No accounts found in the configured keystore.")
			fmt.Println("  Create one with: gtkm account new")
			return nil
		}
		fmt.Printf("\n  1) %s\n", walletText("menu.portfolio", "Portfolio"))
		fmt.Printf("  2) %s\n", walletText("menu.send", "Send"))
		fmt.Printf("  3) %s\n", walletText("menu.accounts", "Accounts"))
		fmt.Printf("  4) %s\n", walletText("menu.phone", "TKM Phone"))
		fmt.Printf("  5) %s\n", walletText("menu.email", "Email"))
		fmt.Printf("  6) %s\n", walletText("menu.kings", "Kings"))
		fmt.Printf("  7) %s\n", walletText("menu.stamp", "Stamp address"))
		fmt.Printf("  8) %s\n", walletText("menu.refresh", "Refresh"))
		fmt.Printf("  9) %s\n", walletText("menu.language", "Language"))
		fmt.Printf("  10) %s\n", walletText("menu.migrate", "Migrate ECDSA → ML-DSA-87"))
		fmt.Printf("  11) %s\n", walletText("menu.shield3", "Show Shield3 address"))
		fmt.Printf("  12) %s\n", walletText("menu.stampSponsor", "Stamp sponsorship"))
		fmt.Printf("  13) %s\n", walletText("menu.validator", "Validator"))
		fmt.Printf("  14) %s\n", walletText("menu.username", "Shield3 username"))
		fmt.Printf("  0) %s\n", walletText("menu.exit", "Exit"))
		choice, err := readWalletLine(reader, "\n  "+walletText("select", "Select an option"))
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
			showWalletKings(reader, rpcClient, accounts)
		case "7":
			if err := stampWalletAddress(reader, rpcClient, client, ks, accounts, chainID); err != nil {
				showWalletError(reader, err)
			}
		case "8":
			continue
		case "9":
			preference, selected, selectErr := chooseWalletLanguage(reader)
			if selectErr != nil {
				showWalletError(reader, selectErr)
				continue
			}
			languagePreference = preference
			walletActiveLanguage = selected
			if saveErr := saveWalletLanguage(cfg.DataDir, languagePreference); saveErr != nil {
				showWalletError(reader, saveErr)
				continue
			}
			fmt.Printf("\n  %s: %s — %s\n", walletText("language.saved", "Language saved"), selected.Name, selected.NativeName)
			pauseWallet(reader)
		case "10":
			if err := migrateECDSAWallet(reader, client, ks, accounts, chainID); err != nil {
				showWalletError(reader, err)
			}
		case "11":
			if err := showWalletShield3Address(reader, ks, accounts, chainID); err != nil {
				showWalletError(reader, err)
			}
		case "12":
			if err := walletStampSponsorship(reader, rpcClient, client, ks, accounts, chainID); err != nil {
				showWalletError(reader, err)
			}
		case "13":
			if err := walletValidatorMenu(reader, client, ks, accounts, chainID); err != nil {
				showWalletError(reader, err)
			}
		case "14":
			if err := walletShield3Username(reader, rpcClient, ks, accounts, chainID); err != nil {
				showWalletError(reader, err)
			}
		case "0", "q", "Q":
			fmt.Printf("\n  %s\n", walletText("closed", "Wallet closed."))
			return nil
		default:
			fmt.Println("\n  Choose 1 through 14, or 0.")
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
	Address         common.Address `json:"address"`
	Hash            common.Hash    `json:"hash"`
	Registered      bool           `json:"registered"`
	Current         bool           `json:"current"`
	Next            bool           `json:"next"`
	LockedAmount    *hexutil.Big   `json:"lockedAmount"`
	RegistrationFee *hexutil.Big   `json:"registrationFee"`
	UnlockTime      *time.Time     `json:"unlockTime,omitempty"`
	UnlockHeight    uint64         `json:"unlockHeight,omitempty"`
	AddedHeight     uint64         `json:"addedHeight,omitempty"`
	TotalReceived   *hexutil.Big   `json:"totalReceived"`
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

func showWalletPhoneIdentities(ctx context.Context, client *rpc.Client, walletAccounts []accounts.Account) {
	var mailboxes []walletMailboxView
	mailboxErr := client.CallContext(ctx, &mailboxes, "tkmdomain_mailboxes", "")
	fmt.Println("\n  Your phone and email identities")
	for _, account := range walletAccounts {
		fmt.Printf("    %s\n", account.Address.Hex())
		var numbers []walletPhoneNumberView
		if err := client.CallContext(ctx, &numbers, "tkmphone_numbersForOwner", account.Address); err != nil {
			fmt.Printf("      Phone lookup unavailable: %v\n", err)
		} else if len(numbers) == 0 {
			fmt.Println("      Phone: none")
		} else {
			for _, number := range numbers {
				state := "device not registered"
				if number.InUse {
					state = "device registered"
				}
				fmt.Printf("      Phone: %s (%s)\n", number.Number, state)
			}
		}
		if mailboxErr != nil {
			fmt.Printf("      Email lookup unavailable: %v\n", mailboxErr)
			continue
		}
		found := false
		for _, mailbox := range mailboxes {
			if mailbox.Owner != account.Address {
				continue
			}
			if !found {
				found = true
			}
			fmt.Printf("      Email: %s\n", mailbox.Address)
		}
		if !found {
			fmt.Println("      Email: none")
		}
	}
}

func showWalletPhoneInventory(ctx context.Context, client *rpc.Client) {
	var available []walletPhoneNumberView
	if err := client.CallContext(ctx, &available, "tkmphone_availableNumbers", hexutil.Uint64(100)); err != nil {
		fmt.Printf("\n  Available phone inventory unavailable: %v\n", err)
		return
	}
	if len(available) == 0 {
		fmt.Println("\n  Available phone inventory: no numbers currently offered for sale")
		return
	}
	var buckets []walletPhoneBucketView
	bucketErr := client.CallContext(ctx, &buckets, "tkmphone_buckets")
	counts := make(map[uint64]int)
	for _, number := range available {
		counts[uint64(number.BucketID)]++
	}
	fmt.Printf("\n  Available phone inventory (%d shown, 10000 TKM each)\n", len(available))
	if bucketErr == nil {
		for _, bucket := range buckets {
			if count := counts[uint64(bucket.ID)]; count > 0 {
				operator := bucket.Operator.Hex()
				if bucket.Operator == (common.Address{}) {
					operator = "unassigned"
				}
				fmt.Printf("    Bucket #%d: %d number(s), operator %s\n", uint64(bucket.ID), count, operator)
			}
		}
	}
	for index, number := range available {
		operator := number.Operator.Hex()
		if number.Operator == (common.Address{}) {
			operator = "unassigned"
		}
		fmt.Printf("      %d) %s  bucket #%d  operator %s\n", index+1, number.Number, uint64(number.BucketID), operator)
	}
}

func showWalletPhone(reader *bufio.Reader, rpcClient *rpc.Client, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) {
	clearWalletScreen()
	fmt.Println(walletText("section.phone", "TKM PHONE"))
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

	identityCtx, identityCancel := walletRPCContext()
	showWalletPhoneIdentities(identityCtx, rpcClient, walletAccounts)
	identityCancel()
	inventoryCtx, inventoryCancel := walletRPCContext()
	showWalletPhoneInventory(inventoryCtx, rpcClient)
	inventoryCancel()

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
	BucketID  hexutil.Uint64 `json:"bucketId"`
	InUse     bool           `json:"inUse"`
}

type walletPhoneBucketView struct {
	ID         hexutil.Uint64 `json:"id"`
	Hash       common.Hash    `json:"hash"`
	Operator   common.Address `json:"operator"`
	AssignedAt hexutil.Uint64 `json:"assignedAt"`
}

type walletMailboxView struct {
	Address       string         `json:"address"`
	Owner         common.Address `json:"owner"`
	Domain        string         `json:"domain"`
	EncryptionKey hexutil.Bytes  `json:"encryptionKey,omitempty"`
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

func walletPQKey(ks *keystore.KeyStore, account accounts.Account, password string) (*keystore.PQKey, error) {
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
	key, err := walletPQKey(ks, account, password)
	if err != nil {
		return nil, err
	}
	defer clear(key.Seed)
	return common.CopyBytes(key.PublicKey), nil
}

func signWalletPhoneDigest(ks *keystore.KeyStore, account accounts.Account, password string, digest common.Hash) (hexutil.Bytes, error) {
	key, err := walletPQKey(ks, account, password)
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
	ctx, cancel := walletRPCContext()
	defer cancel()
	var available []walletPhoneNumberView
	if err := rpcClient.CallContext(ctx, &available, "tkmphone_availableNumbers", hexutil.Uint64(100)); err == nil && len(available) > 0 {
		fmt.Println("\n  Available numbers (enter the list number or the exact number):")
		for index, candidate := range available {
			fmt.Printf("    %d) %s  bucket #%d\n", index+1, candidate.Number, uint64(candidate.BucketID))
		}
	}
	selection, err := readWalletLine(reader, "Phone number to buy")
	if err != nil {
		return err
	}
	number := strings.TrimSpace(selection)
	if index, parseErr := strconv.Atoi(number); parseErr == nil && index >= 1 && index <= len(available) {
		number = available[index-1].Number
	}
	if number == "" {
		return errors.New("phone number is required")
	}
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
	fmt.Println(walletText("section.email", "EMAILVM"))
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

func showWalletKings(reader *bufio.Reader, client *rpc.Client, walletAccounts []accounts.Account) {
	for {
		clearWalletScreen()
		fmt.Println(walletText("section.kings", "ROTATING KINGS"))
		fmt.Println("──────────────")
		ctx, cancel := walletRPCContext()
		var stats walletKingStatsView
		statsErr := client.CallContext(ctx, &stats, "rk_getKingStats", nil)
		cancel()
		if statsErr != nil {
			fmt.Printf("  Rotating-king service unavailable: %v\n", statsErr)
			pauseWallet(reader)
			return
		}
		fmt.Printf("  Current king:       %s\n", stats.CurrentKing.Hex())
		fmt.Printf("  Next king:          %s\n", stats.NextKing.Hex())
		fmt.Printf("  Rotation interval:  %d blocks\n", stats.RotationInterval)
		fmt.Printf("  Current block:      #%d\n", stats.CurrentBlock)
		fmt.Printf("  Next rotation:      #%d (%d blocks)\n", stats.NextRotationHeight, stats.BlocksUntilRotation)
		fmt.Printf("  Registered kings:   %d/%d\n", stats.RegisteredKings, stats.TotalKings)

		ctx, cancel = walletRPCContext()
		var kings []walletKingStatusView
		listErr := client.CallContext(ctx, &kings, "rk_list")
		cancel()
		if listErr == nil && len(kings) > 0 {
			fmt.Println("\n  Registered schedule:")
			for _, king := range kings {
				printWalletKingStatus(king, "    ")
			}
		}

		ctx, cancel = walletRPCContext()
		var history []walletRotationHistoryView
		if err := client.CallContext(ctx, &history, "rotatingking_getRotationHistory", hexutil.Uint64(5)); err == nil && len(history) > 0 {
			fmt.Println("\n  Recent rotations:")
			for _, entry := range history {
				fmt.Printf("    #%d  %s → %s\n", entry.BlockHeight, entry.PreviousKing.Hex(), entry.NewKing.Hex())
			}
		}
		cancel()

		fmt.Println("\n  r) Register a local account as rotating king")
		fmt.Println("  s) Query rk_status for an address")
		fmt.Println("  Enter) Back")
		action, err := readWalletLine(reader, "Action")
		if err != nil {
			return
		}
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "r", "register":
			if err := registerWalletKing(reader, client, walletAccounts); err != nil {
				showWalletError(reader, err)
			}
		case "s", "status":
			if err := queryWalletKingStatus(reader, client, walletAccounts); err != nil {
				showWalletError(reader, err)
			}
		default:
			return
		}
	}
}

// stampWalletAddress creates the encrypted name/country stamp in the local
// PQ keyfile, builds the canonical owner-proof registration, and waits until
// the immutable registry entry is visible on-chain. The stamp is persisted
// only after the proof has been built successfully, but before broadcast, so
// an interrupted submission can be retried without creating a different
// identity.
func stampWalletAddress(reader *bufio.Reader, rpcClient *rpc.Client, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) error {
	if chainID == nil || !chainID.IsUint64() || chainID.Sign() == 0 {
		return errors.New("invalid chain ID for Shield3 stamping")
	}
	if len(walletAccounts) == 0 {
		return errors.New("no local accounts available")
	}
	clearWalletScreen()
	fmt.Println(walletText("section.stamp", "SHIELD3 ADDRESS STAMP"))
	fmt.Println("────────────────────────")
	fmt.Println("A stamp encrypts your name and country into this address.")
	fmt.Println("The labels never enter the transaction or public registry.")
	fmt.Println("The registration is immutable and required before Shield3 payments.")
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	algorithm, err := ks.AccountAlgorithm(account)
	if err != nil {
		return fmt.Errorf("read account algorithm: %w", err)
	}
	if algorithm != pqcrypto.AlgorithmMLDSA87 {
		return errors.New("Shield3 stamping requires an ML-DSA-87 account; migrate this ECDSA account first")
	}
	var onChain core.AntarticalStampStatus
	statusCtx, statusCancel := walletRPCContext()
	statusErr := rpcClient.CallContext(statusCtx, &onChain, "tkmprivacy_antarticalStamp", account.Address)
	statusCancel()
	if statusErr == nil && onChain.Registered {
		fmt.Printf("\n  This address already has an immutable confirmed stamp.\n  Registration transaction: %s\n", onChain.TransactionHash.Hex())
		pauseWallet(reader)
		return nil
	}

	password := utils.GetPassPhrase("PQ account password", false)
	defer clearWalletBytes([]byte(password))
	key, err := walletPQKey(ks, account, password)
	if err != nil {
		return err
	}
	defer clearWalletBytes(key.Seed)

	stamp := key.Shield3Stamp
	newStamp := stamp == nil
	var stampName, stampCountry string
	if newStamp {
		stampName, err = readWalletLine(reader, "Private name")
		if err != nil {
			return err
		}
		stampCountry, err = readWalletLine(reader, "Private country")
		if err != nil {
			return err
		}
		stamp, err = pqcrypto.CreateShieldedV3Stamp(key.Seed, chainID.Uint64(), stampName, stampCountry)
		if err != nil {
			return fmt.Errorf("create encrypted stamp: %w", err)
		}
		fmt.Println("\n  The encrypted stamp is ready locally. The name and country will not be printed or sent in plaintext.")
	} else {
		fmt.Println("\n  This account already has a local immutable stamp.")
		fmt.Println("  The wallet will submit or resume its on-chain registration.")
	}

	confirm, err := readWalletLine(reader, "Type STAMP to build and submit the registration")
	if err != nil {
		return err
	}
	if strings.ToUpper(strings.TrimSpace(confirm)) != "STAMP" {
		fmt.Println("  Cancelled. No proof was built and no transaction was submitted.")
		pauseWallet(reader)
		return nil
	}

	identity, err := shield3wallet.NewIdentity(key.Seed, chainID.Uint64(), stamp)
	if err != nil {
		return fmt.Errorf("derive Shield3 identity: %w", err)
	}
	defer identity.Clear()

	fmt.Println("\n  Building the Shield3 stamp proof locally. This can take a minute on slower CPUs...")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	tx, err := shield3wallet.BuildAndSignStamp(buildCtx, rpcClient, key.Seed, identity)
	buildCancel()
	if err != nil {
		fmt.Println("  For an unfunded address, choose Stamp sponsorship from the main menu.")
		return fmt.Errorf("build Shield3 stamp registration: %w", err)
	}

	if newStamp {
		if err := ks.StampPQAccountRecord(account, password, stamp); err != nil {
			return fmt.Errorf("persist Shield3 stamp: %w", err)
		}
	}

	submitCtx, submitCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = client.SendTransaction(submitCtx, tx)
	submitCancel()
	if err != nil {
		return fmt.Errorf("submit Shield3 stamp registration: %w (the local stamp is preserved for retry)", err)
	}
	fmt.Printf("\n  Stamp registration submitted: %s\n  Waiting for canonical confirmation...\n", tx.Hash().Hex())

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer waitCancel()
	receipt, err := waitWalletReceipt(waitCtx, client, tx.Hash())
	if err != nil {
		return fmt.Errorf("stamp %s was submitted but confirmation failed: %w", tx.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("stamp %s was mined but the registration reverted", tx.Hash().Hex())
	}
	var status core.AntarticalStampStatus
	if err := rpcClient.CallContext(waitCtx, &status, "tkmprivacy_antarticalStamp", account.Address); err != nil {
		return fmt.Errorf("stamp %s was mined but registry lookup failed: %w", tx.Hash().Hex(), err)
	}
	if !status.Registered || status.Owner != identity.Owner || status.Commitment != stamp.Commitment {
		return fmt.Errorf("stamp %s was mined but the immutable registry does not match this keyfile", tx.Hash().Hex())
	}
	fmt.Println("  Shield3 address stamp confirmed on-chain.")
	fmt.Printf("  Registration transaction: %s\n", tx.Hash().Hex())
	pauseWallet(reader)
	return nil
}

func printWalletKingStatus(king walletKingStatusView, indent string) {
	slot := "standby"
	if king.Current {
		slot = "current"
	} else if king.Next {
		slot = "next"
	}
	line := fmt.Sprintf("%s%-8s %s", indent, slot, king.Address.Hex())
	if king.LockedAmount != nil {
		line += "  stake " + formatTKM((*big.Int)(king.LockedAmount))
	}
	if king.UnlockHeight != 0 {
		line += fmt.Sprintf("  unlock #%d", king.UnlockHeight)
	}
	fmt.Println(line)
}

func registerWalletKing(reader *bufio.Reader, client *rpc.Client, walletAccounts []accounts.Account) error {
	if len(walletAccounts) == 0 {
		return errors.New("no local accounts available")
	}
	clearWalletScreen()
	fmt.Println(walletText("section.kings", "ROTATING KINGS"))
	fmt.Println("\n  Registration checks the active stake requirement and fee reserve.")
	fmt.Println("  The address must remain funded or it will be removed from the schedule.")
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	fmt.Printf("\n  Register: %s\n", account.Address.Hex())
	confirm, err := readWalletLine(reader, "Type REGISTER to confirm")
	if err != nil {
		return err
	}
	if strings.ToUpper(confirm) != "REGISTER" {
		fmt.Println("  Cancelled. No registration was submitted.")
		pauseWallet(reader)
		return nil
	}
	ctx, cancel := walletRPCContext()
	defer cancel()
	var status walletKingStatusView
	if err := client.CallContext(ctx, &status, "rk_add", account.Address); err != nil {
		return fmt.Errorf("rk_add: %w", err)
	}
	fmt.Println("\n  Rotating-king registration accepted.")
	printWalletKingStatus(status, "  ")
	if status.Hash != (common.Hash{}) {
		fmt.Printf("  Registration hash: %s\n", status.Hash.Hex())
	}
	pauseWallet(reader)
	return nil
}

func queryWalletKingStatus(reader *bufio.Reader, client *rpc.Client, walletAccounts []accounts.Account) error {
	clearWalletScreen()
	fmt.Println(walletText("section.kings", "ROTATING KINGS"))
	fmt.Println("\n  Local accounts:")
	for i, account := range walletAccounts {
		fmt.Printf("    %d) %s\n", i+1, account.Address.Hex())
	}
	value, err := readWalletLine(reader, "Address or local account number")
	if err != nil {
		return err
	}
	var address common.Address
	var index int
	if _, scanErr := fmt.Sscanf(value, "%d", &index); scanErr == nil && index >= 1 && index <= len(walletAccounts) {
		address = walletAccounts[index-1].Address
	} else if common.IsHexAddress(value) {
		address = common.HexToAddress(value)
	} else {
		return errors.New("enter a local account number or hexadecimal address")
	}
	ctx, cancel := walletRPCContext()
	defer cancel()
	var status walletKingStatusView
	if err := client.CallContext(ctx, &status, "rk_status", address); err != nil {
		return fmt.Errorf("rk_status: %w", err)
	}
	fmt.Printf("\n  rk_status %s\n", address.Hex())
	printWalletKingStatus(status, "  ")
	fmt.Printf("  Registered: %t    Current: %t    Next: %t\n", status.Registered, status.Current, status.Next)
	if status.RegistrationFee != nil {
		fmt.Printf("  Fee reserve: %s\n", formatTKM((*big.Int)(status.RegistrationFee)))
	}
	if status.AddedHeight != 0 {
		fmt.Printf("  Added at block: #%d\n", status.AddedHeight)
	}
	if status.Hash != (common.Hash{}) {
		fmt.Printf("  Registration hash: %s\n", status.Hash.Hex())
	}
	pauseWallet(reader)
	return nil
}

func walletValidatorMenu(reader *bufio.Reader, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) error {
	for {
		clearWalletScreen()
		fmt.Println("VALIDATOR")
		fmt.Println("─────────")
		fmt.Printf("  Bond: %s TKM    Registration fee: %s TKM\n", formatTKM(walletAmountTKM(core.ValidatorBondTKM)), formatTKM(walletAmountTKM(core.ValidatorRegistrationFeeTKM)))
		fmt.Printf("  Activation delay: %d blocks    Unbonding: %d blocks\n", core.ValidatorActivationDelay, core.ValidatorUnbondingPeriod)
		fmt.Println("  Registration requires an ML-DSA-87 account and is available after Antartical activation.")
		fmt.Println("\n  r) Register validator")
		fmt.Println("  e) Request validator exit")
		fmt.Println("  w) Withdraw an unlocked validator bond")
		fmt.Println("  Enter) Back")
		choice, err := readWalletLine(reader, "Action")
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "r", "register":
			err = submitWalletValidatorAction(reader, client, ks, walletAccounts, chainID, "register")
		case "e", "exit":
			err = submitWalletValidatorAction(reader, client, ks, walletAccounts, chainID, "exit")
		case "w", "withdraw":
			err = submitWalletValidatorAction(reader, client, ks, walletAccounts, chainID, "withdraw")
		default:
			return nil
		}
		if err != nil {
			showWalletError(reader, err)
		}
	}
}

func walletAmountTKM(amount uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(amount), big.NewInt(params.Ether))
}

func submitWalletValidatorAction(reader *bufio.Reader, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int, action string) error {
	if len(walletAccounts) == 0 {
		return errors.New("no local accounts available")
	}
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	algorithm, err := ks.AccountAlgorithm(account)
	if err != nil {
		return err
	}
	if algorithm != pqcrypto.AlgorithmMLDSA87 {
		return errors.New("validator actions require an ML-DSA-87 account; create or migrate a PQ account first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	block, err := client.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("read current block: %w", err)
	}
	keyPassword := utils.GetPassPhrase("ML-DSA-87 account password", false)
	defer clearWalletBytes([]byte(keyPassword))
	key, err := walletPQKey(ks, account, keyPassword)
	if err != nil {
		return err
	}
	defer clear(key.Seed)

	var data []byte
	value := new(big.Int)
	destination := params.ShieldedPoolAddress
	switch action {
	case "register":
		if block > ^uint64(0)-core.ValidatorActivationDelay {
			return errors.New("current block is too large to schedule validator activation")
		}
		data, err = core.EncodeValidatorRegistration(&core.ValidatorRegistration{
			Version: core.ValidatorEnvelopeVersion, PublicKey: common.CopyBytes(key.PublicKey),
			RewardAddress: account.Address, ActivationHeight: block + core.ValidatorActivationDelay,
		})
		value = core.ValidatorBondWei()
	case "exit":
		data, err = core.EncodeValidatorExit(&core.ValidatorAction{Version: core.ValidatorEnvelopeVersion})
	case "withdraw":
		data, err = core.EncodeValidatorWithdrawal(&core.ValidatorAction{Version: core.ValidatorEnvelopeVersion})
	default:
		return errors.New("unknown validator action")
	}
	if err != nil {
		return fmt.Errorf("encode validator action: %w", err)
	}
	nonce, err := client.PendingNonceAt(ctx, account.Address)
	if err != nil {
		return fmt.Errorf("read pending nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("read network fee: %w", err)
	}
	gas := uint64(500000)
	unsigned := types.NewTx(&types.PQTkmTx{ChainID: chainID, Nonce: nonce, GasTipCap: new(big.Int).Set(gasPrice), GasFeeCap: new(big.Int).Set(gasPrice), Gas: gas, To: &destination, Value: value, Data: data})
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), gasPrice)
	needed := new(big.Int).Add(new(big.Int).Set(value), fee)
	if action == "register" {
		needed.Add(needed, core.ValidatorRegistrationFeeWei())
	}
	balance, err := client.BalanceAt(ctx, account.Address, nil)
	if err != nil {
		return fmt.Errorf("read account balance: %w", err)
	}
	if balance.Cmp(needed) < 0 {
		return fmt.Errorf("insufficient balance: need %s TKM including bond, registration fee and maximum transaction fee; have %s TKM", formatTKM(needed), formatTKM(balance))
	}
	fmt.Printf("\n  Action: %s\n  Account: %s\n  Current block: #%d\n", action, account.Address.Hex(), block)
	if action == "register" {
		fmt.Printf("  Bond: %s TKM\n  Registration fee: %s TKM (burned)\n  Scheduled activation: #%d\n", formatTKM(value), formatTKM(core.ValidatorRegistrationFeeWei()), block+core.ValidatorActivationDelay)
	}
	fmt.Printf("  Maximum network fee: %s TKM\n", formatTKM(fee))
	confirm, err := readWalletLine(reader, "Type "+strings.ToUpper(action)+" to sign and submit")
	if err != nil {
		return err
	}
	if confirm != strings.ToUpper(action) {
		fmt.Println("  Cancelled. No transaction was signed.")
		pauseWallet(reader)
		return nil
	}
	signed, err := ks.SignTxWithPassphrase(account, keyPassword, unsigned, chainID)
	if err != nil {
		return fmt.Errorf("sign validator transaction: %w", err)
	}
	submitCtx, submitCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer submitCancel()
	if err := client.SendTransaction(submitCtx, signed); err != nil {
		return fmt.Errorf("submit validator transaction: %w", err)
	}
	fmt.Printf("\n  Validator transaction submitted: %s\n  Waiting for confirmation...\n", signed.Hash().Hex())
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer waitCancel()
	receipt, err := waitWalletReceipt(waitCtx, client, signed.Hash())
	if err != nil {
		return fmt.Errorf("transaction %s was submitted but confirmation failed: %w", signed.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("validator transaction %s was mined but failed; inspect node logs for the consensus rejection reason", signed.Hash().Hex())
	}
	fmt.Printf("  Confirmed in block #%d. Transaction: %s\n", receipt.BlockNumber.Uint64(), signed.Hash().Hex())
	pauseWallet(reader)
	return nil
}

func printWalletHeader(chainID *big.Int, endpoint string) {
	fmt.Println("╭────────────────────────────────────────────────────────────╮")
	fmt.Printf("│ %-58s │\n", walletText("title", "TKM WALLET"))
	fmt.Printf("│ %-58s │\n", walletText("subtitle", "Secure local signing console"))
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
	fmt.Println(walletText("section.accounts", "LOCAL ACCOUNTS"))
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

// showWalletShield3Address derives the authenticated, shareable Shield3
// receiving code from a stamped ML-DSA account. The seed is unlocked only in
// memory and is never printed or sent over RPC.
func showWalletShield3Address(reader *bufio.Reader, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) error {
	if chainID == nil || !chainID.IsUint64() || chainID.Sign() <= 0 {
		return errors.New("invalid chain ID for Shield3 address")
	}
	clearWalletScreen()
	fmt.Println(walletText("section.shield3", "SHIELD3 RECEIVING ADDRESS"))
	fmt.Println("──────────────────────────")
	fmt.Println("This code is public and safe to share. It contains no private key or plaintext stamp.")
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	algorithm, err := ks.AccountAlgorithm(account)
	if err != nil {
		return fmt.Errorf("read account algorithm: %w", err)
	}
	if algorithm != pqcrypto.AlgorithmMLDSA87 {
		return errors.New("Shield3 requires an ML-DSA-87 account; migrate this ECDSA account first")
	}
	password := utils.GetPassPhrase("PQ account password", false)
	defer clearWalletBytes([]byte(password))
	key, err := walletPQKey(ks, account, password)
	if err != nil {
		return err
	}
	defer clearWalletBytes(key.Seed)
	identity, err := shield3wallet.NewIdentity(key.Seed, chainID.Uint64(), key.Shield3Stamp)
	if err != nil {
		return fmt.Errorf("derive Shield3 address: %w", err)
	}
	defer identity.Clear()
	fmt.Printf("\n  Account address: %s\n", identity.Address.Hex())
	fmt.Printf("  Shield3 address: %s\n", identity.Code)
	pauseWallet(reader)
	return nil
}

func walletShield3Username(reader *bufio.Reader, client *rpc.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) error {
	if chainID == nil || !chainID.IsUint64() || chainID.Sign() <= 0 {
		return errors.New("invalid chain ID for Shield3 username")
	}
	clearWalletScreen()
	fmt.Println("SHIELD3 USERNAME")
	fmt.Println("────────────────")
	fmt.Println("Register a short, checksummed name for your Shield3 receiving code.")
	var before eth.TkmNameDirectoryStatus
	if err := client.Call(&before, "tkmname_status"); err != nil {
		return fmt.Errorf("check username network status: %w", err)
	}
	if before.NetworkActive {
		if before.NetworkPeersConfigured {
			fmt.Printf("Network: active · %d directory operators configured · ready=%t\n", before.DirectoryOperators, before.NetworkReady)
		} else {
			fmt.Println("Network: active · directory peers are not configured; registration cannot complete until they are.")
			return errors.New("configure at least two pinned username directory operators and two independent transit relays in the node's TKMNet settings")
		}
	} else {
		fmt.Println("Network: not active yet · registration is saved locally and will replicate after activation.")
	}
	account, err := chooseWalletAccount(reader, walletAccounts)
	if err != nil {
		return err
	}
	name, err := readWalletLine(reader, "Username (3–32 ASCII letters, digits, _ or -)")
	if err != nil {
		return err
	}
	name, err = shield3wallet.NormalizeUsername(name)
	if err != nil {
		return err
	}
	handle, err := shield3wallet.UsernameHandle(name, chainID.Uint64())
	if err != nil {
		return err
	}
	fmt.Printf("\n  You are registering %s on chain %s.\n", handle, chainID.String())
	confirmation, err := readWalletLine(reader, "Type REGISTER to confirm")
	if err != nil {
		return err
	}
	if confirmation != "REGISTER" {
		return errors.New("username registration cancelled")
	}
	password := utils.GetPassPhrase("PQ account password", false)
	key, err := walletPQKey(ks, account, password)
	clearWalletBytes([]byte(password))
	if err != nil {
		return err
	}
	defer clearWalletBytes(key.Seed)
	if key.Shield3Stamp == nil {
		return errors.New("stamp this ML-DSA-87 wallet before registering a Shield3 username")
	}
	identity, err := shield3wallet.NewIdentity(key.Seed, chainID.Uint64(), key.Shield3Stamp)
	if err != nil {
		return fmt.Errorf("derive Shield3 payment code: %w", err)
	}
	defer identity.Clear()
	payload, err := shield3wallet.DecodePaymentCode(identity.Code, chainID.Uint64())
	if err != nil {
		return err
	}
	if err := shield3wallet.RequireRegisteredStamp(context.Background(), client, payload); err != nil {
		return err
	}
	sequence := uint64(1)
	if handle, handleErr := shield3wallet.UsernameHandle(name, chainID.Uint64()); handleErr == nil {
		var prior shield3wallet.UsernameBinding
		if client.Call(&prior, "tkmname_resolve", handle) == nil {
			if prior.Address != identity.Address {
				return errors.New("username belongs to another Shield3 identity and cannot be claimed")
			}
			sequence = prior.Sequence + 1
		}
	}
	expires := time.Now().UTC().Add(90 * 24 * time.Hour)
	fmt.Println("\n  Creating owner-signed name record and proof of work…")
	record, err := shield3wallet.CreateUsernameBinding(key.Seed, name, identity.Code, chainID.Uint64(), sequence, expires, time.Now())
	if err != nil {
		return fmt.Errorf("create username record: %w", err)
	}
	var accepted bool
	if err := client.Call(&accepted, "tkmname_register", record); err != nil {
		return fmt.Errorf("register username: %w", err)
	}
	if !accepted {
		return errors.New("node did not confirm username registration")
	}
	fmt.Printf("\n  Registered: %s\n", handle)
	fmt.Println("  Shield3 code remains private to your local wallet until you share it.")
	var after eth.TkmNameDirectoryStatus
	if err := client.Call(&after, "tkmname_status"); err != nil {
		fmt.Println("  The node accepted the record, but its replication status could not be checked.")
	} else if after.NetworkActive && after.NetworkReady {
		fmt.Printf("  Replicated to all %d configured directory operators over TKMNet.\n", after.DirectoryOperators)
	} else if after.NetworkActive {
		fmt.Println("  Network activation is live, but directory replication is not confirmed. Check node peer configuration before sharing the name.")
	} else {
		fmt.Println("  Saved in this node's directory; network replication begins at username-network activation.")
	}
	pauseWallet(reader)
	return nil
}

func showWalletPortfolio(reader *bufio.Reader, client *ethclient.Client, ks *keystore.KeyStore, accounts []accounts.Account) {
	clearWalletScreen()
	fmt.Println(walletText("section.portfolio", "PORTFOLIO"))
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
	fmt.Println(walletText("section.send", "SEND FUNDS"))
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

func migrateECDSAWallet(reader *bufio.Reader, client *ethclient.Client, ks *keystore.KeyStore, walletAccounts []accounts.Account, chainID *big.Int) error {
	clearWalletScreen()
	fmt.Println(walletText("section.migrate", "MIGRATE ECDSA → ML-DSA-87"))
	fmt.Println("────────────────────────────")
	fmt.Println("This creates a new ML-DSA-87 account and sends the source balance minus the network fee to it.")
	fmt.Println("The legacy private key is never printed. The new PQ recovery phrase is shown only after the migration transaction is mined successfully.")

	var legacyAccounts []accounts.Account
	for _, account := range walletAccounts {
		algorithm, err := ks.AccountAlgorithm(account)
		if err == nil && algorithm == keystore.AlgorithmECDSA {
			legacyAccounts = append(legacyAccounts, account)
		}
	}
	if len(legacyAccounts) == 0 {
		return errors.New("no ECDSA-secp256k1 accounts are available to migrate")
	}
	for i, account := range legacyAccounts {
		fmt.Printf("  %d) %s\n", i+1, account.Address.Hex())
	}
	choice, err := readWalletLine(reader, "Legacy ECDSA account number")
	if err != nil {
		return err
	}
	var index int
	if _, err := fmt.Sscanf(choice, "%d", &index); err != nil || index < 1 || index > len(legacyAccounts) {
		return errors.New("invalid ECDSA account selection")
	}
	legacy := legacyAccounts[index-1]

	legacyPassphrase := utils.GetPassPhrase("Legacy ECDSA account password", false)
	defer clearWalletBytes([]byte(legacyPassphrase))
	pqPassphrase := utils.GetPassPhrase("New ML-DSA-87 account password", false)
	defer clearWalletBytes([]byte(pqPassphrase))

	migration, err := ks.PreparePQMigration(legacy, legacyPassphrase, pqPassphrase)
	if err != nil {
		return fmt.Errorf("prepare PQ migration: %w", err)
	}
	defer clearWalletBytes(migration.PQSeed)
	if len(migration.PQSeed) != pqcrypto.MLDSA87SeedSize {
		return errors.New("generated PQ migration seed has an invalid length")
	}
	// BIP39 encodes the exact 32-byte ML-DSA seed as a standard 24-word
	// recovery phrase. Compute it before submission, but do not display it
	// until the migration receipt is canonical. The raw seed is never logged.
	pqRecoveryPhrase, err := walletPQSeedMnemonic(migration.PQSeed)
	if err != nil {
		return fmt.Errorf("encode generated PQ seed as a recovery phrase: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	nonce, err := client.PendingNonceAt(ctx, legacy.Address)
	if err != nil {
		return fmt.Errorf("read legacy account nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("read network fee: %w", err)
	}
	balance, err := client.BalanceAt(ctx, legacy.Address, nil)
	if err != nil {
		return fmt.Errorf("read legacy account balance: %w", err)
	}
	// Estimate the ordinary value-transfer cost with the migration marker. The
	// marker is calldata only; the value is set after the fee is known so the
	// source account can be migrated without leaving a spendable ECDSA balance.
	call := ethereum.CallMsg{
		From:     legacy.Address,
		To:       &migration.PQAccount.Address,
		GasPrice: gasPrice,
		Data:     migration.MigrationData,
	}
	gas := uint64(params.TxGas) + uint64(len(migration.MigrationData))*16
	if estimated, estimateErr := client.EstimateGas(ctx, call); estimateErr == nil && estimated > gas {
		gas = estimated
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), gasPrice)
	if balance.Cmp(fee) <= 0 {
		return fmt.Errorf("legacy account balance %s does not cover migration fee %s", formatTKM(balance), formatTKM(fee))
	}
	value := new(big.Int).Sub(new(big.Int).Set(balance), fee)
	unsigned := walletMigrationTransaction(chainID, nonce, migration.PQAccount.Address, value, gas, gasPrice, migration.MigrationData)

	fmt.Println("\nReview migration")
	fmt.Printf("  From (ECDSA): %s\n  To (ML-DSA-87): %s\n  Amount: %s\n  Fee limit: %s\n  Transaction type: ECDSA migration transfer\n", legacy.Address.Hex(), migration.PQAccount.Address.Hex(), formatTKM(value), formatTKM(fee))
	confirm, err := readWalletLine(reader, "Type MIGRATE to sign and submit")
	if err != nil {
		return err
	}
	if confirm != "MIGRATE" {
		fmt.Println("  Cancelled. The new PQ account remains encrypted in the keystore; no migration transaction was signed.")
		pauseWallet(reader)
		return nil
	}

	signed, err := ks.SignTxWithPassphrase(legacy, legacyPassphrase, unsigned, chainID)
	if err != nil {
		return fmt.Errorf("sign migration transaction: %w", err)
	}
	submitCtx, submitCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer submitCancel()
	if err := client.SendTransaction(submitCtx, signed); err != nil {
		return fmt.Errorf("submit migration transaction: %w", err)
	}
	fmt.Printf("\n  Migration submitted: %s\n  Waiting for canonical confirmation...\n", signed.Hash().Hex())
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer waitCancel()
	receipt, err := waitWalletReceipt(waitCtx, client, signed.Hash())
	if err != nil {
		return fmt.Errorf("migration %s was submitted but confirmation failed: %w", signed.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("migration %s was mined but failed; the new PQ account remains in the keystore", signed.Hash().Hex())
	}

	fmt.Println("\n  Migration complete.")
	fmt.Printf("  New ML-DSA-87 address: %s\n", migration.PQAccount.Address.Hex())
	fmt.Println("  New ML-DSA-87 recovery phrase (24 words; save exactly):")
	fmt.Printf("  %s\n", pqRecoveryPhrase)
	fmt.Printf("  Migration transaction: %s\n", signed.Hash().Hex())
	fmt.Println("  The phrase is displayed once. Keep it and the new account password in separate secure backups.")
	pauseWallet(reader)
	return nil
}

// walletPQSeedMnemonic is the user-facing representation of an ML-DSA-87
// compact seed. ML-DSA-87 seeds are 256 bits, so BIP39 produces exactly 24
// English words. Check the round trip here to prevent displaying a phrase that
// does not recover the seed held by the keystore.
func walletPQSeedMnemonic(seed []byte) (string, error) {
	if len(seed) != pqcrypto.MLDSA87SeedSize {
		return "", fmt.Errorf("invalid ML-DSA-87 seed length %d", len(seed))
	}
	phrase, err := bip39.NewMnemonic(seed)
	if err != nil {
		return "", err
	}
	if words := strings.Fields(phrase); len(words) != 24 {
		return "", fmt.Errorf("BIP39 returned %d words, want 24", len(words))
	}
	recovered, err := bip39.EntropyFromMnemonic(phrase)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(recovered, seed) {
		return "", errors.New("BIP39 recovery phrase does not reproduce the ML-DSA-87 seed")
	}
	clear(recovered)
	return phrase, nil
}

func walletMigrationTransaction(chainID *big.Int, nonce uint64, to common.Address, value *big.Int, gas uint64, gasPrice *big.Int, data []byte) *types.Transaction {
	return types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: new(big.Int).Set(gasPrice),
		GasFeeCap: new(big.Int).Set(gasPrice),
		Gas:       gas,
		To:        &to,
		Value:     new(big.Int).Set(value),
		Data:      common.CopyBytes(data),
	})
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
