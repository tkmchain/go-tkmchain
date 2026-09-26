package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestPrivateExecutionPolicyKeepsTransparentCompatibilityBeforeAntartical(t *testing.T) {
	to := common.HexToAddress("0x1234")
	tx := types.NewTx(&types.LegacyTx{Nonce: 1, To: &to, Value: big.NewInt(1), Gas: 21_000, GasPrice: big.NewInt(1)})
	if err := ValidatePrivateExecutionPolicy(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime-1, tx); err != nil {
		t.Fatalf("transparent transaction rejected before Antartical: %v", err)
	}
}

func TestPrivateExecutionPolicyRejectsTransparentEVMAndTVMAfterAntartical(t *testing.T) {
	to := common.HexToAddress("0x1234")
	for name, tx := range map[string]*types.Transaction{
		"legacy EVM": types.NewTx(&types.LegacyTx{Nonce: 1, To: &to, Value: big.NewInt(1), Gas: 21_000, GasPrice: big.NewInt(1)}),
		"PQ EVM":     types.NewTx(&types.PQTkmTx{ChainID: params.MainnetChainConfig.ChainID, Nonce: 1, To: &to, Value: new(big.Int), Gas: 100_000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1), Algorithm: "ML-DSA-87", PublicKey: []byte{1}, Signature: []byte{2}}),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePrivateExecutionPolicy(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); !errors.Is(err, ErrPublicExecutionDisabled) {
				t.Fatalf("transparent transaction error = %v, want %v", err, ErrPublicExecutionDisabled)
			}
		})
	}
}

func TestPrivateExecutionPolicyAllowsOnlyShieldedEnvelopes(t *testing.T) {
	for name, data := range map[string][]byte{
		"Shield3": []byte(ShieldedV3Magic),
		"Shield4": []byte(ShieldedV4Magic),
	} {
		t.Run(name, func(t *testing.T) {
			tx := types.NewTx(&types.PQTkmTx{ChainID: params.MainnetChainConfig.ChainID, Nonce: 1, To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: 100_000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1), Data: data})
			if err := ValidatePrivateExecutionPolicy(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); err != nil {
				t.Fatalf("private envelope rejected: %v", err)
			}
		})
	}
}

func TestPrivateExecutionPolicyRejectsUnwrappedProtocolEnvelopes(t *testing.T) {
	for name, data := range map[string][]byte{
		"stamp":       []byte(AntarticalStampMagic),
		"private TVM": []byte(PrivateTVMMagic),
	} {
		t.Run(name, func(t *testing.T) {
			tx := types.NewTx(&types.PQTkmTx{ChainID: params.MainnetChainConfig.ChainID, Nonce: 1, To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: 100_000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1), Data: data})
			if err := ValidatePrivateExecutionPolicy(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); err == nil {
				t.Fatalf("malformed %s accepted", name)
			}
		})
	}
}
