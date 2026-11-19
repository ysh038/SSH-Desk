// Package desktop is the shell: taskbar, launcher, window menu,
// keyboard move/resize modes, dialogs and toasts around a wm.Manager.
// It consumes term events and draws onto a wm.Canvas; it does not use tcell.
package desktop

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"tuidesk/internal/apps/blank"
	"tuidesk/internal/config"
	"tuidesk/internal/host"
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
	// NewFiles creates a file manager for dir. When set, "New window" opens
	// a file manager at RootDir instead of an empty window.
	NewFiles func(dir string) wm.App
	// RootDir is where "New window" file managers start (default "/").
	RootDir string
	// OpenText opens a text file in the editor (set by main).
	OpenText func(path string)
	// NewMyPC creates the My PC window (SPEC 7.8, set by main).
	NewMyPC func() wm.App
	// NewTaskManager creates the Task Manager window (SPEC 7.9, set by main).
	NewTaskManager func() wm.App
	tasks          *wm.Window
	// NewSettings creates the Settings window (SPEC 7.7, set by main).
	NewSettings func() wm.App
	settings    *wm.Window
	// HomeDir is where new file managers start.
	HomeDir string
	Quit    bool
	mypc    *wm.Window

	menu       *ui.Menu
	menuTarget *wm.Window
	menuFn     func(action string) // app menu callback (nil: desktop action)
	modals     []ui.Modal
	toasts     ui.Toasts
	svc        services
	cfg        config.Config
	cfgPath    string
	mode       Mode
	barItems   []barItem
	created    int
	help       *wm.Window
	iconSel    int // selected desktop icon (-1: none)
	iconHover  int // desktop icon under a file drag (-1: none)
	apps       []config.AppManifest
}

// New creates a desktop of w×h cells.
func New(w, h int, host string, now time.Time) *Desktop {
	d := &Desktop{HostName: host, Now: now, M: &wm.Manager{}, iconSel: -1, iconHover: -1}
	d.M.Background = d.drawIcons
	d.svc.init()
	d.cfg = config.Default()
	d.NewApp = func(n int) wm.App { return blank.New(n) }
	d.Resize(w, h)
	return d
}

// SetConfig installs settings loaded from path. When the keyboard style has
// not been chosen yet (and this is not the Linux console), it asks once.
func (d *Desktop) SetConfig(cfg config.Config, path string) {
	d.cfg, d.cfgPath = cfg, path
	d.applyConfig()
	if cfg.Keyboard == "" && !d.svc.console {
		d.Confirm("Keyboard", "Which keyboard do you use?\n\nShortcuts are shown for that keyboard, e.g.\nMac: ⌃S  ⌥↓  fn F2      PC: Ctrl+S  Alt+↓  F2\n\nYou can change this later in Menu › Settings.",
			[]string{"Mac keyboard", "PC keyboard"}, func(c int) {
				if c == 0 {
					d.setKeyboard("mac")
				} else {
					d.setKeyboard("pc")
				}
			})
	}
}

func (d *Desktop) applyConfig() {
	style := term.LabelPC
	if d.cfg.Keyboard == "mac" && !d.svc.console { // the console cannot draw ⌃⌥
		style = term.LabelMac
	}
	term.SetLabelStyle(style)
	ui.WheelLines = d.cfg.WheelLines
	d.toasts.Duration = time.Duration(d.cfg.NotifySeconds) * time.Second
}

func (d *Desktop) saveConfig() {
	if d.cfgPath == "" {
		return
	}
	if err := config.Save(d.cfgPath, d.cfg); err != nil {
		d.Notify("Cannot save settings: "+err.Error(), true)
	}
}

func (d *Desktop) setKeyboard(k string) {
	d.cfg.Keyboard = k
	d.applyConfig()
	d.saveConfig()
	d.Notify("Shortcuts are now shown for a "+map[string]string{"mac": "Mac", "pc": "PC"}[k]+" keyboard", false)
}

