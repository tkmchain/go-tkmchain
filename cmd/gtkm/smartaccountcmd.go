package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"
)

var (
	smartAccountMethodFlag = &cli.StringFlag{Name: "method", Usage: "tkmaccount RPC method suffix, for example createData", Required: true}
	smartAccountParamsFlag = &cli.StringFlag{Name: "params", Usage: "JSON array of RPC parameters", Value: "[]"}
	smartAccountRPCFlags   = []cli.Flag{utils.DataDirFlag, utils.HttpHeaderFlag}

	smartAccountCommand = &cli.Command{
		Name: "smartaccount", Usage: "Build and inspect non-consensus TKM smart-account operations", ArgsUsage: "[endpoint]",
		Description: `The smartaccount command calls the non-consensus tkmaccount helper RPC.
It never unlocks keys or changes consensus. Use IPC by default, or provide an HTTP endpoint.`,
		Flags: smartAccountRPCFlags, Action: smartAccountStatus,
		Subcommands: []*cli.Command{
			{Name: "status", Usage: "Show smart-account version and deployment status", ArgsUsage: "[endpoint]", Flags: smartAccountRPCFlags, Action: smartAccountStatus},
			{Name: "call", Usage: "Call an allowlisted tkmaccount builder or hash helper", ArgsUsage: "[endpoint]", Flags: append(smartAccountRPCFlags, smartAccountMethodFlag, smartAccountParamsFlag), Action: smartAccountCall},
		},
	}
)

type smartAccountCLIStatus struct {
	Version          string           `json:"version"`
	Consensus        bool             `json:"consensus"`
	RequiresHardfork bool             `json:"requiresHardfork"`
	Deployed         bool             `json:"deployed"`
	EntryPoint       common.Address   `json:"entryPoint"`
	Factory          common.Address   `json:"factory"`
	Paymaster        common.Address   `json:"paymaster"`
	AuthModes        map[string]uint8 `json:"authorizationModes"`
}

func smartAccountStatus(ctx *cli.Context) error {
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	var out smartAccountCLIStatus
	if err := client.CallContext(context.Background(), &out, "tkmaccount_status"); err != nil {
		return err
	}
	return tkmPhonePrintJSON(out)
}

var smartAccountAllowedMethods = map[string]bool{
	"operationHash": true, "createData": true, "executeData": true, "setOwnersData": true,
	"setLimitsData": true, "setGuardianData": true, "setRecoveryPolicyData": true,
	"recoveryHash": true, "approveRecoveryData": true, "cancelRecoveryData": true,
	"completeRecoveryData": true, "setSessionData": true, "revokeSessionData": true,
	"ownerAuthorization": true, "sessionAuthorization": true, "sponsorshipHash": true, "sponsorshipData": true,
	"predictAddress": true,
}

func smartAccountCall(ctx *cli.Context) error {
	method := strings.TrimSpace(ctx.String(smartAccountMethodFlag.Name))
	if !smartAccountAllowedMethods[method] {
		return fmt.Errorf("unsupported tkmaccount method %q", method)
	}
	var params []json.RawMessage
	if err := json.Unmarshal([]byte(ctx.String(smartAccountParamsFlag.Name)), &params); err != nil {
		return fmt.Errorf("invalid --params JSON: %w", err)
	}
	client, err := tkmPhoneDial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	args := make([]interface{}, len(params))
	for i := range params {
		args[i] = params[i]
	}
	var out json.RawMessage
	if err := client.CallContext(context.Background(), &out, "tkmaccount_"+method, args...); err != nil {
		return err
	}
	if len(out) == 0 {
		return errors.New("empty RPC response")
	}
	var printable interface{}
	if err := json.Unmarshal(out, &printable); err != nil {
		return err
	}
	return tkmPhonePrintJSON(printable)
}
