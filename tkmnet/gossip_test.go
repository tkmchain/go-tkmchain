package tkmnet

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestDescriptorGossipSendsVerifiedLocalDescriptorToPeer(t *testing.T) {
	service, err := NewService(ServiceConfig{
		Enabled: true, ListenAddr: "127.0.0.1:0", PrivateKeyPath: filepath.Join(t.TempDir(), "relay-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := Descriptor{
		Version: Version, ID: service.RelayID(), Onion: strings.Repeat("c", 56) + ".onion",
		PublicKey: service.PublicKey(), SigningPublicKey: pqcrypto.PublicKeyBytes(signer),
		Expires: uint64(time.Now().Add(time.Hour).Unix()),
	}
	if err := SignDescriptor(&descriptor, func(message []byte) ([]byte, error) {
		return pqcrypto.SignMLDSA87(signer, message)
	}); err != nil {
		t.Fatal(err)
	}
	gossip := NewDescriptorGossip()
	if added, err := gossip.Add(descriptor); err != nil || !added {
		t.Fatalf("add local descriptor: added=%t err=%v", added, err)
	}
	local, remote := p2p.MsgPipe()
	defer local.Close()
	defer remote.Close()
	var peerID enode.ID
	peer := p2p.NewPeerPipe(peerID, "gossip-test", []p2p.Cap{{Name: descriptorProtocolName, Version: descriptorProtocolVersion}}, local)
	result := make(chan error, 1)
	go func() { result <- gossip.Protocols()[0].Run(peer, local) }()

	msg, err := remote.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Code != descriptorMessageCode || msg.Size > descriptorMessageMaxSize {
		t.Fatalf("gossip message code/size = %d/%d", msg.Code, msg.Size)
	}
	var received Descriptor
	if err := msg.Decode(&received); err != nil {
		t.Fatal(err)
	}
	if err := received.Verify(time.Now()); err != nil || received.ID != descriptor.ID {
		t.Fatalf("peer received invalid descriptor: id-match=%t err=%v", received.ID == descriptor.ID, err)
	}
	_ = remote.Close()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("gossip protocol did not stop after peer disconnect")
	}
}
