package gui

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

func (g *GUI) shield6PreparedProver(pin, provingPin common.Hash, registration ...bool) (*recipient.PreparedProver, error) {
	isRegistration := len(registration) == 1 && registration[0]
	target := &g.shield6Prover
	prefix, manifest := "recipient", "manifest.json"
	if isRegistration {
		target = &g.shield6RegistrationProver
		prefix, manifest = "registration", "registration-manifest.json"
	}
	if *target != nil && common.Hash((*target).Pins().VerifyingKey) == pin && common.Hash((*target).Pins().ProvingKey) == provingPin {
		return *target, nil
	}
	directory := g.opts.Shield6ParametersDir
	if directory == "" {
		directory = os.Getenv("TKM_SHIELD6_RECIPIENT_PARAMETERS")
	}
	if directory == "" {
		return nil, errors.New("install authenticated Shield6 recipient parameters in the local wallet before sending")
	}
	file, err := os.Open(filepath.Join(directory, manifest))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var pins recipient.ArtifactPins
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pins); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || (provingPin == (common.Hash{}) || common.Hash(pins.ProvingKey) != provingPin || common.Hash(pins.VerifyingKey) != pin) {
		return nil, errors.New("recipient parameters do not match this chain")
	}
	open := recipient.OpenPreparedProver
	if isRegistration {
		open = recipient.OpenRegistrationProver
	}
	prepared, err := open(filepath.Join(directory, prefix+".r1cs"), filepath.Join(directory, prefix+".pk"), filepath.Join(directory, prefix+".vk"), pins)
	if err != nil {
		return nil, err
	}
	*target = prepared
	return prepared, nil
}

