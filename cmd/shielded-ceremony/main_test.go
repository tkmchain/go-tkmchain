package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateLocalRequiresAcknowledgement(t *testing.T) {
	previous := selectedCircuit
	t.Cleanup(func() { selectedCircuit = previous })
	selectedCircuit = "shield6-recipient-v2"
	if err := generateLocal([]string{"-out-dir", t.TempDir()}); err == nil {
		t.Fatal("setup accepted without single-operator acknowledgement")
	}
}

func TestGenerateLocalRefusesExistingArtifacts(t *testing.T) {
	previous := selectedCircuit
	t.Cleanup(func() { selectedCircuit = previous })
	for _, circuit := range []string{"shield6-recipient-v2", "shield6-registration-v2"} {
		selectedCircuit = circuit
		prefix := "recipient"
		if circuit == "shield6-registration-v2" {
			prefix = "registration"
		}
		dir := t.TempDir()
		path := filepath.Join(dir, prefix+".pk")
		if err := os.WriteFile(path, []byte("existing public parameters"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := generateLocal([]string{"-single-operator", "-out-dir", dir}); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Fatalf("setup did not reject existing parameters: %v", err)
		}
	}
}
