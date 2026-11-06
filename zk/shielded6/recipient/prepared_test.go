package recipient

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type artifactTestReader struct {
	called bool
	read   func(io.Reader) (int64, error)
}

func (r *artifactTestReader) ReadFrom(input io.Reader) (int64, error) {
	r.called = true
	return r.read(input)
}

// This guard is particularly important for the trusted proving-key decoder:
// a self-selected or corrupted file must fail authentication before decoding.
func TestRecipientPinnedArtifactDecodeBoundary(t *testing.T) {
	data := []byte("authenticated setup artifact")
	for _, name := range []string{"valid", "wrong pin", "changed file", "trailing bytes"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			pin := sha256.Sum256(data)
			if name == "wrong pin" {
				pin[0] ^= 1
			}
			reader := &artifactTestReader{read: func(input io.Reader) (int64, error) {
				if name == "changed file" {
					changed := append([]byte(nil), data...)
					changed[0] ^= 1
					if err := os.WriteFile(path, changed, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if name == "trailing bytes" {
					return io.CopyN(io.Discard, input, int64(len(data)-1))
				}
				return io.Copy(io.Discard, input)
			}}
			err := readPinnedObject(path, pin, 1024, reader)
			if name == "valid" && err != nil {
				t.Fatal(err)
			}
			if name != "valid" && err == nil {
				t.Fatal("accepted unauthenticated, changed or incompletely decoded artifact")
			}
			if reader.called != (name != "wrong pin") {
				t.Fatal("artifact reached decoder before its hash was authenticated")
			}
		})
	}
}
