package editor

import (
	"strings"
	"testing"
	"time"
)

func clock() (*time.Time, func() time.Time) {
	t := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	return &t, func() time.Time { return t }
}

func typed(e *Editor, s string) {
	for _, r := range s {
		if r == '\n' {
			e.Newline()
		} else {
			e.Insert(string(r))
		}
	}
}

func TestTextRoundTripKeepsLineEndings(t *testing.T) {
	for _, in := range []string{"a\nb\n", "a\r\nb\r\n", "no newline", "", "\n\n"} {
		if got := New(in).Text(); got != in {
			t.Errorf("%q → %q", in, got)
		}
	}
}

func TestTypingIsOneUndoStep(t *testing.T) {
	now, fn := clock()
	e := New("")
	e.Now = fn
	typed(e, "server")
	*now = now.Add(2 * time.Second) // pause ends the group
	typed(e, " {")
	if got := e.Text(); got != "server {" {
		t.Fatalf("text %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "server" {
		t.Fatalf("after undo %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "" || e.Modified() {
		t.Fatalf("after 2nd undo %q modified=%v", got, e.Modified())
	}
	e.Redo()
	e.Redo()
	if got := e.Text(); got != "server {" {
		t.Fatalf("after redo %q", got)
	}
}

func TestMovingBreaksUndoGroup(t *testing.T) {
	_, fn := clock()
	e := New("")
	e.Now = fn
	typed(e, "ab")
	e.MoveCursor(Left, false, 10)
	typed(e, "X")
	e.Undo()
	if e.Text() != "ab" {
		t.Fatalf("got %q", e.Text())
	}
}

func TestNewlineKeepsIndentAndIndentsAfterBrace(t *testing.T) {
	e := New("    server {")
	e.MoveCursor(LineEnd, false, 10)
	e.Newline()
	typed(e, "listen 80;")
	if got := e.Text(); got != "    server {\n        listen 80;" {
		t.Fatalf("got %q", got)
	}
}

func TestDisplayColumnsWithTabsAndHangul(t *testing.T) {
	e := New("\t한글a")
	e.TabSize = 4
	if x := e.DisplayX(Pos{0, 1}); x != 4 {
		t.Fatalf("after tab x=%d", x)
	}
	if x := e.DisplayX(Pos{0, 3}); x != 8 {
		t.Fatalf("after 한글 x=%d", x)
	}
	if c := e.ColAtX(0, 6); c != 2 { // second half of 한 → next rune
		t.Fatalf("col at x=6 → %d", c)
	}
	// Vertical moves keep the display column across wide runes.
	e2 := New("가나다라\nabcdefgh")
	e2.SetCursor(Pos{0, 2}, false) // x=4
	e2.MoveCursor(Down, false, 10)
	if e2.Cur != (Pos{1, 4}) {
		t.Fatalf("down → %+v", e2.Cur)
	}
}

func TestWordAndDocMovement(t *testing.T) {
	e := New("proxy_pass http://app;\nnext")
	e.MoveCursor(WordRight, false, 10)
	if e.Cur.Col != 11 {
		t.Fatalf("word right → %d", e.Cur.Col)
	}
	e.MoveCursor(WordRight, false, 10)
	e.MoveCursor(WordLeft, false, 10)
	if e.Cur.Col != 11 {
		t.Fatalf("word left → %d", e.Cur.Col)
	}
	e.MoveCursor(DocEnd, false, 10)
	if e.Cur != (Pos{1, 4}) {
		t.Fatalf("doc end → %+v", e.Cur)
	}
	e.MoveCursor(DocStart, true, 10)
	if s := e.SelectedText(); s != "proxy_pass http://app;\nnext" {
		t.Fatalf("selection %q", s)
	}
}

func TestPageMovesOneScreen(t *testing.T) {
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "x")
	}
	e := New(strings.Join(lines, "\n"))
	e.MoveCursor(PageDown, false, 20)
	if e.Cur.Line != 19 {
		t.Fatalf("page down → %d", e.Cur.Line)
	}
	e.MoveCursor(PageUp, false, 20)
	if e.Cur.Line != 0 {
		t.Fatalf("page up → %d", e.Cur.Line)
	}
}

func TestSmartHome(t *testing.T) {
	e := New("    location / {")
	e.MoveCursor(LineEnd, false, 10)
	e.MoveCursor(LineStart, false, 10)
	if e.Cur.Col != 4 {
		t.Fatalf("first home → %d", e.Cur.Col)
	}
	e.MoveCursor(LineStart, false, 10)
	if e.Cur.Col != 0 {
		t.Fatalf("second home → %d", e.Cur.Col)
	}
}

func TestSelectionTypingAndMarkMode(t *testing.T) {
	e := New("hello world")
	e.MoveCursor(WordRight, true, 10)
	typed(e, "bye ")
	if e.Text() != "bye world" {
		t.Fatalf("got %q", e.Text())
	}
	// Mark mode (console): plain moves extend.
	e.SetCursor(Pos{0, 0}, false)
	e.Mark = true
	e.MoveCursor(Right, false, 10)
	e.MoveCursor(Right, false, 10)
	if e.SelectedText() != "by" {
		t.Fatalf("mark selection %q", e.SelectedText())
	}
}

func TestBackspaceRemovesIndentLevel(t *testing.T) {
	e := New("        x")
	e.SetCursor(Pos{0, 8}, false)
	e.Backspace()
	if e.Text() != "    x" {
		t.Fatalf("got %q", e.Text())
	}
}