// Called only after handleShield3 has authenticated loopback origin/token and
// bounded/decoded the request. No seed or witness is exposed through JSON-RPC.
func (g *GUI) handleShield6(w http.ResponseWriter, r *http.Request, req shield3Request, operation string) {
	reply := func(code int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(value)
	}
	fail := func(err error) { reply(400, map[string]string{"error": err.Error()}) }
	if operation == "relay-status" {
		if len(req.RequestID) < 8 || len(req.RequestID) > 128 {
			fail(errors.New("invalid Shield6 request ID"))
			return
		}
		g.shield3Mu.Lock()
		defer g.shield3Mu.Unlock()
		id := "shield6/" + req.RequestID
		record := g.shield3Requests[id]
		var err error
		if record == nil && g.opts.WalletStateDir != "" {
			record, err = g.loadShield3Submission(id)
			if err != nil {
				fail(err)
				return
			}
		}
		if record == nil {
			reply(200, map[string]any{"found": false})
			return
		}
		var tx types.Transaction
		if err := tx.UnmarshalBinary(record.Raw); err != nil {
			fail(err)
			return
		}
		from, err := types.Sender(types.NewQuantumSigner(tx.ChainId()), &tx)
		if err != nil || from != req.Account {
			fail(errors.New("request account mismatch"))
			return
		}
		client := ethclient.NewClient(g.client)
		included := false
		receipt, err := client.TransactionReceipt(r.Context(), record.Hash)
		if err == nil {
			header, headErr := client.HeaderByNumber(r.Context(), receipt.BlockNumber)
			included = headErr == nil && header.Hash() == receipt.BlockHash
		}
		nonce, nonceErr := client.NonceAt(r.Context(), from, nil)
		expired := false
		if envelope, recognized, decodeErr := core.DecodeShieldedV6Transaction(tx.Data()); decodeErr == nil && recognized && envelope.ValidUntil > 0 {
			if head, headErr := client.HeaderByNumber(r.Context(), nil); headErr == nil {
				expired = head.Time >= envelope.ValidUntil
			}
		}
		reply(200, map[string]any{"found": true, "transactionHash": record.Hash, "included": included, "successful": included && receipt.Status == types.ReceiptStatusSuccessful, "obsolete": nonceErr == nil && nonce > tx.Nonce(), "expired": expired})
		return
	}
	if operation != "identity" && operation != "scan" && operation != "send" && operation != "shield" && operation != "register-stamp" && operation != "withdraw" {
		fail(errors.New("unsupported Shield6 wallet operation"))
		return
	}
	client := ethclient.NewClient(g.client)
	chainID, err := client.ChainID(r.Context())
	if err != nil {
		fail(err)
		return
	}
	if !chainID.IsUint64() || chainID.Sign() <= 0 {
		fail(errors.New("invalid Shield6 chain"))
		return
	}
	identity, err := shield3wallet.NewIdentity(req.Seed, chainID.Uint64(), req.Stamp)
	if err != nil {
		fail(err)
		return
	}
	defer identity.Clear()
	if identity.Address != req.Account {
		fail(errors.New("Shield6 account identity mismatch"))
		return
	}
	keys, err := shielded6.DeriveWalletKeys(req.Seed, chainID.Uint64())
	if err != nil {
		fail(err)
		return
	}
	defer keys.Clear()
	g.shield3Mu.Lock()
	defer g.shield3Mu.Unlock()
	if operation == "identity" {
		code, err := shield3wallet.ShieldedV6PaymentCode(r.Context(), g.client, identity, keys)
		if err != nil {
			fail(err)
			return
		}
		encoded, err := recipient.EncodePaymentCode(code)
		if err != nil {
			fail(err)
			return
		}
		reply(200, map[string]any{"paymentCode": encoded, "account": identity.Address})
		return
	}
	if operation == "scan" {
		scan, err := shield3wallet.ScanShieldedV6(r.Context(), g.client, keys)
		if err != nil {
			fail(err)
			return
		}
		for _, owned := range scan.Notes {
			owned.Note.Clear()
		}
		reply(200, map[string]any{"balanceWei": scan.BalanceWei.String(), "noteCount": len(scan.Notes)})
		return
	}
	if len(req.RequestID) < 8 || len(req.RequestID) > 128 {
		fail(errors.New("a stable Shield6 request ID is required"))
		return
	}
	publicReq := req
	publicReq.Seed = nil // Never persist a seed-derived request hash.
	data, _ := json.Marshal(struct {
		Domain, Operation string
		Request           shield3Request
	}{"TKM_GUI_SHIELD6_V2", operation, publicReq})
	digest := sha512.Sum512(data)
	if g.shield3Requests == nil {
		g.shield3Requests = make(map[string]*shield3RequestRecord)
	}
	id := "shield6/" + req.RequestID
	record := g.shield3Requests[id]
	if record == nil && g.opts.WalletStateDir != "" {
		record, err = g.loadShield3Submission(id)
		if err != nil {
			fail(err)
			return
		}
	}
	if record != nil && record.Digest != digest {
		fail(errors.New("request ID was already used for different Shield6 fields"))
		return
	}
	if record == nil {
		if err := g.shield6CheckPending(r.Context(), identity.Address); err != nil {
			fail(err)
			return
		}
		if len(g.shield3Requests) >= 256 {
			fail(errors.New("wallet submission cache is full"))
			return
		}
		amount := new(big.Int)
		if operation != "register-stamp" {
			if _, ok := amount.SetString(req.AmountWei, 10); !ok || amount.Sign() <= 0 || amount.BitLen() > 128 {
				fail(errors.New("invalid Shield6 amount"))
				return
			}
		}
		nonce, err := client.PendingNonceAt(r.Context(), identity.Address)
		if err != nil {
			fail(err)
			return
		}
		price, err := client.SuggestGasPrice(r.Context())
		if err != nil || price == nil || price.Sign() <= 0 {
			fail(errors.New("gas price unavailable"))
			return
		}
		accountKey, err := pqcrypto.NewMLDSA87FromSeed(req.Seed)
		if err != nil {
			fail(err)
			return
		}
		value := new(big.Int)
		if operation == "shield" {
			value.Set(amount)
		}
		skeleton := types.NewTx(&types.PQTkmTx{ChainID: chainID, Nonce: nonce, Gas: 10_000_000, GasTipCap: price, GasFeeCap: new(big.Int).Mul(price, big.NewInt(2)), To: &params.ShieldedPoolAddress, Value: value, Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(accountKey)})
		var signed *types.Transaction
		if operation == "register-stamp" {
			var status struct {
				Pin        common.Hash `json:"registrationKeyHash"`
				ProvingPin common.Hash `json:"registrationProvingKeyHash"`
			}
			if err := g.client.CallContext(r.Context(), &status, "tkmprivacy_shieldedV6Status"); err != nil {
				fail(err)
				return
			}
			registrationProver, loadErr := g.shield6PreparedProver(status.Pin, status.ProvingPin, true)
			if loadErr != nil {
				fail(loadErr)
				return
			}
			signed, err = shield3wallet.BuildAndSignShieldedV6Binding(r.Context(), g.client, req.Seed, identity, keys, skeleton, registrationProver)
			g.shield6RegistrationProver = nil
		} else {
			var status struct {
				Required   bool        `json:"recipientProofsRequired"`
				Pin        common.Hash `json:"recipientKeyHash"`
				ProvingPin common.Hash `json:"recipientProvingKeyHash"`
			}
			if err = g.client.CallContext(r.Context(), &status, "tkmprivacy_shieldedV6Status"); err != nil {
				fail(err)
				return
			}
			if !status.Required {
				fail(errors.New("recipient-proof fork is not active"))
				return
			}
			prepared, loadErr := g.shield6PreparedProver(status.Pin, status.ProvingPin)
			if loadErr != nil {
				fail(loadErr)
				return
			}
			proofClient := &shield3wallet.ShieldedV6RecipientClient{Prover: prepared}
			if operation == "shield" {
				signed, err = shield3wallet.BuildAndSignShieldedV6Deposit(r.Context(), g.client, req.Seed, identity, keys, skeleton, amount, cryptorand.Reader, proofClient)
			} else {
				var code recipient.PaymentCode
				if operation == "send" {
					var decodeErr error
					code, decodeErr = recipient.DecodePaymentCode(req.Recipient, chainID.Uint64())
					if decodeErr != nil {
						fail(decodeErr)
						return
					}
				} else if !common.IsHexAddress(req.Recipient) {
					fail(errors.New("withdrawal requires a stamped public account address"))
					return
				}

				scan, scanErr := shield3wallet.ScanShieldedV6(r.Context(), g.client, keys)
				if scanErr != nil {
					fail(scanErr)
					return
				}
				defer func() {
					for _, owned := range scan.Notes {
						owned.Note.Clear()
					}
				}()
				notes, total, selectErr := shield3wallet.SelectShieldedV6Notes(scan.Notes, amount)
				if selectErr != nil {
					fail(selectErr)
					return
				}
				defer clear(total.Bits())
				if operation == "withdraw" {
					signed, err = shield3wallet.BuildAndSignShieldedV6Withdrawal(r.Context(), g.client, req.Seed, identity, keys, skeleton, notes, common.HexToAddress(req.Recipient), amount, cryptorand.Reader, proofClient)
				} else {
					outputs := []core.ShieldedV6WalletOutput{{Address: code.Address, Amount: amount}}
					proofClient.Codes = []recipient.PaymentCode{code}
					change := new(big.Int).Sub(total, amount)
					defer clear(change.Bits())
					if change.Sign() > 0 {
						self, selfErr := shield3wallet.ShieldedV6PaymentCode(r.Context(), g.client, identity, keys)
						if selfErr != nil {
							fail(selfErr)
							return
						}
						outputs = append(outputs, core.ShieldedV6WalletOutput{Address: keys.Address(), Amount: change})
						proofClient.Codes = append(proofClient.Codes, self)
					}
					signed, err = shield3wallet.BuildAndSignShieldedV6Payment(r.Context(), g.client, req.Seed, identity, keys, skeleton, notes, outputs, cryptorand.Reader, proofClient)
				}
			}
		}
		if err != nil {
			fail(err)
			return
		}
		raw, err := signed.MarshalBinary()
		if err != nil {
			fail(err)
			return
		}
		record = &shield3RequestRecord{Digest: digest, Raw: raw, Hash: signed.Hash()}
		if err := g.saveShield3Submission(id, record); err != nil {
			fail(err)
			return
		}
		g.shield3Requests[id] = record
	}
	var saved types.Transaction
	if err := saved.UnmarshalBinary(record.Raw); err != nil {
		fail(err)
		return
	}
	from, senderErr := types.Sender(types.NewQuantumSigner(saved.ChainId()), &saved)
	if senderErr != nil || from != identity.Address || !core.HasShieldedV6Prefix(saved.Data()) {
		fail(errors.New("saved Shield6 request account mismatch"))
		return
	}

	var hash common.Hash
	if err := g.client.CallContext(r.Context(), &hash, "eth_sendRawTransaction", record.Raw); err != nil {
		reply(200, map[string]any{"transactionHash": record.Hash, "status": "unconfirmed", "submissionError": err.Error()})
		return
	}
	if hash != record.Hash {
		fail(errors.New("node returned a different transaction hash"))
		return
	}
	record.Submitted = true
	g.shield3Requests[id] = record
	reply(200, map[string]any{"transactionHash": record.Hash, "status": "submitted"})
}

