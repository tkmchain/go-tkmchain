package shield3wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type shieldedOnlyRPC struct{ calls int }

func (r *shieldedOnlyRPC) CallContext(_ context.Context, dst any, method string, _ ...any) error {
	r.calls++
	if method != "tkmprivacy_shieldedV3Status" && method != "tkmprivacy_shieldedV4Status" {
		return fmt.Errorf("builder accessed %s before rejecting a public payment", method)
	}
	return json.Unmarshal([]byte(`{"active":true,"nativeVerifier":true,"shieldedOnly":true}`), dst)
}

func TestShieldedOnlyBuildersRejectBeforeProvingOrQueueing(t *testing.T) {
	identity := &Identity{ChainID: 8979, Stamp: &pqcrypto.ShieldedV3StampRecord{}}
	recipient := PaymentPayload{ChainID: 8979}
	// A saturated native-prover queue must not delay policy errors.
	buildSlot <- struct{}{}
	defer func() { <-buildSlot }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, version := range []int{3, 4} {
		for _, withdrawal := range []bool{false, true} {
			rpc := new(shieldedOnlyRPC)
			var err error
			if withdrawal {
				if version == 3 {
					_, err = BuildAssetWithdrawal(ctx, rpc, nil, identity, shielded3.AssetPTKM, common.HexToAddress("0x1234"), big.NewInt(1))
				} else {
					_, err = BuildV4AssetWithdrawal(ctx, rpc, nil, identity, shielded3.AssetPTKM, common.HexToAddress("0x1234"), big.NewInt(1))
				}
			} else if version == 3 {
				_, err = Build(ctx, rpc, nil, identity, recipient, big.NewInt(1), true)
			} else {
				_, err = BuildV4(ctx, rpc, nil, identity, recipient, big.NewInt(1), true)
			}
			if !errors.Is(err, core.ErrPublicPaymentDisabled) || rpc.calls != 1 {
				t.Fatalf("version %d withdrawal %v: err=%v calls=%d", version, withdrawal, err, rpc.calls)
			}
		}
	}
}
