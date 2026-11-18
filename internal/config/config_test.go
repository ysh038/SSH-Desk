package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDesktopIcons(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.toml")
	if got, _, _ := Load(p); len(got.Icons) != 7 || got.Icons[0].Path != "~" || got.Icons[4].App != "terminal" || got.Icons[6].App != "mypc" {
		t.Fatalf("default icons %+v", got.Icons)
	}
	// A file with its own icons replaces the defaults (not appended).
	os.WriteFile(p, []byte("[[desktop_icon]]\nname = \"웹 설정\"\npath = \"/etc/nginx\"\n"), 0o644)
	if got, _, _ := Load(p); len(got.Icons) != 1 || got.Icons[0].Name != "웹 설정" {
		t.Fatalf("user icons %+v", got.Icons)
	}
	// Removing every icon survives a save/load.
	cfg := Default()
	cfg.Icons, cfg.IconsEdited = nil, true
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := Load(p); len(got.Icons) != 0 {
		t.Fatalf("emptied icons came back: %+v", got.Icons)
	}
}

func TestLoadApps(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "lazygit.toml"), []byte("icon = \"G\"\ncommand = \"lazygit\"\ncwd = \"{selected_dir}\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "top.toml"), []byte("name = \"시스템 모니터\"\ncommand = \"htop\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "bad.toml"), []byte("name = \"x\"\n"), 0o644)
	apps, errs := LoadApps(dir)
	if len(apps) != 2 || apps[0].Name != "lazygit" || apps[0].Cwd != "{selected_dir}" || apps[1].Name != "시스템 모니터" {
		t.Fatalf("apps %+v", apps)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "bad.toml") {
		t.Fatalf("errs %v", errs)
	}
	if apps, errs := LoadApps(filepath.Join(dir, "none")); len(apps)+len(errs) != 0 {
		t.Fatal("missing folder is fine")
	}
}

func TestNormalizeRepairsBadValues(t *testing.T) {
	c := Default()
	c.Clock, c.Names, c.FilesView, c.FilesSort = "25h", "?", "tiles", "kind"
	c.DoubleClickMs, c.WheelLines, c.NotifySeconds, c.Scrollback, c.TabSize = 5, 0, 999, -1, 99
	c.MountDir = "mnt/offline"
	c.Normalize()
	d := Default()
	d.MountDir = ""
	if c.Clock != d.Clock || c.Names != d.Names || c.FilesView != d.FilesView || c.FilesSort != d.FilesSort ||
		c.DoubleClickMs != d.DoubleClickMs || c.WheelLines != d.WheelLines || c.NotifySeconds != d.NotifySeconds ||
		c.Scrollback != d.Scrollback || c.TabSize != d.TabSize || c.MountDir != "" {
		t.Fatalf("not repaired: %+v", c)
	}
}

func TestCleanMountDir(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "  /mnt/offline/ ": "/mnt/offline", "/": "", "relative/dir": "", "/mnt//한글 드라이브/.": "/mnt/한글 드라이브",
		"/mnt/a/../offline": "/mnt/offline",
	} {
		if got := CleanMountDir(in); got != want {
			t.Errorf("CleanMountDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewSettingsSurviveSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	c := Default()
	c.MountDir, c.Clock, c.ClockSeconds, c.DesktopIcons, c.Shell = "/mnt/offline", "12h", true, false, "/bin/zsh"
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.MountDir != "/mnt/offline" || got.Clock != "12h" || !got.ClockSeconds || got.DesktopIcons || got.Shell != "/bin/zsh" {
		t.Fatalf("lost settings: %+v", got)
	}
	// An older file without the new keys gets the defaults, not zero values.
	if err := os.WriteFile(path, []byte("keyboard = \"pc\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old, _, _ := Load(path)
	if !old.DesktopIcons || old.DoubleClickMs != 500 || !old.Clipboard || old.Clock != "24h" {
		t.Fatalf("defaults missing for an old file: %+v", old)
	}
}

func TestResetKeepsIconsAndKeyboard(t *testing.T) {
	c := Default()
	c.Keyboard, c.Icons, c.IconsEdited, c.WheelLines, c.MountDir = "mac", nil, true, 9, "/mnt/offline"
	r := c.Reset()
	if r.Keyboard != "mac" || r.Icons != nil || !r.IconsEdited || r.WheelLines != 3 || r.MountDir != "" {
		t.Fatalf("reset: %+v", r)
	}
}
