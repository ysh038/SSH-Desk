package wm

import (
	"strings"
	"testing"
)

func full(c *Canvas) Rect { return Rect{0, 0, c.W, c.H} }

func TestCanvasWideRuneOccupiesTwoCells(t *testing.T) {
	c := NewCanvas(6, 1)
	c.Text(full(c), 0, 0, "한글", 0)
	if c.At(0, 0).Width != 2 || c.At(1, 0).Width != 0 || c.At(2, 0).Width != 2 {
		t.Fatalf("cells = %+v %+v %+v", c.At(0, 0), c.At(1, 0), c.At(2, 0))
	}
	if got := c.Row(0); got != "한글  " {
		t.Fatalf("row = %q", got)
	}
}

func TestCanvasOverwriteRightHalfBlanksLeftHalf(t *testing.T) {
	c := NewCanvas(4, 1)
	c.Text(full(c), 0, 0, "한", 0)
	c.Set(full(c), 1, 0, '│', 1) // a window border lands on the right half
	if got := c.Row(0); got != " │  " {
		t.Fatalf("row = %q", got)
	}
}

func TestCanvasOverwriteLeftHalfBlanksRightHalf(t *testing.T) {
	c := NewCanvas(4, 1)
	c.Text(full(c), 1, 0, "글", 0)
	c.Set(full(c), 1, 0, 'x', 0)
	if got := c.Row(0); got != " x  " {
		t.Fatalf("row = %q", got)
	}
	if c.At(2, 0).Width != 1 {
		t.Fatal("orphaned right half left behind")
	}
}

func TestCanvasWideOverWide(t *testing.T) {
	c := NewCanvas(5, 1)
	c.Text(full(c), 0, 0, "가나", 0) // cols 0-1, 2-3
	c.Set(full(c), 1, 0, '다', 0)   // straddles both
	if got := c.Row(0); got != " 다  " {
		t.Fatalf("row = %q", got)
	}
}

func TestCanvasClipWideAtEdge(t *testing.T) {
	c := NewCanvas(5, 1)
	clip := Rect{0, 0, 3, 1}
	end := c.Text(clip, 0, 0, "가나", 0)
	if end != 4 {
		t.Fatalf("advance = %d, want 4", end)
	}
	// 나 would need cols 2-3 but the clip ends at col 2: drawn as a space.
	if got := c.Row(0); got != "가   " {
		t.Fatalf("row = %q", got)
	}
}

func newTestManager() *Manager {
	m := &Manager{Desktop: Rect{0, 1, 80, 23}}
	m.Add(&Window{ID: 1, Title: "A", Rect: Rect{2, 2, 20, 8}})
	m.Add(&Window{ID: 2, Title: "B", Rect: Rect{10, 5, 20, 8}})
	return m
}

func TestHitTestPrefersTopWindow(t *testing.T) {
	m := newTestManager()
	w, part := m.HitTest(12, 6) // inside both, B is on top
	if w.ID != 2 || part != PartBody {
		t.Fatalf("hit = %v %v", w.ID, part)
	}
	w, part = m.HitTest(5, 2)
	if w.ID != 1 || part != PartTitle {
		t.Fatalf("hit = %v %v", w.ID, part)
	}
	if w, _ := m.HitTest(50, 20); w != nil {
		t.Fatal("expected no window")
	}
}

func TestClickRaisesAndFocuses(t *testing.T) {
	m := newTestManager()
	m.PointerDown(3, 4) // body of A, not covered by B
	m.PointerUp()
	if m.Top().ID != 1 || m.Focused().ID != 1 {
		t.Fatalf("top=%d focus=%d", m.Top().ID, m.Focused().ID)
	}
}

func TestDragTitleMovesWindow(t *testing.T) {
	m := newTestManager()
	m.PointerDown(5, 2) // title of A, grab offset (3,0)
	m.PointerDrag(30, 10)
	m.PointerDrag(40, 12)
	m.PointerUp()
	a := m.Top()
	if a.ID != 1 || a.Rect.X != 37 || a.Rect.Y != 12 {
		t.Fatalf("A = %+v", a.Rect)
	}
	if m.Dragging() {
		t.Fatal("drag should be over")
	}
}

func TestDragBodyDoesNotMove(t *testing.T) {
	m := newTestManager()
	m.PointerDown(15, 8) // body of B
	m.PointerDrag(40, 15)
	m.PointerUp()
	if b := m.Top(); b.Rect.X != 10 || b.Rect.Y != 5 {
		t.Fatalf("B moved: %+v", b.Rect)
	}
}

func TestClampKeepsTitleReachable(t *testing.T) {
	m := newTestManager()
	a := m.Windows()[0]
	m.MoveTo(a, -100, -100)
	if a.Rect.Y != 1 || a.Rect.X != -(20-MinVisible) {
		t.Fatalf("clamped to %+v", a.Rect)
	}
	m.MoveTo(a, 1000, 1000)
	if a.Rect.Y != 23 || a.Rect.X != 80-MinVisible {
		t.Fatalf("clamped to %+v", a.Rect)
	}
}

func TestFocusNextCycles(t *testing.T) {
	m := newTestManager()
	m.Add(&Window{ID: 3, Title: "C", Rect: Rect{40, 3, 10, 5}})
	var order []int
	for i := 0; i < 3; i++ {
		m.FocusNext()
		order = append(order, m.Focused().ID)
	}
	if order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("order = %v", order)
	}
}

func TestDrawOverlapAndHangulTitles(t *testing.T) {
	m := &Manager{Desktop: Rect{0, 0, 30, 8}}
	m.Add(&Window{ID: 1, Title: "파일", Rect: Rect{0, 0, 14, 5}, Body: []string{"한글 파일명.txt"}})
	m.Add(&Window{ID: 2, Title: "메모", Rect: Rect{7, 2, 14, 5}, Body: []string{"가나다라마바사"}})
	c := NewCanvas(30, 8)
	m.Draw(c)

	if got := c.Row(0); !strings.HasPrefix(got, "┌─ 파일 ─────┐") {
		t.Fatalf("row0 = %q", got)
	}
	// Row 1: A's body "한글 파일명.txt" clipped to 12 inner cols; nothing splits.
	if got := c.Row(1); !strings.HasPrefix(got, "│한글 파일명.│") {
		t.Fatalf("row1 = %q", got)
	}
	// Row 2: B's title bar covers A from col 7.
	if got := c.Row(2); !strings.HasPrefix(got, "│      ┌─ 메모 ─────┐") {
		t.Fatalf("row2 = %q", got)
	}
	// Row 3: B's body: 12 inner cols hold six syllables.
	if got := c.Row(3); !strings.HasPrefix(got, "│      │가나다라마바│") {
		t.Fatalf("row3 = %q", got)
	}
	// Every wide head must be followed by its right half.
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if c.At(x, y).Width == 2 && c.At(x+1, y).Width != 0 {
				t.Fatalf("broken wide glyph at %d,%d", x, y)
			}
			if c.At(x, y).Width == 0 && (x == 0 || c.At(x-1, y).Width != 2) {
				t.Fatalf("orphan right half at %d,%d", x, y)
			}
		}
	}
}
