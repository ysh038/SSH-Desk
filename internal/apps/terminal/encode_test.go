package terminal

import (
	"testing"

	"tuidesk/internal/term"
)

func TestEncodeKey(t *testing.T) {
	r := func(c rune, m term.Mod) term.Key { return term.Key{Name: "Rune", Rune: c, Mod: m} }
	n := func(name string, m term.Mod) term.Key { return term.Key{Name: name, Mod: m} }
	app := Modes{AppCursor: true}
	for _, c := range []struct {
		k    term.Key
		m    Modes
		want string
	}{
		{r('a', 0), Modes{}, "a"},
		{r('한', 0), Modes{}, "한"},
		{r('c', term.ModCtrl), Modes{}, "\x03"},
		{r(' ', term.ModCtrl), Modes{}, "\x00"},
		{r(']', term.ModCtrl), Modes{}, "\x1d"},
		{r('/', term.ModCtrl), Modes{}, "\x1f"},
		{r('b', term.ModAlt), Modes{}, "\x1bb"},
		{r('x', term.ModCtrl | term.ModAlt), Modes{}, "\x1b\x18"},
		{n("Enter", 0), Modes{}, "\r"},
		{n("Backspace", 0), Modes{}, "\x7f"},
		{n("Tab", term.ModShift), Modes{}, "\x1b[Z"},
		{n("Up", 0), Modes{}, "\x1b[A"},
		{n("Up", 0), app, "\x1bOA"},
		{n("Right", term.ModCtrl), app, "\x1b[1;5C"},
		{n("Left", term.ModShift), Modes{}, "\x1b[1;2D"},
		{n("Home", 0), Modes{}, "\x1b[H"},
		{n("End", 0), app, "\x1bOF"},
		{n("PgDn", 0), Modes{}, "\x1b[6~"},
		{n("Delete", term.ModShift), Modes{}, "\x1b[3;2~"},
		{n("F1", 0), Modes{}, "\x1bOP"},
		{n("F10", 0), Modes{}, "\x1b[21~"},
		{n("F6", term.ModShift), Modes{}, "\x1b[17;2~"},
		{n("Esc", 0), Modes{}, "\x1b"},
	} {
		if got := string(EncodeKey(c.k, c.m)); got != c.want {
			t.Errorf("%s: %q, want %q", c.k, got, c.want)
		}
	}
}

func TestEncodePaste(t *testing.T) {
	if got := string(EncodePaste("a\nb", Modes{})); got != "a\rb" {
		t.Fatalf("%q", got)
	}
	if got := string(EncodePaste("x\x1b[201~y", Modes{BracketedPaste: true})); got != "\x1b[200~xy\x1b[201~" {
		t.Fatalf("%q", got)
	}
}

func TestEncodeMouse(t *testing.T) {
	sgr := Modes{MouseNormal: true, MouseSGR: true}
	for _, c := range []struct {
		ev   MouseEvent
		m    Modes
		want string
	}{
		{MouseEvent{X: 0, Y: 0, Button: term.ButtonLeft}, Modes{}, ""},
		{MouseEvent{X: 4, Y: 2, Button: term.ButtonLeft}, sgr, "\x1b[<0;5;3M"},
		{MouseEvent{X: 4, Y: 2, Button: term.ButtonLeft, Release: true}, sgr, "\x1b[<0;5;3m"},
		{MouseEvent{X: 4, Y: 2, Button: term.ButtonRight, Mod: term.ModCtrl}, sgr, "\x1b[<18;5;3M"},
		{MouseEvent{X: 1, Y: 1, Wheel: term.WheelDown}, sgr, "\x1b[<65;2;2M"},
		{MouseEvent{X: 1, Y: 1, Button: term.ButtonLeft, Motion: true}, sgr, ""}, // 1000 has no motion
		{MouseEvent{X: 1, Y: 1, Button: term.ButtonLeft, Motion: true}, Modes{MouseButton: true, MouseSGR: true}, "\x1b[<32;2;2M"},
		{MouseEvent{X: 0, Y: 0, Button: term.ButtonLeft}, Modes{MouseNormal: true}, "\x1b[M !!"},
		{MouseEvent{X: 0, Y: 0, Button: term.ButtonLeft, Release: true}, Modes{MouseNormal: true}, "\x1b[M#!!"},
	} {
		if got := string(EncodeMouse(c.ev, c.m)); got != c.want {
			t.Errorf("%+v: %q, want %q", c.ev, got, c.want)
		}
	}
}
