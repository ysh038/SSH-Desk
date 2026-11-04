package ui

import (
	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// ListSource provides rows lazily so lists of 100k+ entries stay cheap:
// only visible rows are requested.
type ListSource interface {
	Len() int
	Item(i int) string
}

// Strings adapts a slice to ListSource.
type Strings []string

func (s Strings) Len() int          { return len(s) }
func (s Strings) Item(i int) string { return s[i] }

// List is a virtual-scrolling list with a vertical scrollbar.
type List struct {
	Src      ListSource
	Selected int
	Offset   int // first visible row
	Height   int // visible rows, set by Draw (or by the owner before handling keys)
}

// ListResult is what a List reports after handling an event.
type ListResult int

const (
	ListNone      ListResult = iota // ignored
	ListChanged                     // selection or scroll changed
	ListActivated                   // Enter or double-click on Selected
)

func (l *List) clamp() {
	n := l.Src.Len()
	l.Selected = min(max(l.Selected, 0), max(n-1, 0))
	h := max(l.Height, 1)
	if l.Selected < l.Offset {
		l.Offset = l.Selected
	}
	if l.Selected >= l.Offset+h {
		l.Offset = l.Selected - h + 1
	}
	l.Offset = min(max(l.Offset, 0), max(n-h, 0))
}

// HandleEvent processes keys and gestures in the list's local coordinates.
func (l *List) HandleEvent(ev term.Event) ListResult {
	n := l.Src.Len()
	if n == 0 {
		return ListNone
	}
	prevSel, prevOff := l.Selected, l.Offset
	page := max(l.Height-1, 1)
	switch ev := ev.(type) {
	case term.KeyEvent:
		switch ev.Key.String() {
		case "Up":
			l.Selected--
		case "Down":
			l.Selected++
		case "PgUp":
			l.Selected -= page
		case "PgDn":
			l.Selected += page
		case "Home":
			l.Selected = 0
		case "End":
			l.Selected = n - 1
		case "Enter":
			return ListActivated
		default:
			return ListNone
		}
		l.clamp()
	case term.MouseEvent:
		g := ev.Gesture
		switch g.Kind {
		case term.GestureWheel:
			switch g.Wheel {
			case term.WheelUp:
				l.Offset -= 3
			case term.WheelDown:
				l.Offset += 3
			}
			l.Offset = min(max(l.Offset, 0), max(n-max(l.Height, 1), 0))
		case term.GesturePress, term.GestureClick:
			if g.Button != term.ButtonLeft {
				return ListNone
			}
			row := l.Offset + g.Y
			if g.Y < 0 || row >= n {
				return ListNone
			}
			l.Selected = row
			if g.Kind == term.GestureClick && g.Count == 2 {
				return ListActivated
			}
		default:
			return ListNone
		}
	default:
		return ListNone
	}
	if l.Selected != prevSel || l.Offset != prevOff {
		return ListChanged
	}
	return ListNone
}

// Draw paints visible rows and a scrollbar in the last column when needed.
func (l *List) Draw(s wm.Surface, focused bool) {
	w, h := s.Size()
	l.Height = h
	l.clamp()
	n := l.Src.Len()
	textW := w
	if n > h {
		textW = w - 1
		DrawScrollbar(s.Sub(wm.Rect{X: w - 1, Y: 0, W: 1, H: h}), n, l.Offset, h)
	}
	for y := 0; y < h && l.Offset+y < n; y++ {
		i := l.Offset + y
		st := wm.StyleBody
		if i == l.Selected {
			st = wm.StyleMenu
			if focused {
				st = wm.StyleSelected
			}
		}
		row := s.Sub(wm.Rect{X: 0, Y: y, W: textW, H: 1})
		row.Clear(st)
		row.Text(0, 0, l.Src.Item(i), st)
	}
}

// ThumbRange returns the scrollbar thumb's first row and length for a track
// of height rows.
func ThumbRange(total, offset, visible, height int) (start, length int) {
	if total <= visible || height <= 0 {
		return 0, height
	}
	length = max(1, height*visible/total)
	maxStart := height - length
	start = maxStart * offset / max(total-visible, 1)
	return min(start, maxStart), length
}

// DrawScrollbar paints a one-column scrollbar.
func DrawScrollbar(s wm.Surface, total, offset, visible int) {
	_, h := s.Size()
	start, length := ThumbRange(total, offset, visible, h)
	for y := 0; y < h; y++ {
		if y >= start && y < start+length {
			s.Set(0, y, '█', wm.StyleScrollThumb)
		} else {
			s.Set(0, y, '░', wm.StyleScrollbar)
		}
	}
}
