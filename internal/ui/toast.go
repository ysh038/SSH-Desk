package ui

import (
	"time"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// Toast is a short notification shown in the bottom-right corner.
type Toast struct {
	Text    string
	Error   bool
	Expires time.Time
}

// Toasts is a queue of notifications. Expiry is driven by Tick, so the
// caller only needs a timer for NextDeadline (no periodic redraw).
type Toasts struct {
	Items    []Toast
	Duration time.Duration
}

// Push adds a toast that expires after Duration (default 4s).
func (t *Toasts) Push(now time.Time, text string, isErr bool) {
	d := t.Duration
	if d == 0 {
		d = 4 * time.Second
	}
	t.Items = append(t.Items, Toast{Text: text, Error: isErr, Expires: now.Add(d)})
	if len(t.Items) > 5 {
		t.Items = t.Items[len(t.Items)-5:]
	}
}

// Tick drops expired toasts and reports whether anything changed.
func (t *Toasts) Tick(now time.Time) bool {
	kept := t.Items[:0]
	for _, it := range t.Items {
		if now.Before(it.Expires) {
			kept = append(kept, it)
		}
	}
	changed := len(kept) != len(t.Items)
	t.Items = kept
	return changed
}

// NextDeadline returns the earliest expiry (zero time if none).
func (t *Toasts) NextDeadline() time.Time {
	var next time.Time
	for _, it := range t.Items {
		if next.IsZero() || it.Expires.Before(next) {
			next = it.Expires
		}
	}
	return next
}

// Draw stacks toasts upward from the bottom-right corner of area.
func (t *Toasts) Draw(c *wm.Canvas, area wm.Rect) {
	y := area.Y + area.H - 1
	for i := len(t.Items) - 1; i >= 0 && y >= area.Y; i-- {
		it := t.Items[i]
		st := wm.StyleToast
		if it.Error {
			st = wm.StyleToastError
		}
		text := " " + it.Text + " "
		w := min(term.Width(text), area.W)
		r := wm.Rect{X: area.X + area.W - w, Y: y, W: w, H: 1}
		s := wm.NewSurface(c, r)
		s.Clear(st)
		s.Text(0, 0, text, st)
		y--
	}
}
