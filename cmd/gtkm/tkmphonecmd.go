package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
)

var (
	tkmPhoneSeedFlag = &cli.StringFlag{
		Name:  "seed",
		Usage: "32-byte bucket seed as 0x hex",
	}
	tkmPhoneRoundFlag = &cli.Uint64Flag{
		Name:  "round",
		Usage: "bucket generation round; defaults to tkmphone_nextBucketRound",
	}
	tkmPhoneCreationTxFlag = &cli.StringFlag{
		Name:  "creation-tx",
		Usage: "canonical MainKing transaction hash for bucket creation",
	}
	tkmPhoneSignatureFlag = &cli.StringFlag{
		Name:  "signature",
		Usage: "65-byte signature as 0x hex",
	}
	tkmPhoneMainKingFlag = &cli.StringFlag{
		Name:  "mainking",
		Usage: "unlocked MainKing account used for eth_sign when --signature is omitted",
	}
	tkmPhoneOperatorFlag = &cli.StringFlag{
		Name:  "operator",
		Usage: "operator address",
	}
	tkmPhoneBucketFlag = &cli.Uint64Flag{
		Name:  "bucket",
		Usage: "phone number bucket id",
	}

	tkmPhoneRPCFlags = []cli.Flag{utils.DataDirFlag, utils.HttpHeaderFlag}

	tkmPhoneCommand = &cli.Command{
		Name:  "tkmphone",
		Usage: "Manage TKM Phone buckets and operator inventory through RPC",
		Description: `
The tkmphone commands attach to a running gtkm node. If no endpoint is supplied,
they use the local IPC endpoint from --datadir. HTTP endpoints need the tkmphone
RPC API enabled, and automatic signing needs eth_sign for an unlocked account.`,
		Subcommands: []*cli.Command{
			{
				Name:      "status",
				Usage:     "Show TKM Phone hardfork activation status",
				ArgsUsage: "[endpoint]",
				Flags:     tkmPhoneRPCFlags,
				Action:    tkmPhoneStatus,
			},
			{
				Name:      "prices",
				Usage:     "Show MainKing bucket, per-number, and operator resale prices",
				ArgsUsage: "[endpoint]",
				Flags:     tkmPhoneRPCFlags,
				Action:    tkmPhonePrices,
			},
			{
				Name:      "next-round",
				Usage:     "Show the next MainKing bucket generation round",
				ArgsUsage: "[endpoint]",
				Flags:     tkmPhoneRPCFlags,
				Action:    tkmPhoneNextRound,
			},
			{
				Name:      "bucket-hash",
				Usage:     "Calculate the MainKing bucket generation hash to sign",
				ArgsUsage: "[endpoint]",
				Flags:     append(tkmPhoneRPCFlags, tkmPhoneSeedFlag, tkmPhoneCreationTxFlag, tkmPhoneRoundFlag),
				Action:    tkmPhoneBucketHash,
			},
			{
				Name:      "sign-bucket",
				Usage:     "Sign a bucket generation hash with an unlocked MainKing account",
				ArgsUsage: "[endpoint]",
				Flags:     append(tkmPhoneRPCFlags, tkmPhoneSeedFlag, tkmPhoneCreationTxFlag, tkmPhoneRoundFlag, tkmPhoneMainKingFlag),
				Action:    tkmPhoneSignBucket,
			},
			{
				Name:      "generate-buckets",
				Usage:     "Generate the next batch of five MainKing buckets",
				ArgsUsage: "[endpoint]",
				Flags:     append(tkmPhoneRPCFlags, tkmPhoneSeedFlag, tkmPhoneCreationTxFlag, tkmPhoneRoundFlag, tkmPhoneSignatureFlag, tkmPhoneMainKingFlag),
				Action:    tkmPhoneGenerateBuckets,
			},
			{
				Name:      "buckets",
				Usage:     "List generated phone number buckets",
				ArgsUsage: "[endpoint]",
				Flags:     tkmPhoneRPCFlags,
				Action:    tkmPhoneBuckets,
			},
			{
				Name:      "open-bucket",
				Usage:     "Open an assigned bucket and reveal unsold numbers to its operator",
				ArgsUsage: "[endpoint]",
				Flags:     append(tkmPhoneRPCFlags, tkmPhoneOperatorFlag, tkmPhoneBucketFlag, tkmPhoneSignatureFlag),
				Action:    tkmPhoneOpenBucket,
			},
		},
	}
)

