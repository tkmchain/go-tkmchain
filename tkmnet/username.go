// Copyright 2026 The TKMChain Authors.

package tkmnet

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
)

const (
	UsernameLookupFanout = 16
	usernameQueryMagic   = "TKU1"
	usernameQueryHeader  = 6
	usernameCoverMagic   = "TKC1"
	usernameRecordMagic  = "TKNR"
	usernameReplyMagic   = "TKNS"
	usernameHealthMagic  = "TKMH"
)

// UsernameRegistrationRequest adds an authenticated registration envelope.
// Its record bytes are RLP encoded by the eth service and are opaque to relays.
func UsernameRegistrationRequest(record []byte) ([]byte, error) {
	if len(record) == 0 || len(record)+4 > MaxPayload {
		return nil, errors.New("invalid username registration payload size")
	}
	payload := make([]byte, 4+len(record))
	copy(payload, usernameRecordMagic)
	copy(payload[4:], record)
	return payload, nil
}

// UsernameCoverRequest asks a directory to return random registered aliases.
// It is fixed width so the request does not leak a count through payload size.
func UsernameCoverRequest() []byte {
	payload := make([]byte, MaxPayload)
	copy(payload, usernameCoverMagic)
	return payload
}

func IsUsernameCoverRequest(payload []byte) bool {
	if len(payload) != MaxPayload || string(payload[:4]) != usernameCoverMagic {
		return false
	}
	for _, b := range payload[4:] {
		if b != 0 {
			return false
		}
	}
	return true
}

func UsernameHealthRequest() []byte {
	payload := make([]byte, MaxPayload)
	copy(payload, usernameHealthMagic)
	return payload
}

func IsUsernameHealthRequest(payload []byte) bool {
	if len(payload) != MaxPayload || string(payload[:4]) != usernameHealthMagic {
		return false
	}
	for _, b := range payload[4:] {
		if b != 0 {
			return false
		}
	}
	return true
}

func UsernameCoverReply(names []string) ([]byte, error) {
	if len(names) > UsernameLookupFanout-1 {
		return nil, fmt.Errorf("username cover response allows at most %d names", UsernameLookupFanout-1)
	}
	return rlp.EncodeToBytes(names)
}

func DecodeUsernameCoverReply(payload []byte) ([]string, error) {
	var names []string
	if len(payload) == 0 || len(payload) > MaxPayload {
		return nil, errors.New("invalid username cover response size")
	}
	if err := rlp.DecodeBytes(payload, &names); err != nil || len(names) > UsernameLookupFanout-1 {
		return nil, errors.New("invalid username cover response")
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		canonical, err := canonicalLookupName(name)
		if err != nil || canonical != name {
			return nil, errors.New("invalid username cover name")
		}
		if _, ok := seen[name]; ok {
			return nil, errors.New("duplicate username cover name")
		}
		seen[name] = struct{}{}
	}
	return names, nil
}

func IsUsernameRegistrationRequest(payload []byte) ([]byte, bool) {
	if len(payload) <= 4 || len(payload) > MaxPayload || string(payload[:4]) != usernameRecordMagic {
		return nil, false
	}
	return payload[4:], true
}

func UsernameRecordReply(record []byte) ([]byte, error) {
	if len(record) == 0 || len(record)+4 > MaxPayload {
		return nil, errors.New("invalid username record reply size")
	}
	response := make([]byte, 4+len(record))
	copy(response, usernameReplyMagic)
	copy(response[4:], record)
	return response, nil
}

func DecodeUsernameRecordReply(payload []byte) ([]byte, error) {
	if len(payload) <= 4 || len(payload) > MaxPayload || string(payload[:4]) != usernameReplyMagic {
		return nil, errors.New("invalid username record response")
	}
	return append([]byte(nil), payload[4:]...), nil
}

// PinnedDirectoryPeer is a configured directory operator whose signing key
// fingerprint is pinned out of band. A descriptor's self-signature alone
// proves only key possession, not that the operator is trusted.
type PinnedDirectoryPeer struct {
	Descriptor    Descriptor
	SigningKeyPin [sha256.Size]byte
}

