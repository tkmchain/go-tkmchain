// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// keeperpayload combines a block RLP and stateless witness RLP into the
// complete Payload consumed by cmd/keeper and the Ziren host prover.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

type payload struct {
	ChainID uint64
	Block   *types.Block
	Witness *stateless.Witness
}

func main() {
	blockPath := flag.String("block", "cmd/keeper/1192c3_block.rlp", "RLP-encoded block")
	witnessPath := flag.String("witness", "cmd/keeper/1192c3_witness.rlp", "RLP-encoded stateless witness")
	chainID := flag.Uint64("chain-id", uint64(params.TKMMainnetChainID), "chain ID committed in the payload (8979 mainnet or 8980 Egypt)")
	outPath := flag.String("out", "build/keeper-input.rlp", "output keeper Payload RLP")
	inputPath := flag.String("input", "", "validate an existing complete Payload RLP instead of combining block and witness files")
	expectChainID := flag.Uint64("expect-chain-id", 0, "expected chain ID when validating an existing payload")
	expectBlock := flag.Uint64("expect-block", 0, "expected block number when validating an existing payload")
	flag.Parse()
	if *inputPath != "" {
		validatePayload(*inputPath, *expectChainID, *expectBlock)
		return
	}

	blockBytes := mustRead(*blockPath)
	var block types.Block
	if err := rlp.DecodeBytes(blockBytes, &block); err != nil {
		fatal("decode block %s: %v", *blockPath, err)
	}

	witnessBytes := mustRead(*witnessPath)
	var ext stateless.ExtWitness
	if err := rlp.DecodeBytes(witnessBytes, &ext); err != nil {
		fatal("decode witness %s: %v", *witnessPath, err)
	}
	witness := new(stateless.Witness)
	if err := witness.FromExtWitness(&ext); err != nil {
		fatal("convert witness %s: %v", *witnessPath, err)
	}

	encoded, err := rlp.EncodeToBytes(payload{
		ChainID: *chainID,
		Block:   &block,
		Witness: witness,
	})
	if err != nil {
		fatal("encode keeper payload: %v", err)
	}
	if err := os.WriteFile(*outPath, encoded, 0644); err != nil {
		fatal("write keeper payload %s: %v", *outPath, err)
	}
	fmt.Printf("wrote keeper payload %s (%d bytes, chain ID %d, block %d)\n", *outPath, len(encoded), *chainID, block.NumberU64())
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("read %s: %v", path, err)
	}
	return data
}

func validatePayload(path string, expectedChainID, expectedBlock uint64) {
	var input payload
	if err := rlp.DecodeBytes(mustRead(path), &input); err != nil {
		fatal("decode keeper payload %s: %v", path, err)
	}
	if input.Block == nil || input.Witness == nil || len(input.Witness.Headers) == 0 {
		fatal("payload %s does not contain a block and non-empty witness", path)
	}
	if expectedChainID != 0 && input.ChainID != expectedChainID {
		fatal("payload %s chain ID is %d, expected %d", path, input.ChainID, expectedChainID)
	}
	if expectedBlock != 0 && input.Block.NumberU64() != expectedBlock {
		fatal("payload %s block is %d, expected %d", path, input.Block.NumberU64(), expectedBlock)
	}
	fmt.Printf("validated keeper payload %s (chain ID %d, block %d)\n", path, input.ChainID, input.Block.NumberU64())
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
