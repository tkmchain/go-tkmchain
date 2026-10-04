package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

func ledgerTestConfig() *params.ChainConfig {
	cfg := *params.MainnetChainConfig
	at := params.ProposedConfidentialLedgerTime
	cfg.ConfidentialLedgerTime = &at
	return &cfg
}

func TestConfidentialLedgerConservationAndRollback(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	st.AddBalance(params.ShieldedPoolAddress, uint256.NewInt(100), tracing.BalanceChangeUnspecified)
	snapshot := st.Snapshot()
	l, err := ledgerTransition(st, shielded3.AssetTKM, big.NewInt(20), big.NewInt(30), big.NewInt(4))
	if err != nil || l.OpeningBacking.Int64() != 100 || l.Backing.Int64() != 86 {
		t.Fatalf("bad conservation: %+v %v", l, err)
	}
	commitLedger(st, shielded3.AssetTKM, l)
	if ConfidentialLedgerState(st, shielded3.AssetPTKM).Initialized {
		t.Fatal("asset ledger collision")
	}
	for _, bad := range []*big.Int{big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 256), nil} {
		if _, err := ledgerTransition(st, shielded3.AssetTKM, bad, new(big.Int), new(big.Int)); err == nil {
			t.Fatal("invalid deposit accepted")
		}
	}
	if _, err := ledgerTransition(st, shielded3.AssetTKM, new(big.Int), big.NewInt(87), new(big.Int)); err == nil {
		t.Fatal("overdraw accepted")
	}
	st.SetBalance(params.ShieldedPoolAddress, uint256.NewInt(85), tracing.BalanceChangeUnspecified)
	if _, err := ledgerTransition(st, shielded3.AssetTKM, new(big.Int), new(big.Int), new(big.Int)); err == nil {
		t.Fatal("backing shortfall accepted")
	}
	st.RevertToSnapshot(snapshot)
	if ConfidentialLedgerState(st, shielded3.AssetTKM).Initialized || st.GetBalance(params.ShieldedPoolAddress).Uint64() != 100 {
		t.Fatal("ledger did not revert with state")
	}
}

func TestConfidentialLedgerExcludesBondAndWrappedReserves(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	st.SetBalance(params.ShieldedPoolAddress, uint256.NewInt(1000), tracing.BalanceChangeUnspecified)
	setShieldedV3AssetSupply(st, shielded3.AssetPTKM, big.NewInt(200))
	record := &ValidatorRecord{Address: common.HexToAddress("0x1234"), Stake: big.NewInt(300), ExitHeight: 1}
	if err := writeValidatorRecord(st, record); err != nil {
		t.Fatal(err)
	}
	st.SetState(params.ShieldedPoolAddress, validatorIndexSlot(0), common.BytesToHash(record.Address.Bytes()))
	st.SetState(params.ShieldedPoolAddress, validatorCountSlot(), uint64Hash(1))
	l, err := ledgerTransition(st, shielded3.AssetTKM, new(big.Int), new(big.Int), new(big.Int))
	if err != nil || l.OpeningBacking.Int64() != 500 {
		t.Fatalf("reserved balances counted as notes: %+v %v", l, err)
	}
	commitLedger(st, shielded3.AssetTKM, l)
	// Bond withdrawal or slashing debits the pool and clears stake together.
	st.SubBalance(params.ShieldedPoolAddress, uint256.NewInt(300), tracing.BalanceChangeUnspecified)
	record.Stake.SetUint64(0)
	if err := writeValidatorRecord(st, record); err != nil {
		t.Fatal(err)
	}
	if next, err := ledgerTransition(st, shielded3.AssetTKM, new(big.Int), new(big.Int), new(big.Int)); err != nil || next.Backing.Int64() != 500 {
		t.Fatalf("bond removal stranded ledger: %v", err)
	}
	// A legacy migration is a public exit from the same native reserve.
	next, err := ledgerTransition(st, shielded3.AssetTKM, new(big.Int), big.NewInt(100), new(big.Int))
	if err != nil {
		t.Fatal(err)
	}
	st.SubBalance(params.ShieldedPoolAddress, uint256.NewInt(100), tracing.BalanceChangeUnspecified)
	commitLedger(st, shielded3.AssetTKM, next)
	if _, err := ledgerTransition(st, shielded3.AssetTKM, new(big.Int), new(big.Int), new(big.Int)); err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerTransition(st, shielded3.AssetTKM, new(big.Int), big.NewInt(401), new(big.Int)); err == nil {
		t.Fatal("spend used wrapped backing")
	}
}

