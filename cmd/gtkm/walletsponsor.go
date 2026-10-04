package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/console/prompt"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/rpc"
)

func walletStampSponsorship(reader *bufio.Reader, rpcClient *rpc.Client, client *ethclient.Client, ks *keystore.KeyStore, local []accounts.Account, chainID *big.Int) error {
	if chainID == nil || chainID.Sign() <= 0 || !chainID.IsUint64() {
		return errors.New("invalid sponsorship chain ID")
	}
	fmt.Println("\nSTAMP SPONSORSHIP")
	fmt.Println("The recipient needs no public TKM. A funded, confirmed stamped sponsor pays the registration gas.")
	fmt.Println("1) Prepare my receiving address (no funds needed)")
	fmt.Println("2) Sponsor: create an offer code for an external address")
	fmt.Println("3) Recipient: enter offer code and authorize my stamp")
	fmt.Println("4) Sponsor: enter authorization code and pay registration")
	fmt.Println("5) Retry a saved signed registration")
	fmt.Println("0) Back")
	choice, err := readWalletLine(reader, "Select")
	if err != nil || choice == "0" {
		return err
	}
	if choice == "5" {
		return retryWalletStamp(reader, rpcClient, client, chainID)
	}
	if choice != "1" && choice != "2" && choice != "3" && choice != "4" {
		return errors.New("choose 1 through 5, or 0")
	}
	account, err := chooseWalletAccount(reader, local)
	if err != nil {
		return err
	}
	algorithm, err := ks.AccountAlgorithm(account)
	if err != nil {
		return err
	}
	if algorithm != pqcrypto.AlgorithmMLDSA87 {
		return errors.New("stamp sponsorship requires an ML-DSA-87 account; migrate this ECDSA account first")
	}
	password := utils.GetPassPhrase("Selected account password", false)
	key, err := walletPQKey(ks, account, password)
	if err != nil {
		return err
	}
	defer clear(key.Seed)
	if key.Shield3Stamp == nil && choice == "1" {
		name, err := prompt.Stdin.PromptPassword("Private stamp name: ")
		if err != nil {
			return err
		}
		country, err := prompt.Stdin.PromptPassword("Private stamp country: ")
		if err != nil {
			return err
		}
		stamp, err := pqcrypto.CreateShieldedV3Stamp(key.Seed, chainID.Uint64(), name, country)
		if err != nil {
			return err
		}
		if err := ks.StampPQAccountRecord(account, password, stamp); err != nil {
			return err
		}
		key.Shield3Stamp = stamp
	}
	if key.Shield3Stamp == nil {
		return errors.New("prepare the receiving address with option 1 first; this costs no gas")
	}
	identity, err := shield3wallet.NewIdentity(key.Seed, chainID.Uint64(), key.Shield3Stamp)
	if err != nil {
		return err
	}
	defer identity.Clear()
	if choice == "1" {
		fmt.Printf("Account: %s\nThe encrypted stamp is saved locally. Give this receiving address to your sponsor.\n", account.Address.Hex())
		return exportWalletSponsorCode(reader, identity.Code)
	}
	if choice == "2" {
		fmt.Println("Enter the recipient's tkmshield3 address from option 1. A bare 0x address cannot supply its encrypted stamp and public keys.")
	}
	code, err := readWalletSponsorCode(reader)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if choice == "2" {
		offer, err := shield3wallet.BuildStampSponsorshipOffer(ctx, rpcClient, key.Seed, identity, code)
		if err != nil {
			return err
		}
		printWalletSponsorship(offer)
		if err := confirmWalletSponsorship(reader, "OFFER"); err != nil {
			return err
		}
		encoded, err := shield3wallet.EncodeStampSponsorshipCode(offer)
		if err != nil {
			return err
		}
		fmt.Println("Send this offer to the recipient. No gas has been paid. Avoid using the sponsor's nonce for another transaction until completion.")
		return exportWalletSponsorCode(reader, encoded)
	}
	packet, err := shield3wallet.DecodeStampSponsorshipCode(code, chainID.Uint64())
	if err != nil {
		return err
	}
	if choice == "3" {
		if packet.Beneficiary != account.Address {
			return errors.New("offer belongs to another recipient; select the account that created the receiving address")
		}
		printWalletSponsorship(packet)
		if err := confirmWalletSponsorship(reader, "AUTHORIZE"); err != nil {
			return err
		}
		fmt.Println("Building your stamp ownership proof locally...")
		authorized, err := shield3wallet.AuthorizeStampSponsorship(ctx, rpcClient, key.Seed, identity, packet.Transaction)
		if err != nil {
			return err
		}
		encoded, err := shield3wallet.EncodeStampSponsorshipCode(authorized)
		if err != nil {
			return err
		}
		fmt.Println("Return this authorization code to the sponsor. Registration is pending the sponsor's final signature and block confirmation.")
		return exportWalletSponsorCode(reader, encoded)
	}
	unsigned, err := shield3wallet.BuildSponsoredStamp(ctx, rpcClient, key.Seed, identity, packet.Transaction)
	if err != nil {
		return err
	}
	printWalletSponsorship(packet)
	if err := confirmWalletSponsorship(reader, "PAY"); err != nil {
		return err
	}
	signingKey, err := pqcrypto.NewMLDSA87FromSeed(key.Seed)
	if err != nil {
		return err
	}
	signed, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(chainID), signingKey)
	if err != nil {
		return err
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return err
	}
	// Save the exact signed transaction before any network request. Retrying this
	// file reuses its hash, nonce, and proof instead of creating another payment.
	dir := filepath.Join(filepath.Dir(account.URL.Path), ".stamp-submissions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, signed.Hash().Hex()+".txt")
	if err := writeWalletSponsorFile(path, hexutil.Encode(raw)); err != nil {
		return fmt.Errorf("save signed registration before broadcast: %w", err)
	}
	fmt.Printf("Signed registration saved: %s\n", path)
	return submitWalletSponsoredStamp(reader, rpcClient, client, signed)
}

