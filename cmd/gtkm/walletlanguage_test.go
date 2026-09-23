package main

import (
	"bufio"
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWalletLanguageCatalogPriority(t *testing.T) {
	want := []string{"zh", "ru", "en", "ja", "ko"}
	if len(walletLanguageCatalog) < len(want) {
		t.Fatalf("language catalog has %d entries, want at least %d", len(walletLanguageCatalog), len(want))
	}
	for i, code := range want {
		if walletLanguageCatalog[i].Code != code {
			t.Fatalf("language %d is %q, want %q", i+1, walletLanguageCatalog[i].Code, code)
		}
	}
}

func TestWalletLanguagePreferenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := walletLanguagePreference{Language: "ja"}
	if err := saveWalletLanguage(dir, want); err != nil {
		t.Fatal(err)
	}
	got, ok := loadWalletLanguage(dir)
	if !ok {
		t.Fatal("saved preference was not loaded")
	}
	if got != want {
		t.Fatalf("loaded preference %#v, want %#v", got, want)
	}
}

func TestDetectWalletLanguage(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ko_KR.UTF-8")
	if got := detectWalletLanguage(); got.Code != "ko" {
		t.Fatalf("detected %q, want ko", got.Code)
	}
}

func TestChooseWalletLanguageAutoDetect(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ru_RU.UTF-8")
	preference, language, err := chooseWalletLanguage(bufio.NewReader(strings.NewReader("a\n")))
	if err != nil {
		t.Fatal(err)
	}
	if preference.Language != "auto" || language.Code != "ru" {
		t.Fatalf("got preference %#v and language %q, want auto and ru", preference, language.Code)
	}
}

func TestWalletDaemonTranslations(t *testing.T) {
	previous := walletActiveLanguage
	defer func() { walletActiveLanguage = previous }()
	walletActiveLanguage = walletLanguageByCode("zh")
	if got := walletText("daemon.starting", "fallback"); got != "正在启动 RandomX 主网…" {
		t.Fatalf("daemon startup text = %q", got)
	}
	walletActiveLanguage = walletLanguageByCode("ko")
	if got := walletText("daemon.language", "fallback"); got == "fallback" {
		t.Fatal("daemon language message did not resolve")
	}
}

func TestWalletLogTranslationHandler(t *testing.T) {
	var output bytes.Buffer
	handler := &walletLogTranslationHandler{next: slog.NewTextHandler(&output, nil), language: "zh"}
	record := slog.NewRecord(time.Unix(0, 0), slog.LevelInfo, "Started P2P networking", 0)
	if err := handler.Handle(nil, record); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "点对点网络已启动") {
		t.Fatalf("translated log output = %q", output.String())
	}
}
