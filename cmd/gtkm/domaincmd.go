package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli/v2"
)

var (
	domainRPCFlag    = &cli.StringFlag{Name: "rpc", Usage: "RPC endpoint; defaults to local IPC"}
	domainPayoutFlag = &cli.StringFlag{Name: "payout", Usage: "subscriber revenue payout address (defaults to the PQ transaction signer)"}
	domainFlags      = []cli.Flag{utils.DataDirFlag, utils.HttpHeaderFlag, domainRPCFlag}
	domainCommand    = &cli.Command{
		Name:  "domain",
		Usage: "Prepare and inspect shielded TKM domain operations",
		Description: `Domain writes are paid from shielded balance. Commands return a
withdrawal/application-data plan which the PQ wallet signs locally. No wallet
password, private key, shielded note, or proof is sent to the RPC server.`,
		Subcommands: []*cli.Command{
			{Name: "status", Usage: "Show EmailVM/domain index status", Flags: domainFlags, Action: domainStatus},
			{Name: "claim-tkm", Usage: "Prepare the first canonical @tkm super-address claim", Flags: domainFlags, Action: domainClaimSuper},
			{Name: "quote", Usage: "Quote a custom domain and subscriber capacity", ArgsUsage: "<total-units>", Flags: domainFlags, Action: domainQuote},
			{Name: "operator", Usage: "Prepare custom domain operator registration", ArgsUsage: "<total-units> <amount-tkm> <domain>", Flags: append(domainFlags, domainPayoutFlag), Action: domainOperator},
			{Name: "set-payout", Usage: "Prepare a signed operator payout-address change", ArgsUsage: "<domain> <payout-address>", Flags: domainFlags, Action: domainSetPayout},
			{Name: "buy", Usage: "Prepare a mailbox purchase", ArgsUsage: "<username> <operator-domain>", Flags: domainFlags, Action: domainBuy},
			{Name: "expand", Usage: "Prepare an operator capacity expansion", ArgsUsage: "<additional-units> <amount-tkm> <domain>", Flags: domainFlags, Action: domainExpand},
			{Name: "list", Usage: "List registered operator domains", Flags: domainFlags, Action: domainList},
			{Name: "hash", Usage: "Calculate a canonical domain registry hash", ArgsUsage: "<domain>", Flags: domainFlags, Action: domainHash},
			{Name: "mailbox-hash", Usage: "Calculate a canonical mailbox registry hash", ArgsUsage: "<username> <domain>", Flags: domainFlags, Action: domainMailboxHash},
			{Name: "registration", Usage: "Resolve a permanent name registration by hash", ArgsUsage: "<registry-hash>", Flags: domainFlags, Action: domainRegistration},
			{Name: "mailbox", Usage: "Inspect one mailbox", ArgsUsage: "<username@domain>", Flags: domainFlags, Action: domainMailbox},
			{Name: "mailboxes", Usage: "List mailboxes, optionally under one domain", ArgsUsage: "[domain]", Flags: domainFlags, Action: domainMailboxes},
			{Name: "pending", Usage: "List partially confirmed domain and mailbox payments", Flags: domainFlags, Action: domainPending},
		},
	}
	emailVMCommand = &cli.Command{
		Name:  "emailvm",
		Usage: "Inspect canonical encrypted EmailVM messages",
		Subcommands: []*cli.Command{
			{Name: "status", Usage: "Show EmailVM index status", Flags: domainFlags, Action: emailVMStatus},
			{Name: "key", Usage: "Show a mailbox's canonical X25519 public key", ArgsUsage: "<username@domain>", Flags: domainFlags, Action: emailVMKey},
			{Name: "inbox", Usage: "List canonical encrypted inbox messages", ArgsUsage: "<username@domain>", Flags: domainFlags, Action: emailVMInbox},
			{Name: "outbox", Usage: "List canonical encrypted outbox messages", ArgsUsage: "<username@domain>", Flags: domainFlags, Action: emailVMOutbox},
		},
	}
)

func domainStatus(ctx *cli.Context) error     { return domainRPCPrint(ctx, "tkmdomain_status") }
func domainClaimSuper(ctx *cli.Context) error { return domainRPCPrint(ctx, "tkmdomain_claimSuper") }
func domainList(ctx *cli.Context) error       { return domainRPCPrint(ctx, "tkmdomain_domains") }
func domainPending(ctx *cli.Context) error    { return domainRPCPrint(ctx, "tkmdomain_pending") }
func emailVMStatus(ctx *cli.Context) error    { return domainRPCPrint(ctx, "emailvm_status") }

func emailVMKey(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return errors.New("usage: gtkm emailvm key <username@domain>")
	}
	return domainRPCPrint(ctx, "emailvm_key", ctx.Args().First())
}

