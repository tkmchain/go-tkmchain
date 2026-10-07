package tkmnet

import (
	"bytes"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/naoina/toml"
)

func TestTkmnetDescriptorsAndPinsTOMLRoundTrip(t *testing.T) {
	now := time.Now()
	directories := pinnedDirectoryPeers(t, 2, now)
	peers := []Descriptor{directories[0].Descriptor, directories[1].Descriptor}
	type config struct {
		RelayPeers     []Descriptor
		DirectoryPeers []PinnedDirectoryPeer
	}
	want := config{RelayPeers: peers, DirectoryPeers: directories}
	settings := toml.Config{
		NormFieldName: func(_ reflect.Type, key string) string { return key },
		FieldToKey:    func(_ reflect.Type, field string) string { return field },
	}
	var encoded bytes.Buffer
	if err := settings.NewEncoder(&encoded).Encode(want); err != nil {
		t.Fatalf("encode peer config: %v", err)
	}
	var got config
	if err := settings.NewDecoder(&encoded).Decode(&got); err != nil {
		t.Fatalf("decode peer config: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("TOML peer descriptors or key pins changed during round trip")
	}
	for _, peer := range got.DirectoryPeers {
		if err := peer.Descriptor.Verify(now); err != nil {
			t.Fatalf("decoded descriptor is invalid: %v", err)
		}
		if sha256.Sum256(peer.Descriptor.SigningPublicKey) != peer.SigningKeyPin {
			t.Fatal("decoded directory signing-key pin does not match descriptor")
		}
	}
}

func pinnedDirectoryPeers(t *testing.T, count int, now time.Time) []PinnedDirectoryPeer {
	t.Helper()
	peers := make([]PinnedDirectoryPeer, count)
	for i := range peers {
		kem, err := mlkem.GenerateKey1024()
		if err != nil {
			t.Fatal(err)
		}
		signer, err := pqcrypto.GenerateMLDSA87()
		if err != nil {
			t.Fatal(err)
		}
		descriptor := Descriptor{
			Version:          Version,
			Onion:            strings.Repeat(string(rune('a'+i)), 56) + ".onion",
			PublicKey:        kem.EncapsulationKey().Bytes(),
			SigningPublicKey: pqcrypto.PublicKeyBytes(signer),
			Expires:          uint64(now.Add(time.Hour).Unix()),
		}
		copy(descriptor.ID[:], relayIDDigest(descriptor.PublicKey))
		if err := SignDescriptor(&descriptor, func(message []byte) ([]byte, error) {
			return pqcrypto.SignMLDSA87(signer, message)
		}); err != nil {
			t.Fatal(err)
		}
		peers[i] = PinnedDirectoryPeer{Descriptor: descriptor, SigningKeyPin: sha256.Sum256(descriptor.SigningPublicKey)}
	}
	return peers
}

func TestPlanPrivateUsernameLookupUsesDistinctPinnedOperators(t *testing.T) {
	now := time.Now().UTC()
	peers := pinnedDirectoryPeers(t, UsernameLookupFanout, now)
	names := []string{"alice", "cover01", "cover02", "cover03", "cover04", "cover05", "cover06", "cover07", "cover08", "cover09", "cover10", "cover11", "cover12", "cover13", "cover14", "cover15"}
	queries, err := PlanPrivateUsernameLookup("alice", names, peers, uint64(now.Unix()), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != UsernameLookupFanout {
		t.Fatalf("query count = %d", len(queries))
	}
	seenNames := make(map[string]bool)
	seenIDs := make(map[[LayerNextIDSize]byte]bool)
	for _, query := range queries {
		if seenNames[query.Name] || seenIDs[query.Peer.ID] {
			t.Fatal("lookup plan repeated a name or operator")
		}
		seenNames[query.Name] = true
		seenIDs[query.Peer.ID] = true
	}
	if _, err := PlanPrivateUsernameLookup("alice", names, peers[:UsernameLookupFanout-1], uint64(now.Unix()), nil); err == nil {
		t.Fatal("accepted fewer operators than lookup aliases")
	}
	peers[1].SigningKeyPin[0] ^= 1
	if _, err := PlanPrivateUsernameLookup("alice", names, peers, uint64(now.Unix()), nil); err == nil {
		t.Fatal("accepted an unpinned operator key")
	}
	if _, err := PlanPrivateUsernameLookup("missing", names, pinnedDirectoryPeers(t, UsernameLookupFanout, now), uint64(now.Unix()), rand.Reader); err == nil {
		t.Fatal("accepted a lookup set that omitted its target")
	}
}

func TestPlanPrivateUsernameLookupSupportsOneOrTwoDirectoryNodes(t *testing.T) {
	now := time.Now().UTC()
	allPeers := pinnedDirectoryPeers(t, 2, now)
	for count, names := range map[int][]string{
		1: {"alice"},
		2: {"alice", "cover1"},
	} {
		queries, err := PlanPrivateUsernameLookup("alice", names, allPeers[:count], uint64(now.Unix()), rand.Reader)
		if err != nil {
			t.Fatalf("%d directory nodes: %v", count, err)
		}
		if len(queries) != count {
			t.Fatalf("%d directory nodes produced %d queries", count, len(queries))
		}
	}
}

func TestUsernameQueryFixedWidthAndCanonical(t *testing.T) {
	payload, err := EncodeUsernameQuery("alice-7")
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != MaxPayload {
		t.Fatalf("payload size = %d", len(payload))
	}
	if got, err := DecodeUsernameQuery(payload); err != nil || got != "alice-7" {
		t.Fatalf("decoded name = %q, err=%v", got, err)
	}
	if _, err := EncodeUsernameQuery("Alice"); err == nil {
		t.Fatal("accepted non-canonical name")
	}
	payload[len(payload)-1] = 1
	if _, err := DecodeUsernameQuery(payload); err == nil {
		t.Fatal("accepted non-zero padding")
	}
}

func TestBuildPrivateUsernameRequestUsesDirectoryAsFinalRelay(t *testing.T) {
	now := time.Now().UTC()
	peers := pinnedDirectoryPeers(t, 3, now)
	packet, err := BuildPrivateUsernameRequest([]Descriptor{peers[0].Descriptor, peers[1].Descriptor}, peers[2].Descriptor, "alice", now)
	if err != nil {
		t.Fatal(err)
	}
	header, err := parseHeader(packet)
	if err != nil || header.service != ServiceUsername || header.hopIndex != 0 {
		t.Fatalf("request header = %+v, err=%v", header, err)
	}
	if _, err := BuildPrivateUsernameRequest([]Descriptor{peers[0].Descriptor, peers[0].Descriptor}, peers[2].Descriptor, "alice", now); err == nil {
		t.Fatal("accepted a repeated relay")
	}
}

func TestUsernameCoverAndRegistrationFrames(t *testing.T) {
	cover := UsernameCoverRequest()
	if !IsUsernameCoverRequest(cover) || len(cover) != MaxPayload {
		t.Fatal("cover query is not a canonical fixed-width request")
	}
	cover[len(cover)-1] = 1
	if IsUsernameCoverRequest(cover) {
		t.Fatal("cover query accepted nonzero padding")
	}
	names := []string{"alice", "bob_1", "carol-2", "dave", "eve", "frank", "grace", "heidi", "ivan", "judy", "karen", "leo", "mallory", "niaj", "olivia"}
	emptyResponse, err := UsernameCoverReply(nil)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeUsernameCoverReply(emptyResponse); err != nil || len(decoded) != 0 {
		t.Fatalf("empty cover response decoded as %v, err=%v", decoded, err)
	}
	response, err := UsernameCoverReply(names)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeUsernameCoverReply(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(names) {
		t.Fatalf("decoded %d cover names", len(decoded))
	}
	registration, err := UsernameRegistrationRequest([]byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	record, ok := IsUsernameRegistrationRequest(registration)
	if !ok || !bytes.Equal(record, []byte{1, 2, 3}) {
		t.Fatal("registration frame did not round trip")
	}
	frame, err := UsernameRecordReply([]byte{4, 5, 6})
	if err != nil {
		t.Fatal(err)
	}
	recordReply, err := DecodeUsernameRecordReply(frame)
	if err != nil || !bytes.Equal(recordReply, []byte{4, 5, 6}) {
		t.Fatalf("record reply round trip failed: %v", err)
	}
}
