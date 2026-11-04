package wm

import "tuidesk/internal/term"

// App is the content of a window (SPEC 6 App interface).
type App interface {
	Title() string
	MinSize() (w, h int)
	// Draw paints the window's inner area.
	Draw(s Surface, focused bool)
	// HandleEvent receives events while the window is focused. Mouse
	// gestures are in the window's inner (local) coordinates. It returns
	// false to let the desktop handle the event.
	HandleEvent(ev term.Event) bool
	Close() error
}

// Part is the region of a window under a point.
type Part uint8

const (
	PartNone     Part = iota
	PartTitle         // top border row: drag to move, double-click to maximize
	PartMinimize      // [_]
	PartMaximize      // [^]
	PartClose         // [x]
	PartResize        // bottom-right corner: drag to resize
	PartBorder        // other border cells
	PartBody          // inside the frame
)

// Title-bar buttons, drawn right-aligned on the top border.
const (
	btnMin   = "[_]"
	btnMax   = "[^]"
	btnClose = "[x]"
)

// Window is a framed rectangle. Rect includes the frame.
type Window struct {
	ID        int
	Title     string // used when App is nil
	Rect      Rect
	Body      []string // static text, used when App is nil
	App       App
	Minimized bool
	Maximized bool

	restore Rect // Rect before maximizing
}

// Inner is the area inside the frame.
func (w *Window) Inner() Rect { return w.Rect.Inset(1) }

// Name is the window title (from the App if there is one).
func (w *Window) Name() string {
	if w.App != nil {
		return w.App.Title()
	}
	return w.Title
}

func (w *Window) minSize() (int, int) {
	mw, mh := 16, 4
	if w.App != nil {
		aw, ah := w.App.MinSize()
		mw, mh = max(mw, aw+2), max(mh, ah+2)
	}
	return mw, mh
}

// buttonsX returns the column of the first title button, or -1 if the window
// is too narrow to show them.
func (w *Window) buttonsX() int {
	if w.Rect.W < minWidthForButtons {
		return -1
	}
	return w.Rect.X + w.Rect.W - 1 - 3*len(btnClose)
}

// minWidthForButtons: narrower windows show only the title (buttons stay
// reachable through the window menu).
const minWidthForButtons = 24

// MinVisible is how many columns of a window must stay on screen so it can
// always be grabbed again.
const MinVisible = 6

// Manager owns the windows, their stacking order and focus.
type Manager struct {
	// Desktop is the area windows live in (the screen minus bars).
	Desktop Rect

	wins  []*Window // bottom to top, minimized ones included
	focus *Window

	grab       *Window
	grabPart   Part
	grabDX     int
	grabDY     int
	grabActive bool
	nextID     int
}

// Add puts w on top and focuses it. A zero ID is assigned automatically.
func (m *Manager) Add(w *Window) {
	if w.ID == 0 {
		m.nextID++
		w.ID = m.nextID
	} else if w.ID > m.nextID {
		m.nextID = w.ID
	}
	m.wins = append(m.wins, w)
	m.focus = w
	m.Clamp(w)
}

// Windows returns all windows from bottom to top (including minimized).
func (m *Manager) Windows() []*Window { return m.wins }

// Visible returns non-minimized windows from bottom to top.
func (m *Manager) Visible() []*Window {
	var out []*Window
	for _, w := range m.wins {
		if !w.Minimized {
			out = append(out, w)
		}
	}
	return out
}

// Focused returns the focused window (nil if none).
func (m *Manager) Focused() *Window { return m.focus }

// Top returns the topmost visible window (nil if none).
func (m *Manager) Top() *Window {
	for i := len(m.wins) - 1; i >= 0; i-- {
		if !m.wins[i].Minimized {
			return m.wins[i]
		}
	}
	return nil
}

func (m *Manager) index(w *Window) int {
	for i, x := range m.wins {
		if x == w {
			return i
		}
	}
	return -1
}

// Raise restores w if minimized, moves it to the top and focuses it.
func (m *Manager) Raise(w *Window) {
	i := m.index(w)
	if i < 0 {
		return
	}
	w.Minimized = false
	copy(m.wins[i:], m.wins[i+1:])
	m.wins[len(m.wins)-1] = w
	m.focus = w
}

// FocusNext raises the bottom-most visible window, cycling through all
// visible windows.
func (m *Manager) FocusNext() {
	vis := m.Visible()
	if len(vis) > 1 {
		m.Raise(vis[0])
	}
}

// FocusPrev is the reverse of FocusNext: the top window goes to the bottom
// and the one below it becomes the focused top window.
func (m *Manager) FocusPrev() {
	vis := m.Visible()
	if len(vis) < 2 {
		return
	}
	top := vis[len(vis)-1]
	i := m.index(top)
	copy(m.wins[1:i+1], m.wins[:i])
	m.wins[0] = top
	m.focus = m.Top()
}

// Minimize hides w; focus moves to the next visible window.
func (m *Manager) Minimize(w *Window) {
	w.Minimized = true
	if m.focus == w {
		m.focus = m.Top()
	}
}

// ToggleMaximize fills the desktop with w, or restores its previous size.
func (m *Manager) ToggleMaximize(w *Window) {
	if w.Maximized {
		w.Maximized = false
		w.Rect = w.restore
		m.Clamp(w)
		return
	}
	w.restore = w.Rect
	w.Maximized = true
	w.Rect = m.Desktop
}

// Remove deletes w (the caller closes its App).
func (m *Manager) Remove(w *Window) {
	i := m.index(w)
	if i < 0 {
		return
	}
	m.wins = append(m.wins[:i], m.wins[i+1:]...)
	if m.grab == w {
		m.PointerUp()
	}
	if m.focus == w {
		m.focus = m.Top()
	}
}

