package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/urfave/cli/v2"
)

var (
	govKindFlag = &cli.StringFlag{
		Name:  "kind",
		Usage: "governance record kind, for example checkpoint, rotating-king-selection, roadmap, fund-commitment, hardfork",
	}
	govTitleFlag = &cli.StringFlag{
		Name:  "title",
		Usage: "human-readable governance record title",
	}
	govVersionFlag = &cli.Uint64Flag{
		Name:  "version",
		Usage: "governance record version",
		Value: 1,
	}
	govFileFlag = &cli.StringFlag{
		Name:  "file",
		Usage: "document file to hash and publish",
	}
	govURIFlag = &cli.StringFlag{
		Name:  "uri",
		Usage: "public URI or repository path for the full document; defaults to --file",
	}
	govPreviousHashFlag = &cli.StringFlag{
		Name:  "previous-hash",
		Usage: "previous governance disclosure hash for append-only updates",
	}
	govTimestampFlag = &cli.Uint64Flag{
		Name:  "timestamp",
		Usage: "record timestamp as Unix seconds; defaults to current time for hash/sign/publish",
	}
	govSignatureFlag = &cli.StringFlag{
		Name:  "signature",
		Usage: "65-byte Main King signature as 0x hex",
	}
	govMainKingFlag = &cli.StringFlag{
		Name:  "mainking",
		Usage: "unlocked Main King account used for eth_sign when --signature is omitted",
	}
	govAnchorTxFlag = &cli.StringFlag{
		Name:  "anchor-tx",
		Usage: "optional canonical Main King transaction hash anchoring the disclosure hash",
	}
	govHashFlag = &cli.StringFlag{
		Name:  "hash",
		Usage: "governance disclosure hash to sign",
	}
	govIDFlag = &cli.Uint64Flag{
		Name:  "id",
		Usage: "governance disclosure id",
	}
	govLimitFlag = &cli.Uint64Flag{
		Name:  "limit",
		Usage: "maximum records to list",
		Value: 100,
	}
	govVoteAddressFlag = &cli.StringFlag{
		Name:     "address",
		Usage:    "address to vote on or unvote",
		Required: true,
	}
	govVoteReasonFlag = &cli.StringFlag{
		Name:  "reason",
		Usage: "public reason for a vote (required for vote, omitted for unvote)",
	}
	govVoteFromFlag = &cli.StringFlag{
		Name:     "from",
		Usage:    "stamped ML-DSA-87 account that casts the vote",
		Required: true,
	}
	govVoteGasFlag = &cli.Uint64Flag{
		Name:  "gas",
		Usage: "gas limit for the vote transaction",
		Value: 100000,
	}

	governanceRPCFlags = []cli.Flag{utils.DataDirFlag, utils.HttpHeaderFlag}

	governanceCommand = &cli.Command{
		Name:  "governance",
		Usage: "Publish and verify Main King signed governance disclosure records",
		Description: `
The governance commands attach to a running gtkm node. Records are non-consensus
public disclosures: the full document stays in a public repository or URI, while
its hash, Main King signature, optional anchor transaction, and append-only link
are stored through the tkmgov RPC namespace.`,
		Subcommands: []*cli.Command{
			{
				Name:      "hash",
				Usage:     "Calculate a governance disclosure hash for a document",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govKindFlag, govTitleFlag, govVersionFlag, govFileFlag, govURIFlag, govPreviousHashFlag, govTimestampFlag),
				Action:    governanceHash,
			},
			{
				Name:      "sign",
				Usage:     "Sign a governance disclosure hash with an unlocked Main King account",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govHashFlag, govMainKingFlag, govKindFlag, govTitleFlag, govVersionFlag, govFileFlag, govURIFlag, govPreviousHashFlag, govTimestampFlag),
				Action:    governanceSign,
			},
			{
				Name:      "publish",
				Usage:     "Publish a Main King signed governance disclosure record",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govKindFlag, govTitleFlag, govVersionFlag, govFileFlag, govURIFlag, govPreviousHashFlag, govTimestampFlag, govAnchorTxFlag, govSignatureFlag, govMainKingFlag),
				Action:    governancePublish,
			},
			{
				Name:      "list",
				Usage:     "List governance disclosures",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govKindFlag, govLimitFlag),
				Action:    governanceList,
			},
			{
				Name:      "verify",
				Usage:     "Verify a stored governance disclosure signature and anchor",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govIDFlag),
				Action:    governanceVerify,
			},
			{
				Name:      "vote",
				Usage:     "cast a public, stamped-owner address suspension vote (burns 50 TKM)",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govVoteAddressFlag, govVoteReasonFlag, govVoteFromFlag, govVoteGasFlag),
				Action:    governanceVote,
			},
			{
				Name:      "unvote",
				Usage:     "remove your own address suspension vote (burns 50 TKM)",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govVoteAddressFlag, govVoteFromFlag, govVoteGasFlag),
				Action:    governanceUnvote,
			},
			{
				Name:      "status",
				Usage:     "show the canonical address-vote count and suspension status",
				ArgsUsage: "[endpoint]",
				Flags:     append(governanceRPCFlags, govVoteAddressFlag),
				Action:    governanceVoteStatus,
			},
		},
	}
)

