package eth

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/tkmnet"
)

func TestTkmNameAPIRequiresCompleteFixedBatch(t *testing.T) {
	directory, err := shield3wallet.NewUsernameDirectory(8979)
	if err != nil {
		t.Fatal(err)
	}
	api := &TkmNameAPI{directory: directory}
	if _, err := api.LookupBatch([]string{"alice"}); err == nil {
		t.Fatal("accepted a short lookup batch")
	}
	names := make([]string, shield3wallet.UsernameLookupBatchLen)
	for i := range names {
		names[i] = "cover" + string(rune('a'+i))
	}
	if _, err := api.LookupBatch(names); err == nil {
		t.Fatal("returned a partial batch containing missing records")
	}
}

func TestGossipedDescriptorCanBecomeTransitButNotDirectory(t *testing.T) {
	tmp := t.TempDir()
	relay, err := tkmnet.NewService(tkmnet.ServiceConfig{
		Enabled: true, ListenAddr: "127.0.0.1:0", PrivateKeyPath: filepath.Join(tmp, "relay-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := tkmnet.Descriptor{
		Version: tkmnet.Version, ID: relay.RelayID(), Onion: strings.Repeat("b", 56) + ".onion",
		PublicKey: relay.PublicKey(), SigningPublicKey: pqcrypto.PublicKeyBytes(signer),
		Expires: uint64(time.Now().Add(time.Hour).Unix()),
	}
	if err := tkmnet.SignDescriptor(&descriptor, func(message []byte) ([]byte, error) {
		return pqcrypto.SignMLDSA87(signer, message)
	}); err != nil {
		t.Fatal(err)
	}
	api := &TkmNameAPI{}
	if err := api.AddDiscoveredTransit(descriptor); err != nil {
		t.Fatal(err)
	}
	if err := api.AddDiscoveredTransit(descriptor); err != nil {
		t.Fatal(err)
	}
	_, _, transit, directories := api.networkConfig()
	if len(transit) != 1 || transit[0].ID != descriptor.ID {
		t.Fatalf("discovered transit set = %d entries, want the verified descriptor", len(transit))
	}
	if len(directories) != 0 {
		t.Fatal("gossip promoted a relay descriptor to an authorized directory")
	}
}
