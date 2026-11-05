package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingGivesDefaults(t *testing.T) {
	cfg, exists, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || exists {
		t.Fatalf("err=%v exists=%v", err, exists)
	}
	if !cfg.HintBar || cfg.TabSize != 4 || len(cfg.Validators) == 0 || cfg.Keyboard != "" {
		t.Fatalf("defaults %+v", cfg)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tuidesk", "config.toml")
	cfg := Default()
	cfg.Keyboard = "mac"
	cfg.HintBar = false
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, exists, err := Load(p)
	if err != nil || !exists || got.Keyboard != "mac" || got.HintBar || got.Validators[0].Command != "nginx -t" {
		t.Fatalf("got %+v exists=%v err=%v", got, exists, err)
	}
}

func TestPartialFileKeepsOtherDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(p, []byte("keyboard = \"pc\"\nunknown_key = 1\n"), 0o644)
	got, _, err := Load(p)
	if err != nil || got.Keyboard != "pc" || !got.HintBar || got.TabSize != 4 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestBadFileReportsErrorAndUsesDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(p, []byte("keyboard = \n"), 0o644)
	got, _, err := Load(p)
	if err == nil || got.TabSize != 4 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