type tkmPhonePriceView struct {
	BucketPriceTKM         string       `json:"bucketPriceTkm"`
	MainKingNumberPriceTKM string       `json:"mainKingNumberPriceTkm"`
	OperatorSalePriceTKM   string       `json:"operatorSalePriceTkm"`
	BucketPriceWei         *hexutil.Big `json:"bucketPriceWei"`
	MainKingNumberPriceWei *hexutil.Big `json:"mainKingNumberPriceWei"`
	OperatorSalePriceWei   *hexutil.Big `json:"operatorSalePriceWei"`
}

type tkmPhoneStatusView struct {
	Active              bool           `json:"active"`
	ActivationTimestamp hexutil.Uint64 `json:"activationTimestamp"`
	HeadNumber          hexutil.Uint64 `json:"headNumber"`
	HeadTimestamp       hexutil.Uint64 `json:"headTimestamp"`
	CurrentTimestamp    hexutil.Uint64 `json:"currentTimestamp"`
	UsingChainHead      bool           `json:"usingChainHead"`
}

type tkmPhoneBucketHashView struct {
	Round      hexutil.Uint64 `json:"round"`
	Seed       common.Hash    `json:"seed"`
	CreationTx common.Hash    `json:"creationTx"`
	Hash       common.Hash    `json:"hash"`
}

type tkmPhoneSignatureView struct {
	Round      hexutil.Uint64 `json:"round"`
	Seed       common.Hash    `json:"seed"`
	CreationTx common.Hash    `json:"creationTx"`
	Hash       common.Hash    `json:"hash"`
	Signer     common.Address `json:"signer"`
	Signature  hexutil.Bytes  `json:"signature"`
}

func tkmPhoneStatus(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var status tkmPhoneStatusView
	if err := client.CallContext(context.Background(), &status, "tkmphone_status"); err != nil {
		return err
	}
	return tkmPhonePrintJSON(status)
}

func tkmPhonePrices(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var bucketPrice, mainKingPrice, salePrice hexutil.Big
	if err := client.CallContext(context.Background(), &bucketPrice, "tkmphone_bucketPrice"); err != nil {
		return err
	}
	if err := client.CallContext(context.Background(), &mainKingPrice, "tkmphone_mainKingNumberPrice"); err != nil {
		return err
	}
	if err := client.CallContext(context.Background(), &salePrice, "tkmphone_numberSalePrice"); err != nil {
		return err
	}
	return tkmPhonePrintJSON(tkmPhonePriceView{
		BucketPriceTKM:         tkmPhoneWeiToTKM((*big.Int)(&bucketPrice)),
		MainKingNumberPriceTKM: tkmPhoneWeiToTKM((*big.Int)(&mainKingPrice)),
		OperatorSalePriceTKM:   tkmPhoneWeiToTKM((*big.Int)(&salePrice)),
		BucketPriceWei:         &bucketPrice,
		MainKingNumberPriceWei: &mainKingPrice,
		OperatorSalePriceWei:   &salePrice,
	})
}

func tkmPhoneNextRound(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	round, err := tkmPhoneNextBucketRound(context.Background(), client)
	if err != nil {
		return err
	}
	return tkmPhonePrintJSON(map[string]hexutil.Uint64{"round": hexutil.Uint64(round)})
}

func tkmPhoneBucketHash(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	seed, err := tkmPhoneSeed(ctx)
	if err != nil {
		return err
	}
	creationTx, err := tkmPhoneRequiredHashFlag(ctx, tkmPhoneCreationTxFlag.Name)
	if err != nil {
		return err
	}
	round, err := tkmPhoneRound(ctx, client)
	if err != nil {
		return err
	}
	digest, err := tkmPhoneBucketGenerationHash(context.Background(), client, round, seed, creationTx)
	if err != nil {
		return err
	}
	return tkmPhonePrintJSON(tkmPhoneBucketHashView{Round: hexutil.Uint64(round), Seed: seed, CreationTx: creationTx, Hash: digest})
}

