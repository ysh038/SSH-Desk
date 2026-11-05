package desktop

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"tuidesk/internal/term"
)

// The wheel scrolls the editor without moving the cursor, and Draw must not
// pull the view back to the cursor (it used to stop after a few notches).
func TestEditorWheelScrollStays(t *testing.T) {
	f := newFM(t)
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "line %03d\n", i)
	}
	f.touch("long.conf", b.String())
	f.withEditor(&fakeRunner{})
	f.d.OpenText(filepath.Join(f.root, "long.conf"))
	ed := f.editor()
	in := f.d.M.Focused().Inner()
	textY := in.Y + 2
	wheel := func(dir term.Wheel, n int) {
		for range n {
			f.d.Handle(term.MouseEvent{Gesture: term.Gesture{Kind: term.GestureWheel, Wheel: dir, X: in.X + 8, Y: textY + 3}})
			f.draw()
		}
	}
	wheel(term.WheelDown, 20) // 3 lines per notch
	if row := f.draw().Row(textY); !strings.Contains(row, "line 061") {
		t.Fatalf("after 20 notches down, top row %q", row)
	}
	wheel(term.WheelUp, 5)
	if row := f.draw().Row(textY); !strings.Contains(row, "line 046") {
		t.Fatalf("after 5 notches up, top row %q", row)
	}
	if c := ed.Doc().Ed.Cur; c.Line != 0 {
		t.Fatalf("the wheel must not move the cursor: %v", c)
	}
	f.key("Down", 0) // a key brings the cursor back into view
	if row := f.draw().Row(textY); !strings.Contains(row, "line 001") && !strings.Contains(row, "line 002") {
		t.Fatalf("Down after scrolling should show the cursor, top row %q", row)
	}
}

// Dragging selected text moves it, like a GUI editor; Ctrl+Z undoes it in
// one step. A click inside the selection just places the cursor.
func TestEditorDragMovesSelectedText(t *testing.T) {
	f := newFM(t)
	f.touch("m.txt", "hello world\n한글 line\n")
	f.d.SetConfig(pcConfig(), "")
	f.withEditor(&fakeRunner{})
	f.d.OpenText(filepath.Join(f.root, "m.txt"))
	ed := f.editor()
	in := f.d.M.Focused().Inner()
	f.draw()
	g := 3 // gutter: 1 digit + 2
	textY := in.Y + 2
	e := ed.Doc().Ed

	// Double-click "world", then drag it to the start of line 2.
	f.click(in.X+g+7, textY)
	f.click(in.X+g+7, textY)
	f.wait()
	if s := e.SelectedText(); s != "world" {
		t.Fatalf("double click → %q", s)
	}
	f.drag(in.X+g+8, textY, in.X+g, textY+1)
	if got := e.Text(); got != "hello \nworld한글 line\n" {
		t.Fatalf("after drag-move: %q", got)
	}
	if s := e.SelectedText(); s != "world" {
		t.Fatalf("moved text stays selected: %q", s)
	}
	f.rune('z', term.ModCtrl)
	if got := e.Text(); got != "hello world\n한글 line\n" {
		t.Fatalf("one undo: %q", got)
	}

	// Drag into the middle of a Hangul line (after "한글", 4 columns).
	e.Select(e.PosAt(0), e.PosAt(5)) // "hello"
	f.draw()
	f.wait()
	f.drag(in.X+g+1, textY, in.X+g+4, textY+1)
	if got := e.Text(); got != " world\n한글hello line\n" {
		t.Fatalf("drop after Hangul: %q", got)
	}

	// A plain click inside a selection places the cursor and clears it.
	f.wait()
	f.click(in.X+g+2, textY+1)
	if _, _, sel := e.Selection(); sel {
		t.Fatalf("click inside the selection should clear it, still %q", e.SelectedText())
	}
}

// Every Shift+movement extends the selection from where the cursor was:
// Shift+Option+←/→ (Alt+Shift+Left/Right once Terminal.app sends it) by
// words, Shift+Home/End to the line ends, Shift+Option+↑/↓ to the document.
func TestEditorShiftExtendsAnyMove(t *testing.T) {
	f := newFM(t)
	f.touch("s.conf", "server_name 한글.example.com;\nlisten 80;\n")
	mac := pcConfig()
	mac.Keyboard = "mac"
	f.d.SetConfig(mac, "")
	f.withEditor(&fakeRunner{})
	f.d.OpenText(filepath.Join(f.root, "s.conf"))
	e := f.editor().Doc().Ed

	f.key("Right", term.ModAlt|term.ModShift)
	if s := e.SelectedText(); s != "server_name" {
		t.Fatalf("Shift+Option+→: %q", s)
	}
	f.key("Right", term.ModAlt|term.ModShift)
	if s := e.SelectedText(); s != "server_name 한글" { // Mac: to the end of the next word
		t.Fatalf("second Shift+Option+→ adds the next word: %q", s)
	}
	f.key("Left", term.ModAlt|term.ModShift)
	if s := e.SelectedText(); s != "server_name " {
		t.Fatalf("Shift+Option+← shrinks it back: %q", s)
	}
	f.key("End", term.ModShift)
	if s := e.SelectedText(); s != "server_name 한글.example.com;" {
		t.Fatalf("Shift+End: %q", s)
	}
	f.key("Down", term.ModAlt|term.ModShift) // to the end of the document
	if s := e.SelectedText(); s != "server_name 한글.example.com;\nlisten 80;" { // the final newline is kept apart
		t.Fatalf("Shift+Option+↓: %q", s)
	}
	f.key("Left", 0) // a plain move ends the selection
	if _, _, sel := e.Selection(); sel {
		t.Fatal("plain Left should clear the selection")
	}
}

func TestEditorDoubleClickSelectsWord(t *testing.T) {
	f := newFM(t)
	f.touch("w.conf", "proxy_pass http://한글서버:8080;    # note\n")
	f.withEditor(&fakeRunner{})
	f.d.OpenText(filepath.Join(f.root, "w.conf"))
	ed := f.editor()
	in := f.d.M.Focused().Inner()
	f.draw()
	g, y := 3, in.Y+2
	dbl := func(col int) string {
		f.wait()
		f.click(in.X+g+col, y)
		f.click(in.X+g+col, y)
		return ed.Doc().Ed.SelectedText()
	}
	if s := dbl(3); s != "proxy_pass" {
		t.Fatalf("word with _: %q", s)
	}
	if s := dbl(20); s != "한글서버" { // "proxy_pass http://" is 18 columns; Hangul is 2 wide
		t.Fatalf("Hangul word: %q", s)
	}
	if s := dbl(32); s != "    " {
		t.Fatalf("blank run: %q", s)
	}
}
