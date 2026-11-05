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
