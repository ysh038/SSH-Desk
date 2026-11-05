// Package editor is the text editor app (SPEC 7.3, ADR 0003): tabs, menu
// bar, find/replace bar, mouse selection, syntax colors, sudo save and
// on-save validators, on top of the internal/editor core.
package editor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	core "tuidesk/internal/editor"
	"tuidesk/internal/host"
	"tuidesk/internal/syntax"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// MaxEditBytes: larger files open read-only in the viewer (SPEC 7.3).
const MaxEditBytes = 50 << 20

// Doc is one open file.
type Doc struct {
	Path     string
	Ed       *core.Editor
	Lang     *syntax.Lang
	ReadOnly bool
	ModTime  time.Time
	Size     int64
	Mode     os.FileMode
	Sudo     bool // last save needed sudo
	top      int  // first visible line
	left     int  // first visible display column
	scrolled bool // the wheel moved the view: keep it until the cursor moves
}

// Name is the file name shown on the tab.
func (d *Doc) Name() string {
	if d.Path == "" {
		return "untitled"
	}
	return filepath.Base(d.Path)
}

type barMode int

const (
	barNone barMode = iota
	barFind
	barReplace
	barGoto
)

// App is an editor window with tabs.
type App struct {
	h      host.Host
	docs   []*Doc
	active int
	cmds   ui.Commands
	chrome *host.Chrome

	bar       barMode
	findIn    *ui.Input
	replIn    *ui.Input
	gotoIn    *ui.Input
	barFocus  int // 0 find/goto field, 1 replace field
	matchCase bool
	findMsg   string

	// layout from the last Draw
	vw, vh   int
	gutter   int
	tabs     []tabHit
	dragging bool // selecting with the mouse
	// Dragging the selected text to move it (copy with Ctrl or Alt held).
	moving   bool     // pressed inside the selection
	moveDrag bool     // and the mouse moved: dropAt is the target
	dropAt   core.Pos // where the text would land
	runSave  saveRunner
	snip     *session
}

type tabHit struct {
	x0, x1 int
	close  int // column of the close mark
	doc    int
}

// New returns an empty editor window; use Open to add files.
func New(h host.Host) *App {
	a := &App{h: h, findIn: ui.NewInput(""), replIn: ui.NewInput(""), gotoIn: ui.NewInput("")}
	a.runSave = execRunner
	a.buildCommands()
	a.chrome = host.NewChrome(h, a, &a.cmds)
	return a
}

// SetRunner replaces how sudo and check commands run (tests).
func (a *App) SetRunner(r func(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)) {
	a.runSave = r
}

