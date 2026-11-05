package editor

import (
	"fmt"
	"strconv"
	"strings"

	core "tuidesk/internal/editor"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

func (a *App) buildCommands() {
	has := func() bool { return a.Doc() != nil }
	editable := func() bool { d := a.Doc(); return d != nil && !d.ReadOnly }
	ed := func(fn func(d *Doc)) func() {
		return func() {
			if d := a.Doc(); d != nil {
				fn(d)
				a.scrollToCursor()
			}
		}
	}
	c := func(id, menu, title, hint string, keys []string, en func() bool, run func()) *ui.Command {
		return &ui.Command{ID: id, Menu: menu, Title: title, Hint: hint, Keys: keys, Enabled: en, Run: run}
	}
	sep := func(menu string) *ui.Command { return &ui.Command{Menu: menu, Title: "-"} }

	a.cmds.Add(
		// File
		c("save", "File", "Save", "Save", []string{"Ctrl+S"}, editable, func() { a.save(false, nil) }),
		c("saveas", "File", "Save as...", "", nil, has, func() { a.save(true, nil) }),
		c("open", "File", "Open file...", "", []string{"Ctrl+O"}, nil, a.promptOpen),
		c("new", "File", "New file", "", nil, nil, a.NewDoc),
		c("reload", "File", "Reload from disk", "", nil, has, a.reload),
		sep("File"),
		c("nexttab", "File", "Next tab", "", []string{"Ctrl+PgDn"}, has, func() { a.active = (a.active + 1) % len(a.docs) }),
		c("prevtab", "File", "Previous tab", "", []string{"Ctrl+PgUp"}, has, func() { a.active = (a.active + len(a.docs) - 1) % len(a.docs) }),
		c("close", "File", "Close tab", "Close", []string{"Ctrl+Q"}, has, func() { a.closeTab(a.active) }),

		// Edit
		c("undo", "Edit", "Undo", "Undo", []string{"Ctrl+Z"}, editable, ed(func(d *Doc) { d.Ed.Undo() })),
		c("redo", "Edit", "Redo", "", []string{"Ctrl+Y"}, editable, ed(func(d *Doc) { d.Ed.Redo() })),
		sep("Edit"),
		c("cut", "Edit", "Cut", "", []string{"Ctrl+X"}, editable, ed(func(d *Doc) { a.h.SetClipboardText(d.Ed.Cut()) })),
		c("copy", "Edit", "Copy", "", []string{"Ctrl+C"}, has, ed(func(d *Doc) {
			a.h.SetClipboardText(d.Ed.Copy())
			a.h.Notify("Copied", false)
		})),
		c("paste", "Edit", "Paste", "", []string{"Ctrl+V"}, func() bool { return editable() && a.h.ClipboardText() != "" },
			ed(func(d *Doc) { d.Ed.Insert(a.h.ClipboardText()) })),
		sep("Edit"),
		c("selectall", "Edit", "Select all", "", []string{"Ctrl+A"}, has, ed(func(d *Doc) { d.Ed.SelectAll() })),
		c("mark", "Edit", "Start / stop selecting", "", []string{"Ctrl+Space"}, has, ed(func(d *Doc) {
			if d.Ed.Mark {
				d.Ed.Mark = false
				return
			}
			p := d.Ed.Cur
			d.Ed.Anchor, d.Ed.Mark = &p, true
		})),
		c("indent", "Edit", "Indent", "", nil, editable, ed(func(d *Doc) { d.Ed.Tab() })),
		c("unindent", "Edit", "Unindent", "", nil, editable, ed(func(d *Doc) { d.Ed.Untab() })),
		c("comment", "Edit", "Toggle comment", "", []string{"Ctrl+/"}, func() bool { d := a.Doc(); return editable() && d.Lang.Comment != "" },
			ed(func(d *Doc) { toggleComment(d) })),
		sep("Edit"),
		c("snippet", "Edit", "Insert snippet...", "", []string{"Ctrl+T"}, editable, a.snippetPalette),

		// Search
		c("find", "Search", "Find...", "Find", []string{"Ctrl+F"}, has, func() { a.openBar(barFind) }),
		c("findnext", "Search", "Find next", "", []string{"F3"}, has, func() { a.findNext(false) }),
		c("findprev", "Search", "Find previous", "", []string{"Shift+F3"}, has, func() { a.findNext(true) }),
		c("replace", "Search", "Replace...", "", []string{"Ctrl+R"}, editable, func() { a.openBar(barReplace) }),
		c("gotoline", "Search", "Go to line...", "Go to line", []string{"Ctrl+G"}, has, func() { a.openBar(barGoto) }),

		// View
		c("tabsize", "View", "Tab size / spaces...", "", nil, has, a.promptIndent),
		c("palette", "View", "Command search...", "Commands", []string{"Ctrl+K"}, nil, func() { a.h.ShowPalette(&a.cmds) }),
	)
	a.cmds.ByID("palette").HintPinned = true
	// Movement is listed for the command search and the help, with the
	// keys that reach the server on each keyboard (ADR 0003 §2).
	move := []struct{ title, pc, mac string }{
		{"Go to start of document", "Ctrl+Home", "Alt+Up"},
		{"Go to end of document", "Ctrl+End", "Alt+Down"},
		{"Previous word", "Ctrl+Left", "Alt+Left"},
		{"Next word", "Ctrl+Right", "Alt+Right"},
	}
	for _, m := range move {
		m := m
		a.cmds.Add(&ui.Command{ID: "help:" + m.title, Title: m.title, Keys: []string{m.pc}, MacKey: m.mac, Enabled: has, Run: func() {
			if d := a.Doc(); d != nil {
				a.handleEditKey(d, keyFromName(m.pc))
				a.scrollToCursor()
			}
		}})
	}
}

// keyFromName builds a Key from a binding like "Ctrl+Home".
func keyFromName(b string) term.Key {
	parts := strings.Split(b, "+")
	k := term.Key{Name: parts[len(parts)-1]}
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "Ctrl":
			k.Mod |= term.ModCtrl
		case "Alt":
			k.Mod |= term.ModAlt
		case "Shift":
			k.Mod |= term.ModShift
		}
	}
	return k
}