func domainQuote(ctx *cli.Context) error {
	units, err := domainUnitsArg(ctx, 1)
	if err != nil {
		return err
	}
	return domainRPCPrint(ctx, "tkmdomain_quote", hexutil.Uint64(units))
}

func domainOperator(ctx *cli.Context) error {
	if ctx.Args().Len() != 3 {
		return errors.New("usage: gtkm domain operator <total-units> <amount-tkm> <domain>")
	}
	units, err := domainUnitsArg(ctx, 3)
	if err != nil {
		return err
	}
	if payout := ctx.String(domainPayoutFlag.Name); payout != "" {
		return domainRPCPrint(ctx, "tkmdomain_operatorWithPayout", hexutil.Uint64(units), ctx.Args().Get(1), ctx.Args().Get(2), payout)
	}
	return domainRPCPrint(ctx, "tkmdomain_operator", hexutil.Uint64(units), ctx.Args().Get(1), ctx.Args().Get(2))
}

func domainSetPayout(ctx *cli.Context) error {
	if ctx.Args().Len() != 2 {
		return errors.New("usage: gtkm domain set-payout <domain> <payout-address>")
	}
	return domainRPCPrint(ctx, "tkmdomain_setPayout", ctx.Args().Get(0), ctx.Args().Get(1))
}

func domainBuy(ctx *cli.Context) error {
	if ctx.Args().Len() != 2 {
		return errors.New("usage: gtkm domain buy <username> <operator-domain>")
	}
	return domainRPCPrint(ctx, "tkmdomain_buy", ctx.Args().Get(0), ctx.Args().Get(1))
}

func domainExpand(ctx *cli.Context) error {
	if ctx.Args().Len() != 3 {
		return errors.New("usage: gtkm domain expand <additional-units> <amount-tkm> <domain>")
	}
	units, err := domainUnitsArg(ctx, 3)
	if err != nil {
		return err
	}
	return domainRPCPrint(ctx, "tkmdomain_expand", ctx.Args().Get(2), hexutil.Uint64(units), ctx.Args().Get(1))
}

func domainMailbox(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return errors.New("usage: gtkm domain mailbox <username@domain>")
	}
	return domainRPCPrint(ctx, "tkmdomain_mailbox", ctx.Args().First())
}

func domainHash(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return errors.New("usage: gtkm domain hash <domain>")
	}
	return domainRPCPrint(ctx, "tkmdomain_domainHash", ctx.Args().First())
}

func domainMailboxHash(ctx *cli.Context) error {
	if ctx.Args().Len() != 2 {
		return errors.New("usage: gtkm domain mailbox-hash <username> <domain>")
	}
	return domainRPCPrint(ctx, "tkmdomain_mailboxHash", ctx.Args().Get(0), ctx.Args().Get(1))
}

func domainRegistration(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return errors.New("usage: gtkm domain registration <registry-hash>")
	}
	return domainRPCPrint(ctx, "tkmdomain_registration", ctx.Args().First())
}

func domainMailboxes(ctx *cli.Context) error {
	if ctx.Args().Len() > 1 {
		return errors.New("usage: gtkm domain mailboxes [domain]")
	}
	return domainRPCPrint(ctx, "tkmdomain_mailboxes", ctx.Args().First())
}

func emailVMInbox(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return errors.New("usage: gtkm emailvm inbox <username@domain>")
	}
	return domainRPCPrint(ctx, "emailvm_inbox", ctx.Args().First())
}

func emailVMOutbox(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return errors.New("usage: gtkm emailvm outbox <username@domain>")
	}
	return domainRPCPrint(ctx, "emailvm_outbox", ctx.Args().First())
}

func domainUnitsArg(ctx *cli.Context, exactArgs int) (uint64, error) {
	if ctx.Args().Len() != exactArgs {
		return 0, errors.New("subscriber unit count is required")
	}
	units, err := strconv.ParseUint(ctx.Args().First(), 10, 64)
	if err != nil || units == 0 {
		return 0, errors.New("subscriber unit count must be a positive integer")
	}
	return units, nil
}

func domainRPCPrint(ctx *cli.Context, method string, args ...any) error {
	client, err := domainDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var out json.RawMessage
	if err := client.CallContext(context.Background(), &out, method, args...); err != nil {
		return err
	}
	var formatted any
	if json.Unmarshal(out, &formatted) == nil {
		return tkmPhonePrintJSON(formatted)
	}
	fmt.Println(string(out))
	return nil
}

func domainDial(ctx *cli.Context) (*rpc.Client, error) {
	endpoint := ctx.String(domainRPCFlag.Name)
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
