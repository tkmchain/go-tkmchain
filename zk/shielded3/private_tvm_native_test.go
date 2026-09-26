//go:build shield3 && cgo

package shielded3

import (
	"context"
	"encoding/binary"
	"testing"
)

func privateTVMTestDigest(t *testing.T, words ...uint64) Digest {
	t.Helper()
	d, err := HashWords(words)
	if err != nil {
		t.Fatalf("HashWords: %v", err)
	}
	return d
}

func privateTVMTestRoot(t *testing.T, leaf Digest, index uint32, path [PrivateTVMPathDepth]Digest) Digest {
	t.Helper()
	root := leaf
	for level, sibling := range path {
		var err error
		if index&(1<<uint(level)) == 0 {
			root, err = HashPair(root, sibling)
		} else {
			root, err = HashPair(sibling, root)
		}
		if err != nil {
			t.Fatalf("HashPair: %v", err)
		}
	}
	return root
}

func TestNativePrivateTVMStateTransitionProof(t *testing.T) {
	code := Digest{11, 22, 33, 44, 55}
	key := Digest{101, 102, 103, 104, 105}
	oldValue := Digest{201, 202, 203, 204, 205}
	var path [PrivateTVMPathDepth]Digest
	for i := range path {
		path[i] = privateTVMTestDigest(t, 7000, uint64(i))
	}
	const index = uint32(17)
	oldLeaf := privateTVMTestDigest(t, privateTVMLeafDomain, code[0], code[1], code[2], code[3], code[4], key[0], key[1], key[2], key[3], key[4], oldValue[0], oldValue[1], oldValue[2], oldValue[3], oldValue[4])
	oldRoot := privateTVMTestRoot(t, oldLeaf, index, path)

	var intent [64]byte
	for i := 0; i < len(intent); i += 4 {
		binary.BigEndian.PutUint32(intent[i:i+4], uint32(900+i))
	}
	newValue, err := PrivateTVMExpectedWriteValue(code, key, oldValue, intent)
	if err != nil {
		t.Fatalf("expected write value: %v", err)
	}
	newLeaf := privateTVMTestDigest(t, privateTVMLeafDomain, code[0], code[1], code[2], code[3], code[4], key[0], key[1], key[2], key[3], key[4], newValue[0], newValue[1], newValue[2], newValue[3], newValue[4])
	newRoot := privateTVMTestRoot(t, newLeaf, index, path)
	statement := PrivateTVMStatement{ChainID: 8979, CodeHash: code, OldRoot: oldRoot, NewRoot: newRoot, Intent: intent, Operation: 1}
	witness := PrivateTVMWitness{CodeHash: code, Key: key, OldValue: oldValue, NewValue: newValue, LeafIndex: index, Path: path}

	proof, err := (NativeBackend{}).ProvePrivateTVM(context.Background(), statement, witness)
	if err != nil {
		t.Fatalf("prove private TVM transition: %v", err)
	}
	if err := (NativeBackend{}).VerifyPrivateTVM(context.Background(), statement, proof); err != nil {
		t.Fatalf("verify private TVM transition: %v", err)
	}
	changed := statement
	changed.NewRoot[0] ^= 1
	if err := (NativeBackend{}).VerifyPrivateTVM(context.Background(), changed, proof); err == nil {
		t.Fatal("accepted proof after new-root substitution")
	}
}