func toggleComment(d *Doc) {
	e := d.Ed
	mark := d.Lang.Comment + " "
	a, z, ok := e.Selection()
	if !ok {
		a, z = e.Cur, e.Cur
	}
	last := z.Line
	if ok && z.Col == 0 && z.Line > a.Line {
		last--
	}
	all := true
	for l := a.Line; l <= last; l++ {
		t := strings.TrimLeft(string(e.Lines[l]), " \t")
		if t != "" && !strings.HasPrefix(t, d.Lang.Comment) {
			all = false
		}
	}
	text := make([]string, 0, last-a.Line+1)
	for l := a.Line; l <= last; l++ {
		line := string(e.Lines[l])
		ind := len(line) - len(strings.TrimLeft(line, " \t"))
		switch {
		case strings.TrimSpace(line) == "":
		case all:
			rest := strings.TrimPrefix(line[ind:], d.Lang.Comment)
			line = line[:ind] + strings.TrimPrefix(rest, " ")
		default:
			line = line[:ind] + mark + line[ind:]
		}
		text = append(text, line)
	}
	e.Select(posOf(a.Line, 0), posOf(last, len(e.Lines[last])))
	e.Insert(strings.Join(text, "\n"))
}

// ---- find / replace / go to line bar ----

func (a *App) openBar(m barMode) {
	a.bar, a.barFocus, a.findMsg = m, 0, ""
	if d := a.Doc(); d != nil && m != barGoto {
		if s := d.Ed.SelectedText(); s != "" && !strings.Contains(s, "\n") {
			a.findIn.SetValue(s)
		}
	}
	if m == barGoto {
		a.gotoIn.SetValue("")
	}
}

func (a *App) barInput() *ui.Input {
	switch {
	case a.bar == barGoto:
		return a.gotoIn
	case a.bar == barReplace && a.barFocus == 1:
		return a.replIn
	}
	return a.findIn
}

