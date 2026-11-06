package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

type shield6InactiveRecipientRPC struct{}

func (shield6InactiveRecipientRPC) CallContext(_ context.Context, result any, method string, args ...any) error {
	if method != "tkmprivacy_shieldedV6Status" {
		return errors.New("unexpected RPC")
	}
	return json.Unmarshal([]byte(`{"recipientProofsRequired":false}`), result)
}

type shield6ActiveParameterRPC struct{}

func (shield6ActiveParameterRPC) CallContext(_ context.Context, result any, method string, args ...any) error {
	if method != "tkmprivacy_shieldedV6Status" {
		return errors.New("unexpected RPC")
	}
	data, _ := json.Marshal(map[string]any{"recipientProofsRequired": true, "recipientKeyHash": common.Hash{1}, "recipientProvingKeyHash": common.Hash{2}})
	return json.Unmarshal(data, result)
}
func TestShield6CLIRejectsUnapprovedProvingKeyManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TKM_SHIELD6_RECIPIENT_PARAMETERS", dir)
	pins := recipient.ArtifactPins{Version: recipient.RelationVersion, VerifyingKey: [32]byte{1}, ProvingKey: [32]byte{3}}
	data, err := json.Marshal(pins)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = walletShield6RecipientClient(context.Background(), shield6ActiveParameterRPC{}, nil)
	if err == nil || !strings.Contains(err.Error(), "authenticated local release manifest") {
		t.Fatalf("unapproved proving key reached artifact decoding: %v", err)
	}
}
func TestShield6CLIRefusesRecipientProofDowngrade(t *testing.T) {
	if _, err := walletShield6RecipientClient(context.Background(), shield6InactiveRecipientRPC{}, []recipient.PaymentCode{{}}); err == nil {
		t.Fatal("recipient proof code downgraded to the experimental envelope")
	}
}