func TestTabIndentsSelectedLines(t *testing.T) {
	e := New("a\nb\nc")
	e.Select(Pos{0, 0}, Pos{1, 1})
	e.Tab()
	if e.Text() != "    a\n    b\nc" {
		t.Fatalf("indent %q", e.Text())
	}
	e.Untab()
	if e.Text() != "a\nb\nc" {
		t.Fatalf("untab %q", e.Text())
	}
	e.Undo()
	if e.Text() != "    a\n    b\nc" {
		t.Fatalf("undo untab %q", e.Text())
	}
}

func TestCutCopyWithoutSelectionUseLine(t *testing.T) {
	e := New("one\ntwo\nthree")
	e.SetCursor(Pos{1, 1}, false)
	if c := e.Copy(); c != "two\n" {
		t.Fatalf("copy %q", c)
	}
	if c := e.Cut(); c != "two\n" || e.Text() != "one\nthree" {
		t.Fatalf("cut %q → %q", c, e.Text())
	}
	e.Insert("pasted\n")
	if e.Text() != "one\npasted\nthree" {
		t.Fatalf("paste %q", e.Text())
	}
}

func TestFindReplace(t *testing.T) {
	e := New("listen 80;\nserver_name example.com;\nlisten 443;")
	if !e.Find("LISTEN", false, false) || e.Cur != (Pos{0, 6}) {
		t.Fatalf("find 1 → %+v", e.Cur)
	}
	if !e.Find("listen", false, false) || e.Cur.Line != 2 {
		t.Fatalf("find 2 → %+v", e.Cur)
	}
	if !e.Find("listen", false, false) || e.Cur.Line != 0 {
		t.Fatalf("wrap → %+v", e.Cur)
	}
	if !e.Find("listen", true, false) || e.Cur.Line != 2 {
		t.Fatalf("backward wrap → %+v", e.Cur)
	}
	if n := e.ReplaceAll("listen", "LISTEN", true); n != 2 {
		t.Fatalf("replaced %d", n)
	}
	if !strings.HasPrefix(e.Text(), "LISTEN 80;") {
		t.Fatalf("text %q", e.Text())
	}
	e.Undo()
	if strings.Contains(e.Text(), "LISTEN") {
		t.Fatal("replace all must be one undo step")
	}
	if e.Find("absent", false, false) {
		t.Fatal("should not find")
	}
	// Hangul search.
	h := New("주석: 한글 설정\n한글")
	if !h.Find("한글", false, false) || h.Cur != (Pos{0, 6}) {
		t.Fatalf("hangul find → %+v", h.Cur)
	}
}

func TestMatchBracket(t *testing.T) {
	e := New("server {\n  location / {\n  }\n}")
	e.SetCursor(Pos{0, 7}, false)
	_, m, ok := e.MatchBracket()
	if !ok || m != (Pos{3, 0}) {
		t.Fatalf("match → %+v %v", m, ok)
	}
	e.SetCursor(Pos{2, 3}, false) // after the inner '}'
	_, m, ok = e.MatchBracket()
	if !ok || m != (Pos{1, 13}) {
		t.Fatalf("match back → %+v %v", m, ok)
	}
}

func TestWordAtAndSelectLine(t *testing.T) {
	e := New("proxy_pass http://app;\nx")
	a, z := e.WordAt(Pos{0, 3})
	if a.Col != 0 || z.Col != 10 {
		t.Fatalf("word %+v %+v", a, z)
	}
	e.SelectLine(0)
	if e.SelectedText() != "proxy_pass http://app;\n" {
		t.Fatalf("line %q", e.SelectedText())
	}
}

func TestModifiedTracksSave(t *testing.T) {
	e := New("a")
	typed(e, "b")
	if !e.Modified() {
		t.Fatal("modified")
	}
	e.MarkSaved()
	if e.Modified() {
		t.Fatal("saved")
	}
	typed(e, "c")
	e.Undo()
	if e.Modified() {
		t.Fatal("undo back to saved state")
	}
}

func TestMoveTextDragAndDrop(t *testing.T) {
	// Move forward across lines, with Hangul.
	e := New("한글 alpha\nbeta gamma\n")
	if !e.MoveText(Pos{0, 0}, Pos{0, 3}, Pos{1, 5}, false) { // "한글 " → before "gamma"
		t.Fatal("move refused")
	}
	if got := e.Text(); got != "alpha\nbeta 한글 gamma\n" {
		t.Fatalf("forward move: %q", got)
	}
	if got := e.SelectedText(); got != "한글 " {
		t.Fatalf("moved text should stay selected: %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "한글 alpha\nbeta gamma\n" {
		t.Fatalf("one undo restores the move: %q", got)
	}
	e.Redo()
	if got := e.Text(); got != "alpha\nbeta 한글 gamma\n" {
		t.Fatalf("redo: %q", got)
	}

	// Move backward, including a newline.
	e = New("one\ntwo\nthree")
	e.MoveText(Pos{1, 0}, Pos{2, 0}, Pos{0, 0}, false) // "two\n" to the top
	if got := e.Text(); got != "two\none\nthree" {
		t.Fatalf("backward move: %q", got)
	}

	// Dropping inside the text itself does nothing.
	e = New("abcdef")
	if e.MoveText(Pos{0, 1}, Pos{0, 4}, Pos{0, 2}, false) || e.Text() != "abcdef" || e.Modified() {
		t.Fatalf("drop inside: %q", e.Text())
	}

	// Copy keeps the original.
	e = New("ab cd")
	e.MoveText(Pos{0, 0}, Pos{0, 2}, Pos{0, 5}, true)
	if got := e.Text(); got != "ab cdab" {
		t.Fatalf("copy: %q", got)
	}
}