// UnmarshalTOML validates the fixed-size out-of-band signing-key pin while
// decoding peer configuration. This keeps malformed pins from being silently
// truncated or padded by a configuration parser.
func (p *PinnedDirectoryPeer) UnmarshalTOML(decode func(interface{}) error) error {
	if p == nil {
		return errors.New("tkmnet: nil pinned directory peer")
	}
	var raw struct {
		Descriptor    Descriptor
		SigningKeyPin []byte
	}
	if err := decode(&raw); err != nil {
		return err
	}
	if len(raw.SigningKeyPin) != sha256.Size {
		return fmt.Errorf("tkmnet: directory signing-key pin must contain %d bytes", sha256.Size)
	}
	p.Descriptor = raw.Descriptor
	copy(p.SigningKeyPin[:], raw.SigningKeyPin)
	return nil
}

// UsernameQuery is one independently routed member of a private lookup set.
// Send each query over its own onion circuit to a different operator. Never
// combine these into one request to a single directory server.
type UsernameQuery struct {
	Name string
	Peer Descriptor
}

// PlanPrivateUsernameLookup assigns each name to a different pinned onion
// directory in random order. A directory sees one queried name, not the
// target plus its cover names. The caller may use a smaller batch when the
// network has fewer than UsernameLookupFanout directory operators. This
// protects the target only if operators do
// not collude and the supplied cover names are real, independently selected
// aliases. The caller must send each result over a separate TKMNet circuit.
func PlanPrivateUsernameLookup(target string, names []string, peers []PinnedDirectoryPeer, nowUnix uint64, random io.Reader) ([]UsernameQuery, error) {
	if len(names) == 0 || len(names) > UsernameLookupFanout {
		return nil, fmt.Errorf("private username lookup needs 1 to %d names", UsernameLookupFanout)
	}
	canonicalTarget, err := canonicalLookupName(target)
	if err != nil || canonicalTarget != target {
		return nil, errors.New("private lookup target is not canonical")
	}
	if len(peers) < len(names) {
		return nil, fmt.Errorf("private username lookup has %d aliases but only %d directory operators", len(names), len(peers))
	}
	if random == nil {
		random = rand.Reader
	}
	seenNames := make(map[string]struct{}, len(names))
	targetIncluded := false
	for i, name := range names {
		canonical, err := canonicalLookupName(name)
		if err != nil || canonical != name {
			return nil, fmt.Errorf("private lookup name %d is not canonical", i)
		}
		if _, exists := seenNames[name]; exists {
			return nil, errors.New("private username lookup names must be distinct")
		}
		seenNames[name] = struct{}{}
		targetIncluded = targetIncluded || name == target
	}
	if !targetIncluded {
		return nil, errors.New("private lookup set does not contain its target")
	}
	selected := make([]PinnedDirectoryPeer, 0, len(peers))
	seenIDs := make(map[[LayerNextIDSize]byte]struct{}, len(peers))
	seenPins := make(map[[sha256.Size]byte]struct{}, len(peers))
	seenOnions := make(map[string]struct{}, len(peers))
	for _, peer := range peers {
		d := peer.Descriptor
		if err := d.Verify(time.Unix(int64(nowUnix), 0)); err != nil {
			return nil, fmt.Errorf("invalid username directory descriptor: %w", err)
		}
		fingerprint := sha256.Sum256(d.SigningPublicKey)
		if peer.SigningKeyPin == ([sha256.Size]byte{}) || peer.SigningKeyPin != fingerprint {
			return nil, errors.New("username directory signing key is not pinned")
		}
		if _, duplicate := seenIDs[d.ID]; duplicate {
			return nil, errors.New("username lookup peers must have distinct relay keys")
		}
		if _, duplicate := seenPins[peer.SigningKeyPin]; duplicate {
			return nil, errors.New("username lookup peers must have distinct pinned operators")
		}
		onion, _ := validateOnionHostname(d.Onion) // Verify already checked it.
		if _, duplicate := seenOnions[onion]; duplicate {
			return nil, errors.New("username lookup peers must have distinct onion services")
		}
		seenIDs[d.ID] = struct{}{}
		seenPins[peer.SigningKeyPin] = struct{}{}
		seenOnions[onion] = struct{}{}
		selected = append(selected, peer)
	}
	// Fisher-Yates with rejection sampling avoids modulo bias.
	for i := len(selected) - 1; i > 0; i-- {
		bound := uint64(i + 1)
		threshold := -bound % bound
		var value uint64
		for {
			var b [8]byte
			if _, err := io.ReadFull(random, b[:]); err != nil {
				return nil, fmt.Errorf("shuffle private lookup routes: %w", err)
			}
			value = uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 | uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
			if value >= threshold {
				break
			}
		}
		j := int(value % bound)
		selected[i], selected[j] = selected[j], selected[i]
	}
	queries := make([]UsernameQuery, len(names))
	for i := range queries {
		queries[i] = UsernameQuery{Name: names[i], Peer: selected[i].Descriptor}
	}
	return queries, nil
}

