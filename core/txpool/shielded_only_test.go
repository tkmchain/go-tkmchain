package txpool

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestShieldedOnlyAdmissionBoundary(t *testing.T) {
	cfg := *params.MainnetChainConfig
	at := params.MainnetShieldedOnlyTime
	cfg.ShieldedOnlyTime = &at
	opts := &ValidationOptions{Config: &cfg, Accept: 0xff, MaxSize: core.ShieldedV3MaxTxSize, MinTip: new(big.Int)}
	for _, version := range []int{3, 4} {
		var data []byte
		var err error
		if version == 3 {
			data, err = core.EncodeShieldedV3Transaction(&core.ShieldedV3Transaction{Version: 3, Deposit: true, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)})
		} else {
			data, err = core.EncodeShieldedV4Transaction(&core.ShieldedV4Transaction{Version: 4, Deposit: true, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)})
		}
		if err != nil {
			t.Fatal(err)
		}
		tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Gas: 7_000_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Value: big.NewInt(1000), Data: data})
		for _, timestamp := range []uint64{params.MainnetShieldedOnlyTime - 1, params.MainnetShieldedOnlyTime, params.MainnetShieldedOnlyTime + 1} {
			head := &types.Header{Number: big.NewInt(1), Time: timestamp, Difficulty: big.NewInt(1), GasLimit: 30_000_000}
			err := ValidateTransaction(tx, head, types.NewQuantumSigner(big.NewInt(8979)), opts)
			if errors.Is(err, core.ErrPublicPaymentDisabled) != (timestamp >= params.MainnetShieldedOnlyTime) {
				t.Fatalf("version %d timestamp %d: %v", version, timestamp, err)
			}
		}
	}
}
