package term

import "testing"

func TestKeyString(t *testing.T) {
	cases := []struct {
		k    Key
		want string
	}{
		{Key{Name: "Rune", Rune: 'w', Mod: ModCtrl}, "Ctrl+W"},
		{Key{Name: "Rune", Rune: 'a'}, "a"},
		{Key{Name: "Rune", Rune: '한'}, "한"},
		{Key{Name: "Rune", Rune: ' ', Mod: ModCtrl}, "Ctrl+Space"},
		{Key{Name: "F6", Mod: ModShift}, "Shift+F6"},
		{Key{Name: "Esc"}, "Esc"},
		{Key{Name: "Left", Mod: ModCtrl | ModShift}, "Ctrl+Shift+Left"},
		{Key{Name: "Rune", Rune: 'x', Mod: ModAlt}, "Alt+X"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("%+v → %q, want %q", c.k, got, c.want)
		}
	}
}

func TestNormalizeJamoShortcut(t *testing.T) {
	p := ProfileFor("xterm-256color")
	k := p.Normalize(Key{Name: "Rune", Rune: 'ㅈ', Mod: ModCtrl})
	if k.String() != "Ctrl+W" {
		t.Fatalf("got %s", k)
	}
	// Typing Hangul without modifiers is untouched.
	if k := p.Normalize(Key{Name: "Rune", Rune: 'ㅈ'}); k.String() != "ㅈ" {
		t.Fatalf("got %s", k)
	}
}

func TestNormalizeLinuxConsoleShiftF(t *testing.T) {
	p := ProfileFor("linux")
	cases := map[string]string{
		"F20": "Shift+F10", // measured in the QEMU VT
		"F16": "Shift+F6",
		"F13": "Shift+F3",
		"F11": "F11", // ambiguous with Shift+F1: keep as F11
		"F6":  "F6",
	}
	for in, want := range cases {
		if got := p.Normalize(Key{Name: in}).String(); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}

func TestNormalizeXtermShiftF(t *testing.T) {
	p := ProfileFor("xterm-256color")
	if got := p.Normalize(Key{Name: "F18"}).String(); got != "Shift+F6" {
		t.Fatalf("got %s", got)
	}
	if got := p.Normalize(Key{Name: "F22"}).String(); got != "Shift+F10" {
		t.Fatalf("got %s", got)
	}
	// Already-decoded modifiers are left alone.
	if got := p.Normalize(Key{Name: "F6", Mod: ModShift}).String(); got != "Shift+F6" {
		t.Fatalf("got %s", got)
	}
}

func TestNormalizeAltEnterFromConsole(t *testing.T) {
	p := ProfileFor("linux")
	// Measured in the QEMU VT: Alt+Enter arrives as Alt+Ctrl+M.
	if got := p.Normalize(Key{Name: "Rune", Rune: 'm', Mod: ModCtrl | ModAlt}).String(); got != "Alt+Enter" {
		t.Fatalf("got %s", got)
	}
	if got := p.Normalize(Key{Name: "Rune", Rune: 'i', Mod: ModCtrl}).String(); got != "Tab" {
		t.Fatalf("got %s", got)
	}
}