// BuildPrivateUsernameRequest constructs one three-relay packet. The last
// relay is the assigned directory operator; the first two must be separate
// onion relays. Build and transmit one such request per entry in a lookup
// plan. The directory is the only relay that can open the alias payload.
func BuildPrivateUsernameRequest(firstTwo []Descriptor, directory Descriptor, name string, now time.Time) ([]byte, error) {
	if len(firstTwo) != MaxHops-1 {
		return nil, fmt.Errorf("username lookup route requires exactly %d intermediate relays", MaxHops-1)
	}
	if err := directory.Verify(now); err != nil {
		return nil, fmt.Errorf("invalid username directory descriptor: %w", err)
	}
	payload, err := EncodeUsernameQuery(name)
	if err != nil {
		return nil, err
	}
	hops := make([]Hop, MaxHops)
	seenIDs := map[[LayerNextIDSize]byte]struct{}{directory.ID: {}}
	seenOnions := map[string]struct{}{strings.ToLower(directory.Onion): {}}
	for i, relay := range firstTwo {
		if err := relay.Verify(now); err != nil {
			return nil, fmt.Errorf("invalid intermediate relay descriptor %d: %w", i, err)
		}
		if _, duplicate := seenIDs[relay.ID]; duplicate {
			return nil, errors.New("username lookup route repeats a relay")
		}
		onion := strings.ToLower(relay.Onion)
		if _, duplicate := seenOnions[onion]; duplicate {
			return nil, errors.New("username lookup route repeats an onion service")
		}
		seenIDs[relay.ID] = struct{}{}
		seenOnions[onion] = struct{}{}
		hops[i] = Hop{ID: relay.ID, PublicKey: relay.PublicKey}
	}
	hops[MaxHops-1] = Hop{ID: directory.ID, PublicKey: directory.PublicKey}
	return Build(BuildOptions{Service: ServiceUsername, Hops: hops}, payload)
}

// EncodeUsernameQuery creates a full-width application payload. Padding keeps
// short aliases from changing the TKMNet packet size.
func EncodeUsernameQuery(name string) ([]byte, error) {
	canonical, err := canonicalLookupName(name)
	if err != nil || canonical != name {
		return nil, errors.New("username query is not canonical")
	}
	payload := make([]byte, MaxPayload)
	copy(payload[:4], usernameQueryMagic)
	payload[4] = Version
	payload[5] = byte(len(name))
	copy(payload[usernameQueryHeader:], name)
	return payload, nil
}

func DecodeUsernameQuery(payload []byte) (string, error) {
	if len(payload) != MaxPayload || string(payload[:4]) != usernameQueryMagic || payload[4] != Version {
		return "", errors.New("invalid TKMNet username query")
	}
	size := int(payload[5])
	if size < 3 || size > 32 || usernameQueryHeader+size > len(payload) {
		return "", errors.New("invalid TKMNet username query length")
	}
	name := string(payload[usernameQueryHeader : usernameQueryHeader+size])
	canonical, err := canonicalLookupName(name)
	if err != nil || canonical != name {
		return "", errors.New("invalid TKMNet username query name")
	}
	for _, b := range payload[usernameQueryHeader+size:] {
		if b != 0 {
			return "", errors.New("invalid TKMNet username query padding")
		}
	}
	return name, nil
}

func canonicalLookupName(name string) (string, error) {
	if len(name) < 3 || len(name) > 32 || strings.ToLower(name) != name || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return "", errors.New("invalid username")
	}
	previousHyphen := false
	for _, b := range []byte(name) {
		if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '-') {
			return "", errors.New("invalid username")
		}
		if b == '-' && previousHyphen {
			return "", errors.New("invalid username")
		}
		previousHyphen = b == '-'
	}
	return name, nil
}