func printWalletSponsorship(packet shield3wallet.StampSponsorship) {
	fee, _ := new(big.Int).SetString(packet.MaxFeeWei, 10)
	fmt.Printf("Sponsor: %s\nRecipient: %s\nMaximum gas fee: %s TKM (%s wei)\nExpires at chain timestamp: %s\n", packet.Sponsor.Hex(), packet.Beneficiary.Hex(), formatTKM(fee), packet.MaxFeeWei, time.Unix(int64(packet.ValidUntil), 0).UTC().Format(time.RFC3339))
	fmt.Println("This pays registration gas only. It does not transfer a TKM balance to the recipient.")
}

func confirmWalletSponsorship(reader *bufio.Reader, action string) error {
	answer, err := readWalletLine(reader, "Type "+action+" to approve, or anything else to cancel")
	if err != nil {
		return err
	}
	if answer != action {
		return errors.New("cancelled; nothing signed or submitted")
	}
	return nil
}

// Receiving addresses and proof codes exceed some terminals' paste limits.
// @file imports a complete public code without truncating it at the terminal.
func readWalletSponsorCode(reader *bufio.Reader) (string, error) {
	input, err := readWalletLine(reader, "Paste public address/code, or @/path/to/code.txt (recommended for long codes)")
	if err != nil {
		return "", err
	}
	return loadWalletSponsorCode(input)
}

func loadWalletSponsorCode(input string) (string, error) {
	const limit = 2 * shield3wallet.MaxStampSponsorshipCodeSize
	if strings.HasPrefix(input, "@") {
		file, err := os.Open(strings.TrimPrefix(input, "@"))
		if err != nil {
			return "", err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return "", errors.New("code file must be a regular file")
		}
		data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
		if err != nil {
			return "", err
		}
		input = string(data)
	}
	if len(input) > limit {
		return "", errors.New("sponsorship input is too large")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty sponsorship input")
	}
	return input, nil
}

