package antartical

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestBlockHashAnchorRoundTripAndRollingCommitment(t *testing.T) {
	parent := common.HexToHash("0x1234")
	previous := common.HexToHash("0xabcd")
	anchor := BlockHashAnchor{
		Height:  41_913,
		Hash:    parent,
		Rolling: BlockHashAnchorCommitment(previous, 41_913, parent),
	}
	extra, err := AttachBlockHashAnchor([]byte("RK\x03"), anchor)
	if err != nil {
		t.Fatal(err)
	}
	if len(extra) != len("RK\x03")+BlockHashAnchorEncodedSize {
		t.Fatalf("anchor envelope length = %d", len(extra))
	}
	got, found, err := BlockHashAnchorFromHeaderExtra(extra)
	if err != nil || !found {
		t.Fatalf("parse anchor: found=%v err=%v", found, err)
	}
	if got != anchor {
		t.Fatalf("round trip anchor = %#v, want %#v", got, anchor)
	}
	if err := ValidateBlockHashAnchor(got, 41_914, parent, previous); err != nil {
		t.Fatalf("valid anchor rejected: %v", err)
	}
}

func TestBlockHashAnchorRejectsMalformedAndConflictingMetadata(t *testing.T) {
	anchor := BlockHashAnchor{Height: 2, Hash: common.HexToHash("0x99")}
	extra, err := AttachBlockHashAnchor(nil, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BlockHashAnchorFromHeaderExtra(append([]byte("prefix"), extra...)); err != nil {
		t.Fatal("a valid prefix should remain parseable")
	}
	malformed := append([]byte(nil), extra...)
	malformed[0] = 'X'
	if _, found, err := BlockHashAnchorFromHeaderExtra(malformed); err != nil || found {
		t.Fatalf("non-anchor metadata parsed as anchor: found=%v err=%v", found, err)
	}
	duplicate := append(append([]byte(nil), extra...), extra...)
	if _, _, err := BlockHashAnchorFromHeaderExtra(duplicate); err == nil {
		t.Fatal("duplicate anchor accepted")
	}
	if err := ValidateBlockHashAnchor(anchor, 3, common.HexToHash("0x98"), common.Hash{}); err == nil {
		t.Fatal("conflicting parent accepted")
	}
	full := bytes.Repeat([]byte{0}, int(params.AntarticalMaximumExtraDataSize))
	if _, err := AttachBlockHashAnchor(full, anchor); err == nil {
		t.Fatal("full non-empty envelope accepted")
	}
}
