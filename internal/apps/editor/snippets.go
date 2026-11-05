package editor

import (
	"path/filepath"
	"strings"

	"tuidesk/internal/config"
	"tuidesk/internal/snippets"
	"tuidesk/internal/ui"
)

// session tracks an expanded snippet's tab stops as document offsets.
type session struct {
	stops []snippets.Stop // absolute rune offsets
	idx   int
	doc   *Doc
}

var snippetSet *snippets.Set

func loadSnippets() *snippets.Set {
	if snippetSet == nil {
		snippetSet = snippets.Load(filepath.Join(filepath.Dir(config.Path()), "snippets"))
	}
	return snippetSet
}

// expandSnippet replaces the trigger word before the cursor (if any) with sn.
func (a *App) expandSnippet(d *Doc, sn snippets.Snippet, trigger string) {
	e := d.Ed
	if trigger != "" {
		e.Select(posOf(e.Cur.Line, e.Cur.Col-len([]rune(trigger))), e.Cur)
	}
	// Indent continuation lines like the line the snippet starts on.
	line := string(e.Lines[e.Cur.Line])
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	body := sn.Body
	if !e.ExpandTabs {
		body = strings.ReplaceAll(body, strings.Repeat(" ", e.TabSize), "\t")
	}
	text, stops := snippets.Expand(body)
	text = strings.ReplaceAll(text, "\n", "\n"+indent)
	// Offsets shift by the indent added on every earlier line.
	rs := []rune(snippets.ExpandPlain(body))
	a0, _, _ := e.Selection()
	base := e.Offset(a0)
	for i := range stops {
		before := strings.Count(string(rs[:stops[i].Start]), "\n") * len([]rune(indent))
		inside := strings.Count(string(rs[stops[i].Start:stops[i].End]), "\n") * len([]rune(indent))
		stops[i].Start += base + before
		stops[i].End += base + before + inside
	}
	e.Insert(text)
	a.snip = &session{stops: stops, doc: d}
	a.gotoStop(0)
}

func (a *App) gotoStop(i int) {
	s := a.snip
	if s == nil || i < 0 || i >= len(s.stops) {
		return
	}
	s.idx = i
	e := s.doc.Ed
	st := s.stops[i]
	e.Select(e.PosAt(st.Start), e.PosAt(st.End))
	if st.N == 0 { // final position: the session is over
		e.SetCursor(e.PosAt(st.Start), false)
		a.snip = nil
	}
}

// trackEdit shifts the remaining stops after an edit inside the session.
func (a *App) trackEdit(before, after int) {
	s := a.snip
	if s == nil || before == after {
		return
	}
	delta := after - before
	cur := &s.stops[s.idx]
	cur.End += delta
	for i := s.idx + 1; i < len(s.stops); i++ {
		if s.stops[i].Start >= cur.Start {
			s.stops[i].Start += delta
			s.stops[i].End += delta
		}
	}
}

// snippetTab handles Tab/Shift+Tab for snippets; false = not handled.
func (a *App) snippetTab(d *Doc, back bool) bool {
	if a.snip != nil && a.snip.doc == d {
		if back {
			a.gotoStop(a.snip.idx - 1)
		} else {
			a.gotoStop(a.snip.idx + 1)
		}
		return true
	}
	if back {
		return false
	}
	if _, _, sel := d.Ed.Selection(); sel {
		return false
	}
	word := d.Ed.WordBefore()
	if word == "" {
		return false
	}
	sn, ok := loadSnippets().Find(d.Lang.Key, word)
	if !ok {
		return false
	}
	a.expandSnippet(d, sn, word)
	return true
}

// snippetPalette lists snippets for the current language (Ctrl+T).
func (a *App) snippetPalette() {
	d := a.Doc()
	if d == nil {
		return
	}
	var cmds ui.Commands
	for _, sn := range loadSnippets().For(d.Lang.Key) {
		sn := sn
		cmds.Add(&ui.Command{ID: "snip:" + sn.Trigger, Title: sn.Trigger + " - " + sn.Title, Run: func() {
			a.expandSnippet(d, sn, "")
			a.scrollToCursor()
		}})
	}
	if len(cmds.All()) == 0 {
		a.h.Notify("No snippets for "+d.Lang.Name, true)
		return
	}
	a.h.ShowPalette(&cmds)
}
