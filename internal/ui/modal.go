package ui

import (
	"fmt"
	"strings"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// Modal is a popup that receives all input until it closes. The desktop keeps
// a stack of them; only the top one gets events.
type Modal interface {
	// Layout positions the modal inside screen (called on open and resize).
	Layout(screen wm.Rect)
	// HandleEvent returns true when the modal has closed.
	HandleEvent(ev term.Event) (closed bool)
	Draw(c *wm.Canvas)
}

// ---- Dialog as a modal ----

type dialogModal struct {
	d       *Dialog
	onClose func(choice int)
}

// AsModal wraps a Dialog; onClose gets the chosen button index (or Cancel).
func AsModal(d *Dialog, onClose func(choice int)) Modal { return &dialogModal{d: d, onClose: onClose} }

func (m *dialogModal) Layout(screen wm.Rect) { m.d.Center(screen) }
func (m *dialogModal) Draw(c *wm.Canvas)     { m.d.Draw(c) }
func (m *dialogModal) HandleEvent(ev term.Event) bool {
	if m.d.HandleEvent(ev) != DialogDone {
		return false
	}
	if m.onClose != nil {
		m.onClose(m.d.Chosen)
	}
	return true
}

// ---- Prompt: a dialog with a text field ----

// Prompt asks for one line of text.
type Prompt struct {
	Title    string
	Label    string
	Input    *Input
	Validate func(string) error // optional; an error keeps the prompt open
	OnOK     func(string)
	// OnSecret, when set, receives a copy of the runes instead of OnOK and
	// the field is wiped right away (passwords never become Go strings).
	OnSecret func([]rune)
	OnCancel func()
	Rect     wm.Rect
	errMsg   string
	focus    int // 0 field, 1 OK, 2 Cancel
}

// NewPrompt creates a prompt with an initial value.
func NewPrompt(title, label, initial string, onOK func(string)) *Prompt {
	return &Prompt{Title: title, Label: label, Input: NewInput(initial), OnOK: onOK}
}

func (p *Prompt) Layout(screen wm.Rect) {
	w := min(max(50, term.Width(p.Label)+6, term.Width(p.Title)+8), screen.W)
	h := min(8, screen.H)
	p.Rect = wm.Rect{X: screen.X + (screen.W-w)/2, Y: screen.Y + (screen.H-h)/3, W: w, H: h}
}

func (p *Prompt) fieldRect() wm.Rect {
	return wm.Rect{X: p.Rect.X + 2, Y: p.Rect.Y + 3, W: p.Rect.W - 4, H: 1}
}

func (p *Prompt) buttons() []wm.Rect {
	y := p.Rect.Y + p.Rect.H - 2
	ok := wm.Rect{X: p.Rect.X + p.Rect.W/2 - 11, Y: y, W: 8, H: 1}
	cancel := wm.Rect{X: p.Rect.X + p.Rect.W/2 + 1, Y: y, W: 10, H: 1}
	return []wm.Rect{ok, cancel}
}

func (p *Prompt) submit() bool {
	if p.OnSecret != nil {
		secret := append([]rune(nil), p.Input.Text...)
		p.Input.Clear()
		p.OnSecret(secret)
		return true
	}
	v := p.Input.Value()
	if p.Validate != nil {
		if err := p.Validate(v); err != nil {
			p.errMsg = err.Error()
			p.focus = 0
			return false
		}
	}
	if p.OnOK != nil {
		p.OnOK(v)
	}
	return true
}

func (p *Prompt) cancel() bool {
	p.Input.Clear()
	if p.OnCancel != nil {
		p.OnCancel()
	}
	return true
}

func (p *Prompt) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		switch ev.Key.String() {
		case "Tab":
			p.focus = (p.focus + 1) % 3
			return false
		case "Shift+Tab":
			p.focus = (p.focus + 2) % 3
			return false
		case "Esc":
			return p.cancel()
		case "Enter":
			if p.focus == 2 {
				return p.cancel()
			}
			return p.submit()
		}
		if p.focus == 0 {
			if p.Input.HandleKey(ev.Key) == InputChanged {
				p.errMsg = ""
			}
		}
	case term.PasteEvent:
		p.Input.Paste(ev.Text)
	case term.MouseEvent:
		g := ev.Gesture
		if g.Kind != term.GestureClick || g.Button != term.ButtonLeft {
			return false
		}
		if f := p.fieldRect(); f.Contains(g.X, g.Y) {
			p.focus = 0
			p.Input.ClickAt(g.X - f.X)
		}
		b := p.buttons()
		if b[0].Contains(g.X, g.Y) {
			return p.submit()
		}
		if b[1].Contains(g.X, g.Y) {
			return p.cancel()
		}
	}
	return false
}

