package ui

import (
	"sort"
	"strings"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// Command is one user action. The same record feeds key handling, the menu
// bar, the command search (⌃K) and the hint bar, so they always agree
// (ADR 0003 C/D).
type Command struct {
	ID    string
	Title string
	Menu  string   // menu bar group, e.g. "File" ("" = not in the menu bar)
	Keys  []string // bindings, e.g. {"Ctrl+End", "Alt+Down"}
	// MacKey is the binding shown on Mac keyboards when it differs from
	// Keys[0] (ADR 0003 B: commands use ⌃, big moves use ⌥).
	MacKey string
	Hint   string // short label for the hint bar ("" = not shown)
	// HintPinned keeps the hint at the right end even when space runs out
	// (used for the command search, which leads to everything else).
	HintPinned bool
	Enabled    func() bool
	Run        func()
}

// On reports whether the command can run now.
func (c *Command) On() bool { return c.Enabled == nil || c.Enabled() }

// Shortcut is the binding to display in the current label style.
func (c *Command) Shortcut() string {
	if term.CurrentLabelStyle() == term.LabelMac && c.MacKey != "" {
		return c.MacKey
	}
	if len(c.Keys) > 0 {
		return c.Keys[0]
	}
	return ""
}

// Commands is an ordered command registry.
type Commands struct {
	list  []*Command
	menus []string
}

// Add registers commands; menus appear in first-use order.
func (cs *Commands) Add(cmds ...*Command) {
	for _, c := range cmds {
		cs.list = append(cs.list, c)
		if c.Menu != "" && !contains(cs.menus, c.Menu) {
			cs.menus = append(cs.menus, c.Menu)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// All returns every command.
func (cs *Commands) All() []*Command { return cs.list }

// Menus returns the menu bar groups.
func (cs *Commands) Menus() []string { return cs.menus }

// ByID finds a command.
func (cs *Commands) ByID(id string) *Command {
	for _, c := range cs.list {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// ForKey returns the enabled command bound to k, if any.
func (cs *Commands) ForKey(k term.Key) *Command {
	name := k.String()
	for _, c := range cs.list {
		for _, b := range c.Keys {
			if b == name && c.On() {
				return c
			}
		}
	}
	return nil
}

// HandleKey runs the command bound to k and reports whether one ran.
func (cs *Commands) HandleKey(k term.Key) bool {
	if c := cs.ForKey(k); c != nil {
		c.Run()
		return true
	}
	return false
}

// Run executes the command with id if enabled.
func (cs *Commands) Run(id string) {
	if c := cs.ByID(id); c != nil && c.On() {
		c.Run()
	}
}

// MenuItems builds the dropdown for a menu bar group. A command whose
// Title is "-" is a separator.
func (cs *Commands) MenuItems(menu string) []MenuItem {
	var items []MenuItem
	for _, c := range cs.list {
		if c.Menu != menu {
			continue
		}
		if c.Title == "-" {
			items = append(items, MenuItem{Separator: true})
			continue
		}
		items = append(items, MenuItem{Label: c.Title, Shortcut: c.Shortcut(), Action: c.ID, Disabled: !c.On()})
	}
	return items
}

// Search returns enabled commands whose title matches query (all words,
// any order, case-insensitive), best matches first.
func (cs *Commands) Search(query string) []*Command {
	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		c     *Command
		score int
	}
	var hits []scored
	for _, c := range cs.list {
		if c.Title == "-" || !c.On() {
			continue
		}
		t := strings.ToLower(c.Title)
		ok, score := true, 0
		for _, w := range words {
			i := strings.Index(t, w)
			if i < 0 {
				ok = false
				break
			}
			if i == 0 || t[i-1] == ' ' {
				score -= 10 // word start
			}
			score += i
		}
		if ok {
			hits = append(hits, scored{c, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score < hits[j].score })
	out := make([]*Command, len(hits))
	for i, h := range hits {
		out[i] = h.c
	}
	return out
}

// ---- menu bar ----

// MenuBar draws the menu titles on one row and remembers where they are.
type MenuBar struct {
	xs   []int // start column of each menu title
	Open int   // index of the open menu (-1: none), for highlighting
}

// NewMenuBar returns a closed bar.
func NewMenuBar() *MenuBar { return &MenuBar{Open: -1} }

// Draw paints titles on row 0 of s.
func (b *MenuBar) Draw(s wm.Surface, menus []string) {
	w, _ := s.Size()
	row := s.Sub(wm.Rect{X: 0, Y: 0, W: w, H: 1})
	row.Clear(wm.StyleMenu)
	b.xs = b.xs[:0]
	x := 1
	for i, m := range menus {
		st := wm.StyleMenu
		if i == b.Open {
			st = wm.StyleMenuSelected
		}
		b.xs = append(b.xs, x)
		x = row.Text(x, 0, " "+m+" ", st)
	}
	label := term.KeyLabel("F10") + " menu · " + term.KeyLabel("Ctrl+K") + " commands"
	if lx := w - term.Width(label) - 1; lx > x+2 {
		row.Text(lx, 0, label, wm.StyleMenuDisabled)
	}
}

// At returns the menu index under local column x on the bar row (-1: none).
func (b *MenuBar) At(x int, menus []string) int {
	for i, x0 := range b.xs {
		if i < len(menus) && x >= x0 && x < x0+term.Width(menus[i])+2 {
			return i
		}
	}
	return -1
}

// X returns the start column of menu i.
func (b *MenuBar) X(i int) int {
	if i >= 0 && i < len(b.xs) {
		return b.xs[i]
	}
	return 1
}

// ---- hint bar ----

// DrawHints paints "key title" pairs for commands with a Hint, in order;
// pinned hints are drawn at the right end first so they always fit.
func DrawHints(s wm.Surface, cmds []*Command) {
	w, _ := s.Size()
	s.Clear(wm.StyleBar)
	right := w - 1
	for _, c := range cmds {
		if !c.HintPinned || c.Hint == "" || !c.On() {
			continue
		}
		k := term.KeyLabel(c.Shortcut())
		right -= term.Width(k) + term.Width(c.Hint) + 1
		if right < 1 {
			return
		}
		x := s.Text(right, 0, k, wm.StyleBarActive)
		s.Text(x, 0, " "+c.Hint, wm.StyleBar)
		right -= 2
	}
	x := 1
	for _, c := range cmds {
		if c.HintPinned || c.Hint == "" || !c.On() {
			continue
		}
		k := term.KeyLabel(c.Shortcut())
		if x+term.Width(k)+term.Width(c.Hint)+1 > right {
			break
		}
		x = s.Text(x, 0, k, wm.StyleBarActive)
		x = s.Text(x, 0, " "+c.Hint+"  ", wm.StyleBar)
	}
}

// ---- command search (palette) ----

// Palette is the ⌃K command search modal.
type Palette struct {
	cmds    *Commands
	Input   *Input
	matches []*Command
	sel     int
	Rect    wm.Rect
	run     *Command
}

// NewPalette searches cs.
func NewPalette(cs *Commands) *Palette {
	p := &Palette{cmds: cs, Input: NewInput("")}
	p.refresh()
	return p
}

func (p *Palette) refresh() {
	p.matches = p.cmds.Search(p.Input.Value())
	p.sel = min(p.sel, max(len(p.matches)-1, 0))
}

func (p *Palette) Layout(screen wm.Rect) {
	w := min(64, screen.W)
	h := min(16, screen.H)
	p.Rect = wm.Rect{X: screen.X + (screen.W-w)/2, Y: screen.Y + 1, W: w, H: h}
}

func (p *Palette) visibleRows() int { return p.Rect.H - 4 }

// HandleEvent runs the chosen command after closing.
func (p *Palette) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		switch ev.Key.String() {
		case "Esc":
			return true
		case "Enter":
			return p.choose(p.sel)
		case "Up":
			p.sel = max(p.sel-1, 0)
			return false
		case "Down":
			p.sel = min(p.sel+1, max(len(p.matches)-1, 0))
			return false
		}
		if p.Input.HandleKey(ev.Key) == InputChanged {
			p.sel = 0
			p.refresh()
		}
	case term.PasteEvent:
		p.Input.Paste(ev.Text)
		p.refresh()
	case term.MouseEvent:
		g := ev.Gesture
		if g.Kind == term.GestureClick && g.Button == term.ButtonLeft {
			if !p.Rect.Contains(g.X, g.Y) {
				return true
			}
			if i := g.Y - (p.Rect.Y + 3); i >= 0 && i < min(len(p.matches), p.visibleRows()) {
				return p.choose(i)
			}
		}
	}
	return false
}

func (p *Palette) choose(i int) bool {
	if i < 0 || i >= len(p.matches) {
		return false
	}
	p.run = p.matches[i]
	// Popups the command opens stay: the desktop removes only the palette.
	p.run.Run()
	return true
}

func (p *Palette) Draw(c *wm.Canvas) {
	s := wm.NewSurface(c, p.Rect)
	s.Clear(wm.StyleMenu)
	s.Box(wm.Rect{X: 0, Y: 0, W: p.Rect.W, H: p.Rect.H}, wm.StyleMenu)
	s.Sub(wm.Rect{X: 1, Y: 0, W: p.Rect.W - 2, H: 1}).Text(1, 0, " Commands ", wm.StyleMenu)
	s.Text(2, 1, "Search:", wm.StyleMenu)
	p.Input.Draw(wm.NewSurface(c, wm.Rect{X: p.Rect.X + 10, Y: p.Rect.Y + 1, W: p.Rect.W - 12, H: 1}), true)
	for i, cmd := range p.matches {
		if i >= p.visibleRows() {
			break
		}
		st := wm.StyleMenu
		if i == p.sel {
			st = wm.StyleMenuSelected
		}
		row := s.Sub(wm.Rect{X: 1, Y: 3 + i, W: p.Rect.W - 2, H: 1})
		row.Clear(st)
		title := cmd.Title
		if cmd.Menu != "" {
			title = cmd.Menu + ": " + title
		}
		row.Text(1, 0, title, st)
		if k := term.KeyLabel(cmd.Shortcut()); k != "" {
			row.Text(p.Rect.W-3-term.Width(k), 0, k, st)
		}
	}
	if len(p.matches) == 0 {
		s.Text(2, 3, "No matching command", wm.StyleMenuDisabled)
	}
}
