package ui

import (
	"errors"
	"strings"
	"testing"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

func typeText(in *Input, s string) {
	for _, r := range s {
		in.HandleKey(runeKey(r))
	}
}

func TestInputEditing(t *testing.T) {
	in := NewInput("nginx.conf")
	in.HandleKey(key("Home").Key)
	typeText(in, "old-")
	if in.Value() != "old-nginx.conf" {
		t.Fatalf("value %q", in.Value())
	}
	in.HandleKey(key("End").Key)
	in.HandleKey(key("Backspace").Key)
	in.HandleKey(key("Left").Key)
	in.HandleKey(key("Delete").Key)
	if in.Value() != "old-nginx.co" {
		t.Fatalf("value %q", in.Value())
	}
	in.HandleKey(term.Key{Name: "Rune", Rune: 'u', Mod: term.ModCtrl})
	if in.Value() != "" {
		t.Fatalf("Ctrl+U %q", in.Value())
	}
	typeText(in, "한글 이름")
	if in.Value() != "한글 이름" || in.Cursor != 5 {
		t.Fatalf("hangul %q cursor %d", in.Value(), in.Cursor)
	}
	if in.HandleKey(term.Key{Name: "Rune", Rune: 'x', Mod: term.ModCtrl}) != InputIgnored {
		t.Fatal("Ctrl+X must not insert text")
	}
	in.Paste("first line\nsecond")
	if !strings.HasSuffix(in.Value(), "first line") {
		t.Fatalf("paste %q", in.Value())
	}
}

func TestInputScrollsAndPassword(t *testing.T) {
	in := NewInput(strings.Repeat("가", 20)) // 40 columns
	c := wm.NewCanvas(10, 1)
	in.Draw(wm.NewSurface(c, wm.Rect{X: 0, Y: 0, W: 10, H: 1}), true)
	// 40 columns of text in a 10-column field: the end (cursor) must be visible.
	if got := c.At(8, 0); got.Style != wm.StyleInputCursor {
		t.Fatalf("cursor cell not visible: %+v / row %q", got, c.Row(0))
	}
	pw := &Input{Password: true}
	typeText(pw, "s3cret")
	c = wm.NewCanvas(10, 1)
	pw.Draw(wm.NewSurface(c, wm.Rect{X: 0, Y: 0, W: 10, H: 1}), false)
	if got := c.Row(0); !strings.HasPrefix(got, "******") || strings.Contains(got, "s3cret") {
		t.Fatalf("password row %q", got)
	}
	pw.Clear()
	if pw.Value() != "" {
		t.Fatal("clear")
	}
}

func TestPromptValidateAndSubmit(t *testing.T) {
	var got string
	p := NewPrompt("Rename", "New name:", "a.log", func(v string) { got = v })
	p.Validate = func(v string) error {
		if strings.Contains(v, "/") {
			return errors.New("name must not contain '/'")
		}
		return nil
	}
	p.Layout(wm.Rect{X: 0, Y: 0, W: 80, H: 24})
	typeText(p.Input, "/x")
	if p.HandleEvent(key("Enter")) {
		t.Fatal("invalid value must keep the prompt open")
	}
	c := wm.NewCanvas(80, 24)
	p.Draw(c)
	if !strings.Contains(c.Row(p.Rect.Y+4), "must not contain") {
		t.Fatalf("error row %q", c.Row(p.Rect.Y+4))
	}
	p.Input.HandleKey(key("Backspace").Key)
	p.Input.HandleKey(key("Backspace").Key)
	typeText(p.Input, ".1")
	if !p.HandleEvent(key("Enter")) || got != "a.log.1" {
		t.Fatalf("submit %q", got)
	}
	cancelled := false
	p2 := NewPrompt("x", "y", "", nil)
	p2.OnCancel = func() { cancelled = true }
	if !p2.HandleEvent(key("Esc")) || !cancelled {
		t.Fatal("esc")
	}
}

func TestProgressCancel(t *testing.T) {
	n := 0
	p := &ProgressModal{Title: "Copying", Total: 100, Done: 50, OnCancel: func() { n++ }}
	p.Layout(wm.Rect{X: 0, Y: 0, W: 80, H: 24})
	p.HandleEvent(key("Esc"))
	p.HandleEvent(key("Esc"))
	if n != 1 {
		t.Fatalf("OnCancel called %d times", n)
	}
	if p.HandleEvent(key("Esc")) {
		t.Fatal("must stay open until the owner closes it")
	}
	p.Close()
	if !p.HandleEvent(key("Esc")) {
		t.Fatal("closed")
	}
	if HumanSize(512) != "512" || HumanSize(2150) != "2.1K" || HumanSize(19293798) != "18.4M" {
		t.Fatal("HumanSize")
	}
}

func TestErrorDialogDetails(t *testing.T) {
	opened := false
	m := ErrorDialog("Permission denied", "Cannot move 2 items.", "a.log: permission denied\nb.log: permission denied", func() { opened = true })
	m.Layout(wm.Rect{X: 0, Y: 0, W: 80, H: 24})
	if !m.HandleEvent(runeKeyEvent('d')) || !opened {
		t.Fatal("Details hotkey")
	}
}

func runeKeyEvent(r rune) term.KeyEvent { return term.KeyEvent{Key: runeKey(r)} }