func tkmPhoneSignBucket(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	seed, err := tkmPhoneSeed(ctx)
	if err != nil {
		return err
	}
	creationTx, err := tkmPhoneRequiredHashFlag(ctx, tkmPhoneCreationTxFlag.Name)
	if err != nil {
		return err
	}
	round, err := tkmPhoneRound(ctx, client)
	if err != nil {
		return err
	}
	signer, err := tkmPhoneAddressFlag(ctx, tkmPhoneMainKingFlag.Name)
	if err != nil {
		return err
	}
	digest, err := tkmPhoneBucketGenerationHash(context.Background(), client, round, seed, creationTx)
	if err != nil {
		return err
	}
	sig, err := tkmPhoneSign(context.Background(), client, signer, digest)
	if err != nil {
		return err
	}
	return tkmPhonePrintJSON(tkmPhoneSignatureView{Round: hexutil.Uint64(round), Seed: seed, CreationTx: creationTx, Hash: digest, Signer: signer, Signature: sig})
}

func tkmPhoneGenerateBuckets(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	seed, err := tkmPhoneSeed(ctx)
	if err != nil {
		return err
	}
	creationTx, err := tkmPhoneRequiredHashFlag(ctx, tkmPhoneCreationTxFlag.Name)
	if err != nil {
		return err
	}
	sig, err := tkmPhoneSignature(ctx)
	if err != nil {
		return err
	}
	if len(sig) == 0 {
		round, err := tkmPhoneRound(ctx, client)
		if err != nil {
			return err
		}
		signer, err := tkmPhoneAddressFlag(ctx, tkmPhoneMainKingFlag.Name)
		if err != nil {
			return fmt.Errorf("either --signature or --mainking is required: %w", err)
		}
		digest, err := tkmPhoneBucketGenerationHash(context.Background(), client, round, seed, creationTx)
		if err != nil {
			return err
		}
		sig, err = tkmPhoneSign(context.Background(), client, signer, digest)
		if err != nil {
			return err
		}
	}
	var buckets json.RawMessage
	if err := client.CallContext(context.Background(), &buckets, "tkmphone_generateBuckets", seed, creationTx, sig); err != nil {
		return err
	}
	fmt.Println(string(buckets))
	return nil
}

func tkmPhoneBuckets(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var buckets json.RawMessage
	if err := client.CallContext(context.Background(), &buckets, "tkmphone_buckets"); err != nil {
		return err
	}
	fmt.Println(string(buckets))
	return nil
}

func tkmPhoneOpenBucket(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	operator, err := tkmPhoneAddressFlag(ctx, tkmPhoneOperatorFlag.Name)
	if err != nil {
		return err
	}
	bucketID := ctx.Uint64(tkmPhoneBucketFlag.Name)
	if bucketID == 0 {
		return errors.New("--bucket is required")
	}
	sig, err := tkmPhoneSignature(ctx)
	if err != nil {
		return err
	}
	if len(sig) == 0 {
		var digest common.Hash
		if err := client.CallContext(context.Background(), &digest, "tkmphone_openBucketHash", operator, hexutil.Uint64(bucketID)); err != nil {
			return err
		}
		sig, err = tkmPhoneSign(context.Background(), client, operator, digest)
		if err != nil {
			return err
		}
	}
	var numbers json.RawMessage
	if err := client.CallContext(context.Background(), &numbers, "tkmphone_openBucket", operator, hexutil.Uint64(bucketID), sig); err != nil {
		return err
	}
	fmt.Println(string(numbers))
	return nil
}