// HitTest returns the topmost visible window under (x, y) and the part hit.
func (m *Manager) HitTest(x, y int) (*Window, Part) {
	for i := len(m.wins) - 1; i >= 0; i-- {
		w := m.wins[i]
		if w.Minimized || !w.Rect.Contains(x, y) {
			continue
		}
		r := w.Rect
		switch {
		case y == r.Y:
			if bx := w.buttonsX(); bx >= 0 && x >= bx && x < bx+9 {
				return w, [...]Part{PartMinimize, PartMaximize, PartClose}[(x-bx)/3]
			}
			return w, PartTitle
		case x == r.X+r.W-1 && y == r.Y+r.H-1:
			return w, PartResize
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
	if w.Maximized {
		return
	}
	w.Rect.X, w.Rect.Y = x, y
	m.Clamp(w)
}

// MoveBy moves w by (dx, dy), clamped to the desktop.
func (m *Manager) MoveBy(w *Window, dx, dy int) { m.MoveTo(w, w.Rect.X+dx, w.Rect.Y+dy) }

// ResizeTo sets w's size, respecting its minimum size and the desktop.
func (m *Manager) ResizeTo(w *Window, width, height int) {
	if w.Maximized {
		return
	}
	mw, mh := w.minSize()
	d := m.Desktop
	w.Rect.W = clamp(width, mw, max(mw, d.X+d.W-w.Rect.X))
	w.Rect.H = clamp(height, mh, max(mh, d.Y+d.H-w.Rect.Y))
}

// ResizeBy changes w's size by (dw, dh).
func (m *Manager) ResizeBy(w *Window, dw, dh int) { m.ResizeTo(w, w.Rect.W+dw, w.Rect.H+dh) }

// Clamp keeps the title bar inside the desktop vertically and at least
// MinVisible columns of the window on screen horizontally.
func (m *Manager) Clamp(w *Window) {
	d := m.Desktop
	if d.Empty() {
		return
	}
	if w.Maximized {
		w.Rect = d
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

// PointerDown handles a primary-button press on the desktop area. It raises
// the window under the pointer and arms a move (title) or resize (corner).
// It returns the window and part that were hit.
func (m *Manager) PointerDown(x, y int) (*Window, Part) {
	w, part := m.HitTest(x, y)
	m.grab, m.grabActive = nil, false
	if w == nil {
		return nil, PartNone
	}
	m.Raise(w)
	switch part {
	case PartTitle:
		if !w.Maximized {
			m.grab, m.grabPart = w, PartTitle
			m.grabDX, m.grabDY = x-w.Rect.X, y-w.Rect.Y
		}
	case PartResize:
		if !w.Maximized {
			m.grab, m.grabPart = w, PartResize
			m.grabDX, m.grabDY = w.Rect.X+w.Rect.W-1-x, w.Rect.Y+w.Rect.H-1-y
		}
	}
	return w, part
}

// PointerDrag moves or resizes the grabbed window.
func (m *Manager) PointerDrag(x, y int) {
	if m.grab == nil {
		return
	}
	m.grabActive = true
	w := m.grab
	switch m.grabPart {
	case PartTitle:
		m.MoveTo(w, x-m.grabDX, y-m.grabDY)
	case PartResize:
		m.ResizeTo(w, x+m.grabDX-w.Rect.X+1, y+m.grabDY-w.Rect.Y+1)
	}
}

// PointerUp ends a move or resize.
func (m *Manager) PointerUp() { m.grab, m.grabActive = nil, false }

// Grabbing reports whether a press armed a move or resize (even before the
// pointer has moved).
func (m *Manager) Grabbing() bool { return m.grab != nil }

// Dragging reports whether a window is currently being moved or resized.
func (m *Manager) Dragging() bool { return m.grabActive }

// Draw paints the desktop background and all visible windows back to front.
func (m *Manager) Draw(c *Canvas) {
	c.Fill(m.Desktop, ' ', StyleDesktop)
	for _, w := range m.wins {
		if !w.Minimized {
			m.drawWindow(c, w, w == m.focus)
		}
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

	body := NewSurface(c, w.Inner())
	body.clip = body.clip.Intersect(clip)
	body.Clear(StyleBody)
	if w.App != nil {
		w.App.Draw(body, focused) // the App draws in its own coordinates, clipped
	} else {
		for i, line := range w.Body {
			body.Text(0, i, line, StyleBody)
		}
	}

	outer := NewSurface(c, r)
	outer.clip = outer.clip.Intersect(clip)
	outer.Box(Rect{0, 0, r.W, r.H}, frame)
	if !w.Maximized && r.W > 1 && r.H > 1 {
		outer.Set(r.W-1, r.H-1, '┘', frame) // resize handle stays a corner glyph
	}

	// Title, clipped before the buttons so it never overwrites them.
	titleEnd := r.W - 1
	if bx := w.buttonsX(); bx >= 0 {
		titleEnd = bx - r.X
		outer.Text(titleEnd, 0, btnMin, StyleButton)
		maxBtn := btnMax
		if w.Maximized {
			maxBtn = "[v]"
		}
		outer.Text(titleEnd+3, 0, maxBtn, StyleButton)
		outer.Text(titleEnd+6, 0, btnClose, StyleButton)
	}
	if titleEnd > 2 {
		outer.Sub(Rect{1, 0, titleEnd - 1, 1}).Text(1, 0, " "+w.Name()+" ", title)
	}
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
