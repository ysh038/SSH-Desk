package ui

import (
	"strings"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// Dialog is a modal message box with buttons.
type Dialog struct {
	Title   string
	Message string // may contain newlines
	Buttons []string
	Focused int
	// Cancel is the button index chosen by Esc (-1: Esc only closes).
	Cancel int
	Rect   wm.Rect
	Chosen int // set when DialogDone is returned (-1 = cancelled)
	ID     string
	Error  bool // draw the title in the error style
}

// DialogResult is what a Dialog reports after handling an event.
type DialogResult int

const (
	DialogNone DialogResult = iota // still open
	DialogDone                     // closed; see Chosen
)

// NewDialog creates a dialog. The last button is the Esc/cancel button.
func NewDialog(id, title, msg string, buttons ...string) *Dialog {
	return &Dialog{ID: id, Title: title, Message: msg, Buttons: buttons, Cancel: len(buttons) - 1}
}

func (d *Dialog) lines() []string { return strings.Split(d.Message, "\n") }

func (d *Dialog) buttonsWidth() int {
	w := 0
	for _, b := range d.Buttons {
		w += term.Width(b) + 4 + 2 // "[ b ]" + gap
	}
	return w - 2
}

// Size is the frame size the dialog needs.
func (d *Dialog) Size() (w, h int) {
	w = max(term.Width(d.Title)+4, d.buttonsWidth())
	for _, l := range d.lines() {
		w = max(w, term.Width(l))
	}
	return w + 4, len(d.lines()) + 5
}

// Center places the dialog in the middle of screen.
func (d *Dialog) Center(screen wm.Rect) {
	w, h := d.Size()
	w, h = min(w, screen.W), min(h, screen.H)
	d.Rect = wm.Rect{X: screen.X + (screen.W-w)/2, Y: screen.Y + (screen.H-h)/2, W: w, H: h}
}

func (d *Dialog) buttonRects() []wm.Rect {
	x := d.Rect.X + (d.Rect.W-d.buttonsWidth())/2
	y := d.Rect.Y + d.Rect.H - 2
	var out []wm.Rect
	for _, b := range d.Buttons {
		w := term.Width(b) + 4
		out = append(out, wm.Rect{X: x, Y: y, W: w, H: 1})
		x += w + 2
	}
	return out
}

func (d *Dialog) done(i int) DialogResult {
	d.Chosen = i
	return DialogDone
}

// HandleEvent processes keys and mouse gestures (screen coordinates).
func (d *Dialog) HandleEvent(ev term.Event) DialogResult {
	n := len(d.Buttons)
	switch ev := ev.(type) {
	case term.KeyEvent:
		switch ev.Key.String() {
		case "Left", "Shift+Tab", "Up":
			d.Focused = (d.Focused - 1 + n) % n
		case "Right", "Tab", "Down":
			d.Focused = (d.Focused + 1) % n
		case "Enter", "Space":
			return d.done(d.Focused)
		case "Esc":
			return d.done(d.Cancel)
		default:
			// Button hotkey: first letter of a button label.
			if ev.Key.IsRune() && ev.Key.Mod == 0 {
				r := strings.ToLower(string(term.ShortcutRune(ev.Key.Rune, term.ModCtrl)))
				for i, b := range d.Buttons {
					if strings.HasPrefix(strings.ToLower(b), r) {
						return d.done(i)
					}
				}
			}
		}
	case term.MouseEvent:
		g := ev.Gesture
		if g.Kind != term.GestureClick || g.Button != term.ButtonLeft {
			return DialogNone
		}
		for i, r := range d.buttonRects() {
			if r.Contains(g.X, g.Y) {
				return d.done(i)
			}
		}
	}
	return DialogNone
}

// Draw paints the dialog.
func (d *Dialog) Draw(c *wm.Canvas) {
	s := wm.NewSurface(c, d.Rect)
	s.Clear(wm.StyleDialog)
	s.Box(wm.Rect{X: 0, Y: 0, W: d.Rect.W, H: d.Rect.H}, wm.StyleDialog)
	titleStyle := wm.StyleDialog
	if d.Error {
		titleStyle = wm.StyleDialogError
	}
	s.Sub(wm.Rect{X: 1, Y: 0, W: d.Rect.W - 2, H: 1}).Text(1, 0, " "+d.Title+" ", titleStyle)
	for i, l := range d.lines() {
		s.Sub(wm.Rect{X: 2, Y: 2 + i, W: d.Rect.W - 4, H: 1}).Text(0, 0, l, wm.StyleDialog)
	}
	for i, r := range d.buttonRects() {
		st := wm.StyleDialogButton
		label := "[ " + d.Buttons[i] + " ]"
		if i == d.Focused {
			st = wm.StyleDialogButtonFocused
			label = "[>" + d.Buttons[i] + "<]"
		}
		c.Text(wm.Rect{X: 0, Y: 0, W: c.W, H: c.H}, r.X, r.Y, label, st)
	}
}