func tkmPhoneDial(ctx *cli.Context) (*rpc.Client, error) {
	if ctx.Args().Len() > 1 {
		return nil, errors.New("too many arguments")
	}
	endpoint := ctx.Args().First()
	if endpoint == "" {
		cfg := defaultNodeConfig()
		utils.SetDataDir(ctx, &cfg)
		endpoint = cfg.IPCEndpoint()
	}
	client, err := utils.DialRPCWithHeaders(endpoint, ctx.StringSlice(utils.HttpHeaderFlag.Name))
	if err != nil {
		return nil, fmt.Errorf("unable to attach to gtkm RPC: %w", err)
	}
	return client, nil
}

func tkmPhoneSeed(ctx *cli.Context) (common.Hash, error) {
	value := ctx.String(tkmPhoneSeedFlag.Name)
	if value == "" {
		return common.Hash{}, errors.New("--seed is required")
	}
	seed, err := hexutil.Decode(value)
	if err != nil || len(seed) != common.HashLength {
		return common.Hash{}, errors.New("--seed must be a 32-byte 0x hex value")
	}
	return common.BytesToHash(seed), nil
}

func tkmPhoneRequiredHashFlag(ctx *cli.Context, name string) (common.Hash, error) {
	value := ctx.String(name)
	if value == "" {
		return common.Hash{}, fmt.Errorf("--%s is required", name)
	}
	decoded, err := hexutil.Decode(value)
	if err != nil || len(decoded) != common.HashLength {
		return common.Hash{}, fmt.Errorf("--%s must be a 32-byte 0x hex value", name)
	}
	hash := common.BytesToHash(decoded)
	if hash == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("--%s cannot be zero", name)
	}
	return hash, nil
}

func tkmPhoneRound(ctx *cli.Context, client *rpc.Client) (uint64, error) {
	if ctx.IsSet(tkmPhoneRoundFlag.Name) {
		round := ctx.Uint64(tkmPhoneRoundFlag.Name)
		if round == 0 {
			return 0, errors.New("--round must be greater than zero")
		}
		return round, nil
	}
	return tkmPhoneNextBucketRound(context.Background(), client)
}

func tkmPhoneNextBucketRound(ctx context.Context, client *rpc.Client) (uint64, error) {
	var round hexutil.Uint64
	if err := client.CallContext(ctx, &round, "tkmphone_nextBucketRound"); err != nil {
		return 0, err
	}
	return uint64(round), nil
}

func tkmPhoneBucketGenerationHash(ctx context.Context, client *rpc.Client, round uint64, seed common.Hash, creationTx common.Hash) (common.Hash, error) {
	var digest common.Hash
	if err := client.CallContext(ctx, &digest, "tkmphone_bucketGenerationHash", hexutil.Uint64(round), seed, creationTx); err != nil {
		return common.Hash{}, err
	}
	return digest, nil
}

func tkmPhoneSign(ctx context.Context, client *rpc.Client, signer common.Address, digest common.Hash) (hexutil.Bytes, error) {
	var sig hexutil.Bytes
	if err := client.CallContext(ctx, &sig, "eth_sign", signer, hexutil.Bytes(digest.Bytes())); err != nil {
		return nil, fmt.Errorf("eth_sign failed for %s: %w", signer.Hex(), err)
	}
	return sig, nil
}

func tkmPhoneAddressFlag(ctx *cli.Context, name string) (common.Address, error) {
	value := ctx.String(name)
	if value == "" {
		return common.Address{}, fmt.Errorf("--%s is required", name)
	}
	if !common.IsHexAddress(value) {
		return common.Address{}, fmt.Errorf("--%s must be an address", name)
	}
	return common.HexToAddress(value), nil
}

func tkmPhoneSignature(ctx *cli.Context) (hexutil.Bytes, error) {
	value := ctx.String(tkmPhoneSignatureFlag.Name)
	if value == "" {
		return nil, nil
	}
	sig, err := hexutil.Decode(value)
	if err != nil {
		return nil, err
	}
	return hexutil.Bytes(sig), nil
}

func tkmPhoneWeiToTKM(value *big.Int) string {
	unit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole, frac := new(big.Int).QuoRem(new(big.Int).Set(value), unit, new(big.Int))
	if frac.Sign() == 0 {
		return whole.String()
	}
	return fmt.Sprintf("%s.%018s", whole.String(), frac.String())
}

func tkmPhonePrintJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
