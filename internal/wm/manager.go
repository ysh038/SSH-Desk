package wm

// Styles used when painting windows. The caller maps them to terminal colors.
const (
	StyleDesktop StyleID = iota
	StyleBar
	StyleFrame
	StyleFrameFocused
	StyleTitle
	StyleTitleFocused
	StyleBody
)

// Part is the region of a window under a point.
type Part uint8

const (
	PartNone   Part = iota
	PartTitle       // top border row: drag to move
	PartBorder      // other border cells
	PartBody        // inside the frame
)

// Window is a framed, movable rectangle. Rect includes the frame.
type Window struct {
	ID    int
	Title string
	Rect  Rect
	Body  []string
}

// Inner is the area inside the frame.
func (w *Window) Inner() Rect { return w.Rect.Inset(1) }

// MinVisible is how many columns of a window must stay on screen so it can
// always be grabbed again.
const MinVisible = 6

// Manager owns the windows, their stacking order and focus.
type Manager struct {
	// Desktop is the area windows live in (the screen minus bars).
	Desktop Rect

	wins  []*Window // bottom to top
	focus *Window

	drag       *Window
	dragDX     int // pointer offset from the window origin while dragging
	dragDY     int
	dragActive bool
}

// Add puts w on top and focuses it.
func (m *Manager) Add(w *Window) {
	m.wins = append(m.wins, w)
	m.focus = w
	m.Clamp(w)
}

// Windows returns the windows from bottom to top.
func (m *Manager) Windows() []*Window { return m.wins }

// Focused returns the focused window (nil if none).
func (m *Manager) Focused() *Window { return m.focus }

// Top returns the topmost window (nil if none).
func (m *Manager) Top() *Window {
	if len(m.wins) == 0 {
		return nil
	}
	return m.wins[len(m.wins)-1]
}

// Raise moves w to the top and focuses it.
func (m *Manager) Raise(w *Window) {
	for i, x := range m.wins {
		if x == w {
			copy(m.wins[i:], m.wins[i+1:])
			m.wins[len(m.wins)-1] = w
			break
		}
	}
	m.focus = w
}

// FocusNext raises the bottom-most window, cycling through all windows
// (keyboard alternative to clicking a window).
func (m *Manager) FocusNext() {
	if len(m.wins) > 1 {
		m.Raise(m.wins[0])
	}
}

// FocusPrev is the reverse of FocusNext: the top window goes to the bottom
// and the one below it becomes the focused top window.
func (m *Manager) FocusPrev() {
	n := len(m.wins)
	if n < 2 {
		return
	}
	top := m.wins[n-1]
	copy(m.wins[1:], m.wins[:n-1])
	m.wins[0] = top
	m.focus = m.wins[n-1]
}

// HitTest returns the topmost window under (x, y) and which part was hit.
func (m *Manager) HitTest(x, y int) (*Window, Part) {
	for i := len(m.wins) - 1; i >= 0; i-- {
		w := m.wins[i]
		if !w.Rect.Contains(x, y) {
			continue
		}
		switch {
		case y == w.Rect.Y:
			return w, PartTitle
		case w.Inner().Contains(x, y):
			return w, PartBody
		default:
			return w, PartBorder
		}
	}
	return nil, PartNone
}

// MoveTo places w's top-left corner at (x, y), clamped to the desktop.
func (m *Manager) MoveTo(w *Window, x, y int) {
	w.Rect.X, w.Rect.Y = x, y
	m.Clamp(w)
}

// MoveBy moves w by (dx, dy), clamped to the desktop.
func (m *Manager) MoveBy(w *Window, dx, dy int) { m.MoveTo(w, w.Rect.X+dx, w.Rect.Y+dy) }

// Clamp keeps the title bar inside the desktop vertically and at least
// MinVisible columns of the window on screen horizontally.
func (m *Manager) Clamp(w *Window) {
	d := m.Desktop
	if d.Empty() {
		return
	}
	vis := min(MinVisible, w.Rect.W)
	w.Rect.X = clamp(w.Rect.X, d.X-w.Rect.W+vis, d.X+d.W-vis)
	w.Rect.Y = clamp(w.Rect.Y, d.Y, d.Y+d.H-1)
}

// ClampAll re-clamps every window (e.g. after a terminal resize).
func (m *Manager) ClampAll() {
	for _, w := range m.wins {
		m.Clamp(w)
	}
}

// PointerDown handles a primary-button press: it raises and focuses the window
// under the pointer and, on the title bar, arms a move.
func (m *Manager) PointerDown(x, y int) {
	w, part := m.HitTest(x, y)
	m.drag, m.dragActive = nil, false
	if w == nil {
		return
	}
	m.Raise(w)
	if part == PartTitle {
		m.drag = w
		m.dragDX, m.dragDY = x-w.Rect.X, y-w.Rect.Y
	}
}

// PointerDrag moves the armed window so the grab point follows the pointer.
func (m *Manager) PointerDrag(x, y int) {
	if m.drag == nil {
		return
	}
	m.dragActive = true
	m.MoveTo(m.drag, x-m.dragDX, y-m.dragDY)
}

// PointerUp ends a move.
func (m *Manager) PointerUp() { m.drag, m.dragActive = nil, false }

// Dragging reports whether a window is currently being moved.
func (m *Manager) Dragging() bool { return m.dragActive }

// Draw paints the desktop and all windows back to front.
func (m *Manager) Draw(c *Canvas) {
	c.Fill(m.Desktop, ' ', StyleDesktop)
	for _, w := range m.wins {
		m.drawWindow(c, w, w == m.focus)
	}
}

func (m *Manager) drawWindow(c *Canvas, w *Window, focused bool) {
	clip := w.Rect.Intersect(m.Desktop)
	if clip.Empty() {
		return
	}
	frame, title := StyleFrame, StyleTitle
	if focused {
		frame, title = StyleFrameFocused, StyleTitleFocused
	}
	r := w.Rect
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W-1, r.Y+r.H-1

	// Body first, then the frame on top of it.
	c.Fill(w.Inner().Intersect(clip), ' ', StyleBody)
	inner := w.Inner().Intersect(clip)
	for i, line := range w.Body {
		y := r.Y + 1 + i
		if y > y1-1 {
			break
		}
		c.Text(inner, r.X+1, y, line, StyleBody)
	}

	for x := x0 + 1; x < x1; x++ {
		c.Set(clip, x, y0, '─', frame)
		c.Set(clip, x, y1, '─', frame)
	}
	for y := y0 + 1; y < y1; y++ {
		c.Set(clip, x0, y, '│', frame)
		c.Set(clip, x1, y, '│', frame)
	}
	c.Set(clip, x0, y0, '┌', frame)
	c.Set(clip, x1, y0, '┐', frame)
	c.Set(clip, x0, y1, '└', frame)
	c.Set(clip, x1, y1, '┘', frame)

	// Title on the top border, clipped so it never overwrites the corners.
	titleClip := Rect{x0 + 1, y0, max(r.W-2, 0), 1}.Intersect(clip)
	c.Text(titleClip, x0+2, y0, " "+w.Title+" ", title)
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
