package main

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
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

type supplyRPCEntry struct {
	BlockNumber         hexutil.Uint64 `json:"blockNumber"`
	BlockHash           common.Hash    `json:"blockHash"`
	GenesisSupply       *hexutil.Big   `json:"genesisSupply"`
	TotalIssued         *hexutil.Big   `json:"totalIssued"`
	TotalSupply         *hexutil.Big   `json:"totalSupply"`
	MainKingRewards     *hexutil.Big   `json:"mainKingRewards"`
	RotatingKingRewards *hexutil.Big   `json:"rotatingKingRewards"`
	MinerRewards        *hexutil.Big   `json:"minerRewards"`
	IndexedTo           hexutil.Uint64 `json:"indexedTo"`
}

type supplyHumanEntry struct {
	BlockNumber            uint64      `json:"blockNumber"`
	BlockHash              common.Hash `json:"blockHash"`
	GenesisSupplyTKM       string      `json:"genesisSupplyTKM"`
	TotalIssuedTKM         string      `json:"totalIssuedTKM"`
	TotalSupplyTKM         string      `json:"totalSupplyTKM"`
	MainKingRewardsTKM     string      `json:"mainKingRewardsTKM"`
	RotatingKingRewardsTKM string      `json:"rotatingKingRewardsTKM"`
	MinerRewardsTKM        string      `json:"minerRewardsTKM"`
	GenesisSupplyWei       string      `json:"genesisSupplyWei"`
	TotalIssuedWei         string      `json:"totalIssuedWei"`
	TotalSupplyWei         string      `json:"totalSupplyWei"`
	MainKingRewardsWei     string      `json:"mainKingRewardsWei"`
	RotatingKingRewardsWei string      `json:"rotatingKingRewardsWei"`
	MinerRewardsWei        string      `json:"minerRewardsWei"`
	IndexedTo              uint64      `json:"indexedTo"`
}

func supplyLatest(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var out supplyRPCEntry
	if err := client.CallContext(context.Background(), &out, "tkmsupply_latest"); err != nil {
		return err
	}
	return tkmPhonePrintJSON(supplyHumanize(out))
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
	var out supplyRPCEntry
	if err := client.CallContext(context.Background(), &out, "tkmsupply_atBlock", hexutil.Uint64(block)); err != nil {
		return err
	}
	return tkmPhonePrintJSON(supplyHumanize(out))
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
	var out supplyRPCEntry
	if err := client.CallContext(context.Background(), &out, "tkmsupply_sync", hexutil.Uint64(block)); err != nil {
		return err
	}
	return tkmPhonePrintJSON(supplyHumanize(out))
}

func supplyHumanize(entry supplyRPCEntry) supplyHumanEntry {
	return supplyHumanEntry{
		BlockNumber:            uint64(entry.BlockNumber),
		BlockHash:              entry.BlockHash,
		GenesisSupplyTKM:       supplyBigToTKM(entry.GenesisSupply),
		TotalIssuedTKM:         supplyBigToTKM(entry.TotalIssued),
		TotalSupplyTKM:         supplyBigToTKM(entry.TotalSupply),
		MainKingRewardsTKM:     supplyBigToTKM(entry.MainKingRewards),
		RotatingKingRewardsTKM: supplyBigToTKM(entry.RotatingKingRewards),
		MinerRewardsTKM:        supplyBigToTKM(entry.MinerRewards),
		GenesisSupplyWei:       supplyBigToWei(entry.GenesisSupply),
		TotalIssuedWei:         supplyBigToWei(entry.TotalIssued),
		TotalSupplyWei:         supplyBigToWei(entry.TotalSupply),
		MainKingRewardsWei:     supplyBigToWei(entry.MainKingRewards),
		RotatingKingRewardsWei: supplyBigToWei(entry.RotatingKingRewards),
		MinerRewardsWei:        supplyBigToWei(entry.MinerRewards),
		IndexedTo:              uint64(entry.IndexedTo),
	}
}

func supplyBigToTKM(value *hexutil.Big) string {
	if value == nil {
		return "0"
	}
	return tkmPhoneWeiToTKM((*big.Int)(value))
}

func supplyBigToWei(value *hexutil.Big) string {
	if value == nil {
		return "0"
	}
	return (*big.Int)(value).String()
}

func supplyRequiredBlock(ctx *cli.Context) (uint64, error) {
	if !ctx.IsSet(supplyBlockFlag.Name) {
		return 0, errors.New("--block is required")
	}
	return ctx.Uint64(supplyBlockFlag.Name), nil
}
