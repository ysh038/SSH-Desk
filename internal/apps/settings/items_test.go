package settings

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tuidesk/internal/config"
)

func testItems() []Item {
	return Items(About{Version: "v1", Build: "lite", ConfigPath: "/home/kim/.config/tuidesk/config.toml",
		Details: [][2]string{{"Terminal type", "xterm-256color"}}})
}

func TestItemsAreWellFormed(t *testing.T) {
	items := testItems()
	seen := map[string]bool{}
	perSection := map[string]int{}
	cfg := config.Default()
	for _, it := range items {
		if it.ID == "" || seen[it.ID] {
			t.Errorf("missing or repeated id %q", it.ID)
		}
		seen[it.ID] = true
		if !slices.Contains(Sections, it.Section) {
			t.Errorf("%s: unknown section %q", it.ID, it.Section)
		}
		perSection[it.Section]++
		if it.Label == "" || it.Help == "" {
			t.Errorf("%s: label and help are required", it.ID)
		}
		switch it.Kind {
		case Toggle:
			if it.Bool == nil {
				t.Errorf("%s: toggle without a value", it.ID)
			}
		case Choice:
			if it.Str == nil || len(it.Options) < 2 {
				t.Errorf("%s: choice without options", it.ID)
			}
		case Number:
			if it.Int == nil || it.Min >= it.Max || it.Step <= 0 || it.Unit == "" {
				t.Errorf("%s: number without a range", it.ID)
			} else if v := *it.Int(&cfg); v < it.Min || v > it.Max {
				t.Errorf("%s: default %d is outside %d-%d", it.ID, v, it.Min, it.Max)
			}
		case Text:
			if it.Str == nil || it.Check == nil || it.Empty == "" {
				t.Errorf("%s: text without a check", it.ID)
			}
		case Action:
			if it.Button == "" || it.ActionKey == "" {
				t.Errorf("%s: action without a button", it.ID)
			}
		}
		if it.Kind != Probe && it.Display(cfg) == "" {
			t.Errorf("%s: nothing to show", it.ID)
		}
	}
	for _, s := range Sections {
		if perSection[s] == 0 {
			t.Errorf("section %q is empty", s)
		}
	}
}

func ids(items []Item, idx []int) []string {
	var out []string
	for _, i := range idx {
		out = append(out, items[i].ID)
	}
	return out
}

func TestSearchFindsSettingsByEverydayWords(t *testing.T) {
	items := testItems()
	for query, want := range map[string]string{
		"mount":          "mount_dir",
		"usb location":   "mount_dir", // every word must match, in any order
		"CLOCK":          "clock",
		"am pm":          "clock",
		"hangul":         "names",
		"double":         "double_click",
		"dotfiles":       "show_hidden",
		"bash":           "shell",
		"osc52":          "clipboard",
		"icon view":      "files_view", // a choice label
		"privacy root":   "warn_root",  // section name + label
		"reset":          "reset",
		"terminal type":  "about:Terminal type",
		"recycle":        "confirm_trash",
		"notifications ": "notify_seconds",
	} {
		if got := ids(items, Search(items, query)); !slices.Contains(got, want) {
			t.Errorf("search %q = %v, want %s", query, got, want)
		}
	}
	if got := ids(items, Search(items, "clock")); got[0] != "clock" {
		t.Errorf("a match in the name must come first: %v", got)
	}
	for _, query := range []string{"", "   ", "zzzz", "mount zzzz"} {
		if got := Search(items, query); len(got) != 0 {
			t.Errorf("search %q found %v", query, ids(items, got))
		}
	}
}

func TestToggleNumberAndDisplay(t *testing.T) {
	items := testItems()
	by := func(id string) Item {
		for _, it := range items {
			if it.ID == id {
				return it
			}
		}
		t.Fatalf("no item %s", id)
		return Item{}
	}
	c := config.Default()
	by("clock_seconds").Flip(&c)
	if !c.ClockSeconds || by("clock_seconds").Display(c) != "[x] On" {
		t.Fatal("toggle did not turn on")
	}
	w := by("wheel")
	w.SetNumber(&c, 99)
	if c.WheelLines != config.MaxWheelLines || w.Display(c) != "10 lines" {
		t.Fatalf("number not clamped: %d %q", c.WheelLines, w.Display(c))
	}
	if _, err := w.ParseNumber("abc"); err == nil {
		t.Error("abc accepted as a number")
	}
	if _, err := w.ParseNumber("0"); err == nil {
		t.Error("0 accepted")
	}
	if n, err := w.ParseNumber(" 5 "); err != nil || n != 5 {
		t.Errorf("5 → %d, %v", n, err)
	}
	if got := by("mount_dir").Display(c); !strings.HasPrefix(got, "Default") {
		t.Errorf("empty mount folder shows %q", got)
	}
	if got := by("keyboard").Display(c); got != "PC" {
		t.Errorf("keyboard not chosen shows %q", got)
	}
}

func TestMountFolderAndShellAreChecked(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "mnt", "오프라인")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "notes.txt")
	prog := filepath.Join(root, "myshell")
	os.WriteFile(file, []byte("x"), 0o644)
	os.WriteFile(prog, []byte("#!/bin/sh\n"), 0o755)

	if got, err := checkFolder(dir + "/"); err != nil || got != dir {
		t.Errorf("existing folder: %q, %v", got, err)
	}
	if got, err := checkFolder("  "); err != nil || got != "" {
		t.Errorf("empty = default: %q, %v", got, err)
	}
	for _, bad := range []string{"mnt/offline", "/", filepath.Join(root, "missing"), file} {
		if _, err := checkFolder(bad); err == nil {
			t.Errorf("folder %q accepted", bad)
		}
	}
	if got, err := checkProgram(prog); err != nil || got != prog {
		t.Errorf("program: %q, %v", got, err)
	}
	for _, bad := range []string{"bash", file, dir, filepath.Join(root, "missing")} {
		if _, err := checkProgram(bad); err == nil {
			t.Errorf("shell %q accepted", bad)
		}
	}
}

func TestWrap(t *testing.T) {
	got := Wrap("한글 이름은 폭이 두 칸입니다 and English words follow after them", 20, 2)
	if len(got) != 2 || got[0] != "한글 이름은 폭이 두" {
		t.Fatalf("wrap = %q", got)
	}
	if got := Wrap("short", 20, 2); len(got) != 1 || got[0] != "short" {
		t.Fatalf("wrap = %q", got)
	}
}
