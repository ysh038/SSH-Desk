package terminal

import (
	"testing"

	"github.com/charmbracelet/x/vt"
)

// The patched emulator (third_party/vt) wraps a wide character that does
// not fit at the right margin instead of dropping it (ADR 0004).
func TestWideCharWrapsAtRightMargin(t *testing.T) {
	e := vt.NewEmulator(10, 3)
	go func() {
		b := make([]byte, 64)
		for {
			if _, err := e.Read(b); err != nil {
				return
			}
		}
	}()
	defer e.Close()
	e.WriteString("123456789한x")
	if c := e.CellAt(0, 1); c == nil || c.Content != "한" {
		t.Fatalf("row 1 col 0 = %+v", c)
	}
	if c := e.CellAt(2, 1); c == nil || c.Content != "x" {
		t.Fatalf("row 1 col 2 = %+v", c)
	}
	// A wide character ending exactly at the margin stays; the next
	// character wraps (here on the last row, so the screen scrolls).
	e.WriteString("\x1b[3;1H12345678한y")
	if c := e.CellAt(8, 1); c == nil || c.Content != "한" {
		t.Fatalf("row 1 col 8 = %+v", c)
	}
	if c := e.CellAt(0, 2); c == nil || c.Content != "y" {
		t.Fatalf("row 2 col 0 = %+v", c)
	}
}
