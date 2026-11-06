package build

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractArchiveDirectorySelection(t *testing.T) {
	for _, format := range []string{"tar.gz", "zip"} {
		t.Run(format, func(t *testing.T) {
			base := t.TempDir()
			archive := filepath.Join(base, "fixtures."+format)
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			names := []string{"./fixtures/blockchain_tests/a.json", "fixtures/state_tests/b.json", "fixtures/transaction_tests/c.json", "fixtures/blockchain_tests_engine/a.json"}
			if format == "tar.gz" {
				gz := gzip.NewWriter(file)
				tw := tar.NewWriter(gz)
				for _, name := range names {
					if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 2}); err != nil {
						t.Fatal(err)
					}
					if _, err := tw.Write([]byte("{}")); err != nil {
						t.Fatal(err)
					}
				}
				if err := tw.Close(); err != nil {
					t.Fatal(err)
				}
				if err := gz.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				zw := zip.NewWriter(file)
				for _, name := range names {
					writer, err := zw.Create(name)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := writer.Write([]byte("{}")); err != nil {
						t.Fatal(err)
					}
				}
				if err := zw.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(base, "selected")
			if err := ExtractArchive(archive, destination, "fixtures/blockchain_tests", "fixtures/state_tests", "fixtures/transaction_tests"); err != nil {
				t.Fatal(err)
			}
			for _, name := range names[:3] {
				data, err := os.ReadFile(filepath.Join(destination, name))
				if err != nil || string(data) != "{}" {
					t.Fatalf("selected fixture missing or changed: %s: %v", name, err)
				}
			}
			if _, err := os.Stat(filepath.Join(destination, names[3])); !os.IsNotExist(err) {
				t.Fatalf("unselected sibling directory extracted: %v", err)
			}
		})
	}
}