type governanceHashView struct {
	Kind           string         `json:"kind"`
	Title          string         `json:"title"`
	Version        hexutil.Uint64 `json:"version"`
	File           string         `json:"file"`
	URI            string         `json:"uri"`
	ContentHash    common.Hash    `json:"contentHash"`
	PreviousHash   common.Hash    `json:"previousHash"`
	Timestamp      hexutil.Uint64 `json:"timestamp"`
	DisclosureHash common.Hash    `json:"disclosureHash"`
}

type governanceSignatureView struct {
	Hash      common.Hash    `json:"hash"`
	Signer    common.Address `json:"signer"`
	Signature hexutil.Bytes  `json:"signature"`
}

func governanceHash(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	view, err := governanceBuildHashView(ctx, client)
	if err != nil {
		return err
	}
	return tkmPhonePrintJSON(view)
}

func governanceSign(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	signer, err := tkmPhoneAddressFlag(ctx, govMainKingFlag.Name)
	if err != nil {
		return err
	}
	digest, err := governanceDigest(ctx, client)
	if err != nil {
		return err
	}
	sig, err := tkmPhoneSign(context.Background(), client, signer, digest)
	if err != nil {
		return err
	}
	return tkmPhonePrintJSON(governanceSignatureView{Hash: digest, Signer: signer, Signature: sig})
}

