package desktop

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// The wheel scrolls the file list without moving the cursor, and the next
// draw must not snap the view back to the cursor (reported on Terminal.app:
// the list moved a little, then stopped, and would not go back up).
func TestFileListWheelScrollStays(t *testing.T) {
	f := newFM(t)
	for i := range 60 {
		f.touch(fmt.Sprintf("many/f%02d.txt", i), "x")
	}
	_, w := f.open(filepath.Join(f.root, "many"), wm.Rect{X: 1, Y: 2, W: 60, H: 16})
	in := w.Inner()
	wheel := func(dir term.Wheel, n int) {
		for range n {
			f.d.Handle(term.MouseEvent{Gesture: term.Gesture{Kind: term.GestureWheel, Wheel: dir, X: in.X + 5, Y: in.Y + 5}})
		}
	}
	top := func() string { f.draw(); return f.draw().Row(row(w, 0)) }

	wheel(term.WheelDown, 5) // 3 rows per notch
	if got := top(); !strings.Contains(got, "f15.txt") {
		t.Fatalf("after 5 notches down, first row %q", got)
	}
	wheel(term.WheelUp, 2)
	if got := top(); !strings.Contains(got, "f09.txt") {
		t.Fatalf("after 2 notches up, first row %q", got)
	}
	wheel(term.WheelDown, 100) // clamps at the end
	if got := f.draw().Row(row(w, 0)); strings.Contains(got, "f00.txt") {
		t.Fatalf("scrolled to the end, first row %q", got)
	}
	wheel(term.WheelUp, 100)
	if got := top(); !strings.Contains(got, "f00.txt") {
		t.Fatalf("back at the top, first row %q", got)
	}

	// A key moves the cursor again and brings it into view.
	wheel(term.WheelDown, 10)
	f.key("Down", 0)
	if got := top(); !strings.Contains(got, "f00.txt") && !strings.Contains(got, "f01.txt") {
		t.Fatalf("Down after scrolling should show the cursor, first row %q", got)
	}
}

// Back returns to the previous folder at the same scroll position (even
// one reached with the wheel) and with the cursor on the folder that was
// opened; Forward goes in again. Keys and the path-row buttons both work.
func TestFileManagerBackKeepsScroll(t *testing.T) {
	f := newFM(t)
	for i := range 60 {
		f.touch(fmt.Sprintf("many/d%02d/inner.txt", i), "x")
	}
	app, w := f.open(filepath.Join(f.root, "many"), wm.Rect{X: 1, Y: 2, W: 60, H: 16})
	in := w.Inner()
	for range 5 {
		f.d.Handle(term.MouseEvent{Gesture: term.Gesture{Kind: term.GestureWheel, Wheel: term.WheelDown, X: in.X + 5, Y: in.Y + 5}})
	}
	if got := f.draw().Row(row(w, 0)); !strings.Contains(got, "d15") {
		t.Fatalf("scrolled top row %q", got)
	}
	// Double-click d17 (third visible row).
	f.click(in.X+5, row(w, 2))
	f.click(in.X+5, row(w, 2))
	f.settle()
	if !strings.HasSuffix(app.Dir(), "d17") {
		t.Fatalf("double click opened %s", app.Dir())
	}

	f.key("Left", term.ModAlt) // Back (Option+← arrives as Alt+B on Terminal.app)
	f.settle()
	if !strings.HasSuffix(app.Dir(), "many") {
		t.Fatalf("Back went to %s", app.Dir())
	}
	if got := f.draw().Row(row(w, 0)); !strings.Contains(got, "d15") {
		t.Fatalf("Back should restore the scroll position, top row %q", got)
	}
	if e := app.Selection(); len(e) != 1 || e[0].Name != "d17" {
		t.Fatalf("cursor after Back: %v", e)
	}

	f.key("Right", term.ModAlt) // Forward
	f.settle()
	if !strings.HasSuffix(app.Dir(), "d17") {
		t.Fatalf("Forward went to %s", app.Dir())
	}
	f.rune('b', term.ModAlt) // Back again, the Terminal.app way
	f.settle()
	if !strings.HasSuffix(app.Dir(), "many") {
		t.Fatalf("Alt+B went to %s", app.Dir())
	}

	// Mouse: the "<" and ">" buttons at the left of the path row.
	pathY := in.Y + 1
	if got := f.draw().Row(pathY); !strings.Contains(got, " < > ") {
		t.Fatalf("path row %q", got)
	}
	f.click(in.X+3, pathY) // ">"
	f.settle()
	if !strings.HasSuffix(app.Dir(), "d17") {
		t.Fatalf("> button went to %s", app.Dir())
	}
	f.click(in.X+1, pathY) // "<"
	f.settle()
	if !strings.HasSuffix(app.Dir(), "many") || !strings.Contains(f.draw().Row(row(w, 0)), "d15") {
		t.Fatalf("< button: %s, top row %q", app.Dir(), f.draw().Row(row(w, 0)))
	}
}
