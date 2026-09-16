package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

func TestAntarticalUnstampedTransactionsAreIllegal(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	from, to := common.HexToAddress("0x123"), common.HexToAddress("0x456")
	if err := ValidateAntarticalStampState(st, from, &to, big.NewInt(1), nil); !errors.Is(err, ErrUnstampedAddress) {
		t.Fatal("unstamped sender accepted", err)
	}
	if err := ValidateAntarticalStampState(st, from, &params.ShieldedPoolAddress, new(big.Int), []byte(ShieldedV3Magic)); !errors.Is(err, ErrUnstampedAddress) {
		t.Fatal("zero outer value bypassed private spend gate", err)
	}
	if err := ValidateAntarticalStampState(st, from, &params.ShieldedPoolAddress, big.NewInt(1), []byte(AntarticalStampMagic)); err == nil {
		t.Fatal("value transfer disguised as registration accepted")
	}
	if err := ValidateAntarticalStampState(st, from, &params.ShieldedPoolAddress, new(big.Int), []byte(AntarticalStampMagic)); err != nil {
		t.Fatal("zero-value registration gate failed", err)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", from.Bytes()), common.HexToHash("0x1"))
	if err := ValidateAntarticalStampState(st, from, &to, big.NewInt(1), nil); !errors.Is(err, ErrUnstampedAddress) {
		t.Fatal("unstamped public recipient accepted", err)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", to.Bytes()), common.HexToHash("0x2"))
	if err := ValidateAntarticalStampState(st, from, &to, big.NewInt(1), nil); err != nil {
		t.Fatal(err)
	}
	e := &ShieldedV3Transaction{WithdrawalRecipient: common.HexToAddress("0x789"), WithdrawalValue: big.NewInt(1), GasSponsorValue: new(big.Int), StampRoot: shielded3.Digest{1}}
	data, err := EncodeShieldedV3Transaction(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAntarticalStampState(st, from, &params.ShieldedPoolAddress, new(big.Int), data); !errors.Is(err, ErrUnstampedAddress) {
		t.Fatal("unstamped withdrawal accepted", err)
	}
}

func TestAntarticalRawMessageCannotBypassStampGate(t *testing.T) {
	for _, data := range [][]byte{nil, []byte(AntarticalStampMagic)} {
		st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
		if err != nil {
			t.Fatal(err)
		}
		h := &types.Header{Number: big.NewInt(1), Time: params.MainnetAntarticalTime, GasLimit: 8_000_000, BaseFee: big.NewInt(1), Difficulty: big.NewInt(1)}
		evm := vm.NewEVM(NewEVMBlockContext(h, nil, &h.Coinbase), st, params.MainnetChainConfig, vm.Config{})
		msg := &Message{From: common.HexToAddress("0x123"), To: &params.ShieldedPoolAddress, GasLimit: 100000, GasPrice: uint256.NewInt(1), GasFeeCap: uint256.NewInt(1), GasTipCap: uint256.NewInt(1), Value: new(uint256.Int), TxType: types.PQTkmTxType, Data: data}
		_, err = ApplyMessage(evm, msg, NewGasPool(h.GasLimit))
		evm.Release()
		if err == nil {
			t.Fatal("raw message bypassed consensus stamping")
		}
		if IsAntarticalStamped(st, msg.From) {
			t.Fatal("unverified registration modified stamp state")
		}
	}
}
