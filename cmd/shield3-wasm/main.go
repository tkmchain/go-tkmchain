//go:build js && wasm

// Command shield3-wasm packages the canonical Go Shield3 wallet builder for
// a browser Worker. It deliberately has no HTTP client: all chain reads are
// delegated to a JavaScript callback, while the seed and witness remain in
// this worker. Proof generation is delegated to the pinned Rust WASM module
// through shielded3/native_wasm.go.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"syscall/js"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
)

const maxRequestSize = 1 << 20

type buildRequest struct {
	// RequestID is an idempotency/correlation value owned by the wallet UI.
	// The canonical transaction bytes do not depend on it, but accepting it
	// lets the browser use the same request envelope as the wallet history and
	// relay layers while keeping unknown fields rejected.
	RequestID     string                          `json:"requestId,omitempty"`
	Seed          string                          `json:"seed"`
	ChainID       uint64                          `json:"chainId"`
	Account       common.Address                  `json:"account"`
	Stamp         *pqcrypto.ShieldedV3StampRecord `json:"stamp"`
	Payments      []shield3wallet.PaymentRequest  `json:"payments"`
	Deposit       bool                            `json:"deposit"`
	AmountWei     string                          `json:"amountWei"`
	RegisterStamp bool                            `json:"registerStamp"`
}

type buildResult struct {
	Raw  hexutil.Bytes `json:"raw"`
	Hash common.Hash   `json:"hash"`
}

type browserRPC struct{}

type rpcReply struct {
	value string
	err   string
}

func (browserRPC) CallContext(ctx context.Context, out any, method string, args ...any) error {
	callbackFn := js.Global().Get("tkmShield3Rpc")
	if callbackFn.Type() != js.TypeFunction {
		return errors.New("browser Shield3 RPC bridge is unavailable")
	}
	params, err := json.Marshal(args)
	if err != nil {
		return err
	}
	result := make(chan rpcReply, 1)
	callback := js.FuncOf(func(_ js.Value, values []js.Value) any {
		reply := rpcReply{}
		if len(values) > 0 && values[0].Type() != js.TypeNull && values[0].Type() != js.TypeUndefined {
			reply.value = values[0].String()
		}
		if len(values) > 1 && values[1].Type() != js.TypeNull && values[1].Type() != js.TypeUndefined {
			reply.err = values[1].String()
		}
		result <- reply
		return nil
	})
	defer callback.Release()
	callbackFn.Invoke(method, string(params), callback)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case reply := <-result:
		if reply.err != "" {
			return errors.New(reply.err)
		}
		if reply.value == "" {
			return errors.New("browser Shield3 RPC returned an empty result")
		}
		return json.Unmarshal([]byte(reply.value), out)
	}
}

func build(raw string) (buildResult, error) {
	if len(raw) == 0 || len(raw) > maxRequestSize {
		return buildResult{}, errors.New("browser Shield3 request is too large")
	}
	var request buildRequest
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return buildResult{}, fmt.Errorf("invalid browser Shield3 request: %w", err)
	}
	if request.ChainID == 0 || request.Stamp == nil || (!request.Deposit && !request.RegisterStamp && len(request.Payments) == 0) {
		return buildResult{}, errors.New("browser Shield3 request is missing chain, stamp, or payment")
	}
	seedText := strings.TrimPrefix(strings.TrimPrefix(request.Seed, "0x"), "0X")
	seed, err := hex.DecodeString(seedText)
	if err != nil || len(seed) != pqcrypto.MLDSA87SeedSize {
		return buildResult{}, errors.New("browser Shield3 seed must be exactly 32 bytes")
	}
	defer clear(seed)
	identity, err := shield3wallet.NewIdentity(seed, request.ChainID, request.Stamp)
	if err != nil {
		return buildResult{}, err
	}
	defer identity.Clear()
	if request.Account != (common.Address{}) && request.Account != identity.Address {
		return buildResult{}, errors.New("browser Shield3 account does not match the spending seed")
	}
	var tx *types.Transaction
	if request.RegisterStamp {
		tx, err = shield3wallet.BuildAndSignStamp(context.Background(), browserRPC{}, seed, identity)
	} else if request.Deposit {
		amount, ok := new(big.Int).SetString(strings.TrimSpace(request.AmountWei), 10)
		if !ok || amount.Sign() <= 0 {
			return buildResult{}, errors.New("browser Shield3 deposit amount must be a positive integer")
		}
		tx, err = shield3wallet.BuildAndSignDeposit(context.Background(), browserRPC{}, seed, identity, amount)
	} else {
		payments, decodeErr := shield3wallet.DecodePayments(request.ChainID, request.Payments)
		if decodeErr != nil {
			return buildResult{}, decodeErr
		}
		tx, err = shield3wallet.BuildAndSignBatch(context.Background(), browserRPC{}, seed, identity, payments)
	}
	if err != nil {
		return buildResult{}, err
	}
	rawTx, err := tx.MarshalBinary()
	if err != nil {
		return buildResult{}, err
	}
	return buildResult{Raw: rawTx, Hash: tx.Hash()}, nil
}

func main() {
	buildFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeFunction {
			return nil
		}
		request := args[0].String()
		callback := args[1]
		go func() {
			result, err := build(request)
			if err != nil {
				callback.Invoke("", err.Error())
				return
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				callback.Invoke("", err.Error())
				return
			}
			callback.Invoke(string(encoded), "")
		}()
		return nil
	})
	defer buildFn.Release()
	js.Global().Set("tkmShield3Build", buildFn)
	select {}
}