// Open loads path into a new tab (or focuses it if already open).
func (a *App) Open(path string) error {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	for i, d := range a.docs {
		if d.Path == path {
			a.active = i
			return nil
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > MaxEditBytes {
		return fmt.Errorf("%s is larger than %s; open it in the viewer instead", filepath.Base(path), ui.HumanSize(MaxEditBytes))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cfg := a.h.Config()
	d := &Doc{Path: path, Ed: core.New(string(data)), ModTime: info.ModTime(), Size: info.Size(), Mode: info.Mode()}
	d.Ed.TabSize, d.Ed.ExpandTabs = cfg.TabSize, cfg.ExpandTabs
	applyEditorConfig(d)
	first := ""
	if len(d.Ed.Lines) > 0 {
		first = string(d.Ed.Lines[0])
	}
	d.Lang = syntax.Detect(path, first)
	d.Ed.ColonIndent = d.Lang.ColonIndent
	if f, err := os.OpenFile(path, os.O_WRONLY, 0); err != nil {
		d.Sudo = true // will need sudo to save
	} else {
		f.Close()
	}
	a.docs = append(a.docs, d)
	a.active = len(a.docs) - 1
	return nil
}

// NewDoc opens an empty untitled tab.
func (a *App) NewDoc() {
	d := &Doc{Ed: core.New(""), Lang: syntax.ByName("text")}
	cfg := a.h.Config()
	d.Ed.TabSize, d.Ed.ExpandTabs = cfg.TabSize, cfg.ExpandTabs
	a.docs = append(a.docs, d)
	a.active = len(a.docs) - 1
}

// Doc returns the active document (nil if none).
func (a *App) Doc() *Doc {
	if a.active < 0 || a.active >= len(a.docs) {
		return nil
	}
	return a.docs[a.active]
}

func (a *App) Title() string {
	d := a.Doc()
	if d == nil {
		return "Editor"
	}
	mod := ""
	if d.Ed.Modified() {
		mod = " *"
	}
	return "Edit - " + a.display(d.Name()) + mod
}

func (a *App) display(s string) string {
	if a.h.Console() {
		return term.EscapeNonASCII(s)
	}
	return s
}

func (a *App) MinSize() (int, int) { return 30, 8 }
func (a *App) Close() error        { return nil }

// Unsaved counts modified documents (host.Closer).
func (a *App) Unsaved() int {
	n := 0
	for _, d := range a.docs {
		if d.Ed.Modified() {
			n++
		}
	}
	return n
}

// RequestClose asks about unsaved documents before the window closes.
func (a *App) RequestClose(proceed func()) {
	n := a.Unsaved()
	if n == 0 {
		proceed()
		return
	}
	a.h.Confirm("Unsaved changes", fmt.Sprintf("%d file(s) have unsaved changes.\nClose anyway and lose them?", n),
		[]string{"Discard and close", "Cancel"}, func(c int) {
			if c == 0 {
				proceed()
			}
		})
}

// closeTab closes tab i, asking first if it has unsaved changes.
func (a *App) closeTab(i int) {
	if i < 0 || i >= len(a.docs) {
		return
	}
	do := func() {
		a.docs = append(a.docs[:i], a.docs[i+1:]...)
		if a.active >= len(a.docs) {
			a.active = len(a.docs) - 1
		}
		if len(a.docs) == 0 {
			a.h.CloseApp(a)
		}
	}
	d := a.docs[i]
	if !d.Ed.Modified() {
		do()
		return
	}
	a.h.Confirm("Unsaved changes", a.display(d.Name())+" has unsaved changes.", []string{"Save", "Discard", "Cancel"}, func(c int) {
		switch c {
		case 0:
			a.active = i
			a.save(false, do)
		case 1:
			do()
		}
	})
}

// ---- layout ----

func (a *App) textTop() int { return 2 } // menu bar, tab bar
func (a *App) barRows() int {
	if a.bar == barNone {
		return 0
	}
	return 1
}
func (a *App) textH() int {
	return max(a.vh-a.textTop()-1-a.barRows()-a.chrome.HintRows(), 1)
}
func (a *App) textW() int { return max(a.vw-a.gutter, 1) }

// scrollToCursor keeps the cursor inside the view.
func (a *App) scrollToCursor() {
	d := a.Doc()
	if d == nil {
		return
	}
	d.scrolled = false
	h, w := a.textH(), a.textW()
	if d.Ed.Cur.Line < d.top {
		d.top = d.Ed.Cur.Line
	}
	if d.Ed.Cur.Line >= d.top+h {
		d.top = d.Ed.Cur.Line - h + 1
	}
	x := d.Ed.DisplayX(d.Ed.Cur)
	if x < d.left {
		d.left = x
	}
	if x >= d.left+w-1 {
		d.left = x - w + 2
	}
}

// posAt maps local (x, y) in the text area to a document position.
func (a *App) posAt(x, y int) (core.Pos, bool) {
	d := a.Doc()
	if d == nil {
		return core.Pos{}, false
	}
	line := d.top + y - a.textTop()
	if line < 0 {
		line = 0
	}
	if line >= len(d.Ed.Lines) {
		line = len(d.Ed.Lines) - 1
		return core.Pos{Line: line, Col: len(d.Ed.Lines[line])}, true
	}
	col := d.Ed.ColAtX(line, max(x-a.gutter, 0)+d.left)
	return core.Pos{Line: line, Col: col}, true
}

// ---- events ----

func (a *App) HandleEvent(ev term.Event) bool {
	d := a.Doc()
	switch ev := ev.(type) {
	case term.KeyEvent:
		if a.bar != barNone && a.handleBarKey(ev.Key) {
			return true
		}
		if a.chrome.HandleKey(ev.Key) {
			a.scrollToCursor()
			return true
		}
		if d == nil {
			return false
		}
		before, sess := d.Ed.Len(), a.snip
		handled := a.handleEditKey(d, ev.Key)
		if a.snip != nil && a.snip == sess { // a new expansion already has exact stops
			a.trackEdit(before, d.Ed.Len())
		}
		a.scrollToCursor()
		return handled
	case term.PasteEvent:
		if a.bar != barNone {
			a.barInput().Paste(ev.Text)
			return true
		}
		if d != nil && !d.ReadOnly {
			d.Ed.Insert(ev.Text) // Cmd+V on a Mac keyboard
			a.scrollToCursor()
			return true
		}
	case term.MouseEvent:
		return a.handleMouse(ev.Gesture)
	}
	return false
}

func (a *App) handleEditKey(d *Doc, k term.Key) bool {
	e := d.Ed
	e.WordEnds = a.h.Config().Keyboard == "mac" // Option+→ stops at the word end
	page := a.textH()
	name := k.String()
	shift := k.Mod&term.ModShift != 0
	base := strings.TrimPrefix(name, "Shift+")
	moves := map[string]core.Move{
		"Left": core.Left, "Right": core.Right, "Up": core.Up, "Down": core.Down,
		"Home": core.LineStart, "End": core.LineEnd, "PgUp": core.PageUp, "PgDn": core.PageDown,
		"Ctrl+Left": core.WordLeft, "Ctrl+Right": core.WordRight, "Alt+Left": core.WordLeft, "Alt+Right": core.WordRight,
		"Alt+B": core.WordLeft, "Alt+F": core.WordRight,
		"Ctrl+Home": core.DocStart, "Ctrl+End": core.DocEnd, "Alt+Up": core.DocStart, "Alt+Down": core.DocEnd,
	}
	// "Ctrl+Shift+Left" → base "Ctrl+Left"
	if strings.Contains(name, "+Shift+") {
		base = strings.Replace(name, "Shift+", "", 1)
		shift = true
	}
	if m, ok := moves[base]; ok {
		e.MoveCursor(m, shift, page)
		return true
	}
	if d.ReadOnly {
		return false
	}
	switch name {
	case "Enter":
		e.Newline()
	case "Backspace":
		e.Backspace()
	case "Delete":
		e.DeleteForward()
	case "Tab":
		if !a.snippetTab(d, false) {
			e.Tab()
		}
	case "Shift+Tab":
		if !a.snippetTab(d, true) {
			e.Untab()
		}
	case "Esc":
		a.snip = nil
		e.ClearSelection()
	default:
		if k.IsRune() && k.Mod&(term.ModCtrl|term.ModAlt|term.ModMeta) == 0 {
			e.Insert(string(k.Rune))
			return true
		}
		return false
	}
	return true
}

func (a *App) handleMouse(g term.Gesture) bool {
	d := a.Doc()
	switch {
	case g.Y == 0 && g.Kind == term.GestureClick:
		a.chrome.ClickBar(g.X)
		return true
	case g.Y == 1 && g.Kind == term.GestureClick:
		for _, t := range a.tabs {
			if g.X >= t.x0 && g.X < t.x1 {
				if g.X == t.close {
					a.closeTab(t.doc)
				} else {
					a.active = t.doc
				}
			}
		}
		return true
	case d == nil:
		return false
	}
	if g.Kind == term.GestureWheel {
		switch g.Wheel {
		case term.WheelUp:
			d.top = max(d.top-3, 0)
		case term.WheelDown:
			d.top = min(d.top+3, max(len(d.Ed.Lines)-a.textH(), 0))
		}
		d.scrolled = true // Draw must not pull the view back to the cursor
		return true
	}
	if g.Y < a.textTop() || g.Y >= a.textTop()+a.textH() {
		if g.Kind == term.GestureDrag && (a.dragging || a.moving) { // auto-scroll while selecting or moving
			if g.Y < a.textTop() {
				d.top = max(d.top-1, 0)
			} else {
				d.top = min(d.top+1, max(len(d.Ed.Lines)-1, 0))
			}
		} else if a.moving && (g.Kind == term.GestureRelease || g.Kind == term.GestureDragEnd) {
			a.drop(d, a.dropAt, g.Mod) // released over the bars: drop at the last spot
			return true
		} else if g.Kind != term.GestureDrag {
			a.dragging = a.dragging && g.Kind != term.GestureRelease && g.Kind != term.GestureDragEnd
			return false
		}
	}
	p, ok := a.posAt(g.X, g.Y)
	if !ok {
		return false
	}
	switch g.Kind {
	case term.GesturePress:
		if g.Button == term.ButtonLeft {
			if sa, sz, sel := d.Ed.Selection(); sel && !d.ReadOnly && g.Mod&term.ModShift == 0 && !p.Less(sa) && p.Less(sz) {
				a.moving, a.moveDrag = true, false // maybe a drag of the selected text
				return true
			}
			d.Ed.SetCursor(p, g.Mod&term.ModShift != 0)
			d.scrolled = false
			a.dragging = true
		}
	case term.GestureDragStart, term.GestureDrag:
		switch {
		case a.moving:
			a.moveDrag, a.dropAt = true, p
			d.scrolled = true // the cursor stays on the selection while dropping elsewhere
		case a.dragging:
			d.Ed.SetCursor(p, true)
		}
	case term.GestureRelease, term.GestureDragEnd:
		if a.moving {
			a.drop(d, p, g.Mod)
		}
		a.dragging = false
	case term.GestureClick:
		switch {
		case g.Button == term.ButtonRight:
			if _, _, sel := d.Ed.Selection(); !sel {
				d.Ed.SetCursor(p, false)
			}
			a.contextMenu(g.X, g.Y)
		case g.Count == 2:
			s, e := d.Ed.WordAt(p)
			d.Ed.Select(s, e)
		case g.Count == 3:
			d.Ed.SelectLine(p.Line)
		}
	}
	return true
}

// drop finishes a press inside the selection: a drag moves (or with Ctrl or
// Alt copies) the selected text to p; a plain click places the cursor.
func (a *App) drop(d *Doc, p core.Pos, mod term.Mod) {
	moved := a.moveDrag
	a.moving, a.moveDrag = false, false
	sa, sz, sel := d.Ed.Selection()
	copy := mod&(term.ModCtrl|term.ModAlt|term.ModMeta) != 0
	if !moved || !sel || !d.Ed.MoveText(sa, sz, p, copy) {
		d.Ed.SetCursor(p, false)
	}
	a.scrollToCursor()
}

func (a *App) contextMenu(x, y int) {
	items := []ui.MenuItem{}
	for _, id := range []string{"cut", "copy", "paste", "-", "selectall", "-", "snippet", "find", "gotoline"} {
		if id == "-" {
			items = append(items, ui.MenuItem{Separator: true})
			continue
		}
		c := a.cmds.ByID(id)
		items = append(items, ui.MenuItem{Label: c.Title, Shortcut: c.Shortcut(), Action: id, Disabled: !c.On()})
	}
	a.h.OpenMenu(a, x, y, items, func(id string) { a.cmds.Run(id); a.scrollToCursor() })
}

// ---- drawing ----

var synStyle = map[syntax.Kind]wm.StyleID{
	syntax.Comment: wm.StyleSynComment, syntax.String: wm.StyleSynString, syntax.Keyword: wm.StyleSynKeyword,
	syntax.Number: wm.StyleSynNumber, syntax.Var: wm.StyleSynVar, syntax.Key: wm.StyleSynKey, syntax.Header: wm.StyleSynHeader,
}

func (a *App) Draw(s wm.Surface, focused bool) {
	a.vw, a.vh = s.Size()
	a.chrome.DrawBar(s)
	a.drawTabs(s)
	d := a.Doc()
	if d == nil {
		s.Text(2, 3, "No file open. "+term.KeyLabel("Ctrl+O")+" to open a file.", wm.StyleBodyDim)
		a.chrome.DrawHints(s)
		return
	}
	a.gutter = len(strconv.Itoa(len(d.Ed.Lines))) + 2
	if d.scrolled {
		d.top = min(max(d.top, 0), max(len(d.Ed.Lines)-1, 0))
	} else {
		a.scrollToCursor()
	}
	a.drawText(s, d, focused)
	if a.bar != barNone {
		a.drawBar(s.Sub(wm.Rect{X: 0, Y: a.vh - 2 - a.chrome.HintRows(), W: a.vw, H: 1}))
	}
	a.drawStatus(s.Sub(wm.Rect{X: 0, Y: a.vh - 1 - a.chrome.HintRows(), W: a.vw, H: 1}), d)
	a.chrome.DrawHints(s)
}

func (a *App) drawTabs(s wm.Surface) {
	row := s.Sub(wm.Rect{X: 0, Y: 1, W: a.vw, H: 1})
	row.Clear(wm.StyleBar)
	a.tabs = a.tabs[:0]
	x := 0
	for i, d := range a.docs {
		name := a.display(d.Name())
		if d.Ed.Modified() {
			name += "*"
		}
		label := " " + name + " x "
		st := wm.StyleBar
		if i == a.active {
			st = wm.StyleBarActive
		}
		w := term.Width(label)
		if x+w > a.vw {
			break
		}
		row.Text(x, 0, label, st)
		a.tabs = append(a.tabs, tabHit{x0: x, x1: x + w, close: x + w - 2, doc: i})
		x += w + 1
	}
}

func (a *App) drawText(s wm.Surface, d *Doc, focused bool) {
	e := d.Ed
	h := a.textH()
	selA, selZ, hasSel := e.Selection()
	var brA, brB core.Pos
	hasBr := false
	if focused && !hasSel {
		brA, brB, hasBr = e.MatchBracket()
	}
	for y := 0; y < h; y++ {
		ln := d.top + y
		row := s.Sub(wm.Rect{X: 0, Y: a.textTop() + y, W: a.vw, H: 1})
		if ln >= len(e.Lines) {
			row.Clear(wm.StyleBody)
			continue
		}
		base := wm.StyleBody
		if ln == e.Cur.Line {
			base = wm.StyleCurLine
		}
		row.Clear(base)
		num := strconv.Itoa(ln + 1)
		row.Text(a.gutter-1-len(num), 0, num, wm.StyleGutter)

		line := e.Lines[ln]
		kinds := make([]syntax.Kind, len(line))
		for _, sp := range d.Lang.Line(string(line)) {
			for i := sp.Start; i < sp.End && i < len(kinds); i++ {
				kinds[i] = sp.Kind
			}
		}
		text := row.Sub(wm.Rect{X: a.gutter, Y: 0, W: a.textW(), H: 1})
		x := 0
		for i, r := range line {
			w := e.RuneW(r, x)
			sx := x - d.left
			st := base
			if k := kinds[i]; k != syntax.Plain {
				st = synStyle[k]
			}
			p := core.Pos{Line: ln, Col: i}
			if hasSel && !p.Less(selA) && p.Less(selZ) {
				st = wm.StyleSelection
			}
			if hasBr && (p == brA || p == brB) {
				st = wm.StyleBracket
			}
			if focused && p == e.Cur && !a.moveDrag {
				st = wm.StyleInputCursor
			}
			if a.moveDrag && p == a.dropAt {
				st = wm.StyleInputCursor // where the dragged text will land
			}
			if sx+w > 0 && sx < a.textW() {
				if r == '\t' {
					for j := 0; j < w; j++ {
						text.Set(sx+j, 0, ' ', st)
					}
				} else if sx >= 0 {
					text.Set(sx, 0, r, st)
				}
			}
			x += w
		}
		if hasSel && ln >= selA.Line && ln < selZ.Line { // selected newline
			text.Set(x-d.left, 0, ' ', wm.StyleSelection)
		}
		if (focused && !a.moveDrag && e.Cur.Line == ln && e.Cur.Col == len(line)) ||
			(a.moveDrag && a.dropAt.Line == ln && a.dropAt.Col == len(line)) {
			text.Set(x-d.left, 0, ' ', wm.StyleInputCursor)
		}
	}
}

func (a *App) drawStatus(row wm.Surface, d *Doc) {
	row.Clear(wm.StyleBodyDim)
	e := d.Ed
	var parts []string
	parts = append(parts, fmt.Sprintf("Ln %d, Col %d", e.Cur.Line+1, e.DisplayX(e.Cur)+1))
	if _, _, sel := e.Selection(); sel {
		parts = append(parts, fmt.Sprintf("%d chars selected", len([]rune(e.SelectedText()))))
	}
	if e.Mark {
		parts = append(parts, "SELECTING ("+term.KeyLabel("Ctrl+Space")+" to stop)")
	}
	parts = append(parts, d.Lang.Name)
	if e.CRLF {
		parts = append(parts, "CRLF")
	} else {
		parts = append(parts, "LF")
	}
	if e.ExpandTabs {
		parts = append(parts, fmt.Sprintf("Spaces:%d", e.TabSize))
	} else {
		parts = append(parts, fmt.Sprintf("Tab:%d", e.TabSize))
	}
	if d.ReadOnly {
		parts = append(parts, "READ-ONLY")
	} else if d.Sudo {
		parts = append(parts, "needs sudo to save")
	}
	if e.Modified() {
		parts = append(parts, "modified")
	}
	row.Text(1, 0, strings.Join(parts, " · "), wm.StyleBodyDim)
}
