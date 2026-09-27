package download

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChecksumsRejectsUnsafeEntries(t *testing.T) {
	if _, err := ParseChecksums([]byte("not-a-hash file.bin\n")); err == nil {
		t.Fatal("accepted a malformed digest")
	}
	digest := strings.Repeat("0", sha256.Size*2)
	if _, err := ParseChecksums([]byte(digest + " ../escape.bin\n")); err == nil {
		t.Fatal("accepted a path traversal checksum entry")
	}
}

func TestDownloadFileVerifiesAndRejectsSymlinkDestination(t *testing.T) {
	body := []byte("verified download")
	digest := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifact.bin" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()

	db, err := ParseChecksums([]byte(hex.EncodeToString(digest[:]) + " artifact.bin\n"))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "artifact.bin")
	if err := db.DownloadFile(server.URL+"/artifact.bin", dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != string(body) {
		t.Fatalf("downloaded body = %q, err=%v", got, err)
	}

	link := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.Symlink(dst, link); err != nil {
		t.Fatal(err)
	}
	if err := db.DownloadFile(server.URL+"/artifact.bin", link); err == nil {
		t.Fatal("download followed a symlink destination")
	}
}
