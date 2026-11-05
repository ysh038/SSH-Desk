// Package desktop is the shell: top bar, taskbar, launcher, window menu,
// keyboard move/resize modes, dialogs and toasts around a wm.Manager.
// It consumes term events and draws onto a wm.Canvas; it does not use tcell.
package desktop

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tuidesk/internal/apps/blank"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// Mode is the keyboard interaction mode.
type Mode int

const (
	ModeNormal Mode = iota
	ModeMove        // arrows move the focused window (F7)
	ModeResize      // arrows resize the focused window (F8)
)

type barItem struct {
	r      wm.Rect
	action string
}

// Desktop holds the whole shell state.
type Desktop struct {
	W, H     int
	HostName string
	Now      time.Time
	M        *wm.Manager
	// NewApp creates the app for "New window" (window number n).
	NewApp func(n int) wm.App
	// NewFiles creates a file manager for dir (nil: no "Files" launcher item).
	NewFiles func(dir string) wm.App
	// HomeDir is where new file managers start.
	HomeDir string
	Quit    bool

	menu       *ui.Menu
	menuTarget *wm.Window
	menuFn     func(action string) // app menu callback (nil: desktop action)
	modals     []ui.Modal
	toasts     ui.Toasts
	svc        services
	mode       Mode
	topItems   []barItem
	taskItems  []barItem
	created    int
	help       *wm.Window
}

// New creates a desktop of w×h cells.
func New(w, h int, host string, now time.Time) *Desktop {
	d := &Desktop{HostName: host, Now: now, M: &wm.Manager{}}
	d.svc.init()
	d.NewApp = func(n int) wm.App { return blank.New(n) }
	d.Resize(w, h)
	return d
}

// Area is the region windows live in (between the two bars).
func (d *Desktop) Area() wm.Rect { return wm.Rect{X: 0, Y: 1, W: d.W, H: max(d.H-2, 1)} }

// Resize adapts to a new terminal size.
func (d *Desktop) Resize(w, h int) {
	d.W, d.H = w, h
	d.M.Desktop = d.Area()
	d.M.ClampAll()
	for _, m := range d.modals {
		m.Layout(d.Area())
	}
	d.closeMenu()
}

// Mode returns the current keyboard mode.
func (d *Desktop) Mode() Mode { return d.mode }

// MenuOpen and DialogOpen expose modal state (for tests and the status line).
func (d *Desktop) MenuOpen() bool   { return d.menu != nil }
func (d *Desktop) DialogOpen() bool { return len(d.modals) > 0 }

// Notify shows a toast.
func (d *Desktop) Notify(text string, isErr bool) { d.toasts.Push(d.Now, text, isErr) }

// OpenWindow adds a window for app at a cascading position.
func (d *Desktop) OpenWindow(app wm.App) *wm.Window {
	n := d.created
	d.created++
	a := d.Area()
	w, h := min(48, a.W-2), min(12, a.H-1)
	mw, mh := app.MinSize()
	w, h = max(w, mw+2), max(h, mh+2)
	win := &wm.Window{App: app, Rect: wm.Rect{X: a.X + 2 + (n%8)*4, Y: a.Y + 1 + (n%8)*2, W: w, H: h}}
	d.M.Add(win)
	return win
}

// NextWake is when the desktop next needs a TickEvent: the next clock minute
// or the earliest toast expiry. Nothing else redraws without input.
func (d *Desktop) NextWake() time.Time {
	next := d.Now.Truncate(time.Minute).Add(time.Minute)
	if t := d.toasts.NextDeadline(); !t.IsZero() && t.Before(next) {
		next = t
	}
	return next
}

