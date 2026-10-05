package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
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

func TestShieldedOnlyAllowsValidatedValidatorProtocolTransaction(t *testing.T) {
	chainConfig := *params.MainnetChainConfig
	chainConfig.ShieldedOnlyTime = new(uint64)
	*chainConfig.ShieldedOnlyTime = params.MainnetShieldedOnlyTime
	cfg := &chainConfig
	number := big.NewInt(48_162)
	timestamp := params.MainnetValidatorTransactionTime
	if !cfg.IsAntartical(number, timestamp) || !cfg.IsShieldedOnly(number, timestamp) {
		t.Fatal("test timestamp must have Antartical and shielded-only active")
	}
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	publicKey := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncodeValidatorRegistration(&ValidatorRegistration{
		Version: ValidatorEnvelopeVersion, PublicKey: publicKey, RewardAddress: address,
		ActivationHeight: number.Uint64() + ValidatorActivationDelay,
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := types.SignNewPQTkmTx(key, types.NewQuantumSigner(cfg.ChainID), &types.PQTkmTx{
		ChainID: cfg.ChainID, To: &params.ShieldedPoolAddress, Value: ValidatorBondWei(),
		Gas: 500_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: publicKey, Data: data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateExecutionPolicy(cfg, number, params.MainnetValidatorTransactionTime-1, tx); !errors.Is(err, ErrValidatorTransactionsNotActive) {
		t.Fatalf("validator registration before activation error = %v, want %v", err, ErrValidatorTransactionsNotActive)
	}
	if err := ValidatePrivateExecutionPolicy(cfg, number, timestamp, tx); err != nil {
		t.Fatalf("valid validator registration rejected by transaction policy: %v", err)
	}
	if err := ValidateShieldedTransactionBasics(cfg, number, timestamp, tx); err != nil {
		t.Fatalf("valid validator registration rejected by privacy-envelope admission: %v", err)
	}
	if err := ValidatePrivateExecutionMessage(cfg, number, timestamp, tx.Type(), tx.To(), tx.Value(), tx.Data()); err != nil {
		t.Fatalf("valid validator registration rejected by EVM message policy: %v", err)
	}
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", address.Bytes()), common.Hash{1})
	if err := ProcessShieldedTransaction(cfg, number, timestamp, st, tx, nil); err != nil {
		t.Fatalf("valid validator registration rejected by block shielded-transaction dispatch: %v", err)
	}

	// The exception is limited to the fixed bond transfer into the reserved
	// pool. A different public amount and an ordinary transparent transfer
	// must continue to fail at the privacy boundary.
	wrongValue, err := types.SignNewPQTkmTx(key, types.NewQuantumSigner(cfg.ChainID), &types.PQTkmTx{
		ChainID: cfg.ChainID, To: &params.ShieldedPoolAddress,
		Value: new(big.Int).Add(ValidatorBondWei(), big.NewInt(1)), Gas: 500_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: publicKey, Data: data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateExecutionPolicy(cfg, number, timestamp, wrongValue); err == nil {
		t.Fatal("validator registration accepted an arbitrary public value")
	}
	ordinary := types.NewTx(&types.PQTkmTx{ChainID: cfg.ChainID, To: &params.ShieldedPoolAddress, Value: big.NewInt(1), Gas: 21_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1)})
	if err := ValidatePrivateExecutionPolicy(cfg, number, timestamp, ordinary); !errors.Is(err, ErrPublicPaymentDisabled) {
		t.Fatalf("ordinary transparent transfer error = %v, want ErrPublicPaymentDisabled", err)
	}
}

func shieldedOnlyTestConfig() *params.ChainConfig {
	cfg := *params.MainnetChainConfig
	at := params.MainnetShieldedOnlyTime
	cfg.ShieldedOnlyTime = &at
	return &cfg
}
