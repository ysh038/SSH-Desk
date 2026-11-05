// Package config reads and writes ~/.config/tuidesk/config.toml (SPEC 7.7).
// Missing files and keys fall back to defaults; unknown keys are ignored so
// older binaries keep working with newer files.
package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Validator runs a command after saving files that match Pattern
// (SPEC 7.3 on-save validator). Pattern supports ** for any depth.
type Validator struct {
	Pattern string `toml:"pattern"`
	Command string `toml:"command"`
	Sudo    bool   `toml:"sudo"` // run with sudo when the file needed sudo to save
}

// Config is the user configuration.
type Config struct {
	// Keyboard is "mac" or "pc" (ADR 0003 A); empty = not chosen yet.
	Keyboard string `toml:"keyboard"`
	// HintBar shows the bottom shortcut line (ADR 0003 D).
	HintBar bool `toml:"hint_bar"`
	// Backup writes name.bak before overwriting a file in the editor.
	Backup bool `toml:"backup"`
	// TabSize and ExpandTabs are editor defaults (.editorconfig overrides).
	TabSize    int  `toml:"tab_size"`
	ExpandTabs bool `toml:"expand_tabs"`
	// Validators run after saving matching files.
	Validators []Validator `toml:"validator"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		HintBar:    true,
		Backup:     true,
		TabSize:    4,
		ExpandTabs: true,
		Validators: []Validator{
			{Pattern: "/etc/nginx/**", Command: "nginx -t", Sudo: true},
			{Pattern: "/etc/systemd/system/*", Command: "systemd-analyze verify {file}", Sudo: false},
		},
	}
}

// Path returns $XDG_CONFIG_HOME/tuidesk/config.toml or ~/.config/tuidesk/config.toml.
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tuidesk", "config.toml")
}

// Load reads path; a missing file yields the defaults (and exists=false).
func Load(path string) (cfg Config, exists bool, err error) {
	cfg = Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, err
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Default(), true, err
	}
	if cfg.TabSize <= 0 || cfg.TabSize > 16 {
		cfg.TabSize = 4
	}
	return cfg, true, nil
}

// Save writes cfg to path atomically (temp file + rename), creating the folder.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# tuidesk settings (see docs/SPEC.md 7.7)\n")
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
