package desktop

import (
	"strings"
	"testing"
	"time"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// harness drives a Desktop with raw mouse input through the real gesture
// detector, the same path the terminal layer uses.
type harness struct {
	t   *testing.T
	d   *Desktop
	det *term.Detector
	now time.Time
}

func newHarness(t *testing.T, windows int) *harness {
	t0 := time.Date(2026, 11, 4, 9, 30, 15, 0, time.UTC)
	h := &harness{t: t, d: New(100, 30, "dev-server", t0), det: term.NewDetector(), now: t0}
	for i := 0; i < windows; i++ {
		h.d.OpenWindow(h.d.NewApp(i + 1))
	}
	return h
}

func (h *harness) mouse(x, y int, b term.Button, mod term.Mod) {
	h.now = h.now.Add(30 * time.Millisecond)
	for _, g := range h.det.Feed(term.MouseInput{X: x, Y: y, Button: b, Mod: mod, Time: h.now}) {
		h.d.Handle(term.MouseEvent{Gesture: g})
	}
}

func (h *harness) click(x, y int) {
	h.mouse(x, y, term.ButtonLeft, 0)
	h.mouse(x, y, term.ButtonNone, 0)
}
func (h *harness) rclick(x, y int) {
	h.mouse(x, y, term.ButtonRight, 0)
	h.mouse(x, y, term.ButtonNone, 0)
}
func (h *harness) wait() { h.now = h.now.Add(time.Second) }

func (h *harness) drag(x0, y0, x1, y1 int) {
	h.mouse(x0, y0, term.ButtonLeft, 0)
	h.mouse((x0+x1)/2, (y0+y1)/2, term.ButtonLeft, 0)
	h.mouse(x1, y1, term.ButtonLeft, 0)
	h.mouse(x1, y1, term.ButtonNone, 0)
	h.wait()
}

func (h *harness) key(name string, mod term.Mod) {
	h.d.Handle(term.KeyEvent{Key: term.Key{Name: name, Mod: mod}})
}

func (h *harness) rune(r rune, mod term.Mod) {
	h.d.Handle(term.KeyEvent{Key: term.Key{Name: "Rune", Rune: r, Mod: mod}})
}

func (h *harness) draw() *wm.Canvas {
	c := wm.NewCanvas(h.d.W, h.d.H)
	h.d.Draw(c)
	return c
}

func (h *harness) win(n int) *wm.Window {
	for _, w := range h.d.M.Windows() {
		if w.Name() == "Window "+string(rune('0'+n)) {
			return w
		}
	}
	h.t.Fatalf("window %d not found", n)
	return nil
}

// Phase 1 completion criterion, mouse half: move, resize, maximize,
// minimize/restore and close empty windows with the mouse only.
func TestMouseOnlyWindowOperations(t *testing.T) {
	h := newHarness(t, 3)
	h.draw()
	w1 := h.win(1)

	// Move window 1 (under the others) by its title bar.
	r := w1.Rect
	h.drag(r.X+5, r.Y, r.X+25, r.Y+8)
	if w1.Rect.X != r.X+20 || w1.Rect.Y != r.Y+8 || h.d.M.Top() != w1 {
		t.Fatalf("move: %+v", w1.Rect)
	}

	// Resize from the bottom-right corner.
	r = w1.Rect
	h.drag(r.X+r.W-1, r.Y+r.H-1, r.X+r.W+9, r.Y+r.H+2)
	if w1.Rect.W != r.W+10 || w1.Rect.H != r.H+3 {
		t.Fatalf("resize: %+v", w1.Rect)
	}

	// Maximize with [^], restore with the same button.
	bx := w1.Rect.X + w1.Rect.W - 10
	h.click(bx+4, w1.Rect.Y)
	h.wait()
	if !w1.Maximized || w1.Rect != h.d.Area() {
		t.Fatalf("maximize: %+v", w1.Rect)
	}
	h.click(h.d.W-7, w1.Rect.Y)
	h.wait()
	if w1.Maximized {
		t.Fatal("restore failed")
	}

	// Minimize with [_]; it appears on the taskbar; click it to restore.
	bx = w1.Rect.X + w1.Rect.W - 10
	h.click(bx+1, w1.Rect.Y)
	h.wait()
	if !w1.Minimized {
		t.Fatal("minimize failed")
	}
	c := h.draw()
	if !strings.Contains(c.Row(h.d.H-1), "[Window 1]") {
		t.Fatalf("taskbar = %q", c.Row(h.d.H-1))
	}
	h.click(2, h.d.H-1)
	h.wait()
	if w1.Minimized || h.d.M.Focused() != w1 {
		t.Fatal("restore from taskbar failed")
	}

	// Close with [x].
	h.draw()
	bx = w1.Rect.X + w1.Rect.W - 10
	h.click(bx+7, w1.Rect.Y)
	if len(h.d.M.Windows()) != 2 {
		t.Fatalf("close failed: %d windows", len(h.d.M.Windows()))
	}
}

func TestDoubleClickTitleMaximizes(t *testing.T) {
	h := newHarness(t, 1)
	w := h.win(1)
	h.click(w.Rect.X+4, w.Rect.Y)
	h.click(w.Rect.X+4, w.Rect.Y)
	if !w.Maximized {
		t.Fatal("double click should maximize")
	}
}

func TestRightClickAndCtrlClickOpenWindowMenu(t *testing.T) {
	h := newHarness(t, 2)
	w1 := h.win(1)
	h.rclick(w1.Rect.X+2, w1.Rect.Y+2)
	if !h.d.MenuOpen() || h.d.M.Top() != w1 {
		t.Fatal("right click should raise and open the window menu")
	}
	h.rune('x', 0) // hotkey: Maximize
	if h.d.MenuOpen() || !w1.Maximized {
		t.Fatal("menu hotkey should maximize")
	}

	h2 := newHarness(t, 1)
	w := h2.win(1)
	h2.mouse(w.Rect.X+3, w.Rect.Y+3, term.ButtonLeft, term.ModCtrl)
	h2.mouse(w.Rect.X+3, w.Rect.Y+3, term.ButtonNone, term.ModCtrl)
	if !h2.d.MenuOpen() {
		t.Fatal("Ctrl+click should open the window menu")
	}
	h2.key("Esc", 0)
	if h2.d.MenuOpen() {
		t.Fatal("Esc should close the menu")
	}
}

func TestLauncherFromTopBar(t *testing.T) {
	h := newHarness(t, 0)
	h.draw()
	h.click(2, 0) // "≡ Menu"
	if !h.d.MenuOpen() {
		t.Fatal("launcher should open")
	}
	// First item is "New window": click it.
	h.wait()
	h.click(4, 2)
	if h.d.MenuOpen() || len(h.d.M.Windows()) != 1 {
		t.Fatalf("new window from launcher: open=%v windows=%d", h.d.MenuOpen(), len(h.d.M.Windows()))
	}
	h.click(50, 25) // empty desktop: nothing happens
	if h.d.MenuOpen() {
		t.Fatal("plain click on desktop must not open a menu")
	}
}

// Phase 1 completion criterion, keyboard half.
func TestKeyboardOnlyWindowOperations(t *testing.T) {
	h := newHarness(t, 0)

	h.key("F9", 0)
	h.key("Enter", 0) // New window
	h.rune('n', term.ModCtrl)
	h.rune('n', term.ModCtrl)
	if len(h.d.M.Windows()) != 3 {
		t.Fatalf("windows = %d", len(h.d.M.Windows()))
	}
	top := h.d.M.Focused()

	h.key("F6", 0)
	if h.d.M.Focused() == top {
		t.Fatal("F6 should switch window")
	}
	h.key("F6", term.ModShift)
	if h.d.M.Focused() != top {
		t.Fatal("Shift+F6 should switch back")
	}

	// Move mode.
	r := top.Rect
	h.key("F7", 0)
	h.key("Right", 0)
	h.key("Down", 0)
	h.key("Enter", 0)
	if top.Rect.X != r.X+2 || top.Rect.Y != r.Y+1 || h.d.Mode() != ModeNormal {
		t.Fatalf("move mode: %+v", top.Rect)
	}

	// Resize mode.
	h.key("F8", 0)
	h.key("Right", 0)
	h.key("Right", 0)
	h.key("Down", 0)
	h.key("Esc", 0)
	if top.Rect.W != r.W+4 || top.Rect.H != r.H+1 {
		t.Fatalf("resize mode: %+v", top.Rect)
	}

	// Window menu via Shift+F10: maximize, then minimize.
	h.key("F10", term.ModShift)
	h.rune('x', 0)
	if !top.Maximized {
		t.Fatal("maximize via window menu")
	}
	h.key("F10", term.ModShift)
	h.rune('n', 0)
	if !top.Minimized {
		t.Fatal("minimize via window menu")
	}

	// Restore from the launcher's window list.
	h.key("F9", 0)
	for i := 0; i < 20 && !strings.Contains(h.d.menu.Items[h.d.menu.Selected].Label, "(minimized)"); i++ {
		h.key("Down", 0)
	}
	h.key("Enter", 0)
	if top.Minimized || h.d.M.Focused() != top {
		t.Fatal("restore via launcher")
	}

	// Close with Ctrl+W.
	h.rune('w', term.ModCtrl)
	if len(h.d.M.Windows()) != 2 {
		t.Fatalf("Ctrl+W: windows = %d", len(h.d.M.Windows()))
	}

	// Quit with confirmation.
	h.rune('q', term.ModCtrl)
	if !h.d.DialogOpen() {
		t.Fatal("Ctrl+Q should ask")
	}
	h.key("Right", 0) // Cancel
	h.key("Enter", 0)
	if h.d.Quit || h.d.DialogOpen() {
		t.Fatal("cancel should keep running")
	}
	h.rune('q', term.ModCtrl)
	h.key("Enter", 0)
	if !h.d.Quit {
		t.Fatal("Enter on Quit should quit")
	}
}

func TestHelpWindowSingleInstance(t *testing.T) {
	h := newHarness(t, 0)
	h.key("F1", 0)
	h.key("F1", 0)
	if len(h.d.M.Windows()) != 1 || h.d.M.Focused().Name() != "Help" {
		t.Fatal("help should open once")
	}
	c := h.draw()
	found := false
	for y := 0; y < c.H; y++ {
		if strings.Contains(c.Row(y), "Shift+F10  Window menu") {
			found = true
		}
	}
	if !found {
		t.Fatal("help text not drawn")
	}
}

func TestBarsShowHostClockAndWindows(t *testing.T) {
	h := newHarness(t, 2)
	c := h.draw()
	top := c.Row(0)
	for _, want := range []string{"≡ Menu", "Window 1", "Window 2", "dev-server", "09:30"} {
		if !strings.Contains(top, want) {
			t.Errorf("top bar missing %q: %q", want, top)
		}
	}
	if !strings.Contains(c.Row(h.d.H-1), "F9 Menu") {
		t.Errorf("taskbar = %q", c.Row(h.d.H-1))
	}
}

func TestNextWakeIsClockMinuteOrToast(t *testing.T) {
	h := newHarness(t, 0)
	if got := h.d.NextWake(); !got.Equal(time.Date(2026, 11, 4, 9, 31, 0, 0, time.UTC)) {
		t.Fatalf("wake = %v", got)
	}
	h.d.Notify("Saved", false)
	if got := h.d.NextWake(); !got.Equal(h.d.Now.Add(4 * time.Second)) {
		t.Fatalf("wake with toast = %v", got)
	}
	h.d.Handle(term.TickEvent{Now: h.d.Now.Add(5 * time.Second)})
	if got := h.d.NextWake(); !got.Equal(time.Date(2026, 11, 4, 9, 31, 0, 0, time.UTC)) {
		t.Fatalf("wake after toast = %v", got)
	}
}

func TestEventsReachFocusedApp(t *testing.T) {
	h := newHarness(t, 1)
	w := h.win(1)
	h.click(w.Rect.X+3, w.Rect.Y+2)
	c := h.draw()
	if !strings.Contains(c.Row(w.Rect.Y+3), "Click single Left (2,1)") {
		t.Fatalf("app did not get local click: %q", c.Row(w.Rect.Y+3))
	}
	h.d.Handle(term.PasteEvent{Text: "한글\n두 줄"})
	c = h.draw()
	if !strings.Contains(c.Row(w.Rect.Y+3), "paste, 6 chars") {
		t.Fatalf("paste not delivered: %q", c.Row(w.Rect.Y+3))
	}
}

func TestResizeTerminalKeepsWindowsReachable(t *testing.T) {
	h := newHarness(t, 3)
	h.d.Resize(40, 12)
	for _, w := range h.d.M.Windows() {
		if w.Rect.Y < 1 || w.Rect.Y > 10 || w.Rect.X > 40-wm.MinVisible {
			t.Fatalf("window off screen: %+v", w.Rect)
		}
	}
}
