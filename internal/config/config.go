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
	"strings"

	"github.com/BurntSushi/toml"
)

// Validator runs a command after saving files that match Pattern
// (SPEC 7.3 on-save validator). Pattern supports ** for any depth.
type Validator struct {
	Pattern string `toml:"pattern"`
	Command string `toml:"command"`
	Sudo    bool   `toml:"sudo"` // run with sudo when the file needed sudo to save
}

// DesktopIcon is a desktop shortcut (SPEC 7.1): a folder or file (Path,
// "~" = home), a built-in app (App: "terminal", "editor", "files", "mypc", "tasks", "help") or
// an app manifest by name (SPEC 8.3).
type DesktopIcon struct {
	Name string `toml:"name"`
	Path string `toml:"path,omitempty"`
	App  string `toml:"app,omitempty"`
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
	// Icons are the desktop shortcuts. IconsEdited records that the user
	// changed them, so an empty list stays empty instead of the defaults.
	Icons       []DesktopIcon `toml:"desktop_icon"`
	IconsEdited bool          `toml:"desktop_icons_edited"`

	// ---- Settings window (SPEC 7.7) ----

	// Appearance.
	DesktopIcons bool `toml:"show_desktop_icons"`
	ShowHost     bool `toml:"show_host_name"`
	// Names is how file names with non-ASCII characters (Hangul) are
	// drawn: "auto" (escaped as \uXXXX on the Linux console only), "show"
	// or "escape".
	Names string `toml:"non_ascii_names"`

	// Date & Time: the clock in the top bar. Clock is "24h" or "12h".
	Clock        string `toml:"clock"`
	ClockSeconds bool   `toml:"clock_seconds"`
	ClockDate    bool   `toml:"clock_date"`

	// Mouse.
	DoubleClickMs int `toml:"double_click_ms"`
	WheelLines    int `toml:"wheel_lines"`

	// Notifications.
	NotifySeconds int  `toml:"notify_seconds"`
	NotifyDrives  bool `toml:"notify_drives"`

	// Files: how a new file window starts. FilesView is "list" or "icons",
	// FilesSort is "name", "size" or "date".
	ShowHidden   bool   `toml:"show_hidden"`
	FilesView    string `toml:"files_view"`
	FilesSort    string `toml:"files_sort"`
	ConfirmTrash bool   `toml:"confirm_trash"`

	// Terminal. Shell "" = $SHELL.
	Shell      string `toml:"shell"`
	Scrollback int    `toml:"scrollback"`

	// Storage & USB. MountDir is the folder removable drives are mounted
	// on ("" = /media/<user>/<label>, ADR 0005).
	MountDir      string `toml:"removable_mount_dir"`
	MountReadOnly bool   `toml:"mount_read_only"`

	// TaskSeconds is how often the Task Manager updates: 1, 2 or 5
	// seconds, 0 = paused (SPEC 7.9).
	TaskSeconds int `toml:"task_manager_seconds"`

	// Privacy & Security. Clipboard lets Copy reach the user's own
	// computer (OSC 52).
	Clipboard bool `toml:"copy_to_local_clipboard"`
	WarnRoot  bool `toml:"warn_root"`
}

// Limits of the numeric settings.
const (
	MinDoubleClickMs, MaxDoubleClickMs = 200, 1500
	MinWheelLines, MaxWheelLines       = 1, 10
	MinNotifySeconds, MaxNotifySeconds = 2, 30
	MinScrollback, MaxScrollback       = 100, 100000
	MinTabSize, MaxTabSize             = 1, 16
)

func oneOf(v, def string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

func within(v, def, lo, hi int) int {
	if v < lo || v > hi {
		return def
	}
	return v
}

// Normalize replaces values a hand-edited file got wrong with the defaults.
func (c *Config) Normalize() {
	d := Default()
	c.Keyboard = oneOf(c.Keyboard, "", "mac", "pc")
	c.TabSize = within(c.TabSize, d.TabSize, MinTabSize, MaxTabSize)
	c.Names = oneOf(c.Names, d.Names, "auto", "show", "escape")
	c.Clock = oneOf(c.Clock, d.Clock, "24h", "12h")
	c.DoubleClickMs = within(c.DoubleClickMs, d.DoubleClickMs, MinDoubleClickMs, MaxDoubleClickMs)
	c.WheelLines = within(c.WheelLines, d.WheelLines, MinWheelLines, MaxWheelLines)
	c.NotifySeconds = within(c.NotifySeconds, d.NotifySeconds, MinNotifySeconds, MaxNotifySeconds)
	c.FilesView = oneOf(c.FilesView, d.FilesView, "list", "icons")
	c.FilesSort = oneOf(c.FilesSort, d.FilesSort, "name", "size", "date")
	c.Scrollback = within(c.Scrollback, d.Scrollback, MinScrollback, MaxScrollback)
	if c.TaskSeconds != 0 && c.TaskSeconds != 1 && c.TaskSeconds != 5 {
		c.TaskSeconds = 2
	}
	c.Shell = strings.TrimSpace(c.Shell)
	c.MountDir = CleanMountDir(c.MountDir)
}

// CleanMountDir tidies a mount folder setting; anything that is not an
// absolute path below / becomes "" (the default location).
func CleanMountDir(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		return ""
	}
	p = filepath.Clean(p)
	if p == "/" {
		return ""
	}
	return p
}

// Reset returns the defaults but keeps what is not shown in the Settings
// window: the keyboard choice, validators and desktop icons.
func (c Config) Reset() Config {
	d := Default()
	d.Keyboard, d.Validators, d.Icons, d.IconsEdited = c.Keyboard, c.Validators, c.Icons, c.IconsEdited
	return d
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		HintBar:    true,
		Backup:     true,
		TabSize:    4,
		ExpandTabs: true,

		DesktopIcons:  true,
		ShowHost:      true,
		Names:         "auto",
		Clock:         "24h",
		DoubleClickMs: 500, // compat-matrix finding 7
		WheelLines:    3,
		NotifySeconds: 4,
		NotifyDrives:  true,
		FilesView:     "list",
		FilesSort:     "name",
		Scrollback:    5000,
		TaskSeconds:   2,
		Clipboard:     true,
		WarnRoot:      true,
		Validators: []Validator{
			{Pattern: "/etc/nginx/**", Command: "nginx -t", Sudo: true},
			{Pattern: "/etc/systemd/system/*", Command: "systemd-analyze verify {file}", Sudo: false},
		},
		Icons: []DesktopIcon{
			{Name: "Home", Path: "~"},
			{Name: "Root /", Path: "/"},
			{Name: "/etc", Path: "/etc"},
			{Name: "Logs", Path: "/var/log"},
			{Name: "Terminal", App: "terminal"},
			{Name: "Text editor", App: "editor"},
			{Name: "My PC", App: "mypc"},
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
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return Default(), true, err
	}
	if cfg.IconsEdited && !md.IsDefined("desktop_icon") {
		cfg.Icons = nil // the user removed every icon
	}
	cfg.Normalize()
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
