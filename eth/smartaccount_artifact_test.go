package eth

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

func TestSmartAccountCompiledArtifacts(t *testing.T) {
	required := map[string][]string{
		"TKMEntryPoint":               {"operationHash", "handleOperation", "nonces"},
		"TKMAccount":                  {"validateUserOperation", "executeFromEntryPoint", "setOwners", "setLimits", "setGuardian", "setRecoveryPolicy", "approveRecovery", "cancelRecovery", "completeRecovery", "setSession", "revokeSession"},
		"TKMAccountFactory":           {"createAccount", "predictAccount"},
		"TKMAllowlistPaymaster":       {"validateSponsorship", "setAllowed", "setSigner", "setPaused"},
		"TKMPoolTreasurySmartAccount": {"INITIAL_POOL_OWNER", "ACCOUNT_PURPOSE", "validateUserOperation", "executeFromEntryPoint"},
	}
	for contract, methods := range required {
		abiBytes, err := os.ReadFile("../contracts/smartaccount/artifacts/" + contract + ".abi")
		if err != nil {
			t.Fatalf("%s ABI: %v", contract, err)
		}
		parsed, err := abi.JSON(strings.NewReader(string(abiBytes)))
		if err != nil {
			t.Fatalf("%s ABI invalid: %v", contract, err)
		}
		for _, method := range methods {
			if _, ok := parsed.Methods[method]; !ok {
				t.Errorf("%s missing %s", contract, method)
			}
		}
		binBytes, err := os.ReadFile("../contracts/smartaccount/artifacts/" + contract + ".bin")
		if err != nil {
			t.Fatalf("%s bytecode: %v", contract, err)
		}
		code, err := hex.DecodeString(strings.TrimSpace(string(binBytes)))
		if err != nil || len(code) < 32 {
			t.Fatalf("%s bytecode invalid: length=%d err=%v", contract, len(code), err)
		}
	}
}

func TestSmartAccountContractSecurityGuardsPresent(t *testing.T) {
	source, err := os.ReadFile("../contracts/smartaccount/TKMSmartAccounts.sol")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	required := []string{"block.chainid", "InvalidNonce", "Reentrant", "onlyEntryPoint", "recoveryDelay", "recoveryApproved", "remainingValue", "dailyLimit", "allowed[op.target][selector]", "expiry > block.timestamp + 1 days"}
	for _, guard := range required {
		if !strings.Contains(text, guard) {
			t.Errorf("missing security guard %q", guard)
		}
	}
	forbidden := []string{"tx.origin", "delegatecall", "selfdestruct"}
	for _, item := range forbidden {
		if strings.Contains(strings.ToLower(text), item) {
			t.Errorf("forbidden primitive %q present", item)
		}
	}
}