func (a *App) findNext(backward bool) {
	d := a.Doc()
	if d == nil {
		return
	}
	q := a.findIn.Value()
	if q == "" {
		a.openBar(barFind)
		return
	}
	if d.Ed.Find(q, backward, a.matchCase) {
		a.findMsg = ""
	} else {
		a.findMsg = "not found"
	}
	a.scrollToCursor()
}

// handleBarKey returns true when the bar consumed the key.
func (a *App) handleBarKey(k term.Key) bool {
	d := a.Doc()
	name := k.String()
	switch name {
	case "Esc":
		a.bar = barNone
		return true
	case "Tab", "Shift+Tab":
		if a.bar == barReplace {
			a.barFocus = 1 - a.barFocus
		}
		return true
	case "Down", "F3":
		if a.bar != barGoto {
			a.findNext(false)
		}
		return true
	case "Up", "Shift+F3":
		if a.bar != barGoto {
			a.findNext(true)
		}
		return true
	case "Alt+C":
		a.matchCase = !a.matchCase
		return true
	case "Alt+A", "Ctrl+A":
		if a.bar == barReplace && d != nil {
			n := d.Ed.ReplaceAll(a.findIn.Value(), a.replIn.Value(), a.matchCase)
			a.findMsg = fmt.Sprintf("replaced %d", n)
			a.scrollToCursor()
			return true
		}
	case "Enter":
		switch {
		case d == nil:
		case a.bar == barGoto:
			n, err := strconv.Atoi(strings.TrimSpace(a.gotoIn.Value()))
			if err != nil || n < 1 {
				a.findMsg = "enter a line number"
				return true
			}
			d.Ed.GotoLine(min(n, len(d.Ed.Lines)))
			a.bar = barNone
			a.scrollToCursor()
		case a.bar == barReplace && a.barFocus == 1:
			if !d.Ed.ReplaceSelection(a.findIn.Value(), a.replIn.Value(), a.matchCase) {
				a.findMsg = "no more matches"
			}
			a.scrollToCursor()
		default:
			a.findNext(false)
		}
		return true
	}
	if a.chrome.Cmds.ForKey(k) != nil && k.Mod&term.ModCtrl != 0 {
		return false // let Ctrl+S etc. through
	}
	in := a.barInput()
	if in.HandleKey(k) != ui.InputIgnored {
		if a.bar == barFind && d != nil { // search as you type
			d.Ed.SetCursor(func() posT { s, _, _ := d.Ed.Selection(); return s }(), false)
			a.findNext(false)
		}
		return true
	}
	return false
}

func (a *App) drawBar(row wm.Surface) {
	row.Clear(wm.StyleBar)
	w, _ := row.Size()
	switch a.bar {
	case barGoto:
		row.Text(1, 0, "Go to line:", wm.StyleBar)
		a.gotoIn.Draw(row.Sub(wm.Rect{X: 13, Y: 0, W: 10, H: 1}), true)
	default:
		row.Text(1, 0, "Find:", wm.StyleBar)
		fw := min(30, (w-30)/2)
		a.findIn.Draw(row.Sub(wm.Rect{X: 7, Y: 0, W: fw, H: 1}), a.barFocus == 0)
		x := 8 + fw
		if a.bar == barReplace {
			row.Text(x, 0, "Replace:", wm.StyleBar)
			a.replIn.Draw(row.Sub(wm.Rect{X: x + 9, Y: 0, W: fw, H: 1}), a.barFocus == 1)
			x += 10 + fw
		}
		hint := "↓/↑ next/prev"
		if a.bar == barReplace {
			hint = "↩ replace · " + term.KeyLabel("Ctrl+A") + " all"
		}
		cs := "Aa off"
		if a.matchCase {
			cs = "Aa on"
		}
		x = row.Text(x+1, 0, hint+" · "+term.KeyLabel("Alt+C")+" "+cs, wm.StyleBar)
	}
	if a.findMsg != "" {
		row.Text(w-term.Width(a.findMsg)-2, 0, a.findMsg, wm.StyleBarActive)
	}
}

type posT = core.Pos

func posOf(l, c int) core.Pos { return core.Pos{Line: l, Col: c} }
