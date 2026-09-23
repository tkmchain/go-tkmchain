package main

import (
	"bufio"
	"context"
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
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
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
	rpc, err := utils.DialRPCWithHeaders(endpoint, nil)
	if err != nil {
		return fmt.Errorf("connect to gtkm IPC at %s: %w (start gtkm first)", endpoint, err)
	}
	defer rpc.Close()
	client := ethclient.NewClient(rpc)
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
		fmt.Println("  4) Refresh       Reload node and account data")
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
			continue
		case "0", "q", "Q":
			fmt.Println("\n  Wallet closed.")
			return nil
		default:
			fmt.Println("\n  Choose 1, 2, 3, 4, or 0.")
			pauseWallet(reader)
		}
	}
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