// UpdateConfig changes the settings, applies them and saves the file
// (host.Host). Values out of range are put right first.
func (d *Desktop) UpdateConfig(fn func(c *config.Config)) {
	fn(&d.cfg)
	d.cfg.Normalize()
	d.applyConfig()
	d.saveConfig()
}

// MultiClickInterval is the double-click speed (Settings › Mouse); the
// terminal layer asks for it.
func (d *Desktop) MultiClickInterval() time.Duration {
	return time.Duration(d.cfg.DoubleClickMs) * time.Millisecond
}

// OpenSettings shows the Settings window (SPEC 7.7), raising it when it is
// already open, on section and setting id ("" = leave as it is).
func (d *Desktop) OpenSettings(section, id string) {
	if d.NewSettings == nil {
		d.Notify("Settings are not available", true)
		return
	}
	open := false
	for _, w := range d.M.Windows() {
		if w == d.settings {
			d.M.Raise(w)
			open = true
		}
	}
	if !open {
		d.settings = d.OpenWindow(d.NewSettings())
		d.M.ResizeTo(d.settings, min(96, d.Area().W-2), min(22, d.Area().H-1))
	}
	if s, ok := d.settings.App.(interface{ Show(section, id string) }); ok && section != "" {
		s.Show(section, id)
	}
}

// clockText is the clock in the top bar (Settings › Date & Time).
func (d *Desktop) clockText() string {
	layout := "15:04"
	if d.cfg.ClockSeconds {
		layout = "15:04:05"
	}
	if d.cfg.Clock == "12h" {
		layout = strings.Replace(layout, "15", "3", 1) + " PM"
	}
	if d.cfg.ClockDate {
		layout = "2006-01-02 " + layout
	}
	return d.Now.Format(layout)
}

