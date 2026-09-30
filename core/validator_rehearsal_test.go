package core

import (
	"math/big"
	"testing"

	"github.com/emmansun/gmsm/mldsa"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func rehearsalValidatorTx(t *testing.T, config *params.ChainConfig, key *mldsa.Key87, nonce uint64, value *big.Int, data []byte) *types.Transaction {
	t.Helper()
	pub := pqcrypto.PublicKeyBytes(key)
	inner := &types.PQTkmTx{
		ChainID: config.ChainID, Nonce: nonce, To: &params.ShieldedPoolAddress,
		Value: value, Gas: 4_000_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pub, Data: data,
	}
	tx, err := types.SignNewPQTkmTx(key, types.NewQuantumSigner(config.ChainID), inner)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestEgyptValidatorRehearsalRegistrationReorgReplaySlashAndRewards(t *testing.T) {
	config := params.EgyptChainConfig
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]*mldsa.Key87, 2)
	addresses := make([]common.Address, 2)
	for i := range keys {
		keys[i], err = pqcrypto.GenerateMLDSA87()
		if err != nil {
			t.Fatal(err)
		}
		pub := pqcrypto.PublicKeyBytes(keys[i])
		addresses[i], err = pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
		if err != nil {
			t.Fatal(err)
		}
		st.SetBalance(addresses[i], uint256.MustFromBig(new(big.Int).Add(ValidatorBondWei(), ValidatorRegistrationFeeWei())), tracing.BalanceChangeUnspecified)
		data, err := EncodeValidatorRegistration(&ValidatorRegistration{Version: ValidatorEnvelopeVersion, PublicKey: pub, RewardAddress: addresses[i], ActivationHeight: 1 + ValidatorActivationDelay})
		if err != nil {
			t.Fatal(err)
		}
		tx := rehearsalValidatorTx(t, config, keys[i], 0, ValidatorBondWei(), data)
		if err := ProcessValidatorRegistration(config, big.NewInt(1), 0, st, tx); err != nil {
			t.Fatalf("validator %d registration failed: %v", i, err)
		}
	}
	st.SetBalance(params.ShieldedPoolAddress, uint256.MustFromBig(new(big.Int).Mul(ValidatorBondWei(), big.NewInt(2))), tracing.BalanceChangeUnspecified)
	active, err := ActiveValidatorRecords(st, 1+ValidatorActivationDelay)
	if err != nil || len(active) != 2 {
		t.Fatalf("Egypt activation set = %d, err=%v", len(active), err)
	}
	header := &types.Header{Number: new(big.Int).SetUint64(1 + ValidatorActivationDelay), ParentHash: common.HexToHash("0x100")}
	reward, err := BuildValidatorRewardTx(st, header)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateValidatorRewardTransaction(st, header, reward); err != nil {
		t.Fatalf("validator reward rejected: %v", err)
	}

	// An exit is visible immediately to committee selection, while the bond
	// remains reserved until the unbonding period has elapsed.
	exitData, err := EncodeValidatorExit(&ValidatorAction{Version: ValidatorEnvelopeVersion})
	if err != nil {
		t.Fatal(err)
	}
	exit := rehearsalValidatorTx(t, config, keys[1], 1, new(big.Int), exitData)
	if err := ProcessValidatorAction(config, new(big.Int).SetUint64(1000), 0, st, exit); err != nil {
		t.Fatalf("validator exit failed: %v", err)
	}
	if active, err := ActiveValidatorRecords(st, 1000); err != nil || len(active) != 1 {
		t.Fatalf("exited validator remained active: count=%d err=%v", len(active), err)
	}
	withdrawData, err := EncodeValidatorWithdrawal(&ValidatorAction{Version: ValidatorEnvelopeVersion})
	if err != nil {
		t.Fatal(err)
	}
	tooEarly := rehearsalValidatorTx(t, config, keys[1], 2, new(big.Int), withdrawData)
	if err := ProcessValidatorAction(config, new(big.Int).SetUint64(1000+ValidatorUnbondingPeriod-1), 0, st, tooEarly); err == nil {
		t.Fatal("accepted a validator withdrawal before unbonding elapsed")
	}
	withdraw := rehearsalValidatorTx(t, config, keys[1], 2, new(big.Int), withdrawData)
	if err := ProcessValidatorAction(config, new(big.Int).SetUint64(1000+ValidatorUnbondingPeriod), 0, st, withdraw); err != nil {
		t.Fatalf("validator bond withdrawal failed: %v", err)
	}

	// Two conflicting attestations are slashable. Snapshot/revert models a
	// competing branch; the same evidence remains valid on the winning branch
	// exactly once, and replay is rejected thereafter.
	badA, badB := common.HexToHash("0xaaa"), common.HexToHash("0xbbb")
	sigA, err := pqcrypto.SignMLDSA87(keys[0], ValidatorAttestationDigest(2000, badA).Bytes())
	if err != nil {
		t.Fatal(err)
	}
	sigB, err := pqcrypto.SignMLDSA87(keys[0], ValidatorAttestationDigest(2000, badB).Bytes())
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := EncodeValidatorSlash(&ValidatorSlashEvidence{Version: ValidatorEnvelopeVersion, Validator: addresses[0], Height: 2000, BlockHashA: badA, BlockHashB: badB, SignatureA: sigA, SignatureB: sigB, PublicKey: pqcrypto.PublicKeyBytes(keys[0])})
	if err != nil {
		t.Fatal(err)
	}
	slash := rehearsalValidatorTx(t, config, keys[1], 3, new(big.Int), evidence)
	snapshot := st.Snapshot()
	if err := ProcessValidatorSlash(config, big.NewInt(2000), 0, st, slash); err != nil {
		t.Fatalf("slash failed on rehearsal branch: %v", err)
	}
	st.RevertToSnapshot(snapshot)
	if record, err := readValidatorRecord(st, addresses[0]); err != nil || record.Stake.Sign() == 0 {
		t.Fatalf("reorg did not restore validator bond: %v", err)
	}
	if err := ProcessValidatorSlash(config, big.NewInt(2000), 0, st, slash); err != nil {
		t.Fatalf("slash failed on canonical branch: %v", err)
	}
	if err := ProcessValidatorSlash(config, big.NewInt(2001), 0, st, slash); err == nil {
		t.Fatal("replayed slash evidence was accepted")
	}
	if record, err := readValidatorRecord(st, addresses[0]); err != nil || record.Stake.Sign() != 0 || record.SlashCount != 1 {
		t.Fatalf("slashed validator state = %+v, err=%v", record, err)
	}
}
