package ui

import (
	"strings"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// Input is a single-line text field. It handles Hangul (wide runes),
// horizontal scrolling and a password mode that shows '*' only.
type Input struct {
	Text     []rune
	Cursor   int // rune index
	Password bool
	offset   int // first visible rune
	// Selected range [selA, selZ) (empty: none). A prefilled field starts
	// fully selected, so typing or pasting replaces it, like GUI fields.
	selA, selZ int
}

// InputResult is what an Input reports after a key.
type InputResult int

const (
	InputIgnored InputResult = iota
	InputChanged             // text or cursor changed
	InputSubmit              // Enter
	InputCancel              // Esc
)

// NewInput returns a field with text selected for editing (cursor at end).
func NewInput(text string) *Input {
	r := []rune(text)
	return &Input{Text: r, Cursor: len(r), selZ: len(r)}
}

// Select selects runes [a, z) (e.g. a file name without its extension).
func (in *Input) Select(a, z int) {
	a, z = min(max(a, 0), len(in.Text)), min(max(z, 0), len(in.Text))
	in.selA, in.selZ = min(a, z), max(a, z)
	in.Cursor = in.selZ
}

// Selection returns the selected text ("" if none).
func (in *Input) Selection() string { return string(in.Text[in.selA:in.selZ]) }

func (in *Input) hasSel() bool { return in.selZ > in.selA }

func (in *Input) unselect() { in.selA, in.selZ = 0, 0 }

// deleteSel removes the selected runes and puts the cursor there.
func (in *Input) deleteSel() {
	in.Text = append(in.Text[:in.selA:in.selA], in.Text[in.selZ:]...)
	in.Cursor = in.selA
	in.unselect()
}

// Value returns the text.
func (in *Input) Value() string { return string(in.Text) }

// SetValue replaces the text and moves the cursor to the end.
func (in *Input) SetValue(s string) {
	in.Text = []rune(s)
	in.Cursor = len(in.Text)
	in.selA, in.selZ = 0, len(in.Text)
}

// Clear wipes the text (used for passwords after use).
func (in *Input) Clear() {
	for i := range in.Text {
		in.Text[i] = 0
	}
	in.Text = in.Text[:0]
	in.Cursor, in.offset = 0, 0
	in.unselect()
}

func (in *Input) insert(rs []rune) {
	if in.hasSel() {
		in.deleteSel()
	}
	t := make([]rune, 0, len(in.Text)+len(rs))
	t = append(t, in.Text[:in.Cursor]...)
	t = append(t, rs...)
	t = append(t, in.Text[in.Cursor:]...)
	in.Text = t
	in.Cursor += len(rs)
}

// Paste inserts the first line of s.
func (in *Input) Paste(s string) {
	s, _, _ = strings.Cut(s, "\n")
	in.insert([]rune(strings.TrimRight(s, "\r")))
}

// HandleKey edits the text.
func (in *Input) HandleKey(k term.Key) InputResult {
	n := len(in.Text)
	name := k.String()
	if in.hasSel() {
		switch name {
		case "Left": // collapse to the start of the selection
			in.Cursor = in.selA
			in.unselect()
			return InputChanged
		case "Right":
			in.Cursor = in.selZ
			in.unselect()
			return InputChanged
		case "Backspace", "Delete":
			in.deleteSel()
			return InputChanged
		case "Enter", "Esc":
		default:
			if !k.IsRune() || k.Mod&(term.ModCtrl|term.ModAlt|term.ModMeta) != 0 {
				in.unselect() // Home, End, Ctrl+U, ...: act on the plain text
			}
		}
	}
	switch name {
	case "Enter":
		return InputSubmit
	case "Esc":
		return InputCancel
	case "Left":
		in.Cursor = max(in.Cursor-1, 0)
	case "Right":
		in.Cursor = min(in.Cursor+1, n)
	case "Home", "Ctrl+A":
		in.Cursor = 0
	case "End", "Ctrl+E":
		in.Cursor = n
	case "Backspace":
		if in.Cursor == 0 {
			return InputChanged
		}
		in.Text = append(in.Text[:in.Cursor-1], in.Text[in.Cursor:]...)
		in.Cursor--
	case "Delete":
		if in.Cursor < n {
			in.Text = append(in.Text[:in.Cursor], in.Text[in.Cursor+1:]...)
		}
	case "Ctrl+U":
		in.Text = append([]rune{}, in.Text[in.Cursor:]...)
		in.Cursor = 0
	case "Ctrl+K":
		in.Text = in.Text[:in.Cursor]
	default:
		if k.IsRune() && k.Mod&(term.ModCtrl|term.ModAlt|term.ModMeta) == 0 && k.Rune >= ' ' {
			in.insert([]rune{k.Rune})
			return InputChanged
		}
		return InputIgnored
	}
	return InputChanged
}

// ClickAt moves the cursor to the display column x of the last drawn field.
func (in *Input) ClickAt(x int) {
	in.unselect()
	col := 0
	for i := in.offset; i < len(in.Text); i++ {
		w := in.width(in.Text[i])
		if x < col+w {
			in.Cursor = i
			return
		}
		col += w
	}
	in.Cursor = len(in.Text)
}

func (in *Input) width(r rune) int {
	if in.Password {
		return 1
	}
	return term.RuneWidth(r)
}

// Draw paints the field on s (one row). The cursor cell is shown with
// StyleInputCursor when focused.
func (in *Input) Draw(s wm.Surface, focused bool) {
	w, _ := s.Size()
	if w <= 0 {
		return
	}
	// Scroll so the cursor is visible.
	if in.Cursor < in.offset {
		in.offset = in.Cursor
	}
	for {
		col := 0
		for i := in.offset; i < in.Cursor; i++ {
			col += in.width(in.Text[i])
		}
		if col < w || in.offset >= in.Cursor {
			break
		}
		in.offset++
	}
	s.Clear(wm.StyleInput)
	x := 0
	for i := in.offset; i < len(in.Text) && x < w; i++ {
		r := in.Text[i]
		if in.Password {
			r = '*'
		}
		st := wm.StyleInput
		if in.hasSel() && i >= in.selA && i < in.selZ {
			st = wm.StyleSelection
		}
		if focused && i == in.Cursor {
			st = wm.StyleInputCursor
		}
		x += s.Set(x, 0, r, st)
	}
	if focused && in.Cursor == len(in.Text) {
		cx := 0
		for i := in.offset; i < in.Cursor; i++ {
			cx += in.width(in.Text[i])
		}
		if cx < w {
			s.Set(cx, 0, ' ', wm.StyleInputCursor)
		}
	}
}
