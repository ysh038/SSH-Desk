package settings

import (
	"strconv"
	"strings"

	"tuidesk/internal/config"
	"tuidesk/internal/host"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

type focus int

const (
	focusSections focus = iota
	focusItems
	focusSearch
)

const (
	sectionW = 22 // left pane
	searchY  = 1
	headY    = 2
	listY    = 3
	helpRows = 2 // explanation of the selected setting
)

// App is the Settings window: sections on the left, their settings on the
// right, a search field above both. A change applies and is saved at once.
type App struct {
	h     host.Host
	about About
	items []Item

	sec     int   // selected section
	shown   []int // items listed on the right (a section or search results)
	sel     int   // index into shown
	offset  int
	focus   focus
	search  *ui.Input
	probe   string // result of the double-click test
	bodyH   int
	w       int
	valueX  int // first column of the value, relative to the right pane
	paneX   int
	cmds    ui.Commands
	chrome  *host.Chrome
	lastQry string
}

// New opens the window on the first section.
func New(h host.Host, about About) *App {
	a := &App{h: h, about: about, items: Items(about), search: ui.NewInput("")}
	a.cmds.Add(
		&ui.Command{ID: "search", Menu: "Settings", Title: "Search settings", Hint: "Search", Keys: []string{"Ctrl+F", "/"}, Run: a.focusSearchField},
		&ui.Command{ID: "change", Menu: "Settings", Title: "Change the selected setting", Hint: "Change", Keys: []string{"Enter", "Space"},
			Enabled: func() bool { _, ok := a.current(); return ok }, Run: a.activate},
		&ui.Command{Menu: "Settings", Title: "-"},
		&ui.Command{ID: "open_config", Menu: "Settings", Title: "Edit the settings file", Run: func() { a.run("open_config") }},
		&ui.Command{ID: "reset", Menu: "Settings", Title: "Reset all settings...", Run: func() { a.run("reset") }},
		&ui.Command{Menu: "Settings", Title: "-"},
		&ui.Command{ID: "palette", Menu: "Settings", Title: "Command search...", Hint: "Commands", Keys: []string{"Ctrl+K"}, HintPinned: true,
			Run: func() { a.h.ShowPalette(&a.cmds) }},
	)
	a.chrome = host.NewChrome(h, a, &a.cmds)
	a.refresh()
	return a
}

func (a *App) Title() string       { return "Settings" }
func (a *App) MinSize() (int, int) { return 60, 12 }
func (a *App) Close() error        { return nil }

// Show selects a section by name and, when id is not "", one setting in it
// (My PC's "Mount location..." lands on the mount folder).
func (a *App) Show(section, id string) {
	for i, s := range Sections {
		if s == section {
			a.sec = i
		}
	}
	a.search.Clear()
	a.refresh()
	a.focus = focusItems
	for i, idx := range a.shown {
		if a.items[idx].ID == id {
			a.sel = i
		}
	}
}

// Query is the text in the search field.
func (a *App) Query() string { return strings.TrimSpace(a.search.Value()) }

// Shown returns the IDs of the settings listed on the right (tests).
func (a *App) Shown() []string {
	var out []string
	for _, i := range a.shown {
		out = append(out, a.items[i].ID)
	}
	return out
}

// Selected returns the ID of the selected setting ("" if none).
func (a *App) Selected() string {
	if it, ok := a.current(); ok {
		return it.ID
	}
	return ""
}

// refresh recomputes the right pane after the section or the query changed.
func (a *App) refresh() {
	q := a.Query()
	if q != "" {
		a.shown = Search(a.items, q)
	} else {
		a.shown = InSection(a.items, Sections[a.sec])
	}
	if q != a.lastQry {
		a.sel, a.offset, a.lastQry = 0, 0, q
	}
	a.sel = min(max(a.sel, 0), max(len(a.shown)-1, 0))
}

func (a *App) current() (Item, bool) {
	if a.sel < 0 || a.sel >= len(a.shown) {
		return Item{}, false
	}
	return a.items[a.shown[a.sel]], true
}

func (a *App) focusSearchField() {
	a.focus = focusSearch
	a.search.Select(0, len(a.search.Text))
}

func (a *App) clearSearch() {
	a.search.Clear()
	a.refresh()
}

// ---- changing values ----

func (a *App) update(fn func(c *config.Config)) { a.h.UpdateConfig(fn) }

// activate changes the selected setting the way its kind calls for.
func (a *App) activate() {
	it, ok := a.current()
	if !ok {
		return
	}
	cfg := a.h.Config()
	switch it.Kind {
	case Toggle:
		a.update(it.Flip)
	case Choice:
		cur := *it.Str(&cfg)
		var menu []ui.MenuItem
		for _, o := range it.Options {
			mark := "( ) "
			if o.Value == cur || (it.ID == "keyboard" && cur == "" && o.Value == "pc") {
				mark = "(*) "
			}
			menu = append(menu, ui.MenuItem{Label: mark + o.Label, Action: o.Value})
		}
		a.h.OpenMenu(a, a.paneX+a.valueX, listY+a.sel-a.offset+1, menu, func(v string) {
			a.update(func(c *config.Config) { *it.Str(c) = v })
		})
	case Number:
		p := ui.NewPrompt(it.Label, it.Label+" ("+strconv.Itoa(it.Min)+"-"+strconv.Itoa(it.Max)+" "+it.Unit+"):",
			strconv.Itoa(*it.Int(&cfg)), func(s string) {
				if n, err := it.ParseNumber(s); err == nil {
					a.update(func(c *config.Config) { it.SetNumber(c, n) })
				}
			})
		p.Validate = func(s string) error { _, err := it.ParseNumber(s); return err }
		a.h.Prompt(p)
	case Text:
		p := ui.NewPrompt(it.Label, it.Label+" (empty = default):", *it.Str(&cfg), func(s string) {
			if v, err := it.Check(s); err == nil {
				a.update(func(c *config.Config) { *it.Str(c) = v })
			}
		})
		p.Validate = func(s string) error { _, err := it.Check(s); return err }
		a.h.Prompt(p)
	case Action:
		a.run(it.ActionKey)
	case Info:
		a.h.SetClipboardText(it.Value)
		a.h.Notify("Copied: "+it.Value, false)
	case Probe:
		a.probe = "Use the mouse: double-click this line"
	}
}

// step changes a Number by its step (the - and + keys).
func (a *App) step(dir int) bool {
	it, ok := a.current()
	if !ok || it.Kind != Number {
		return false
	}
	a.update(func(c *config.Config) { it.SetNumber(c, *it.Int(c)+dir*it.Step) })
	return true
}

func (a *App) run(action string) {
	switch action {
	case "open_config":
		a.h.UpdateConfig(func(*config.Config) {}) // make sure the file exists
		a.h.OpenFile(a.about.ConfigPath)
	case "reset":
		a.h.Confirm("Reset all settings", "Put every setting in this window back to its default?\nDesktop icons and the keyboard type are kept.",
			[]string{"Reset", "Cancel"}, func(choice int) {
				if choice == 0 {
					a.update(func(c *config.Config) { *c = c.Reset() })
					a.h.Notify("Settings were reset to the defaults", false)
				}
			})
	default:
		a.h.DesktopAction(action) // "help", "mypc"
	}
}

// ---- events ----

func (a *App) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		return a.handleKey(ev.Key)
	case term.MouseEvent:
		return a.handleMouse(ev.Gesture)
	case term.PasteEvent:
		a.focus = focusSearch
		a.search.Paste(ev.Text)
		a.refresh()
		return true
	}
	return false
}

