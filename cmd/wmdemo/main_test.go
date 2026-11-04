package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"tuidesk/internal/term"
)

func newSimApp(t *testing.T, w, h int) (*app, tcell.SimulationScreen) {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(w, h)
	a := newApp(s)
	a.resize()
	t.Cleanup(s.Fini)
	return a, s
}

func screenRow(s tcell.SimulationScreen, y int) string {
	cells, w, _ := s.GetContents()
	var b strings.Builder
	for x := 0; x < w; x++ {
		c := cells[y*w+x]
		if len(c.Runes) > 0 {
			b.WriteString(string(c.Runes))
			// tcell does not repaint the cell after a wide glyph; skip it like
			// the terminal renderer does.
			if term.RuneWidth(c.Runes[0]) == 2 {
				x++
			}
		}
	}
	return b.String()
}

func mouse(a *app, x, y int, b tcell.ButtonMask) {
	a.handle(tcell.NewEventMouse(x, y, b, tcell.ModNone))
}

func windowByID(a *app, id int) (x, y int) {
	for _, w := range a.m.Windows() {
		if w.ID == id {
			return w.Rect.X, w.Rect.Y
		}
	}
	return -1, -1
}

// Phase 0 completion criterion: overlapping windows can be moved with the mouse.
func TestDragOverlappingWindowsWithMouse(t *testing.T) {
	a, s := newSimApp(t, 100, 30)
	a.draw()

	// Window 1 (Files) is underneath window 2. Grab its title at (6,3).
	mouse(a, 6, 3, tcell.Button1)
	mouse(a, 7, 3, tcell.Button1)
	mouse(a, 20, 18, tcell.Button1)
	mouse(a, 20, 18, tcell.ButtonNone)
	a.draw()

	if x, y := windowByID(a, 1); x != 17 || y != 18 {
		t.Fatalf("window 1 at (%d,%d), want (17,18)", x, y)
	}
	if top := a.m.Top(); top.ID != 1 {
		t.Fatalf("top window = %d, want 1", top.ID)
	}
	if row := screenRow(s, 18); !strings.Contains(row, "Files") {
		t.Fatalf("title not drawn at new position: %q", row)
	}

	// Now drag window 2 by its title; window 1 must stay where it is.
	mouse(a, 30, 7, tcell.Button1)
	mouse(a, 40, 9, tcell.Button1)
	mouse(a, 40, 9, tcell.ButtonNone)
	if x, y := windowByID(a, 2); x != 32 || y != 9 {
		t.Fatalf("window 2 at (%d,%d), want (32,9)", x, y)
	}
	if x, y := windowByID(a, 1); x != 17 || y != 18 {
		t.Fatalf("window 1 moved to (%d,%d)", x, y)
	}
}

func TestKeyboardMovesFrontWindow(t *testing.T) {
	a, _ := newSimApp(t, 100, 30)
	front := a.m.Focused()
	x0, y0 := front.Rect.X, front.Rect.Y
	a.handle(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	a.handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if front.Rect.X != x0+2 || front.Rect.Y != y0+1 {
		t.Fatalf("moved to (%d,%d)", front.Rect.X, front.Rect.Y)
	}
	a.handle(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if a.m.Focused() == front {
		t.Fatal("Tab should focus another window")
	}
}

func TestShiftTabFocusesPreviousWindow(t *testing.T) {
	a, _ := newSimApp(t, 100, 30)
	start := a.m.Focused()
	a.handle(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if a.m.Focused() == start {
		t.Fatal("Tab should change focus")
	}
	a.handle(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModShift))
	if a.m.Focused() != start || a.m.Top() != start {
		t.Fatalf("Shift+Tab should return to %q, got %q", start.Title, a.m.Focused().Title)
	}
	a.handle(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModShift))
	if a.m.Focused() == start {
		t.Fatal("second Shift+Tab should move further back")
	}
}

func TestQuitWithKoreanIME(t *testing.T) {
	a, _ := newSimApp(t, 100, 30)
	a.handle(tcell.NewEventKey(tcell.KeyRune, 'ㅂ', tcell.ModCtrl)) // Ctrl+Q typed with the Korean IME on
	if !a.quit {
		t.Fatal("Ctrl+ㅂ should quit")
	}
}
