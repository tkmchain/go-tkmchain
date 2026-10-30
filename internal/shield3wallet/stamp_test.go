package shield3wallet

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type stampStatusRPC struct{ status core.AntarticalStampStatus }

func (r stampStatusRPC) CallContext(_ context.Context, result any, method string, _ ...any) error {
	if method != "tkmprivacy_antarticalStamp" {
		return nil
	}
	*(result.(*core.AntarticalStampStatus)) = r.status
	return nil
}

func TestRequireRegisteredStampDistinguishesMissingAndMismatchedIdentity(t *testing.T) {
	var owner, otherOwner shielded3.Digest
	owner[0], otherOwner[0] = 1, 2
	var commitment, otherCommitment [64]byte
	commitment[0], otherCommitment[0] = 3, 4
	payload := PaymentPayload{
		Address: common.HexToAddress("0x1234"),
		Owner:   owner,
		Stamp:   pqcrypto.ShieldedV3StampRecord{Commitment: commitment},
	}
	tests := []struct {
		name   string
		status core.AntarticalStampStatus
		want   string
	}{
		{name: "missing", want: "not confirmed on this chain"},
		{name: "owner mismatch", status: core.AntarticalStampStatus{Registered: true, Owner: otherOwner, Commitment: commitment}, want: "owner does not match"},
		{name: "commitment mismatch", status: core.AntarticalStampStatus{Registered: true, Owner: owner, Commitment: otherCommitment}, want: "commitment does not match"},
		{name: "matches", status: core.AntarticalStampStatus{Registered: true, Owner: owner, Commitment: commitment}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := RequireRegisteredStamp(context.Background(), stampStatusRPC{status: test.status}, payload)
			if test.want == "" {
				if err != nil {
					t.Fatalf("unexpected stamp validation error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want message containing %q", err, test.want)
			}
		})
	}
}

func TestRegisteredStampPayloadIncludesIdentityOwner(t *testing.T) {
	var owner shielded3.Digest
	owner[0] = 7
	stamp := &pqcrypto.ShieldedV3StampRecord{ChainID: 8979}
	identity := &Identity{
		ChainID: 8979,
		Address: common.HexToAddress("0x1234"),
		Owner:   owner,
		Stamp:   stamp,
	}
	payload, err := identity.RegisteredStampPayload()
	if err != nil {
		t.Fatalf("build registered stamp payload: %v", err)
	}
	if payload.ChainID != identity.ChainID || payload.Address != identity.Address || payload.Owner != identity.Owner || payload.Stamp.ChainID != identity.Stamp.ChainID || payload.Stamp.Commitment != identity.Stamp.Commitment {
		t.Fatalf("registered stamp payload omitted identity fields: payload=%+v identity=%+v", payload, identity)
	}
}