// Handle processes one event.
func (d *Desktop) Handle(ev term.Event) {
	if t, ok := ev.(term.TickEvent); ok {
		d.Now = t.Now
		d.toasts.Tick(t.Now)
		return
	}
	if me, ok := ev.(term.MouseEvent); ok && d.svc.swallowDragEnd {
		d.svc.swallowDragEnd = false
		if me.Gesture.Kind == term.GestureDragEnd {
			return
		}
	}
	d.dropClosedModals()
	if n := len(d.modals); n > 0 {
		top := d.modals[n-1]
		if top.HandleEvent(ev) {
			d.removeModal(top)
		}
		return
	}
	if d.menu != nil && d.handleMenu(ev) {
		return
	}
	if d.svc.drag != nil {
		if me, ok := ev.(term.MouseEvent); ok {
			d.handleDrag(me.Gesture)
			return
		}
		if ke, ok := ev.(term.KeyEvent); ok && ke.Key.String() == "Esc" {
			d.endDrag()
			return
		}
	}
	switch ev := ev.(type) {
	case term.KeyEvent:
		d.handleKey(ev.Key)
	case term.MouseEvent:
		d.handleMouse(ev.Gesture)
	case term.PasteEvent:
		if f := d.M.Focused(); f != nil && f.App != nil {
			f.App.HandleEvent(ev)
		}
	}
}

// ---- dialogs ----

func (d *Desktop) askQuit() {
	d.closeMenu()
	d.Confirm("Quit", "Quit tuidesk?\nAll open windows will be closed.", []string{"Quit", "Cancel"}, func(choice int) {
		if choice != 0 {
			return
		}
		for _, w := range d.M.Windows() {
			if w.App != nil {
				_ = w.App.Close()
			}
		}
		d.Quit = true
	})
}

// dropClosedModals removes modals their owner closed (e.g. progress).
func (d *Desktop) dropClosedModals() {
	kept := d.modals[:0]
	for _, m := range d.modals {
		if p, ok := m.(*ui.ProgressModal); ok && p.Closed() {
			continue
		}
		kept = append(kept, m)
	}
	d.modals = kept
}

func (d *Desktop) removeModal(m ui.Modal) {
	for i, x := range d.modals {
		if x == m {
			d.modals = append(d.modals[:i], d.modals[i+1:]...)
			return
		}
	}
}

// ---- menus ----

func (d *Desktop) closeMenu() { d.menu, d.menuTarget, d.menuFn = nil, nil, nil }

func (d *Desktop) openMenu(items []ui.MenuItem, target *wm.Window, x, y int) {
	d.mode = ModeNormal
	m := ui.NewMenu(items)
	w, h := m.Size()
	m.Rect = ui.PlacePopup(wm.Rect{X: 0, Y: 0, W: d.W, H: d.H}, x, y, w, h)
	d.menu, d.menuTarget, d.menuFn = m, target, nil
}

func (d *Desktop) openLauncher(x, y int) {
	var items []ui.MenuItem
	if d.NewFiles != nil {
		items = append(items, ui.MenuItem{Label: "Files", Hotkey: 'f', Action: "files"})
	}
	items = append(items, []ui.MenuItem{
		{Label: "New window", Hotkey: 'n', Shortcut: "Ctrl+N", Action: "new"},
		{Label: "Help", Hotkey: 'h', Shortcut: "F1", Action: "help"},
	}...)
	if wins := d.M.Windows(); len(wins) > 0 {
		items = append(items, ui.MenuItem{Separator: true})
		for _, w := range wins {
			label := w.Name()
			if w.Minimized {
				label += " (minimized)"
			}
			items = append(items, ui.MenuItem{Label: label, Action: "raise:" + strconv.Itoa(w.ID)})
		}
	}
	items = append(items, ui.MenuItem{Separator: true}, ui.MenuItem{Label: "Quit", Hotkey: 'q', Shortcut: "Ctrl+Q", Action: "quit"})
	d.openMenu(items, nil, x, y)
}

func (d *Desktop) openWindowMenu(w *wm.Window, x, y int) {
	maxLabel := "Maximize"
	if w.Maximized {
		maxLabel = "Restore"
	}
	d.openMenu([]ui.MenuItem{
		{Label: "Move", Hotkey: 'm', Shortcut: "F7", Action: "move", Disabled: w.Maximized},
		{Label: "Size", Hotkey: 's', Shortcut: "F8", Action: "resize", Disabled: w.Maximized},
		{Label: maxLabel, Hotkey: 'x', Action: "maximize"},
		{Label: "Minimize", Hotkey: 'n', Action: "minimize"},
		{Separator: true},
		{Label: "Close", Hotkey: 'c', Shortcut: "Ctrl+W", Action: "close"},
	}, w, x, y)
}

