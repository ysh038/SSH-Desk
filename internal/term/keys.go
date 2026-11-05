package term

import (
	"fmt"
	"strings"
	"unicode"
)

// Key is a key press normalized across terminals. It does not depend on tcell.
//
// Name is "Rune" for printable input (Rune holds the character) and otherwise
// one of: F1…F24, Up, Down, Left, Right, Home, End, PgUp, PgDn, Insert,
// Delete, Enter, Tab, Esc, Backspace.
type Key struct {
	Name string
	Rune rune
	Mod  Mod
}

// IsRune reports whether k is printable input.
func (k Key) IsRune() bool { return k.Name == "Rune" }

// String returns the canonical binding name, e.g. "Ctrl+W", "Shift+F6",
// "Esc", "a", "한", "Space". Modifier order is Ctrl, Alt, Meta, Shift.
func (k Key) String() string {
	var b strings.Builder
	for _, p := range []struct {
		bit  Mod
		name string
	}{{ModCtrl, "Ctrl"}, {ModAlt, "Alt"}, {ModMeta, "Meta"}, {ModShift, "Shift"}} {
		if k.Mod&p.bit != 0 {
			b.WriteString(p.name)
			b.WriteByte('+')
		}
	}
	switch {
	case !k.IsRune():
		b.WriteString(k.Name)
	case k.Rune == ' ':
		b.WriteString("Space")
	case k.Mod&(ModCtrl|ModAlt|ModMeta) != 0:
		b.WriteRune(unicode.ToUpper(k.Rune))
	default:
		b.WriteRune(k.Rune)
	}
	return b.String()
}

// KeyProfile describes how a terminal reports shifted function keys.
type KeyProfile struct {
	// ShiftFOffset is how far Shift moves a function key number: the Linux
	// console reports Shift+F3 as F13 (offset 10), xterm-style terminfo
	// reports Shift+F1 as F13 (offset 12). 0 disables the mapping.
	ShiftFOffset int
	// FirstShiftedF is the lowest function key that can only be produced
	// with Shift (F13 on both). Lower keys are left alone because on the
	// Linux console F11/F12 are real keys as well as Shift+F1/F2.
	FirstShiftedF int
}

// ProfileFor returns the key profile for a $TERM value.
func ProfileFor(termName string) KeyProfile {
	if termName == "linux" {
		return KeyProfile{ShiftFOffset: 10, FirstShiftedF: 13}
	}
	return KeyProfile{ShiftFOffset: 12, FirstShiftedF: 13}
}

// Normalize maps terminal-specific reports onto canonical keys:
//   - Ctrl/Alt + 2-set Hangul jamo → the QWERTY letter (Ctrl+ㅊ → Ctrl+C)
//   - Ctrl/Alt + letter → lower-case letter (Ctrl+W and Ctrl+w are the same)
//   - shifted function keys reported as F13… → Shift+Fn (Linux console:
//     F20 → Shift+F10)
//   - Ctrl+M → Enter, Ctrl+I → Tab (Alt+Enter on the console is Alt+Ctrl+M)
func (p KeyProfile) Normalize(k Key) Key {
	if k.IsRune() && k.Mod&(ModCtrl|ModAlt|ModMeta) != 0 {
		k.Rune = unicode.ToLower(ShortcutRune(k.Rune, k.Mod))
	}
	// Ctrl+M and Ctrl+I are the same bytes as Enter and Tab. The Linux console
	// sends Alt+Enter as ESC CR, which arrives as Alt+Ctrl+M.
	if k.IsRune() && k.Mod&ModCtrl != 0 {
		switch k.Rune {
		case 'm':
			k = Key{Name: "Enter", Mod: k.Mod &^ ModCtrl}
		case 'i':
			k = Key{Name: "Tab", Mod: k.Mod &^ ModCtrl}
		}
	}
	var n int
	if p.ShiftFOffset > 0 && k.Mod&ModShift == 0 {
		if _, err := fmt.Sscanf(k.Name, "F%d", &n); err == nil && n >= p.FirstShiftedF && n-p.ShiftFOffset >= 1 {
			k.Name = fmt.Sprintf("F%d", n-p.ShiftFOffset)
			k.Mod |= ModShift
		}
	}
	return k
}
