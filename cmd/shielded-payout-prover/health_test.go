package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHealthDoesNotExposePrivateConfiguration(t *testing.T) {
	p := &Prover{
		cfg: Config{
			Listen:           "127.0.0.1:8787",
			NodeRPC:          "http://127.0.0.1:8545",
			SignerAddress:    "0x1234",
			ProvingKeyPath:   "/private/proving.key",
			ProvingKeyV2Path: "/private/proving-v2.key",
			NotesPath:        "/private/notes.json",
			RequestsPath:     "/private/requests",
		},
		startupErr: "/private/proving.key: permission denied",
	}
	recorder := httptest.NewRecorder()
	p.handleHealth(recorder, httptest.NewRequest("GET", "/healthz", nil))

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	for _, key := range []string{
		"listen", "nodeRPC", "signerAddress", "provingKeyPath", "provingKeyV2Path",
		"notesPath", "requestsPath", "noteCount", "availableNoteCount",
		"availableNoteTotalWei", "availableNoteMaxWei", "noteInventoryError",
	} {
		if _, ok := body[key]; ok {
			t.Fatalf("health response exposes private field %q", key)
		}
	}
	if got, want := body["startupError"], "proof builder is not ready"; got != want {
		t.Fatalf("startupError = %v, want %q", got, want)
	}
}