// handleMenu returns true if the open menu consumed the event.
func (d *Desktop) handleMenu(ev term.Event) bool {
	var r ui.MenuResult
	switch ev := ev.(type) {
	case term.KeyEvent:
		r = d.menu.HandleKey(ev.Key)
	case term.MouseEvent:
		r = d.menu.HandleMouse(ev.Gesture)
	default:
		return false
	}
	switch r {
	case ui.MenuActivated:
		item, target, fn := d.menu.Chosen, d.menuTarget, d.menuFn
		d.closeMenu()
		if fn != nil {
			fn(item.Action)
		} else {
			d.run(item.Action, target)
		}
	case ui.MenuClosed:
		d.closeMenu()
	case ui.MenuIgnored:
		return false
	}
	return true
}

// run executes a menu or shortcut action. target is the window the action
// applies to (nil = focused window).
func (d *Desktop) run(action string, target *wm.Window) {
	if target == nil {
		target = d.M.Focused()
	}
	switch {
	case action == "new":
		d.OpenWindow(d.NewApp(d.created + 1))
	case action == "files" && d.NewFiles != nil:
		w := d.OpenWindow(d.NewFiles(d.HomeDir))
		d.M.ResizeTo(w, min(84, d.Area().W-4), min(22, d.Area().H-2))
	case action == "help":
		d.showHelp()
	case action == "quit":
		d.askQuit()
	case action == "next":
		d.M.FocusNext()
	case action == "prev":
		d.M.FocusPrev()
	case strings.HasPrefix(action, "raise:"):
		id, _ := strconv.Atoi(strings.TrimPrefix(action, "raise:"))
		for _, w := range d.M.Windows() {
			if w.ID == id {
				d.M.Raise(w)
			}
		}
	case target == nil:
		return
	case action == "move" && !target.Maximized:
		d.M.Raise(target)
		d.mode = ModeMove
	case action == "resize" && !target.Maximized:
		d.M.Raise(target)
		d.mode = ModeResize
	case action == "maximize":
		d.M.Raise(target)
		d.M.ToggleMaximize(target)
	case action == "minimize":
		d.M.Minimize(target)
	case action == "close":
		d.closeWindow(target)
	}
}

func (d *Desktop) closeWindow(w *wm.Window) {
	if w.App != nil {
		if err := w.App.Close(); err != nil {
			d.Notify("Cannot close window: "+err.Error(), true)
			return
		}
	}
	d.M.Remove(w)
	if d.help == w {
		d.help = nil
	}
	d.mode = ModeNormal
}

var helpLines = []string{
	"Mouse",
	"  Drag title bar: move       Drag bottom-right corner: resize",
	"  [_] minimize  [^] maximize  [x] close  Double-click title: maximize",
	"  Right click or Ctrl+click: window menu",
	"",
	"Keyboard",
	"  F9         Menu            F6 / Shift+F6  Next / previous window",
	"  Shift+F10  Window menu     F7 Move mode   F8 Size mode",
	"  Ctrl+N     New window      Ctrl+W Close   Ctrl+Q Quit",
	"  Tab / Shift+Tab: switch windows (when the app does not use Tab)",
	"  Move/size mode: arrow keys, Enter or Esc to finish",
}

func (d *Desktop) showHelp() {
	if d.help != nil {
		d.M.Raise(d.help)
		return
	}
	d.help = d.OpenWindow(&blank.Text{Name: "Help", Lines: helpLines})
	d.M.ResizeTo(d.help, 74, len(helpLines)+2)
}

// ---- keyboard ----

func (d *Desktop) handleKey(k term.Key) {
	f := d.M.Focused()
	name := k.String()

	if d.mode != ModeNormal && f != nil {
		dx, dy := 0, 0
		switch name {
		case "Up":
			dy = -1
		case "Down":
			dy = 1
		case "Left":
			dx = -2
		case "Right":
			dx = 2
		case "Enter", "Esc", "F7", "F8":
			d.mode = ModeNormal
			return
		default:
			d.mode = ModeNormal // any other key ends the mode and is handled normally
		}
		if dx != 0 || dy != 0 {
			if d.mode == ModeMove {
				d.M.MoveBy(f, dx, dy)
			} else {
				d.M.ResizeBy(f, dx, dy)
			}
			return
		}
	}

	if f != nil && f.App != nil && f.App.HandleEvent(term.KeyEvent{Key: k}) {
		return
	}

	switch name {
	case "F9", "Ctrl+Space":
		d.openLauncher(0, 1)
	case "F6", "Tab":
		d.run("next", nil)
	case "Shift+F6", "Shift+Tab":
		d.run("prev", nil)
	case "Shift+F10":
		if f != nil {
			d.openWindowMenu(f, f.Rect.X+1, f.Rect.Y+1)
		} else {
			d.openLauncher(0, 1)
		}
	case "F7":
		d.run("move", nil)
	case "F8":
		d.run("resize", nil)
	case "Ctrl+W":
		d.run("close", nil)
	case "Ctrl+N":
		d.run("new", nil)
	case "Ctrl+Q":
		d.run("quit", nil)
	case "F1":
		d.run("help", nil)
	}
}

