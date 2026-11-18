package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tuidesk/internal/apps/settings"
	"tuidesk/internal/config"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

type setHarness struct {
	*fmHarness
	path string
}

func newSettings(t *testing.T) *setHarness {
	f := newFM(t)
	path := filepath.Join(f.root, "config", "config.toml")
	cfg := config.Default()
	cfg.Keyboard = "pc" // no first-run question
	f.d.SetConfig(cfg, path)
	f.d.NewSettings = func() wm.App {
		return settings.New(f.d, settings.About{Version: "v-test", Build: "lite", ConfigPath: path})
	}
	wheel := ui.WheelLines
	t.Cleanup(func() { ui.WheelLines = wheel })
	return &setHarness{fmHarness: f, path: path}
}

func (h *setHarness) app() (*settings.App, *wm.Window) {
	h.t.Helper()
	for _, w := range h.d.M.Windows() {
		if a, ok := w.App.(*settings.App); ok {
			return a, w
		}
	}
	h.t.Fatal("no Settings window")
	return nil, nil
}

func (h *setHarness) saved() string {
	h.t.Helper()
	b, err := os.ReadFile(h.path)
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

// rowY finds the screen row that shows text.
func (h *setHarness) rowY(text string) int {
	h.t.Helper()
	c := h.draw()
	for y := 0; y < c.H; y++ {
		if strings.Contains(c.Row(y), text) {
			return y
		}
	}
	h.t.Fatalf("screen does not show %q", text)
	return -1
}

// Keyboard only: open Settings from the launcher, find a setting by typing,
// change it, and see it applied and saved at once.
func TestSettingsKeyboardSearchAndChange(t *testing.T) {
	h := newSettings(t)
	h.key("F9", 0)
	h.rune('s', 0) // Settings
	a, _ := h.app()
	for _, s := range settings.Sections {
		if !h.screenHas(s) {
			t.Fatalf("section %q is not listed", s)
		}
	}

	h.typeText("clock")
	if a.Query() != "clock" || a.Selected() != "clock" {
		t.Fatalf("typing did not search: query %q, shown %v", a.Query(), a.Shown())
	}
	if !h.screenHas("Date & Time > Clock format") {
		t.Fatal("results do not say which group a setting is in")
	}
	h.key("Down", 0) // from the search field to the results
	h.key("Enter", 0)
	if !h.d.MenuOpen() {
		t.Fatal("Enter on a choice opens its options")
	}
	h.key("Down", 0)
	h.key("Enter", 0)
	if h.d.Config().Clock != "12h" || !strings.Contains(h.saved(), `clock = "12h"`) {
		t.Fatalf("clock = %q, file:\n%s", h.d.Config().Clock, h.saved())
	}
	if !h.screenHas("9:30 AM") {
		t.Fatalf("top bar: %q", h.draw().Row(0))
	}

	// Esc leaves the search; sections with Up/Down, a number with + and -.
	h.key("Esc", 0)
	if a.Query() != "" {
		t.Fatal("Esc did not clear the search")
	}
	a.Show("Mouse", "wheel")
	h.rune('+', 0)
	h.rune('+', 0)
	h.rune('-', 0)
	if h.d.Config().WheelLines != 4 || ui.WheelLines != 4 {
		t.Fatalf("wheel lines %d (applied %d)", h.d.Config().WheelLines, ui.WheelLines)
	}
	h.key("Left", 0) // to the groups
	h.key("Up", 0)   // Date & Time
	h.key("Up", 0)   // Appearance
	h.key("Right", 0)
	if a.Selected() != "desktop_icons" {
		t.Fatalf("selected %q", a.Selected())
	}
	h.rune(' ', 0)
	if h.d.Config().DesktopIcons {
		t.Fatal("Space did not turn the desktop icons off")
	}

	// A number typed in a prompt is checked.
	a.Show("Notifications", "notify_seconds")
	h.key("Enter", 0)
	h.typeText("999")
	h.key("Enter", 0)
	if !h.d.DialogOpen() || !h.screenHas("from 2 to 30") {
		t.Fatal("999 seconds was accepted")
	}
	h.key("Esc", 0)
	h.key("Enter", 0)
	h.typeText("10")
	h.key("Enter", 0)
	if h.d.Config().NotifySeconds != 10 || h.d.toasts.Duration != 10*time.Second {
		t.Fatalf("notify seconds %d, toasts %v", h.d.Config().NotifySeconds, h.d.toasts.Duration)
	}
}

// Mouse only: pick a group, click a value to change it.
func TestSettingsMouse(t *testing.T) {
	h := newSettings(t)
	h.draw()
	h.click(2, 0) // ≡ Menu
	h.clickMenuItem("Settings")
	a, w := h.app()
	in := w.Inner()

	h.click(in.X+3, h.rowY("Date & Time"))
	h.wait()
	y := h.rowY("Show seconds")
	h.click(in.X+in.W-6, y) // the value column
	if !h.d.Config().ClockSeconds {
		t.Fatalf("click on the value did not toggle (selected %q)", a.Selected())
	}
	if next := h.d.NextWake(); next.Sub(h.d.Now) > time.Second {
		t.Fatalf("a clock with seconds must wake every second, next wake in %v", next.Sub(h.d.Now))
	}
	if !h.screenHas("09:30:15") {
		t.Fatalf("top bar: %q", h.draw().Row(0))
	}
	// A click on the label only selects.
	h.wait()
	h.click(in.X+30, h.rowY("Show the date"))
	if h.d.Config().ClockDate || a.Selected() != "clock_date" {
		t.Fatal("click on a label must select without changing")
	}

	// The double-click test line.
	h.wait()
	h.click(in.X+3, h.rowY("Mouse"))
	h.wait()
	y = h.rowY("Test: double-click")
	h.click(in.X+30, y)
	h.click(in.X+30, y)
	if !h.screenHas("Double-click recognized") {
		t.Fatal("double-click test did not react")
	}

	// Search field by mouse, then the menu bar.
	h.wait()
	h.click(in.X+12, in.Y+1)
	h.typeText("한글") // no setting has Hangul in it: nothing found, no crash
	if !h.screenHas("No setting matches") {
		t.Fatal("empty result is not explained")
	}
}

// The mount folder must exist; Reset keeps the desktop icons.
func TestSettingsMountFolderAndReset(t *testing.T) {
	h := newSettings(t)
	h.d.OpenSettings("Storage & USB", "mount_dir")
	a, _ := h.app()
	if a.Selected() != "mount_dir" {
		t.Fatalf("selected %q", a.Selected())
	}
	h.key("Enter", 0)
	h.typeText(filepath.Join(h.root, "없는 폴더"))
	h.key("Enter", 0)
	if !h.d.DialogOpen() || !h.screenHas("does not exist") {
		t.Fatal("a missing folder was accepted")
	}
	h.key("Esc", 0)
	dir := filepath.Join(h.root, "mnt", "오프라인")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	h.key("Enter", 0)
	h.typeText(dir + "/")
	h.key("Enter", 0)
	if h.d.Config().MountDir != dir {
		t.Fatalf("mount folder %q", h.d.Config().MountDir)
	}

	h.d.UpdateConfig(func(c *config.Config) { c.Icons = c.Icons[:1] })
	a.Show("About", "reset")
	h.key("Enter", 0)
	h.key("Enter", 0) // Reset
	if c := h.d.Config(); c.MountDir != "" || len(c.Icons) != 1 || c.Keyboard != "pc" {
		t.Fatalf("after reset: mount %q, %d icons, keyboard %q", c.MountDir, len(c.Icons), c.Keyboard)
	}
}

// Settings reach the other windows: hidden desktop icons cannot be clicked,
// Copy stays inside when the clipboard setting is off, names follow the
// non-ASCII setting.
func TestSettingsAreApplied(t *testing.T) {
	h := newSettings(t)
	var sent []string
	h.d.SetClipboardWriter(func(s string) { sent = append(sent, s) })
	h.d.SetClipboardText("a")
	h.d.UpdateConfig(func(c *config.Config) { c.Clipboard = false })
	h.d.SetClipboardText("b")
	if len(sent) != 1 || h.d.ClipboardText() != "b" {
		t.Fatalf("sent to the terminal: %q, inside: %q", sent, h.d.ClipboardText())
	}

	x, y := h.iconCenter(0)
	if h.d.iconAt(x, y) != 0 {
		t.Fatal("no icon to hide")
	}
	h.d.UpdateConfig(func(c *config.Config) { c.DesktopIcons = false })
	if h.d.iconAt(x, y) != -1 || h.screenHas("Home") {
		t.Fatal("hidden desktop icons are still there")
	}

	if h.d.Console() {
		t.Fatal("not a console")
	}
	h.d.UpdateConfig(func(c *config.Config) { c.Names = "escape" })
	if !h.d.Console() || h.d.display("한") != `\uD55C` {
		t.Fatalf("names are not escaped: %q", h.d.display("한"))
	}

	h.d.UpdateConfig(func(c *config.Config) { c.ShowHost, c.ClockDate = false, true })
	if top := h.draw().Row(0); strings.Contains(top, "dev-server") || !strings.Contains(top, "2026-11-04 09:30") {
		t.Fatalf("top bar: %q", top)
	}
	if h.d.MultiClickInterval() != 500*time.Millisecond {
		t.Fatalf("double-click speed %v", h.d.MultiClickInterval())
	}
}
