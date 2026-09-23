package main

import (
	"bufio"
	"context"
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
			showWalletPhone(reader, rpcClient)
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

func showWalletPhone(reader *bufio.Reader, client *rpc.Client) {
	clearWalletScreen()
	fmt.Println("TKM PHONE")
	fmt.Println("─────────")
	ctx, cancel := walletRPCContext()
	defer cancel()

	var status tkmPhoneStatusView
	if err := client.CallContext(ctx, &status, "tkmphone_status"); err != nil {
		fmt.Printf("  Phone service unavailable: %v\n", err)
	} else {
		state := "inactive"
		if status.Active {
			state = "active"
		}
		fmt.Printf("  Status:       %s\n", state)
		fmt.Printf("  Chain head:   #%d\n", uint64(status.HeadNumber))
		fmt.Printf("  Data source:  %s\n", map[bool]string{true: "chain head", false: "local clock"}[status.UsingChainHead])
	}

	var bucket, mainKing, sale hexutil.Big
	if err := client.CallContext(ctx, &bucket, "tkmphone_bucketPrice"); err == nil &&
		client.CallContext(ctx, &mainKing, "tkmphone_mainKingNumberPrice") == nil &&
		client.CallContext(ctx, &sale, "tkmphone_numberSalePrice") == nil {
		fmt.Println("\n  Prices:")
		fmt.Printf("    Bucket:       %s\n", formatTKM((*big.Int)(&bucket)))
		fmt.Printf("    Main King no: %s\n", formatTKM((*big.Int)(&mainKing)))
		fmt.Printf("    Operator no:  %s\n", formatTKM((*big.Int)(&sale)))
	} else {
		fmt.Println("\n  Prices: unavailable (enable the tkmphone RPC namespace)")
	}

	var numbers []json.RawMessage
	if err := client.CallContext(ctx, &numbers, "tkmphone_registeredNumbers"); err == nil {
		fmt.Printf("\n  Registered numbers: %d\n", len(numbers))
	} else {
		fmt.Printf("\n  Registered numbers: unavailable (%v)\n", err)
	}
	fmt.Println("\n  Enter a phone number to inspect its encrypted-device metadata, or leave blank.")
	number, err := readWalletLine(reader, "Phone number")
	if err != nil {
		return
	}
	if number != "" {
		var record json.RawMessage
		if err := client.CallContext(ctx, &record, "tkmphone_registeredNumber", number); err != nil {
			fmt.Printf("\n  Lookup failed: %v\n", err)
		} else {
			fmt.Println("\n  Registered number")
			printWalletRPCJSON(record)
		}
	}
	pauseWallet(reader)
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