func writeWalletSponsorFile(path, code string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := io.WriteString(file, code+"\n")
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func exportWalletSponsorCode(reader *bufio.Reader, code string) error {
	path, err := readWalletLine(reader, "Save public code to a new file (blank to display)")
	if err != nil {
		return err
	}
	if path == "" {
		fmt.Println(code)
	} else if err := writeWalletSponsorFile(path, code); err != nil {
		return fmt.Errorf("save code: %w (use a new filename)", err)
	} else {
		fmt.Printf("Saved: %s\nShare this public code file; keep your keyfile and password private.\n", path)
	}
	pauseWallet(reader)
	return nil
}

func retryWalletStamp(reader *bufio.Reader, rpcClient *rpc.Client, client *ethclient.Client, chainID *big.Int) error {
	code, err := readWalletSponsorCode(reader)
	if err != nil {
		return err
	}
	raw, err := hexutil.Decode(code)
	if err != nil {
		return errors.New("expected the saved signed registration file, entered as @/path/to/file.txt")
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil || tx.ChainId().Cmp(chainID) != 0 {
		return errors.New("invalid signed registration or wrong chain")
	}
	if _, err := types.Sender(types.NewQuantumSigner(chainID), &tx); err != nil {
		return err
	}
	if err := core.ValidateAntarticalStampProof(&tx); err != nil {
		return err
	}
	fmt.Printf("Rebroadcast the SAME signed registration: %s\n", tx.Hash().Hex())
	if err := confirmWalletSponsorship(reader, "RETRY"); err != nil {
		return err
	}
	return submitWalletSponsoredStamp(reader, rpcClient, client, &tx)
}

func submitWalletSponsoredStamp(reader *bufio.Reader, rpcClient *rpc.Client, client *ethclient.Client, tx *types.Transaction) error {
	envelope, err := core.DecodeAntarticalStamp(tx.Data())
	if err != nil {
		return err
	}
	sponsor, err := types.Sender(types.NewQuantumSigner(tx.ChainId()), tx)
	if err != nil {
		return err
	}
	beneficiary, err := core.AntarticalStampBeneficiary(sponsor, envelope)
	if err != nil {
		return err
	}
	fmt.Printf("Registration transaction: %s\n", tx.Hash().Hex())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// A previous submission may have succeeded even if its RPC reply was lost.
	var current core.AntarticalStampStatus
	err = rpcClient.CallContext(ctx, &current, "tkmprivacy_antarticalStamp", beneficiary)
	if err == nil && current.Registered {
		cancel()
		if current.Owner != envelope.Owner || current.Commitment != envelope.Stamp.Commitment {
			return errors.New("recipient is registered with a different stamp")
		}
		fmt.Printf("Already confirmed: %s\n", current.TransactionHash.Hex())
		pauseWallet(reader)
		return nil
	}
	if err != nil {
		cancel()
		return err
	}
	err = client.SendTransaction(ctx, tx)
	cancel()
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already known") {
		return fmt.Errorf("submission of %s not confirmed: %w; retry the saved signed file to keep the same transaction", tx.Hash().Hex(), err)
	}
	fmt.Println("Submitted; waiting for a block. The recipient is not registered until confirmation.")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	receipt, err := waitWalletReceipt(ctx, client, tx.Hash())
	if err != nil {
		return fmt.Errorf("registration %s is unconfirmed: %w; keep the saved transaction for retry", tx.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("registration %s reverted", tx.Hash().Hex())
	}
	if err := rpcClient.CallContext(ctx, &current, "tkmprivacy_antarticalStamp", beneficiary); err != nil {
		return err
	}
	if !current.Registered || current.Owner != envelope.Owner || current.Commitment != envelope.Stamp.Commitment {
		return errors.New("mined transaction did not confirm the expected recipient stamp")
	}
	fmt.Printf("Recipient stamp confirmed: %s\nTransaction: %s\n", beneficiary.Hex(), current.TransactionHash.Hex())
	pauseWallet(reader)
	return nil
}