func TestConfidentialLedgerFeeTicketCannotBeForgedOrReused(t *testing.T) {
	for _, version := range []int{3, 4} {
		st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
		if err != nil {
			t.Fatal(err)
		}
		cfg := ledgerTestConfig()
		fee := new(big.Int).SetUint64(ConfidentialPrepaidGas)
		var data []byte
		if version == 3 {
			data, err = EncodeShieldedV3Transaction(&ShieldedV3Transaction{Version: 3, WithdrawalValue: new(big.Int), GasSponsorValue: fee, FeeMode: ShieldedFeePrepaid})
		} else {
			data, err = EncodeShieldedV4Transaction(&ShieldedV4Transaction{Version: 4, WithdrawalValue: new(big.Int), GasSponsorValue: fee, FeeMode: ShieldedFeePrepaid})
		}
		if err != nil {
			t.Fatal(err)
		}
		tx := types.NewTx(&types.PQTkmTx{ChainID: cfg.ChainID, To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: ConfidentialPrepaidGas, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Data: data})
		msg := &Message{TxType: types.PQTkmTxType, To: tx.To(), Value: new(uint256.Int), GasLimit: tx.Gas(), GasFeeCap: uint256.NewInt(1), GasTipCap: uint256.NewInt(1), GasPrice: uint256.NewInt(1), Data: data, shieldedTransactionHash: tx.Hash()}
		evm := vm.NewEVM(vm.BlockContext{BlockNumber: big.NewInt(1), Time: params.ProposedConfidentialLedgerTime, BaseFee: new(big.Int)}, st, cfg, vm.Config{})
		transition := func() *stateTransition { return newStateTransition(evm, msg, NewGasPool(30000000)) }
		if err := transition().buyGas(); err == nil {
			t.Fatal("forged note fee without proof debit accepted")
		}
		st.AddBalance(params.ShieldedPoolAddress, uint256.MustFromBig(fee), tracing.BalanceChangeUnspecified)
		burnConfidentialFee(st, tx, fee)
		msg.shieldedTransactionHash = common.HexToHash("0x1234")
		if err := transition().buyGas(); err == nil {
			t.Fatal("fee ticket transferred to another transaction")
		}
		msg.shieldedTransactionHash = tx.Hash()
		if err := transition().buyGas(); err != nil {
			t.Fatal(err)
		}
		if st.GetBalance(params.ShieldedPoolAddress).Sign() != 0 || st.GetBalance(msg.From).Sign() != 0 {
			t.Fatal("fee was not burned")
		}
		if err := transition().buyGas(); err == nil {
			t.Fatal("fee ticket replay accepted")
		}
		evm.Release()
	}
}

func TestConfidentialLedgerFeeAuthorizationAndIntent(t *testing.T) {
	cfg := ledgerTestConfig()
	at := params.ProposedConfidentialLedgerTime
	tx := types.NewTx(&types.PQTkmTx{ChainID: cfg.ChainID, To: &params.ShieldedPoolAddress, Value: new(big.Int), Gas: ConfidentialPrepaidGas, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1)})
	fee := new(big.Int).SetUint64(ConfidentialPrepaidGas)
	check := func(time uint64, deposit bool, sponsor *big.Int, mode, asset uint64) error {
		return validateConfidentialLedgerShape(cfg, big.NewInt(1), time, tx, deposit, sponsor, mode, asset)
	}
	if err := check(at, false, fee, ShieldedFeePrepaid, shielded3.AssetTKM); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{check(at-1, false, fee, 1, 1), check(at, true, fee, 1, 1), check(at, false, big.NewInt(1), 1, 1), check(at, false, fee, 1, shielded3.AssetPTKM), check(at, false, fee, 0, 1), check(at, false, fee, 2, 1)} {
		if err == nil {
			t.Fatal("invalid fee authorization accepted")
		}
	}
	for _, version := range []int{3, 4} {
		var before, after [64]byte
		if version == 3 {
			e := &ShieldedV3Transaction{Version: 3, WithdrawalValue: new(big.Int), GasSponsorValue: fee}
			before, _ = ShieldedV3Intent(tx, e)
			e.FeeMode = ShieldedFeePrepaid
			after, _ = ShieldedV3Intent(tx, e)
		} else {
			e := &ShieldedV4Transaction{Version: 4, WithdrawalValue: new(big.Int), GasSponsorValue: fee}
			before, _ = ShieldedV4Intent(tx, e)
			e.FeeMode = ShieldedFeePrepaid
			after, _ = ShieldedV4Intent(tx, e)
		}
		if before == after || before == ([64]byte{}) || after == ([64]byte{}) {
			t.Fatal("fee mode not bound into proof intent")
		}
	}
}