func (p *Prompt) Draw(c *wm.Canvas) {
	s := wm.NewSurface(c, p.Rect)
	s.Clear(wm.StyleDialog)
	s.Box(wm.Rect{X: 0, Y: 0, W: p.Rect.W, H: p.Rect.H}, wm.StyleDialog)
	s.Sub(wm.Rect{X: 1, Y: 0, W: p.Rect.W - 2, H: 1}).Text(1, 0, " "+p.Title+" ", wm.StyleDialog)
	s.Sub(wm.Rect{X: 2, Y: 2, W: p.Rect.W - 4, H: 1}).Text(0, 0, p.Label, wm.StyleDialog)
	f := p.fieldRect()
	p.Input.Draw(wm.NewSurface(c, f), p.focus == 0)
	if p.errMsg != "" {
		s.Sub(wm.Rect{X: 2, Y: 4, W: p.Rect.W - 4, H: 1}).Text(0, 0, p.errMsg, wm.StyleDialogError)
	}
	labels := []string{"OK", "Cancel"}
	for i, r := range p.buttons() {
		drawButton(c, r, labels[i], p.focus == i+1)
	}
}

func drawButton(c *wm.Canvas, r wm.Rect, label string, focused bool) {
	st := wm.StyleDialogButton
	text := "[ " + label + " ]"
	if focused {
		st = wm.StyleDialogButtonFocused
		text = "[>" + label + "<]"
	}
	c.Text(wm.Rect{X: 0, Y: 0, W: c.W, H: c.H}, r.X, r.Y, text, st)
}

// ---- Progress ----

// ProgressModal shows a long operation with a Cancel button.
type ProgressModal struct {
	Title   string
	Done    int64
	Total   int64
	Current string
	Rect    wm.Rect
	// OnCancel is called once when the user cancels; the modal stays open
	// until the owner closes it (the operation has to stop first).
	OnCancel  func()
	cancelled bool
	closed    bool
}

func (p *ProgressModal) Layout(screen wm.Rect) {
	w, h := min(56, screen.W), min(7, screen.H)
	p.Rect = wm.Rect{X: screen.X + (screen.W-w)/2, Y: screen.Y + (screen.H-h)/3, W: w, H: h}
}

// Close lets the owner dismiss the modal; the desktop drops it on the next event.
func (p *ProgressModal) Close() { p.closed = true }

// Closed reports whether the owner closed it.
func (p *ProgressModal) Closed() bool { return p.closed }

func (p *ProgressModal) cancelRect() wm.Rect {
	return wm.Rect{X: p.Rect.X + (p.Rect.W-10)/2, Y: p.Rect.Y + p.Rect.H - 2, W: 10, H: 1}
}

func (p *ProgressModal) cancel() {
	if !p.cancelled && p.OnCancel != nil {
		p.cancelled = true
		p.OnCancel()
	}
}

func (p *ProgressModal) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		if s := ev.Key.String(); s == "Esc" || s == "Enter" {
			p.cancel()
		}
	case term.MouseEvent:
		if g := ev.Gesture; g.Kind == term.GestureClick && p.cancelRect().Contains(g.X, g.Y) {
			p.cancel()
		}
	}
	return p.closed
}

func (p *ProgressModal) Draw(c *wm.Canvas) {
	s := wm.NewSurface(c, p.Rect)
	s.Clear(wm.StyleDialog)
	s.Box(wm.Rect{X: 0, Y: 0, W: p.Rect.W, H: p.Rect.H}, wm.StyleDialog)
	s.Sub(wm.Rect{X: 1, Y: 0, W: p.Rect.W - 2, H: 1}).Text(1, 0, " "+p.Title+" ", wm.StyleDialog)
	s.Sub(wm.Rect{X: 2, Y: 1, W: p.Rect.W - 4, H: 1}).Text(0, 0, p.Current, wm.StyleDialog)
	barW := p.Rect.W - 4
	filled := 0
	if p.Total > 0 {
		filled = int(int64(barW) * min(p.Done, p.Total) / p.Total)
	}
	bar := s.Sub(wm.Rect{X: 2, Y: 2, W: barW, H: 1})
	for x := 0; x < barW; x++ {
		if x < filled {
			bar.Set(x, 0, '#', wm.StyleSelected)
		} else {
			bar.Set(x, 0, '.', wm.StyleDialog)
		}
	}
	s.Sub(wm.Rect{X: 2, Y: 3, W: p.Rect.W - 4, H: 1}).Text(0, 0, fmt.Sprintf("%s / %s", HumanSize(p.Done), HumanSize(p.Total)), wm.StyleDialog)
	label := "Cancel"
	if p.cancelled {
		label = "Stopping"
	}
	drawButton(c, p.cancelRect(), label, true)
}

// HumanSize formats a byte count: 512, 2.1K, 18.4M, 3.0G.
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTP"[exp])
}

// ErrorDialog builds the standard error popup. With details, a "Details"
// button is added; onDetails is called when it is chosen.
func ErrorDialog(title, msg, details string, onDetails func()) Modal {
	buttons := []string{"OK"}
	if details != "" {
		buttons = []string{"OK", "Details"}
	}
	d := NewDialog("error", title, wrap(msg, 60), buttons...)
	d.Cancel = 0
	d.Error = true
	return AsModal(d, func(choice int) {
		if choice == 1 && onDetails != nil {
			onDetails()
		}
	})
}

// wrap breaks long lines at spaces so dialogs stay narrow.
func wrap(s string, width int) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		for term.Width(line) > width {
			cut := -1
			w := 0
			for i, r := range line {
				w += term.RuneWidth(r)
				if w > width {
					break
				}
				if r == ' ' {
					cut = i
				}
			}
			if cut <= 0 {
				break
			}
			out = append(out, line[:cut])
			line = line[cut+1:]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