// ---- mouse ----

func local(g term.Gesture, r wm.Rect) term.Gesture {
	g.X -= r.X
	g.Y -= r.Y
	g.StartX -= r.X
	g.StartY -= r.Y
	return g
}

func (d *Desktop) forward(w *wm.Window, g term.Gesture) {
	if w != nil && w.App != nil {
		w.App.HandleEvent(term.MouseEvent{Gesture: local(g, w.Inner())})
	}
}

func (d *Desktop) handleMouse(g term.Gesture) {
	d.mode = ModeNormal

	// Bars react to clicks.
	if g.Y == 0 || g.Y == d.H-1 {
		if g.Kind == term.GestureClick && g.Button == term.ButtonLeft {
			items := d.topItems
			if g.Y == d.H-1 {
				items = d.taskItems
			}
			for _, it := range items {
				if it.r.Contains(g.X, g.Y) {
					d.barAction(it.action, it.r)
				}
			}
		}
		if g.Kind == term.GestureRelease || g.Kind == term.GestureDragEnd {
			d.M.PointerUp()
		}
		if !(d.M.Grabbing() && g.Kind == term.GestureDrag) {
			return
		}
	}

	ctxClick := g.Kind == term.GestureClick &&
		(g.Button == term.ButtonRight || (g.Button == term.ButtonLeft && g.Mod&term.ModCtrl != 0))
	if ctxClick {
		if w, part := d.M.HitTest(g.X, g.Y); w != nil {
			d.M.Raise(w)
			if part == wm.PartBody && w.App != nil && w.App.HandleEvent(term.MouseEvent{Gesture: local(g, w.Inner())}) {
				return // the app opened its own context menu
			}
			d.openWindowMenu(w, g.X, g.Y)
		} else {
			d.openLauncher(g.X, g.Y)
		}
		return
	}
	if g.Button != term.ButtonLeft && g.Kind != term.GestureWheel {
		return
	}

	switch g.Kind {
	case term.GesturePress:
		if g.Mod&term.ModCtrl != 0 {
			return
		}
		if w, part := d.M.PointerDown(g.X, g.Y); part == wm.PartBody {
			d.forward(w, g)
		}
	case term.GestureDragStart, term.GestureDrag:
		if d.M.Grabbing() {
			d.M.PointerDrag(g.X, g.Y)
		} else if f := d.M.Focused(); f != nil && f.Inner().Contains(g.StartX, g.StartY) {
			d.forward(f, g)
		}
	case term.GestureRelease, term.GestureDragEnd:
		if d.M.Grabbing() {
			d.M.PointerUp()
		} else if f := d.M.Focused(); f != nil && f.Inner().Contains(g.StartX, g.StartY) {
			d.forward(f, g)
		}
	case term.GestureClick:
		w, part := d.M.HitTest(g.StartX, g.StartY)
		switch part {
		case wm.PartMinimize:
			d.M.Minimize(w)
		case wm.PartMaximize:
			d.M.ToggleMaximize(w)
		case wm.PartClose:
			d.closeWindow(w)
		case wm.PartTitle:
			if g.Count == 2 {
				d.M.ToggleMaximize(w)
			}
		case wm.PartBody:
			d.forward(w, g)
		}
	case term.GestureWheel:
		if w, part := d.M.HitTest(g.X, g.Y); part == wm.PartBody {
			d.forward(w, g)
		}
	}
}

