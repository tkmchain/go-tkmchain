package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/urfave/cli/v2"
)

var (
	supplyBlockFlag = &cli.Uint64Flag{
		Name:  "block",
		Usage: "canonical block height to query or index",
	}
	supplyRPCFlags = []cli.Flag{utils.DataDirFlag, utils.HttpHeaderFlag}

	supplyCommand = &cli.Command{
		Name:      "supply",
		Usage:     "Query persisted TKM supply and reward accounting",
		ArgsUsage: "[endpoint]",
		Description: `
The supply command attaches to a running gtkm node and uses the non-consensus
 tkmsupply RPC namespace. It indexes canonical blocks into the node database so
 total supply, miner rewards, rotating king rewards, and main king rewards can be
 queried at historical block heights without changing consensus rules.`,
		Flags:  supplyRPCFlags,
		Action: supplyLatest,
		Subcommands: []*cli.Command{
			{
				Name:      "latest",
				Usage:     "Show supply accounting at the canonical head",
				ArgsUsage: "[endpoint]",
				Flags:     supplyRPCFlags,
				Action:    supplyLatest,
			},
			{
				Name:      "at",
				Usage:     "Show supply accounting at a canonical block height",
				ArgsUsage: "[endpoint]",
				Flags:     append(supplyRPCFlags, supplyBlockFlag),
				Action:    supplyAt,
			},
			{
				Name:      "sync",
				Usage:     "Build or extend the persisted supply index up to a block height",
				ArgsUsage: "[endpoint]",
				Flags:     append(supplyRPCFlags, supplyBlockFlag),
				Action:    supplySync,
			},
		},
	}
)

func supplyLatest(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var out json.RawMessage
	if err := client.CallContext(context.Background(), &out, "tkmsupply_latest"); err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func supplyAt(ctx *cli.Context) error {
	block, err := supplyRequiredBlock(ctx)
	if err != nil {
		return err
	}
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var out json.RawMessage
	if err := client.CallContext(context.Background(), &out, "tkmsupply_atBlock", hexutil.Uint64(block)); err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func supplySync(ctx *cli.Context) error {
	block, err := supplyRequiredBlock(ctx)
	if err != nil {
		return err
	}
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var out json.RawMessage
	if err := client.CallContext(context.Background(), &out, "tkmsupply_sync", hexutil.Uint64(block)); err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func supplyRequiredBlock(ctx *cli.Context) (uint64, error) {
	if !ctx.IsSet(supplyBlockFlag.Name) {
		return 0, errors.New("--block is required")
	}
	return ctx.Uint64(supplyBlockFlag.Name), nil
}
