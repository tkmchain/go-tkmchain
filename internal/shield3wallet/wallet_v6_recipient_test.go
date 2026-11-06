package shield3wallet

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
)

func TestShield6RecipientClientDoesNotDowngrade(t *testing.T) {
	outputs := []core.ShieldedV6WalletOutput{{}}
	status := shieldedV6RPCStatus{RecipientProofsRequired: true, RecipientKeyHash: common.Hash{1}}
	if _, err := prepareShieldedV6ClientRecipients(context.Background(), nil, status, nil, nil, outputs, nil); err == nil {
		t.Fatal("missing recipient prover silently used experimental V6")
	}
	if _, err := prepareShieldedV6ClientRecipients(context.Background(), nil, shieldedV6RPCStatus{}, nil, nil, outputs, []*ShieldedV6RecipientClient{{}}); err == nil {
		t.Fatal("recipient proof configuration silently downgraded before activation")
	}
}

type recipientRootRecheckRPC struct{ calls int }

func (r *recipientRootRecheckRPC) CallContext(_ context.Context, result any, method string, args ...any) error {
	if method != "tkmprivacy_shieldedV6RecipientRootsKnown" {
		return errors.New("unexpected recipient owner query")
	}
	r.calls++
	*result.(*bool) = false
	return nil
}
func TestShield6RecipientClientRejectsReorganizedRoots(t *testing.T) {
	rpc := new(recipientRootRecheckRPC)
	if err := checkShieldedV6RecipientRoots(context.Background(), rpc, []core.ShieldedV6RecipientRoots{{}}); err == nil || rpc.calls != 1 {
		t.Fatal("reorganized recipient roots accepted")
	}
}
