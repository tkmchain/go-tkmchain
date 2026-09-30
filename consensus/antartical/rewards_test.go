package antartical

import (
	"math/big"
	"testing"
)

func TestAntarticalRewardSharesAndHalving(t *testing.T) {
	shares := RewardSharesAt(0, 100)
	wantInitial, _ := new(big.Int).SetString("200000000000000000000", 10)
	if shares.Total().Cmp(wantInitial) != 0 {
		t.Fatalf("initial total = %s, want 200 TKM", shares.Total())
	}
	if shares.Miner.String() != "90000000000000000000" || shares.Validator.String() != "70000000000000000000" || shares.Rotating.String() != "35000000000000000000" || shares.MainKing.String() != "5000000000000000000" {
		t.Fatalf("unexpected initial shares: %+v", shares)
	}
	halved := RewardSharesAt(100, 100)
	wantHalved, _ := new(big.Int).SetString("100000000000000000000", 10)
	if halved.Total().Cmp(wantHalved) != 0 {
		t.Fatalf("halved total = %s, want 100 TKM", halved.Total())
	}
}
