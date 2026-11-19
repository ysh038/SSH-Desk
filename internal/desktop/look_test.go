package desktop

import (
	"strings"
	"testing"

	"tuidesk/internal/config"
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// move reports the pointer at (x, y) with no button held.
func (h *harness) move(x, y int) { h.mouse(x, y, term.ButtonNone, 0) }

func styleAt(c *wm.Canvas, x, y int) wm.StyleID { return c.At(x, y).Style }

// What the pointer is over lights up; a key press hides the highlight
// again, and the setting turns the whole thing off.
func TestHoverTaskbarAndButtons(t *testing.T) {
	h := newHarness(t, 2)
	if !h.d.MouseMotion() {
		t.Fatal("hover is on by default, so the terminal must report movement")
	}
	bar := h.draw().Row(h.d.H - 1)
	x := len([]rune(bar[:strings.Index(bar, "Window 1")]))
	y := h.d.H - 1
	if styleAt(h.draw(), x, y) == wm.StyleBarHover {
		t.Fatal("nothing is lit before the mouse moves")
	}
	h.move(x, y)
	if styleAt(h.draw(), x, y) != wm.StyleBarHover {
		t.Fatal("the tab under the pointer must light up")
	}
	if styleAt(h.draw(), 2, y) == wm.StyleBarHover {
		t.Fatal("only the tab under the pointer lights up")
	}
	// Moving the pointer is not a click: nothing was focused or minimized.
	f := h.d.M.Focused()
	if f == nil || f.Name() != "Window 2" || f.Minimized {
		t.Fatal("moving the pointer must not act on windows")
	}

	// The close button of the focused window.
	cx, cy := f.Rect.X+f.Rect.W-3, f.Rect.Y
	if styleAt(h.draw(), cx, cy) != wm.StyleClose {
		t.Fatalf("close button style %d", styleAt(h.draw(), cx, cy))
	}
	h.move(cx, cy)
	if styleAt(h.draw(), cx, cy) != wm.StyleCloseHover {
		t.Fatal("close button must light up under the pointer")
	}

	// Keyboard use hides the highlight until the mouse moves again.
	h.key("F1", 0)
	h.key("Esc", 0)
	if styleAt(h.draw(), cx, cy) == wm.StyleCloseHover {
		t.Fatal("a key press must hide the hover highlight")
	}
	h.move(cx-1, cy)
	if styleAt(h.draw(), cx, cy) != wm.StyleCloseHover {
		t.Fatal("moving the mouse brings it back")
	}

	// Setting off: no highlight, no movement reports.
	h.d.UpdateConfig(func(c *config.Config) { c.Hover = false })
	h.move(cx, cy)
	if styleAt(h.draw(), cx, cy) == wm.StyleCloseHover || h.d.MouseMotion() {
		t.Fatal("hover must stay off when the setting is off")
	}
}

// A window behind another one does not light up through it, and nothing
// under an open menu or dialog does.
func TestHoverOnlyTopLayer(t *testing.T) {
	h := newHarness(t, 2)
	var back, front *wm.Window
	for _, w := range h.d.M.Windows() {
		if w == h.d.M.Focused() {
			front = w
		} else {
			back = w
		}
	}
	// Put the front window over the back one's buttons.
	h.d.M.MoveTo(front, back.Rect.X+back.Rect.W-12, back.Rect.Y)
	bx, by := back.Rect.X+back.Rect.W-3, back.Rect.Y
	h.move(bx, by)
	if st := styleAt(h.draw(), bx, by); st == wm.StyleCloseHover || st == wm.StyleButtonHover {
		t.Fatalf("a covered button lit up (style %d)", st)
	}

	// With a dialog open, the taskbar under the pointer stays as it is and
	// the dialog's own button lights up.
	h.d.Confirm("Quit", "Really?", []string{"Yes", "No"}, func(int) {})
	h.move(2, h.d.H-1)
	if styleAt(h.draw(), 2, h.d.H-1) == wm.StyleBarHover {
		t.Fatal("the taskbar is behind the dialog")
	}
	x, y, ok := findText(h.draw(), "[ No ]")
	if !ok {
		t.Fatal("no button drawn")
	}
	h.move(x+2, y)
	if styleAt(h.draw(), x+2, y) != wm.StyleDialogButtonHover {
		t.Fatal("a dialog button must light up under the pointer")
	}
	if len(h.d.modals) != 1 {
		t.Fatal("moving the pointer must not answer the dialog")
	}
}

// In an open menu the selection follows the pointer, and the pointer never
// interrupts keyboard move mode.
func TestHoverMenuAndMoveMode(t *testing.T) {
	h := newHarness(t, 1)
	h.key("F9", 0)
	m := h.d.menu
	for i, it := range m.Items {
		if it.Label == "Settings" {
			h.move(m.Rect.X+3, m.Rect.Y+1+i)
			if m.Selected != i {
				t.Fatalf("selection %d, want %d", m.Selected, i)
			}
		}
	}
	h.move(m.Rect.X+m.Rect.W+5, 2) // off the menu: it stays open
	if !h.d.MenuOpen() {
		t.Fatal("moving off a menu must not close it")
	}
	h.key("Esc", 0)

	h.key("F7", 0)
	if h.d.mode != ModeMove {
		t.Fatal("F7 must start move mode")
	}
	h.move(10, 10)
	if h.d.mode != ModeMove {
		t.Fatal("moving the mouse must not end keyboard move mode")
	}
}

// Shadows follow the Appearance setting and never fall on the taskbar.
func TestShadowsSetting(t *testing.T) {
	h := newHarness(t, 1)
	w := h.d.M.Focused()
	sx, sy := w.Rect.X+w.Rect.W, w.Rect.Y+1
	if styleAt(h.draw(), sx, sy) != wm.StyleShadow {
		t.Fatal("a window casts a shadow by default")
	}
	h.key("F9", 0)
	c := h.draw()
	mr := h.d.menu.Rect
	if styleAt(c, mr.X+mr.W, mr.Y+1) != wm.StyleShadow {
		t.Fatal("a menu casts a shadow")
	}
	for x := 0; x < h.d.W; x++ {
		if styleAt(c, x, h.d.H-1) == wm.StyleShadow {
			t.Fatalf("shadow on the taskbar at x=%d", x)
		}
	}
	h.key("Esc", 0)
	h.d.UpdateConfig(func(c *config.Config) { c.Shadows = false })
	c = h.draw()
	for y := 0; y < h.d.H; y++ {
		for x := 0; x < h.d.W; x++ {
			if styleAt(c, x, y) == wm.StyleShadow {
				t.Fatalf("shadow at (%d,%d) with the setting off", x, y)
			}
		}
	}
}