func (a *App) moveSel(delta int) {
	a.sel = min(max(a.sel+delta, 0), max(len(a.shown)-1, 0))
	a.ensureVisible()
}

func (a *App) ensureVisible() {
	h := max(a.bodyH, 1)
	if a.sel < a.offset {
		a.offset = a.sel
	}
	if a.sel >= a.offset+h {
		a.offset = a.sel - h + 1
	}
	a.offset = min(max(a.offset, 0), max(len(a.shown)-h, 0))
}

func (a *App) setSection(i int) {
	a.sec = min(max(i, 0), len(Sections)-1)
	a.search.Clear()
	a.sel, a.offset = 0, 0
	a.refresh()
}

// typed reports a key that starts or continues a search: a letter or digit
// typed outside the search field.
func typed(k term.Key) bool {
	return k.IsRune() && k.Mod&(term.ModCtrl|term.ModAlt|term.ModMeta) == 0 &&
		(k.Rune > 0x7f || k.Rune >= 'a' && k.Rune <= 'z' || k.Rune >= 'A' && k.Rune <= 'Z' || k.Rune >= '0' && k.Rune <= '9')
}

func (a *App) handleKey(k term.Key) bool {
	name := k.String()
	if a.focus == focusSearch {
		switch name {
		case "Ctrl+K", "F10": // not the field's "delete to the end"
			return a.chrome.HandleKey(k)
		case "Esc":
			if a.Query() == "" {
				a.focus = focusSections
			} else {
				a.clearSearch()
			}
		case "Down", "Enter", "Tab":
			if len(a.shown) > 0 {
				a.focus = focusItems
			}
		case "Shift+Tab", "Up":
			a.focus = focusSections
		default:
			if a.search.HandleKey(k) == ui.InputIgnored {
				return a.chrome.HandleKey(k)
			}
			a.refresh()
		}
		return true
	}
	if name != "/" && name != "Enter" && name != "Space" { // those belong to the lists below
		if a.chrome.HandleKey(k) {
			return true
		}
	}
	if name == "/" {
		a.focusSearchField()
		return true
	}
	if typed(k) {
		a.focus = focusSearch
		a.search.Select(len(a.search.Text), len(a.search.Text))
		a.search.HandleKey(k)
		a.refresh()
		return true
	}
	if a.focus == focusSections {
		switch name {
		case "Up":
			if a.sec == 0 {
				a.focusSearchField()
			} else {
				a.setSection(a.sec - 1)
			}
		case "Down":
			a.setSection(a.sec + 1)
		case "Home":
			a.setSection(0)
		case "End":
			a.setSection(len(Sections) - 1)
		case "Right", "Enter", "Tab":
			if len(a.shown) > 0 {
				a.focus = focusItems
			}
		case "Shift+Tab":
			a.focusSearchField()
		default:
			return false
		}
		return true
	}
	page := max(a.bodyH-1, 1)
	switch name {
	case "Up":
		if a.sel == 0 && a.Query() != "" {
			a.focusSearchField()
		} else {
			a.moveSel(-1)
		}
	case "Down":
		a.moveSel(1)
	case "PgUp":
		a.moveSel(-page)
	case "PgDn":
		a.moveSel(page)
	case "Home":
		a.moveSel(-len(a.shown))
	case "End":
		a.moveSel(len(a.shown))
	case "Left", "Shift+Tab":
		a.focus = focusSections
	case "Tab":
		a.focusSearchField()
	case "Esc":
		if a.Query() == "" {
			a.focus = focusSections
		} else {
			a.clearSearch()
		}
	case "Enter", "Space":
		a.activate()
	case "+", "=":
		return a.step(1)
	case "-":
		return a.step(-1)
	default:
		return false
	}
	return true
}

