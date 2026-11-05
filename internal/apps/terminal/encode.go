// Package terminal is the terminal window (SPEC 7.5): a shell or program on a
// pseudo-terminal, shown through a VT emulator (ADR 0004).
package terminal

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"tuidesk/internal/term"
)

// Modes are the terminal modes the program running inside has set. They
// decide how keys, mouse and paste are encoded.
type Modes struct {
	AppCursor      bool // DECCKM (?1): arrows send ESC O A
	MouseX10       bool // ?9: press only
	MouseNormal    bool // ?1000: press and release
	MouseButton    bool // ?1002: also motion while a button is held
	MouseAny       bool // ?1003: all motion (we only see motion with a button)
	MouseSGR       bool // ?1006: CSI < b;x;y M/m encoding
	BracketedPaste bool // ?2004
}

// MouseOn reports that the program wants mouse events.
func (m Modes) MouseOn() bool { return m.MouseX10 || m.MouseNormal || m.MouseButton || m.MouseAny }

// xterm modifier parameter: 1 + Shift(1) + Alt(2) + Ctrl(4) (+ Meta as Alt).
func modParam(m term.Mod) int {
	p := 1
	if m&term.ModShift != 0 {
		p++
	}
	if m&(term.ModAlt|term.ModMeta) != 0 {
		p += 2
	}
	if m&term.ModCtrl != 0 {
		p += 4
	}
	return p
}

var csiLetter = map[string]byte{"Up": 'A', "Down": 'B', "Right": 'C', "Left": 'D', "Home": 'H', "End": 'F'}

var tildeNum = map[string]int{
	"Insert": 2, "Delete": 3, "PgUp": 5, "PgDn": 6,
	"F5": 15, "F6": 17, "F7": 18, "F8": 19, "F9": 20, "F10": 21, "F11": 23, "F12": 24,
}

var ss3F = map[string]byte{"F1": 'P', "F2": 'Q', "F3": 'R', "F4": 'S'}

// ctrlByte is the control character for Ctrl+r (ok=false if none).
func ctrlByte(r rune) (byte, bool) {
	switch {
	case r >= 'a' && r <= 'z':
		return byte(r-'a') + 1, true
	case r >= 'A' && r <= 'Z':
		return byte(r-'A') + 1, true
	}
	switch r {
	case ' ', '@', '2':
		return 0, true
	case '[', '3':
		return 0x1b, true
	case '\\', '4':
		return 0x1c, true
	case ']', '5':
		return 0x1d, true
	case '^', '6':
		return 0x1e, true
	case '_', '/', '7':
		return 0x1f, true
	case '?', '8':
		return 0x7f, true
	}
	return 0, false
}

// EncodeKey returns the bytes a key press sends to the program (xterm
// conventions, as TERM=xterm-256color promises). nil means "nothing".
func EncodeKey(k term.Key, m Modes) []byte {
	alt := k.Mod&(term.ModAlt|term.ModMeta) != 0
	prefix := ""
	if alt {
		prefix = "\x1b"
	}
	if k.IsRune() {
		if k.Mod&term.ModCtrl != 0 {
			if b, ok := ctrlByte(k.Rune); ok {
				return []byte(prefix + string([]byte{b}))
			}
		}
		buf := make([]byte, 0, 8)
		buf = append(buf, prefix...)
		return utf8.AppendRune(buf, k.Rune)
	}
	name := k.Name
	mods := k.Mod
	switch name {
	case "Enter":
		return []byte(prefix + "\r")
	case "Tab":
		if mods&term.ModShift != 0 {
			return []byte(prefix + "\x1b[Z")
		}
		return []byte(prefix + "\t")
	case "Backspace":
		if mods&term.ModCtrl != 0 {
			return []byte(prefix + "\x08")
		}
		return []byte(prefix + "\x7f")
	case "Esc":
		return []byte(prefix + "\x1b")
	}
	p := modParam(mods)
	if c, ok := csiLetter[name]; ok {
		switch {
		case p > 1:
			return []byte(fmt.Sprintf("\x1b[1;%d%c", p, c))
		case m.AppCursor:
			return []byte{0x1b, 'O', c}
		default:
			return []byte{0x1b, '[', c}
		}
	}
	if c, ok := ss3F[name]; ok {
		if p > 1 {
			return []byte(fmt.Sprintf("\x1b[1;%d%c", p, c))
		}
		return []byte{0x1b, 'O', c}
	}
	if n, ok := tildeNum[name]; ok {
		if p > 1 {
			return []byte(fmt.Sprintf("\x1b[%d;%d~", n, p))
		}
		return []byte(fmt.Sprintf("\x1b[%d~", n))
	}
	return nil
}

// EncodePaste wraps text in bracketed-paste markers when the program asked
// for them, and turns line feeds into carriage returns (what a real terminal
// sends for Enter). Embedded end markers are removed so pasted text cannot
// end the paste early.
func EncodePaste(text string, m Modes) []byte {
	text = strings.ReplaceAll(text, "\r\n", "\r")
	text = strings.ReplaceAll(text, "\n", "\r")
	if !m.BracketedPaste {
		return []byte(text)
	}
	text = strings.ReplaceAll(text, "\x1b[201~", "")
	return []byte("\x1b[200~" + text + "\x1b[201~")
}

// MouseEvent is a mouse report for the program, at 0-based cell (X, Y).
type MouseEvent struct {
	X, Y    int
	Button  term.Button // ButtonNone for motion without a button
	Wheel   term.Wheel
	Release bool
	Motion  bool
	Mod     term.Mod
}

// EncodeMouse encodes ev for the program's mouse mode (nil if the mode does
// not report this event).
func EncodeMouse(ev MouseEvent, m Modes) []byte {
	if !m.MouseOn() {
		return nil
	}
	if ev.Motion && !(m.MouseButton || m.MouseAny) {
		return nil
	}
	if ev.Release && m.MouseX10 && !(m.MouseNormal || m.MouseButton || m.MouseAny) {
		return nil
	}
	var cb int
	switch {
	case ev.Wheel == term.WheelUp:
		cb = 64
	case ev.Wheel == term.WheelDown:
		cb = 65
	case ev.Wheel == term.WheelLeft:
		cb = 66
	case ev.Wheel == term.WheelRight:
		cb = 67
	case ev.Button == term.ButtonLeft:
		cb = 0
	case ev.Button == term.ButtonMiddle:
		cb = 1
	case ev.Button == term.ButtonRight:
		cb = 2
	default:
		cb = 3 // no button (motion) or legacy release
	}
	if ev.Motion {
		cb += 32
	}
	if !m.MouseX10 || m.MouseNormal || m.MouseButton || m.MouseAny {
		if ev.Mod&term.ModShift != 0 {
			cb += 4
		}
		if ev.Mod&(term.ModAlt|term.ModMeta) != 0 {
			cb += 8
		}
		if ev.Mod&term.ModCtrl != 0 {
			cb += 16
		}
	}
	x, y := ev.X+1, ev.Y+1
	if m.MouseSGR {
		final := 'M'
		if ev.Release {
			final = 'm'
		}
		return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", cb, x, y, final))
	}
	if ev.Release {
		cb = 3 | cb&^3 // legacy encoding cannot say which button was released
	}
	if x > 223 || y > 223 {
		return nil // does not fit the legacy encoding
	}
	return []byte{0x1b, '[', 'M', byte(32 + cb), byte(32 + x), byte(32 + y)}
}