// Area is the region windows live in (between the two bars).
func (d *Desktop) Area() wm.Rect { return wm.Rect{X: 0, Y: 0, W: d.W, H: max(d.H-1, 1)} }

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
	unit := time.Minute
	if d.cfg.ClockSeconds {
		unit = time.Second // only the clock cells change (CLAUDE.md rule 9)
	}
	next := d.Now.Truncate(unit).Add(unit)
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
	msg := "Quit tuidesk?\nAll open windows will be closed."
	unsaved := 0
	for _, w := range d.M.Windows() {
		if c, ok := w.App.(host.Closer); ok {
			unsaved += c.Unsaved()
		}
	}
	if unsaved > 0 {
		msg = fmt.Sprintf("Quit tuidesk?\n%d file(s) have unsaved changes that will be lost.", unsaved)
	}
	var progs []string
	for _, w := range d.M.Windows() {
		if p := running(w); p != "" {
			progs = append(progs, d.display(p))
		}
	}
	if len(progs) > 0 {
		msg += "\nStill running in terminals: " + strings.Join(progs, ", ") + "."
	}
	d.Confirm("Quit", msg, []string{"Quit", "Cancel"}, func(choice int) {
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
	sep := ui.MenuItem{Separator: true}
	items := []ui.MenuItem{
		// Programs.
		{Label: "New window", Hotkey: 'n', Shortcut: "Ctrl+N", Action: "new"},
		{Label: "Terminal", Hotkey: 't', Action: "terminal"},
		{Label: "Text editor", Hotkey: 'e', Action: "editor"},
	}
	for i, a := range d.apps {
		items = append(items, ui.MenuItem{Label: d.display(a.Name), Action: "app:" + strconv.Itoa(i)})
	}
	items = append(items, sep,
		// This computer.
		ui.MenuItem{Label: "My PC", Hotkey: 'm', Action: "mypc", Disabled: d.NewMyPC == nil},
		ui.MenuItem{Label: "Task Manager", Hotkey: 'k', Action: "tasks", Disabled: d.NewTaskManager == nil},
		ui.MenuItem{Label: "Settings", Hotkey: 's', Action: "settings"},
	)
	if wins := d.M.Windows(); len(wins) > 0 {
		items = append(items, sep)
		for _, w := range wins {
			label := w.Name()
			if w.Minimized {
				label += " (minimized)"
			}
			items = append(items, ui.MenuItem{Label: label, Action: "raise:" + strconv.Itoa(w.ID)})
		}
	}
	items = append(items, sep,
		ui.MenuItem{Label: "Show desktop", Hotkey: 'd', Action: "desktop"},
		ui.MenuItem{Label: "Help", Hotkey: 'h', Shortcut: "F1", Action: "help"},
		sep,
		ui.MenuItem{Label: "Quit", Hotkey: 'q', Shortcut: "Ctrl+Q", Action: "quit"},
	)
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
	case ui.MenuSide:
		side, dir := d.menu.Side, d.menu.SideDir
		d.closeMenu()
		side(dir)
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
	case action == "new" && d.NewFiles != nil:
		root := d.RootDir
		if root == "" {
			root = "/"
		}
		w := d.OpenWindow(d.NewFiles(root))
		d.M.ResizeTo(w, min(84, d.Area().W-4), min(22, d.Area().H-2))
	case action == "new":
		d.OpenWindow(d.NewApp(d.created + 1))
	case action == "editor" && d.OpenText != nil:
		d.OpenText("")
	case action == "mypc":
		d.openMyPC()
	case action == "tasks":
		d.openTaskManager()
	case action == "terminal":
		d.OpenTerminal(d.selectedDir(), nil, false)
	case strings.HasPrefix(action, "app:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(action, "app:")); err == nil && i < len(d.apps) {
			d.runApp(d.apps[i])
		}
	case action == "help":
		d.showHelp()
	case action == "desktop":
		d.showDesktop()
	case (action == "next" || action == "prev") && d.M.Focused() == nil:
		if t := d.M.Top(); t != nil {
			d.M.Raise(t)
		} else if wins := d.M.Windows(); len(wins) > 0 {
			d.M.Raise(wins[len(wins)-1]) // everything minimized: bring one back
		}
	case action == "settings":
		d.OpenSettings("", "")
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
	if d.confirmRunning(w, func() { d.forceClose(w) }) {
		return
	}
	if c, ok := w.App.(host.Closer); ok && c.Unsaved() > 0 {
		c.RequestClose(func() { d.CloseApp(w.App) })
		return
	}
	d.forceClose(w)
}

// forceClose closes w without asking.
func (d *Desktop) forceClose(w *wm.Window) {
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
	if d.settings == w {
		d.settings = nil
	}
	d.mode = ModeNormal
}

type helpRow struct{ keys, what string }

var helpSections = []struct {
	title string
	rows  []helpRow
}{
	{"Desktop", []helpRow{
		{"F9", "Menu (launcher)"}, {"Ctrl+N", "New window (file manager at /)"},
		{"F9 > Show desktop", "Desktop icons: arrows, Enter opens, Delete removes"},
		{"F9 > Settings", "All settings; type to search them"},
		{"F9 > Task Manager", "Running programs: type to filter, Delete ends one"},
		{"F9 > My PC", "Drives and USB devices: Enter opens (mounts first)"},
		{"Ctrl+U / Ctrl+E", "In My PC: unmount / eject (safe to remove)"},
		{"F9 > Terminal", "Shell window (every key goes to the shell)"},
		{"Ctrl+]", "In a terminal: menu (windows, paste, copy, scroll back)"},
		{"F6 / Shift+F6", "Next / previous window"}, {"Shift+F10", "Window menu (also right click)"},
		{"F7 / F8", "Move / size window with arrow keys"}, {"Ctrl+W", "Close window"},
		{"Ctrl+Q", "Quit tuidesk"}, {"F1", "This help"},
	}},
	{"Files", []helpRow{
		{"Enter", "Open"}, {"Backspace", "Parent folder"}, {"Space", "Select / unselect"},
		{"Shift+Down", "Extend selection (Ctrl/Option+click adds one)"}, {"F2", "Rename"},
		{"Delete", "Move to Trash"}, {"Ctrl+Z", "Undo"}, {"Ctrl+C / Ctrl+X / Ctrl+V", "Copy / cut / paste"},
		{"Ctrl+D", "Duplicate"}, {"Ctrl+P", "Permissions"}, {"Ctrl+L", "Go to folder"},
		{"Ctrl+T", "Open terminal here"},
		{"F5", "Refresh"}, {"Alt+.", "Show hidden files"},
	}},
	{"Editor", []helpRow{
		{"Ctrl+S", "Save (offers sudo when needed)"}, {"Ctrl+Z / Ctrl+Y", "Undo / redo"},
		{"Ctrl+F / Ctrl+R", "Find / replace"}, {"Ctrl+G", "Go to line"},
		{"Home / End", "Line start / end"}, {"Alt+Up / Alt+Down", "Document start / end"},
		{"Alt+Left / Alt+Right", "Previous / next word"}, {"PgUp / PgDn", "One screen up / down"},
		{"Shift+arrows", "Select (Linux console: Ctrl+Space, then move)"},
		{"Ctrl+T", "Insert snippet (or type a trigger, then Tab)"}, {"Ctrl+Q", "Close tab"},
		{"Ctrl+K", "Search all commands"},
	}},
}

func helpLines() []string {
	lines := []string{
		"Mouse: drag a title bar to move, drag the bottom-right corner to resize,",
		"[_] minimize  [^] maximize  [x] close, double-click a title to maximize.",
		"Type to filter files; drag selected files onto a folder to move or copy.",
		"Desktop icons: double-click opens; drop files on a folder icon to move/copy,",
		"on empty desktop to add shortcuts. View > Icon view switches the file list.",
	}
	for _, sec := range helpSections {
		lines = append(lines, "", sec.title)
		for _, r := range sec.rows {
			k := term.KeyLabel(r.keys)
			lines = append(lines, "  "+k+strings.Repeat(" ", max(26-term.Width(k), 1))+r.what)
		}
	}
	return lines
}

func (d *Desktop) showHelp() {
	if d.help != nil {
		d.M.Raise(d.help)
		return
	}
	lines := helpLines()
	d.help = d.OpenWindow(&blank.Text{Name: "Help", Lines: lines})
	d.M.ResizeTo(d.help, 78, len(lines)+2)
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
	if f == nil && d.handleIconKey(k) {
		return
	}

	switch name {
	case "F9", "Ctrl+Space":
		d.openLauncher(0, d.H-2)
	case "F6", "Tab":
		d.run("next", nil)
	case "Shift+F6", "Shift+Tab":
		d.run("prev", nil)
	case "Shift+F10":
		if f != nil {
			d.openWindowMenu(f, f.Rect.X+1, f.Rect.Y+1)
		} else {
			d.openLauncher(0, d.H-2)
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

	// The taskbar reacts to clicks.
	if g.Y == d.H-1 {
		if g.Kind == term.GestureClick && g.Button == term.ButtonLeft {
			for _, it := range d.barItems {
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

	// Right click opens menus. Ctrl/Alt+click go to the app (multi-select in
	// the file manager, Windows convention; Cmd never reaches the terminal).
	ctxClick := g.Kind == term.GestureClick && g.Button == term.ButtonRight
	if ctxClick {
		if w, part := d.M.HitTest(g.X, g.Y); w != nil {
			d.M.Raise(w)
			if part == wm.PartBody && w.App != nil && w.App.HandleEvent(term.MouseEvent{Gesture: local(g, w.Inner())}) {
				return // the app opened its own context menu
			}
			d.openWindowMenu(w, g.X, g.Y)
		} else if i := d.iconAt(g.X, g.Y); i >= 0 {
			d.M.Unfocus()
			d.iconMenu(i, g.X, g.Y)
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
		case wm.PartNone:
			d.clickDesktop(g)
		}
	case term.GestureWheel:
		if w, part := d.M.HitTest(g.X, g.Y); part == wm.PartBody {
			d.forward(w, g)
		}
	}
}

// clickDesktop selects (or with a double click opens) a desktop icon.
func (d *Desktop) clickDesktop(g term.Gesture) {
	if !d.Area().Contains(g.X, g.Y) {
		return
	}
	i := d.iconAt(g.X, g.Y)
	d.iconSel = i
	if i < 0 {
		return
	}
	d.M.Unfocus()
	if g.Count == 2 {
		d.openIcon(i)
	}
}

func (d *Desktop) barAction(action string, r wm.Rect) {
	switch {
	case action == "launcher":
		d.openLauncher(r.X, r.Y-1)
	case strings.HasPrefix(action, "raise:"):
		id, _ := strconv.Atoi(strings.TrimPrefix(action, "raise:"))
		for _, w := range d.M.Windows() {
			if w.ID != id {
				continue
			}
			if w == d.M.Focused() && !w.Minimized {
				d.M.Minimize(w) // clicking the focused tab minimizes, like a taskbar
			} else {
				d.M.Raise(w)
			}
			return // Raise reorders the windows being ranged over
		}
	}
}

// ---- drawing ----

// Draw paints the whole desktop.
func (d *Desktop) Draw(c *wm.Canvas) {
	d.M.Draw(c)
	d.drawBar(c)
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

// drawBar paints the taskbar on the bottom row, laid out like the Windows
// one: the menu button, one tab per window (minimized ones in brackets),
// then the host name and the clock. Key hints fill whatever room is left.
func (d *Desktop) drawBar(c *wm.Canvas) {
	y := d.H - 1
	s := wm.NewSurface(c, wm.Rect{X: 0, Y: y, W: d.W, H: 1})
	s.Clear(wm.StyleBar)
	d.barItems = d.barItems[:0]

	switch d.mode {
	case ModeMove:
		s.Text(1, 0, "MOVE: arrow keys move the window · Enter/Esc to finish", wm.StyleBarActive)
		return
	case ModeResize:
		s.Text(1, 0, "SIZE: arrow keys resize the window · Enter/Esc to finish", wm.StyleBarActive)
		return
	}

	right := " " + d.clockText() + " "
	if d.cfg.ShowHost {
		right = " " + d.HostName + " " + right
	}
	limit := d.W - term.Width(right) - 1

	x := 0
	add := func(label, action string, st wm.StyleID) bool {
		w := term.Width(label)
		if x+w > limit {
			return false
		}
		s.Sub(wm.Rect{X: x, Y: 0, W: w, H: 1}).Text(0, 0, label, st)
		d.barItems = append(d.barItems, barItem{r: wm.Rect{X: x, Y: y, W: w, H: 1}, action: action})
		x += w + 1
		return true
	}
	st := wm.StyleBar
	if d.menu != nil && d.menuTarget == nil {
		st = wm.StyleBarActive
	}
	add(" ≡ Menu ", "launcher", st)
	// Tabs stay in the order the windows were opened; the manager's own
	// order is front-to-back and would shuffle them on every click.
	wins := append([]*wm.Window(nil), d.M.Windows()...)
	sort.Slice(wins, func(i, j int) bool { return wins[i].ID < wins[j].ID })
	for _, w := range wins {
		st, label := wm.StyleBar, " "+clip(w.Name(), 16)+" "
		switch {
		case w.Minimized:
			label = " [" + clip(w.Name(), 14) + "] "
		case w == d.M.Focused():
			st = wm.StyleBarActive
		}
		if !add(label, "raise:"+strconv.Itoa(w.ID), st) {
			break
		}
	}
	s.Text(d.W-term.Width(right), 0, right, wm.StyleBar)

	// Key hints, the longest version that fits between the tabs and the clock.
	for _, hint := range []string{
		term.KeyLabel("F9") + " Menu · " + term.KeyLabel("F6") + " Next window · " +
			term.KeyLabel("Shift+F10") + " Window menu · " + term.KeyLabel("F1") + " Help",
		term.KeyLabel("F9") + " Menu · " + term.KeyLabel("F6") + " Next window · " + term.KeyLabel("F1") + " Help",
		term.KeyLabel("F9") + " Menu · " + term.KeyLabel("F1") + " Help",
		term.KeyLabel("F9") + " Menu",
	} {
		if hx := limit - term.Width(hint); hx > x {
			s.Text(hx, 0, hint, wm.StyleBar)
			break
		}
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
