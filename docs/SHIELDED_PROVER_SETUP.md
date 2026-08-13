Shielded prover setup and shielded ceremony

Overview

This doc covers: (1) running the shielded MPC ceremony (dev single-participant example included), (2) how to install the chain-format verifying key, and (3) a compact, concrete example that encodes a TKMSHIELD1 envelope, signs it, and submits the raw signed transaction to a JSON-RPC endpoint. The example uses the repository's helpers (core.EncodeShieldedTransaction and params.ShieldedPoolAddress) so it matches the implementation.

Prereqs

- Go toolchain (go 1.20+)
- gnark & gnark-crypto are used via go modules in this repo; no external install required
- Build/test tooling: make (used by project CI)
- A node RPC endpoint (HTTP) reachable for broadcasting (or use your node's RPC socket)

Quick single-participant ceremony (dev only)

# create and enter working dir
mkdir -p /tmp/shielded-ceremony && cd /tmp/shielded-ceremony

# Phase 1 (init, single contribute, verify)
go run ../../cmd/shielded-ceremony init-phase1 -out phase1-0.bin
# (optionally: have other participants run contribute-phase1)
go run ../../cmd/shielded-ceremony contribute-phase1 -in phase1-0.bin -out phase1-1.bin
go run ../../cmd/shielded-ceremony verify-phase1 -beacon 0xDEADBEEF -out commons.bin phase1-1.bin

# Phase 2 (init, single contribute, finalize)
go run ../../cmd/shielded-ceremony init-phase2 -commons commons.bin -out phase2-0.bin
# (optionally: have other participants run contribute-phase2)
go run ../../cmd/shielded-ceremony contribute-phase2 -in phase2-0.bin -out phase2-1.bin

# Finalize -> produces proving.key (private), verifying.key, and verifying.hex (chain-format hex)
go run ../../cmd/shielded-ceremony finalize -commons commons.bin -beacon 0xDEADBEEF -pk proving.key -vk verifying.key -vk-hex verifying.hex phase2-1.bin

Notes

- Keep proving.key secret and on prover infrastructure only. Do NOT commit it to repo or share it.
- verifying.hex is the chain-format TKMG16VK1 verifying key. It must be embedded into your chain artifact or installed at runtime before activating mainnet privacy.

Installing the verifying key into gtkm

Two options:

1) Embed into chain config / artifact: copy verifying.hex into the chain artifact slot (MainnetShieldedGroth16VerifyingKey) in your genesis/config tooling before node startup.

2) Use a node admin RPC (if the node supports it) to set the verifying key at runtime. After installing, restart or signal the node to reload chain artifacts if required.

How the prover interacts with the chain (implementation notes)

- The prover produces a core.ShieldedTransaction (the TKMSHIELD1 envelope). The repository provides core.EncodeShieldedTransaction(tx) to serialize the envelope into tx.Data() for consensus.
- The consensus requires the transaction To address to be params.ShieldedPoolAddress and the envelope encoded in tx.Data(). For shielded deposits, tx.Value > 0 and the envelope carries a deposit proof; for private spends use tx.Value == 0 and spends in the envelope.
- The shielded circuit's binding hash is computed from a 'clean' envelope (proof bytes removed) via core.ShieldedTransactionIntentHash — this is used during proof generation so signatures are over the correct intent.

Minimal Go example: encode, sign, and submit a raw TKMSHIELD1 transaction

Place this example inside this repository (for imports to resolve replace the module path placeholder with your module path from go.mod). It demonstrates a full flow: encode envelope, build a tx targeting params.ShieldedPoolAddress, sign it, and submit the raw signed bytes with eth_sendRawTransaction.

```go
// cmd/example-prover/main.go
package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	// replace with your module path, e.g. github.com/yourorg/go-tkmchain/core
	"<module>/core"
	"<module>/params"
)

func mustHexToECDSA(h string) *ecdsa.PrivateKey {
	k, err := crypto.HexToECDSA(h)
	if err != nil {
		log.Fatalf("invalid privkey: %v", err)
	}
	return k
}

func main() {
	// ----- BUILD / ENCODE THE SHIELDED ENVELOPE -----
	// NOTE: constructing a valid envelope requires running the prover to produce
	// Groth16 proofs and correct BN254 field elements. This example uses
	// placeholders to demonstrate the encoding/signing/submission flow — replace
	// these with real values produced by your proving code.
	env := &core.ShieldedTransaction{
		Version: 1,
		// Spends/Outputs/BalanceCommitment/BindingSig must be set from the prover
		// result. This example is illustrative only.
		Spends: []core.ShieldedSpend{},
		Outputs: make([]core.ShieldedOutput, core.ShieldedOutputSlots),
		BindingSig: make([]byte, 32), // must be the canonical BN254 field element
	}

	data, err := core.EncodeShieldedTransaction(env)
	if err != nil {
		log.Fatalf("encode shielded tx: %v", err)
	}

	// ----- BUILD THE SIGNABLE TRANSACTION -----
	chainID := big.NewInt(1337) // set to your chain ID
	nonce := uint64(0)         // obtain from the sender account
	gasLimit := uint64(200000)
	maxFeePerGas := big.NewInt(2_000_000_000) // 2 gwei
	maxPriorityFeePerGas := big.NewInt(1_000_000_000) // 1 gwei

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		Gas:       gasLimit,
		GasTipCap: maxPriorityFeePerGas,
		GasFeeCap: maxFeePerGas,
		To:        &params.ShieldedPoolAddress,
		Value:     big.NewInt(0), // 0 for private spends; deposits set a positive value
		Data:      data,
	})

	// ----- SIGN THE TRANSACTION -----
	priv := mustHexToECDSA("YOUR_PRIVATE_KEY_HEX") // replace with signer key (keep private)
	signer := types.LatestSignerForChainID(chainID)
	signedTx, err := types.SignTx(tx, signer, priv)
	if err != nil {
		log.Fatalf("sign tx: %v", err)
	}

	// Get the raw signed bytes (RLP encoded) for eth_sendRawTransaction
	raw, err := signedTx.MarshalBinary()
	if err != nil {
		log.Fatalf("marshal signed tx: %v", err)
	}

	// ----- SUBMIT VIA JSON-RPC (eth_sendRawTransaction) -----
	rpcURL := "http://127.0.0.1:8545" // change to your node RPC
	client, err := rpc.Dial(rpcURL)
	if err != nil {
		log.Fatalf("rpc dial: %v", err)
	}
	defer client.Close()

	var txHash string
	rawHex := hexutil.Encode(raw)
	if err := client.CallContext(context.Background(), &txHash, "eth_sendRawTransaction", rawHex); err != nil {
		log.Fatalf("eth_sendRawTransaction: %v", err)
	}
	fmt.Printf("submitted tx: %s\n", txHash)

	// Alternative: use ethclient to send the signed tx object directly:
	// ethClient, err := ethclient.Dial(rpcURL)
	// if err == nil {
	//   if err := ethClient.SendTransaction(context.Background(), signedTx); err != nil {
	//     log.Fatalf("send tx via ethclient: %v", err)
	//   }
	// }
}
```

Security & correctness notes

- The prover must produce valid Groth16 proofs and all BN254-field-encoded values used in the envelope (nullifiers, anchors, commitments, binding sig). The example uses placeholders — do not broadcast such placeholders.
- Keep proving.key and private signer keys on restricted infrastructure. Rotate or hardware-protect keys as appropriate.
- The consensus code enforces that shielded transactions target params.ShieldedPoolAddress and validates envelope structure and canonical field encodings.