func (a *App) handleMouse(g term.Gesture) bool {
	switch g.Kind {
	case term.GestureWheel:
		n := ui.WheelLines
		if g.Wheel == term.WheelUp {
			n = -n
		}
		a.offset = min(max(a.offset+n, 0), max(len(a.shown)-max(a.bodyH, 1), 0))
		return true
	case term.GesturePress, term.GestureClick:
	default:
		return false
	}
	if g.Button != term.ButtonLeft {
		return g.Kind == term.GestureClick
	}
	click := g.Kind == term.GestureClick
	switch {
	case g.Y == 0:
		return click && a.chrome.ClickBar(g.X)
	case g.Y == searchY:
		a.focus = focusSearch
		a.search.ClickAt(g.X - searchFieldX)
	case g.Y >= listY && g.Y < listY+a.bodyH && g.X < sectionW:
		if i := g.Y - listY; i < len(Sections) {
			a.setSection(i)
			a.focus = focusSections
		}
	case g.Y >= listY && g.Y < listY+a.bodyH && g.X > sectionW:
		i := a.offset + g.Y - listY
		if i >= len(a.shown) {
			break
		}
		a.sel, a.focus = i, focusItems
		if !click {
			break
		}
		it := a.items[a.shown[i]]
		if it.Kind == Probe {
			a.probe = map[int]string{1: "One click - click again, faster", 2: "Double-click recognized", 3: "Triple click recognized"}[g.Count]
			break
		}
		if g.Count == 2 || g.X-a.paneX >= a.valueX {
			a.activate()
		}
	}
	return true
}

// ---- drawing ----

const searchLabel = " Search: "

var searchFieldX = term.Width(searchLabel)

