package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// These tests deliberately omit a proof: policy acceptance is only the first
// validation stage and must never replace the native proof verifier.
func privatePolicyTx(t *testing.T, version int, deposit bool, withdrawal, sponsor int64) *types.Transaction {
	t.Helper()
	var data []byte
	var err error
	if version == 3 {
		e := &ShieldedV3Transaction{Version: 3, Deposit: deposit, Anchor: shielded3.Digest{1}, Nullifier: shielded3.Digest{2}, StampRoot: shielded3.Digest{3}, WithdrawalValue: big.NewInt(withdrawal), GasSponsorValue: big.NewInt(sponsor)}
		for i := range e.Outputs {
			e.Outputs[i].OneTimeKey = shielded3.Digest{uint64(i + 1)}.Bytes()
		}
		data, err = EncodeShieldedV3Transaction(e)
	} else {
		e := &ShieldedV4Transaction{Version: 4, Deposit: deposit, Anchor: shielded3.Digest{1}, Nullifier: shielded3.Digest{2}, StampRoot: shielded3.Digest{3}, LinkTag: shielded3.Digest{4}, WithdrawalValue: big.NewInt(withdrawal), GasSponsorValue: big.NewInt(sponsor)}
		for i := range e.Outputs {
			e.Outputs[i].OneTimeKey = shielded3.Digest{uint64(i + 1)}.Bytes()
		}
		data, err = EncodeShieldedV4Transaction(e)
	}
	if err != nil {
		t.Fatal(err)
	}
	return types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: 100, GasFeeCap: big.NewInt(2), GasTipCap: big.NewInt(1), Data: data})
}

func TestShieldedOnlyRejectsPublicValuesEveryEntryPoint(t *testing.T) {
	for _, version := range []int{3, 4} {
		for name, tx := range map[string]*types.Transaction{
			"deposit with zero outer value": privatePolicyTx(t, version, true, 0, 0),
			"withdrawal":                    privatePolicyTx(t, version, false, 1, 0),
			"sponsorship used as payment":   privatePolicyTx(t, version, false, 0, 99),
			"exact gas reserve refund":      privatePolicyTx(t, version, false, 0, 200),
		} {
			t.Run(big.NewInt(int64(version)).String()+"/"+name, func(t *testing.T) {
				cfg, number, timestamp := shieldedOnlyTestConfig(), big.NewInt(1), params.MainnetShieldedOnlyTime
				for entry, err := range map[string]error{
					"policy":           ValidatePrivateExecutionPolicy(cfg, number, timestamp, tx),
					"admission basics": ValidateShieldedTransactionBasics(cfg, number, timestamp, tx),
					// A nil state proves rejection happens before state mutation,
					// stamp checks, or proof construction/verification.
					"block processing": ProcessShieldedTransaction(cfg, number, timestamp, nil, tx, nil),
				} {
					if !errors.Is(err, ErrPublicPaymentDisabled) {
						t.Fatalf("%s: got %v", entry, err)
					}
				}
				if err := ValidateShieldedOnlyTransaction(cfg, number, timestamp-1, tx); err != nil {
					t.Fatalf("historical rules changed: %v", err)
				}
			})
		}
	}
}

func TestShieldedOnlyPrivateSpendsStillRequireProofs(t *testing.T) {
	for _, version := range []int{3, 4} {
		for _, fee := range []int64{0} {
			tx := privatePolicyTx(t, version, false, 0, fee)
			cfg, number, timestamp := shieldedOnlyTestConfig(), big.NewInt(1), params.MainnetShieldedOnlyTime
			if err := ValidatePrivateExecutionPolicy(cfg, number, timestamp, tx); err != nil {
				t.Fatal(err)
			}
			if err := ValidatePrivateExecutionMessage(cfg, number, timestamp, tx.Type(), tx.To(), tx.Value(), tx.Data()); err != nil {
				t.Fatal(err)
			}
			if err := ValidateShieldedTransactionBasics(cfg, number, timestamp, tx); err == nil {
				t.Fatal("policy acceptance bypassed envelope/proof validation")
			}
			var amount *big.Int
			if version == 3 {
				e, _, _ := DecodeShieldedV3Transaction(tx.Data())
				s, err := ShieldedV3Statement(tx, e)
				if err != nil {
					t.Fatal(err)
				}
				amount = s.PublicValue.Big()
			} else {
				e, _, _ := DecodeShieldedV4Transaction(tx.Data())
				s, err := ShieldedV4Statement(tx, e)
				if err != nil {
					t.Fatal(err)
				}
				amount = s.PublicValue.Big()
			}
			if amount.Cmp(big.NewInt(fee)) != 0 {
				t.Fatal("statement disclosed principal beyond the public gas reserve")
			}
		}
	}
}

func TestShieldedOnlyRejectsPrefixAndDestinationBypasses(t *testing.T) {
	valid := privatePolicyTx(t, 4, false, 0, 0)
	other := common.HexToAddress("0x1234")
	for name, item := range map[string]struct {
		to    *common.Address
		value *big.Int
		data  []byte
	}{
		"outer value":             {valid.To(), big.NewInt(1), valid.Data()},
		"contract creation":       {nil, new(big.Int), valid.Data()},
		"another contract":        {&other, new(big.Int), valid.Data()},
		"empty":                   {valid.To(), new(big.Int), nil},
		"prefix only":             {valid.To(), new(big.Int), []byte(ShieldedV4Magic)},
		"legacy migration":        {valid.To(), new(big.Int), []byte(shieldedTxMagic)},
		"malformed stamp":         {valid.To(), new(big.Int), []byte(AntarticalStampMagic)},
		"unwrapped TVM":           {valid.To(), new(big.Int), []byte(PrivateTVMMagic)},
		"public governance debit": {valid.To(), new(big.Int), []byte(AddressVoteMagic)},
	} {
		t.Run(name, func(t *testing.T) {
			for _, timestamp := range []uint64{params.MainnetShieldedOnlyTime, params.MainnetShieldedOnlyTime + 1} {
				err := ValidatePrivateExecutionMessage(shieldedOnlyTestConfig(), big.NewInt(1), timestamp, types.PQTkmTxType, item.to, item.value, item.data)
				if !errors.Is(err, ErrPublicPaymentDisabled) {
					t.Fatalf("bypass: %v", err)
				}
			}
		})
	}
	if err := ValidatePrivateExecutionMessage(shieldedOnlyTestConfig(), big.NewInt(1), params.MainnetShieldedOnlyTime, types.LegacyTxType, valid.To(), valid.Value(), valid.Data()); !errors.Is(err, ErrPublicPaymentDisabled) {
		t.Fatal("non-PQ transaction accepted")
	}
}

func shieldedOnlyTestConfig() *params.ChainConfig {
	cfg := *params.MainnetChainConfig
	at := params.MainnetShieldedOnlyTime
	cfg.ShieldedOnlyTime = &at
	return &cfg
}
