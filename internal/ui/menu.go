// Package ui holds reusable widgets (menu, dialog, toast, list, scrollbar).
// Widgets keep their own state, draw onto a wm.Surface and take term events
// in local coordinates; none of them depend on tcell.
package ui

import (
	"unicode"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// MenuItem is one entry of a Menu. A zero-value item with Separator=true
// draws a divider line.
type MenuItem struct {
	Label     string
	Hotkey    rune   // single key that activates the item while the menu is open
	Shortcut  string // binding shown on the right in the user's style, e.g. "F6", "Ctrl+S"
	Action    string // identifier returned on activation
	Disabled  bool
	Separator bool
}

// MenuResult is what a Menu reports after handling an event.
type MenuResult int

const (
	MenuNone      MenuResult = iota // event consumed, menu still open
	MenuActivated                   // an item was chosen; see Menu.Chosen
	MenuClosed                      // dismissed (Esc or click outside)
	MenuIgnored                     // event not for the menu
)

// Menu is a vertical popup menu. Its Rect (screen position, frame included)
// is set by the owner, usually with PlacePopup.
type Menu struct {
	Items    []MenuItem
	Selected int
	Rect     wm.Rect
	Chosen   MenuItem
}

// NewMenu returns a menu with the first enabled item selected.
func NewMenu(items []MenuItem) *Menu {
	m := &Menu{Items: items, Selected: -1}
	m.move(1)
	return m
}

// Size is the frame size the menu needs.
func (m *Menu) Size() (w, h int) {
	for _, it := range m.Items {
		w = max(w, term.Width(m.label(it))+2+term.Width(term.KeyLabel(it.Shortcut)))
	}
	return w + 4, len(m.Items) + 2
}

// hotkeyIndex is the rune index of the hotkey letter inside the label, or -1.
func hotkeyIndex(it MenuItem) int {
	if it.Hotkey == 0 {
		return -1
	}
	for i, r := range []rune(it.Label) {
		if unicode.ToLower(r) == unicode.ToLower(it.Hotkey) {
			return i
		}
	}
	return -1
}

// label is the text shown for an item: the hotkey letter is highlighted in
// place when the label contains it, otherwise appended as "(K)".
func (m *Menu) label(it MenuItem) string {
	if it.Hotkey != 0 && hotkeyIndex(it) < 0 {
		return it.Label + "(" + string(unicode.ToUpper(it.Hotkey)) + ")"
	}
	return it.Label
}

func (m *Menu) selectable(i int) bool {
	return i >= 0 && i < len(m.Items) && !m.Items[i].Separator && !m.Items[i].Disabled
}

// move steps the selection by dir (+1/-1), wrapping and skipping
// separators and disabled items.
func (m *Menu) move(dir int) {
	n := len(m.Items)
	for step := 1; step <= n; step++ {
		i := ((m.Selected+dir*step)%n + n) % n
		if m.Selected < 0 && dir > 0 {
			i = step - 1
		}
		if m.selectable(i) {
			m.Selected = i
			return
		}
	}
}

func (m *Menu) activate(i int) MenuResult {
	if !m.selectable(i) {
		return MenuNone
	}
	m.Selected = i
	m.Chosen = m.Items[i]
	return MenuActivated
}

// HandleKey processes a key while the menu is open.
func (m *Menu) HandleKey(k term.Key) MenuResult {
	switch k.String() {
	case "Up":
		m.move(-1)
	case "Down", "Tab":
		m.move(1)
	case "Home":
		m.Selected = -1
		m.move(1)
	case "End":
		m.Selected = len(m.Items)
		m.move(-1)
	case "Enter", "Space":
		return m.activate(m.Selected)
	case "Esc", "F9", "Shift+F10":
		return MenuClosed
	default:
		if k.IsRune() && k.Mod == 0 {
			r := unicode.ToLower(term.ShortcutRune(k.Rune, term.ModCtrl)) // hotkeys work with the Korean IME on
			for i, it := range m.Items {
				if it.Hotkey != 0 && unicode.ToLower(it.Hotkey) == r {
					return m.activate(i)
				}
			}
		}
	}
	return MenuNone
}

// HandleMouse processes a gesture in screen coordinates.
func (m *Menu) HandleMouse(g term.Gesture) MenuResult {
	inside := m.Rect.Contains(g.X, g.Y)
	row := g.Y - m.Rect.Y - 1
	switch g.Kind {
	case term.GesturePress:
		if !inside {
			return MenuClosed
		}
		if m.selectable(row) {
			m.Selected = row
		}
		return MenuNone
	case term.GestureClick:
		if !inside {
			return MenuClosed
		}
		return m.activate(row)
	case term.GestureDrag, term.GestureMove:
		if inside && m.selectable(row) {
			m.Selected = row
		}
		return MenuNone
	case term.GestureRelease, term.GestureDragEnd:
		if inside && g.Button == term.ButtonLeft && (g.StartX != g.X || g.StartY != g.Y) {
			return m.activate(row) // press-drag-release selection
		}
		return MenuNone
	}
	if inside {
		return MenuNone
	}
	return MenuIgnored
}

// Draw paints the menu at its Rect.
func (m *Menu) Draw(c *wm.Canvas) {
	s := wm.NewSurface(c, m.Rect)
	s.Clear(wm.StyleMenu)
	s.Box(wm.Rect{X: 0, Y: 0, W: m.Rect.W, H: m.Rect.H}, wm.StyleMenu)
	inner := m.Rect.W - 2
	for i, it := range m.Items {
		y := i + 1
		if it.Separator {
			s.Set(0, y, '├', wm.StyleMenu)
			for x := 1; x <= inner; x++ {
				s.Set(x, y, '─', wm.StyleMenu)
			}
			s.Set(inner+1, y, '┤', wm.StyleMenu)
			continue
		}
		st := wm.StyleMenu
		switch {
		case it.Disabled:
			st = wm.StyleMenuDisabled
		case i == m.Selected:
			st = wm.StyleMenuSelected
		}
		row := s.Sub(wm.Rect{X: 1, Y: y, W: inner, H: 1})
		row.Clear(st)
		row.Text(1, 0, m.label(it), st)
		if hi := hotkeyIndex(it); hi >= 0 && !it.Disabled {
			hs := wm.StyleMenuHotkey
			if i == m.Selected {
				hs = wm.StyleMenuSelectedHotkey
			}
			x := 1
			for j, r := range []rune(it.Label) {
				if j == hi {
					row.Set(x, 0, r, hs)
					break
				}
				x += term.RuneWidth(r)
			}
		}
		if sc := term.KeyLabel(it.Shortcut); sc != "" {
			row.Text(inner-1-term.Width(sc), 0, sc, st)
		}
	}
}

// PlacePopup positions a w×h popup with its top-left at (x, y), moved so it
// stays inside screen.
func PlacePopup(screen wm.Rect, x, y, w, h int) wm.Rect {
	w, h = min(w, screen.W), min(h, screen.H)
	if x+w > screen.X+screen.W {
		x = screen.X + screen.W - w
	}
	if y+h > screen.Y+screen.H {
		y = max(screen.Y, y-h+1)
		if y+h > screen.Y+screen.H {
			y = screen.Y + screen.H - h
		}
	}
	return wm.Rect{X: max(x, screen.X), Y: max(y, screen.Y), W: w, H: h}
}