func governancePublish(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	view, err := governanceBuildHashView(ctx, client)
	if err != nil {
		return err
	}
	sig, err := governanceSignature(ctx)
	if err != nil {
		return err
	}
	if len(sig) == 0 {
		mainKing, err := tkmPhoneAddressFlag(ctx, govMainKingFlag.Name)
		if err != nil {
			return errors.New("--signature or --mainking is required")
		}
		sig, err = tkmPhoneSign(context.Background(), client, mainKing, view.DisclosureHash)
		if err != nil {
			return err
		}
	}
	anchorTx, err := governanceOptionalHash(ctx.String(govAnchorTxFlag.Name))
	if err != nil {
		return err
	}
	var out json.RawMessage
	if err := client.CallContext(context.Background(), &out, "tkmgov_publishDisclosure", view.Kind, view.Title, view.Version, view.ContentHash, view.URI, view.PreviousHash, view.Timestamp, anchorTx, sig); err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func governanceList(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var out json.RawMessage
	if err := client.CallContext(context.Background(), &out, "tkmgov_listDisclosures", ctx.String(govKindFlag.Name), hexutil.Uint64(0), hexutil.Uint64(ctx.Uint64(govLimitFlag.Name))); err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func governanceVerify(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	id := ctx.Uint64(govIDFlag.Name)
	if id == 0 {
		return errors.New("--id is required")
	}
	var ok bool
	if err := client.CallContext(context.Background(), &ok, "tkmgov_verifyDisclosure", hexutil.Uint64(id)); err != nil {
		return err
	}
	return tkmPhonePrintJSON(map[string]any{"id": id, "valid": ok})
}

func governanceVote(ctx *cli.Context) error {
	return governanceSubmitVote(ctx, false)
}

func governanceUnvote(ctx *cli.Context) error {
	if ctx.String(govVoteReasonFlag.Name) != "" {
		return errors.New("--reason is only valid with governance vote")
	}
	return governanceSubmitVote(ctx, true)
}

func governanceSubmitVote(ctx *cli.Context, unvote bool) error {
	target := common.HexToAddress(ctx.String(govVoteAddressFlag.Name))
	if !common.IsHexAddress(ctx.String(govVoteAddressFlag.Name)) || target == (common.Address{}) || target == params.ShieldedPoolAddress {
		return errors.New("--address must be a non-zero, non-reserved 20-byte address")
	}
	reason := ctx.String(govVoteReasonFlag.Name)
	if !unvote && reason == "" {
		return errors.New("--reason is required for a vote")
	}
	data, err := core.EncodeAddressVote(&core.AddressVote{Version: core.AddressVoteVersion, Unvote: unvote, Target: target, Reason: reason})
	if err != nil {
		return err
	}
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	from := common.HexToAddress(ctx.String(govVoteFromFlag.Name))
	if !common.IsHexAddress(ctx.String(govVoteFromFlag.Name)) || from == (common.Address{}) {
		return errors.New("--from must be a non-zero 20-byte address")
	}
	passphrase := utils.GetPassPhrase("Account password", false)
	defer clearWalletBytes([]byte(passphrase))
	txArgs := map[string]interface{}{
		"from":  from,
		"to":    params.ShieldedPoolAddress,
		"type":  hexutil.Uint64(types.PQTkmTxType),
		"value": hexutil.Uint64(0),
		"gas":   hexutil.Uint64(ctx.Uint64(govVoteGasFlag.Name)),
		"data":  hexutil.Bytes(data),
	}
	var hash common.Hash
	if err := client.CallContext(context.Background(), &hash, "tkm_sendTransactionWithPassphrase", txArgs, passphrase); err != nil {
		return err
	}
	return tkmPhonePrintJSON(map[string]interface{}{"hash": hash, "target": target, "unvote": unvote, "reason": reason})
}

func governanceVoteStatus(ctx *cli.Context) error {
	address := common.HexToAddress(ctx.String(govVoteAddressFlag.Name))
	if !common.IsHexAddress(ctx.String(govVoteAddressFlag.Name)) || address == (common.Address{}) {
		return errors.New("--address must be a non-zero 20-byte address")
	}
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var status json.RawMessage
	if err := client.CallContext(context.Background(), &status, "tkmgov_getAddressVoteStatus", address); err != nil {
		return err
	}
	return tkmPhonePrintJSON(status)
}

func governanceBuildHashView(ctx *cli.Context, client interface {
	CallContext(context.Context, interface{}, string, ...interface{}) error
}) (governanceHashView, error) {
	kind := ctx.String(govKindFlag.Name)
	if kind == "" {
		return governanceHashView{}, errors.New("--kind is required")
	}
	title := ctx.String(govTitleFlag.Name)
	if title == "" {
		return governanceHashView{}, errors.New("--title is required")
	}
	version := ctx.Uint64(govVersionFlag.Name)
	if version == 0 {
		return governanceHashView{}, errors.New("--version must be greater than zero")
	}
	file := ctx.String(govFileFlag.Name)
	if file == "" {
		return governanceHashView{}, errors.New("--file is required")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return governanceHashView{}, err
	}
	uri := ctx.String(govURIFlag.Name)
	if uri == "" {
		uri = file
	}
	previousHash, err := governanceOptionalHash(ctx.String(govPreviousHashFlag.Name))
	if err != nil {
		return governanceHashView{}, err
	}
	timestamp := ctx.Uint64(govTimestampFlag.Name)
	if timestamp == 0 {
		timestamp = uint64(time.Now().Unix())
	}
	contentHash := crypto.Keccak256Hash(body)
	var digest common.Hash
	if err := client.CallContext(context.Background(), &digest, "tkmgov_disclosureHash", kind, title, hexutil.Uint64(version), contentHash, uri, previousHash, hexutil.Uint64(timestamp)); err != nil {
		return governanceHashView{}, err
	}
	return governanceHashView{Kind: kind, Title: title, Version: hexutil.Uint64(version), File: file, URI: uri, ContentHash: contentHash, PreviousHash: previousHash, Timestamp: hexutil.Uint64(timestamp), DisclosureHash: digest}, nil
}

func governanceDigest(ctx *cli.Context, client interface {
	CallContext(context.Context, interface{}, string, ...interface{}) error
}) (common.Hash, error) {
	if value := ctx.String(govHashFlag.Name); value != "" {
		return governanceRequiredHash(value, "--hash")
	}
	view, err := governanceBuildHashView(ctx, client)
	if err != nil {
		return common.Hash{}, err
	}
	return view.DisclosureHash, nil
}

func governanceSignature(ctx *cli.Context) (hexutil.Bytes, error) {
	value := ctx.String(govSignatureFlag.Name)
	if value == "" {
		return nil, nil
	}
	sig, err := hexutil.Decode(value)
	if err != nil {
		return nil, err
	}
	return hexutil.Bytes(sig), nil
}

func governanceOptionalHash(value string) (common.Hash, error) {
	if value == "" {
		return common.Hash{}, nil
	}
	return governanceRequiredHash(value, "hash")
}

func governanceRequiredHash(value string, label string) (common.Hash, error) {
	decoded, err := hexutil.Decode(value)
	if err != nil || len(decoded) != common.HashLength {
		return common.Hash{}, fmt.Errorf("%s must be a 32-byte 0x hex value", label)
	}
	return common.BytesToHash(decoded), nil
}
