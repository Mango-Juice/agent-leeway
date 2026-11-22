package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLanguageOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_CONFIG", dir)
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")
	if l := Language(""); l != "en" {
		t.Errorf("default: %s", l)
	}
	t.Setenv("LANG", "ko_KR.UTF-8")
	if l := Language(""); l != "ko" {
		t.Errorf("LANG: %s", l)
	}
	t.Setenv("LC_ALL", "en_US.UTF-8")
	if l := Language(""); l != "en" {
		t.Errorf("LC_ALL before LANG: %s", l)
	}
	t.Setenv("LC_ALL", "C.UTF-8") // not a report language: falls through to LANG
	if l := Language(""); l != "ko" {
		t.Errorf("unsupported LC_ALL: %s", l)
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"language":"en"}`), 0o600)
	if l := Language(""); l != "en" {
		t.Errorf("settings before env: %s", l)
	}
	if l := Language("ko"); l != "ko" {
		t.Errorf("flag first: %s", l)
	}
	if ValidLanguage("fr") || !ValidLanguage("ko-KR") || !ValidLanguage("") {
		t.Error("ValidLanguage")
	}
}