func (d *Desktop) barAction(action string, r wm.Rect) {
	switch {
	case action == "launcher":
		d.openLauncher(r.X, r.Y+1)
	case strings.HasPrefix(action, "raise:"):
		id, _ := strconv.Atoi(strings.TrimPrefix(action, "raise:"))
		for _, w := range d.M.Windows() {
			if w.ID != id {
				continue
			}
			if w == d.M.Focused() && !w.Minimized && r.Y == 0 {
				d.M.Minimize(w) // clicking the focused tab minimizes, like a taskbar
			} else {
				d.M.Raise(w)
			}
		}
	}
}

// ---- drawing ----

// Draw paints the whole desktop.
func (d *Desktop) Draw(c *wm.Canvas) {
	d.M.Draw(c)
	d.drawTopBar(c)
	d.drawTaskbar(c)
	if d.menu != nil {
		d.menu.Draw(c)
	}
	for _, m := range d.modals {
		m.Draw(c)
	}
	d.drawDrag(c)
	a := d.Area()
	d.toasts.Draw(c, wm.Rect{X: a.X + a.W/3, Y: a.Y, W: a.W - a.W/3, H: a.H})
}

func (d *Desktop) drawTopBar(c *wm.Canvas) {
	s := wm.NewSurface(c, wm.Rect{X: 0, Y: 0, W: d.W, H: 1})
	s.Clear(wm.StyleBar)
	d.topItems = d.topItems[:0]

	right := fmt.Sprintf(" %s  %s ", d.HostName, d.Now.Format("15:04"))
	limit := d.W - term.Width(right) - 1

	x := 0
	add := func(label, action string, st wm.StyleID) bool {
		w := term.Width(label)
		if x+w > limit {
			return false
		}
		s.Sub(wm.Rect{X: x, Y: 0, W: w, H: 1}).Text(0, 0, label, st)
		d.topItems = append(d.topItems, barItem{r: wm.Rect{X: x, Y: 0, W: w, H: 1}, action: action})
		x += w + 1
		return true
	}
	st := wm.StyleBar
	if d.menu != nil && d.menuTarget == nil {
		st = wm.StyleBarActive
	}
	add(" ≡ Menu ", "launcher", st)
	for _, w := range d.M.Windows() {
		if w.Minimized {
			continue
		}
		st := wm.StyleBar
		if w == d.M.Focused() {
			st = wm.StyleBarActive
		}
		if !add(" "+clip(w.Name(), 16)+" ", "raise:"+strconv.Itoa(w.ID), st) {
			break
		}
	}
	s.Text(d.W-term.Width(right), 0, right, wm.StyleBar)
}

func (d *Desktop) drawTaskbar(c *wm.Canvas) {
	y := d.H - 1
	s := wm.NewSurface(c, wm.Rect{X: 0, Y: y, W: d.W, H: 1})
	s.Clear(wm.StyleBar)
	d.taskItems = d.taskItems[:0]

	switch d.mode {
	case ModeMove:
		s.Text(1, 0, "MOVE: arrow keys move the window · Enter/Esc to finish", wm.StyleBarActive)
		return
	case ModeResize:
		s.Text(1, 0, "SIZE: arrow keys resize the window · Enter/Esc to finish", wm.StyleBarActive)
		return
	}

	x := 0
	for _, w := range d.M.Windows() {
		if !w.Minimized {
			continue
		}
		label := " [" + clip(w.Name(), 14) + "] "
		lw := term.Width(label)
		if x+lw > d.W {
			break
		}
		s.Text(x, 0, label, wm.StyleBarActive)
		d.taskItems = append(d.taskItems, barItem{r: wm.Rect{X: x, Y: y, W: lw, H: 1}, action: "raise:" + strconv.Itoa(w.ID)})
		x += lw + 1
	}
	hint := " F9 Menu · F6 Next window · Shift+F10 Window menu · F1 Help "
	if hx := d.W - term.Width(hint); hx > x {
		s.Text(hx, 0, hint, wm.StyleBar)
	}
}

func clip(s string, w int) string {
	if term.Width(s) <= w {
		return s
	}
	var out []rune
	cw := 0
	for _, r := range s {
		rw := term.RuneWidth(r)
		if cw+rw > w-1 {
			break
		}
		out = append(out, r)
		cw += rw
	}
	return string(out) + "…"
}

// Done reports whether the user chose to quit.
func (d *Desktop) Done() bool { return d.Quit }