// Block a fresh request while a previous signed Shield6 transaction could
// still use the account nonce. Retrying its request ID resubmits exact bytes.
func (g *GUI) shield6CheckPending(ctx context.Context, account common.Address) error {
	var nonce hexutil.Uint64
	if err := g.client.CallContext(ctx, &nonce, "eth_getTransactionCount", account, "latest"); err != nil {
		return err
	}
	var head struct {
		Timestamp hexutil.Uint64 `json:"timestamp"`
	}
	if err := g.client.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return err
	}
	inspect := func(record *shield3RequestRecord) (bool, error) {
		var tx types.Transaction
		if record == nil || tx.UnmarshalBinary(record.Raw) != nil || !core.HasShieldedV6Prefix(tx.Data()) {
			return false, nil
		}
		from, err := types.Sender(types.NewQuantumSigner(tx.ChainId()), &tx)
		if err != nil {
			return false, err
		}
		if from != account {
			return false, nil
		}
		expired := false
		if e, recognized, err := core.DecodeShieldedV6Transaction(tx.Data()); err == nil && recognized && e.ValidUntil > 0 {
			expired = uint64(head.Timestamp) >= e.ValidUntil
		}
		if tx.Nonce() < uint64(nonce) || expired {
			return true, nil
		}
		return false, fmt.Errorf("previous Shield6 transaction %s remains unconfirmed; retry its original request", tx.Hash())
	}
	for id, record := range g.shield3Requests {
		retired, err := inspect(record)
		if err != nil {
			return err
		}
		if retired {
			delete(g.shield3Requests, id)
		}
	}
	if g.opts.WalletStateDir != "" {
		directory, err := os.Open(g.opts.WalletStateDir)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer directory.Close()
		for {
			entries, readErr := directory.ReadDir(64)
			if readErr != nil && readErr != io.EOF {
				return readErr
			}
			for _, entry := range entries {
				if entry.IsDir() || len(entry.Name()) != 133 || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				record, err := readShield3Submission(filepath.Join(g.opts.WalletStateDir, entry.Name()))
				if err != nil {
					return err
				}
				if _, err := inspect(record); err != nil {
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
		}
	}
	return nil
}