func (a *App) Draw(s wm.Surface, focused bool) {
	w, h := s.Size()
	a.w = w
	cfg := a.h.Config()
	a.bodyH = max(h-listY-helpRows-a.chrome.HintRows(), 0)
	a.refresh()
	a.offset = min(max(a.offset, 0), max(len(a.shown)-max(a.bodyH, 1), 0))
	s.Clear(wm.StyleBody)
	a.chrome.DrawBar(s)
	a.chrome.DrawHints(s)

	// Search field.
	s.Text(0, searchY, searchLabel, wm.StyleBody)
	fieldW := min(max(w-searchFieldX-1, 1), 40)
	a.search.Draw(s.Sub(wm.Rect{X: searchFieldX, Y: searchY, W: fieldW, H: 1}), focused && a.focus == focusSearch)
	if a.Query() == "" && a.focus != focusSearch {
		s.Sub(wm.Rect{X: searchFieldX, Y: searchY, W: fieldW, H: 1}).Text(0, 0, "Type to search all settings", wm.StyleInput)
	}

	// Headers.
	q := a.Query()
	head := s.Sub(wm.Rect{X: 0, Y: headY, W: w, H: 1})
	head.Clear(wm.StyleHeader)
	head.Text(1, 0, "Settings", wm.StyleHeader)
	title := Sections[a.sec]
	if q != "" {
		title = strconv.Itoa(len(a.shown)) + " found for \"" + q + "\""
	}
	head.Text(sectionW+2, 0, title, wm.StyleHeader)

	// Sections.
	for i, name := range Sections {
		if i >= a.bodyH {
			break
		}
		st := wm.StyleBody
		if i == a.sec && q == "" {
			st = wm.StyleMenu
			if focused && a.focus == focusSections {
				st = wm.StyleSelected
			}
		}
		row := s.Sub(wm.Rect{X: 0, Y: listY + i, W: sectionW, H: 1})
		if st == wm.StyleBody && row.Hovered() {
			st = wm.StyleHover
		}
		row.Clear(st)
		row.Text(1, 0, name, st)
	}
	for y := 0; y < a.bodyH; y++ {
		s.Set(sectionW, listY+y, '│', wm.StyleBodyDim)
	}

	// Settings of the section, or search results.
	a.paneX = sectionW + 1
	paneW := max(w-a.paneX, 1)
	textW := paneW
	if len(a.shown) > a.bodyH && a.bodyH > 0 {
		textW = paneW - 1
		ui.DrawScrollbar(s.Sub(wm.Rect{X: w - 1, Y: listY, W: 1, H: a.bodyH}), len(a.shown), a.offset, a.bodyH)
	}
	// The value column starts after the longest name, but always keeps
	// room for a value.
	a.valueX = 0
	for _, idx := range a.shown {
		lw := term.Width(a.items[idx].Label)
		if q != "" {
			lw += term.Width(a.items[idx].Section) + 3
		}
		a.valueX = max(a.valueX, lw+4)
	}
	a.valueX = max(min(a.valueX, textW-18), 1)
	if len(a.shown) == 0 && a.bodyH > 0 {
		s.Sub(wm.Rect{X: a.paneX, Y: listY, W: textW, H: 1}).Text(1, 0, "No setting matches. Try another word, or press Esc.", wm.StyleBodyDim)
	}
	for y := 0; y < a.bodyH && a.offset+y < len(a.shown); y++ {
		i := a.offset + y
		it := a.items[a.shown[i]]
		st := wm.StyleBody
		if i == a.sel {
			st = wm.StyleMenu
			if focused && a.focus == focusItems {
				st = wm.StyleSelected
			}
		}
		row := s.Sub(wm.Rect{X: a.paneX, Y: listY + y, W: textW, H: 1})
		if st == wm.StyleBody && row.Hovered() {
			st = wm.StyleHover
		}
		row.Clear(st)
		label := it.Label
		if q != "" {
			label = it.Section + " > " + label
		}
		row.Sub(wm.Rect{X: 1, Y: 0, W: a.valueX - 2, H: 1}).Text(0, 0, label, st)
		val := it.Display(cfg)
		if it.Kind == Probe {
			val = a.probe
		}
		row.Sub(wm.Rect{X: a.valueX, Y: 0, W: textW - a.valueX, H: 1}).Text(0, 0, val, st)
	}

	// Help for the selected setting.
	if helpY := listY + a.bodyH; helpY+helpRows <= h-a.chrome.HintRows() {
		box := s.Sub(wm.Rect{X: 0, Y: helpY, W: w, H: helpRows})
		box.Clear(wm.StyleHeader)
		text := "Up/Down choose a group, Right or Enter opens it. Type to search."
		if it, ok := a.current(); ok && a.focus != focusSections {
			text = it.Help
		}
		for i, l := range Wrap(text, w-2, helpRows) {
			box.Text(1, i, l, wm.StyleHeader)
		}
	}
}

// Wrap breaks text at spaces into at most rows lines of width cells.
func Wrap(text string, width, rows int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case term.Width(line)+1+term.Width(word) <= width || len(out) == rows-1:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
