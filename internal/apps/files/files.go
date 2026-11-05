// Package files is the file manager app (SPEC 7.2).
package files

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"tuidesk/internal/fsops"
	"tuidesk/internal/host"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// Layout rows inside the window.
const (
	rowBar    = 0
	rowPath   = 1
	rowHeader = 2
	rowList   = 3
)

type segment struct {
	x0, x1 int
	path   string
}

type column struct {
	key   fsops.SortKey
	x0, w int
}

// App is one file manager window.
type App struct {
	h          host.Host
	dir        string
	entries    []fsops.Entry
	view       []int // indices into entries after hidden/filter
	sortKey    fsops.SortKey
	reverse    bool
	showHidden bool
	filter     []rune
	icons      bool // icon view instead of the list (SPEC 7.2)

	cursor   int // index into view
	offset   int
	scrolled bool // the wheel moved the view: Draw keeps it until the cursor moves
	selected map[string]bool
	anchor   int

	loading bool
	loadErr error
	gen     int
	cancel  context.CancelFunc
	wantCur string // name to put the cursor on after the next load

	// interaction
	renaming   *ui.Input
	renameName string
	boxStart   int  // view index where a box selection started (-1: none)
	pressSel   bool // press landed on a selected row
	dragging   bool
	dropRow    int

	chrome *host.Chrome
	cmds   ui.Commands

	// layout from the last Draw
	vw, vh   int
	segments []segment
	cols     []column
	nameW    int
	toggleX  [2]int // path-row "List|Icons" switch: x of "List", x of "Icons" (0,0: hidden)

	// Back / Forward history; each place remembers cursor and scroll.
	back, fwd  []place
	wantOffset int // scroll position to restore after the next load (-1: none)
}

// place is a visited folder with the cursor and scroll position it had.
type place struct {
	dir, cursor string
	offset      int
}

const maxHistory = 100

// Path-row "<" and ">" buttons (Back, Forward); folder segments follow.
const (
	backX    = 1
	forwardX = 3
	pathX    = 6
)

// New opens a file manager on dir.
func New(h host.Host, dir string) *App {
	a := &App{h: h, selected: map[string]bool{}, boxStart: -1, dropRow: -1}
	a.buildCommands()
	a.chrome = host.NewChrome(h, a, &a.cmds)
	a.navigate(dir, "")
	return a
}

// Dir is the folder shown.
func (a *App) Dir() string { return a.dir }

// Loading reports a directory read in progress.
func (a *App) Loading() bool { return a.loading }

func (a *App) Title() string {
	name := a.dir
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(a.dir, home) {
		name = "~" + strings.TrimPrefix(a.dir, home)
	}
	return "Files - " + a.display(name)
}

func (a *App) MinSize() (int, int) { return 30, 6 }

func (a *App) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

func (a *App) display(s string) string {
	if a.h.Console() {
		return term.EscapeNonASCII(s)
	}
	return s
}

// ---- loading ----

func (a *App) navigate(dir, cursorName string) {
	abs, err := filepath.Abs(dir)
	if err == nil {
		dir = abs
	}
	a.dir = dir
	a.selected = map[string]bool{}
	a.filter = nil
	a.cursor, a.offset, a.anchor = 0, 0, 0
	a.renaming = nil
	a.wantCur = cursorName
	a.wantOffset = -1
	a.entries, a.view = nil, nil
	a.load()
}

// here is the current folder, cursor and scroll position.
func (a *App) here() place {
	p := place{dir: a.dir, offset: a.offset}
	if e, ok := a.cursorEntry(); ok {
		p.cursor = e.Name
	}
	return p
}

// goTo opens dir as a new step in the history (Back returns here).
func (a *App) goTo(dir, cursorName string) {
	if abs, err := filepath.Abs(dir); err == nil && abs == a.dir {
		a.navigate(dir, cursorName)
		return
	}
	a.back = append(a.back, a.here())
	if len(a.back) > maxHistory {
		a.back = a.back[1:]
	}
	a.fwd = nil
	a.navigate(dir, cursorName)
}

// goBack returns to the previous folder with its cursor and scroll position.
func (a *App) goBack() { a.travel(&a.back, &a.fwd) }

// goForward undoes a goBack.
func (a *App) goForward() { a.travel(&a.fwd, &a.back) }

func (a *App) travel(from, to *[]place) {
	if len(*from) == 0 {
		return
	}
	p := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, a.here())
	a.navigate(p.dir, p.cursor)
	a.wantOffset = p.offset
}

func (a *App) load() {
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.gen++
	gen, dir := a.gen, a.dir
	a.loading = true
	go func() {
		es, err := fsops.ReadDir(ctx, dir)
		if ctx.Err() != nil {
			return
		}
		a.h.Post(func() {
			if gen != a.gen {
				return
			}
			a.loading, a.loadErr = false, err
			a.setEntries(es)
		})
	}()
}

// Refresh reloads the folder keeping cursor and selection (host.Refresher).
func (a *App) Refresh() {
	if e, ok := a.cursorEntry(); ok && a.wantCur == "" {
		a.wantCur = e.Name
	}
	a.load()
}

func (a *App) setEntries(es []fsops.Entry) {
	if a.wantCur == "" {
		if e, ok := a.cursorEntry(); ok {
			a.wantCur = e.Name
		}
	}
	a.view = nil // indices refer to the old entries
	fsops.Sort(es, a.sortKey, a.reverse)
	a.entries = es
	present := map[string]bool{}
	for _, e := range es {
		present[e.Name] = true
	}
	for n := range a.selected {
		if !present[n] {
			delete(a.selected, n)
		}
	}
	a.rebuild()
	if a.wantCur != "" {
		a.moveCursorTo(a.wantCur)
		a.wantCur = ""
	}
	if a.wantOffset >= 0 { // Back/Forward: the scroll position the folder had
		n := a.ncols()
		a.offset = a.clampOffsetRow(a.wantOffset/n) * n
		a.wantOffset = -1
		if or, cr := a.offset/n, a.cursor/n; cr < or || cr >= or+a.visRows() {
			a.scrolled = true // the cursor was off screen (wheel): keep the view
		}
	}
}

func (a *App) rebuild() {
	var curName string
	if e, ok := a.cursorEntry(); ok {
		curName = e.Name
	}
	a.view = a.view[:0]
	f := strings.ToLower(string(a.filter))
	for i, e := range a.entries {
		if e.Hidden() && !a.showHidden {
			continue
		}
		if f != "" && !strings.Contains(strings.ToLower(e.Name), f) {
			continue
		}
		a.view = append(a.view, i)
	}
	a.cursor = 0
	if curName != "" {
		a.moveCursorTo(curName)
	}
	a.clampCursor()
}

func (a *App) moveCursorTo(name string) {
	for vi, i := range a.view {
		if a.entries[i].Name == name {
			a.cursor = vi
			a.ensureVisible()
			return
		}
	}
}

func (a *App) clampCursor() {
	a.cursor = min(max(a.cursor, 0), max(len(a.view)-1, 0))
	a.ensureVisible()
}

func (a *App) listH() int { return max(a.vh-rowList-1-a.chrome.HintRows(), 1) }

// ncols is the number of items per grid row (1 in the list view).
func (a *App) ncols() int {
	if !a.icons {
		return 1
	}
	return max((a.vw-1)/cellW, 1)
}

// itemH is the screen height of one grid row.
func (a *App) itemH() int {
	if a.icons {
		return cellH
	}
	return 1
}

// visRows is the number of grid rows that fit.
func (a *App) visRows() int { return max(a.listH()/a.itemH(), 1) }

// pageItems is how far PgUp/PgDn move.
func (a *App) pageItems() int { return max(a.visRows()-1, 1) * a.ncols() }

// ensureVisible scrolls so the cursor is shown. offset is the first visible
// item and always starts a grid row.
func (a *App) ensureVisible() {
	a.scrolled = false
	n, h := a.ncols(), a.visRows()
	or, cr := a.offset/n, a.cursor/n
	if cr < or {
		or = cr
	}
	if cr >= or+h {
		or = cr - h + 1
	}
	a.offset = a.clampOffsetRow(or) * n
}

func (a *App) clampOffsetRow(or int) int {
	n := a.ncols()
	rows := (len(a.view) + n - 1) / n
	return min(max(or, 0), max(rows-a.visRows(), 0))
}

func (a *App) scrollRows(d int) {
	n := a.ncols()
	a.offset = a.clampOffsetRow(a.offset/n+d) * n
}

// cellPos is the local position of item vi (top-left of its row or cell).
func (a *App) cellPos(vi int) (x, y int) {
	n := a.ncols()
	r, c := (vi-a.offset)/n, (vi-a.offset)%n
	if !a.icons {
		return 0, rowList + r
	}
	return c * cellW, rowList + r*cellH
}

func (a *App) setIcons(on bool) {
	a.icons = on
	a.ensureVisible()
}

func (a *App) entryAt(vi int) (fsops.Entry, bool) {
	if vi < 0 || vi >= len(a.view) {
		return fsops.Entry{}, false
	}
	return a.entries[a.view[vi]], true
}

func (a *App) cursorEntry() (fsops.Entry, bool) { return a.entryAt(a.cursor) }

// Selection returns the selected entries; with none selected, the cursor entry.
func (a *App) Selection() []fsops.Entry {
	var out []fsops.Entry
	for _, i := range a.view {
		if a.selected[a.entries[i].Name] {
			out = append(out, a.entries[i])
		}
	}
	if len(out) == 0 {
		if e, ok := a.cursorEntry(); ok {
			out = append(out, e)
		}
	}
	return out
}

func paths(es []fsops.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Path
	}
	return out
}

func (a *App) selectOnly(vi int) {
	a.selected = map[string]bool{}
	if e, ok := a.entryAt(vi); ok {
		a.selected[e.Name] = true
	}
}

func (a *App) selectRange(from, to int) {
	if from > to {
		from, to = to, from
	}
	a.selected = map[string]bool{}
	for vi := max(from, 0); vi <= to && vi < len(a.view); vi++ {
		a.selected[a.entries[a.view[vi]].Name] = true
	}
}

// ---- events ----

func (a *App) HandleEvent(ev term.Event) bool {
	if a.renaming != nil {
		return a.handleRename(ev)
	}
	switch ev := ev.(type) {
	case term.KeyEvent:
		return a.handleKey(ev.Key)
	case term.MouseEvent:
		return a.handleMouse(ev.Gesture)
	case term.PasteEvent:
		// The terminal pasted text (Cmd+V on a Mac keyboard, Win+V on a PC
		// keyboard attached to a Mac). In a file list that means "paste the
		// copied files".
		if len(a.h.Files().Paths) > 0 {
			a.paste()
			return true
		}
	}
	return false
}

func (a *App) moveCursor(delta int) {
	single := len(a.selected) <= 1
	a.cursor += delta
	a.clampCursor()
	if single {
		a.selectOnly(a.cursor)
	}
	a.anchor = a.cursor
}

// extend moves the cursor and selects the range from the anchor (Shift+arrows).
func (a *App) extend(delta int) {
	a.cursor += delta
	a.clampCursor()
	a.selectRange(a.anchor, a.cursor)
}

// toggle adds or removes one row from the selection (Ctrl/Option+click).
func (a *App) toggle(vi int) {
	e, ok := a.entryAt(vi)
	if !ok {
		return
	}
	if len(a.selected) == 0 {
		// The cursor row counts as selected until the first toggle.
		if c, ok := a.cursorEntry(); ok && c.Name != e.Name {
			a.selected[c.Name] = true
		}
	}
	if a.selected[e.Name] {
		delete(a.selected, e.Name)
	} else {
		a.selected[e.Name] = true
	}
	a.cursor, a.anchor = vi, vi
}

func (a *App) handleKey(k term.Key) bool {
	page := a.pageItems()
	n := a.ncols()
	switch k.String() {
	case "Up":
		a.moveCursor(-n)
	case "Down":
		a.moveCursor(n)
	case "Left":
		if !a.icons {
			return false
		}
		a.moveCursor(-1)
	case "Right":
		if !a.icons {
			return false
		}
		a.moveCursor(1)
	case "Shift+Left":
		if !a.icons {
			return false
		}
		a.extend(-1)
	case "Shift+Right":
		if !a.icons {
			return false
		}
		a.extend(1)
	case "PgUp":
		a.moveCursor(-page)
	case "PgDn":
		a.moveCursor(page)
	case "Home", "Ctrl+Home":
		a.moveCursor(-len(a.view))
	case "End", "Ctrl+End":
		a.moveCursor(len(a.view))
	case "Shift+Up":
		a.extend(-n)
	case "Shift+Down":
		a.extend(n)
	case "Shift+PgUp":
		a.extend(-page)
	case "Shift+PgDn":
		a.extend(page)
	case "Shift+Home":
		a.extend(-len(a.view))
	case "Shift+End":
		a.extend(len(a.view))
	case "Space":
		if e, ok := a.cursorEntry(); ok {
			a.selected[e.Name] = !a.selected[e.Name]
			if !a.selected[e.Name] {
				delete(a.selected, e.Name)
			}
			a.cursor++
			a.clampCursor()
		}
	case "Backspace":
		if len(a.filter) > 0 {
			a.filter = a.filter[:len(a.filter)-1]
			a.rebuild()
		} else {
			a.up()
		}
	case "Esc":
		switch {
		case len(a.filter) > 0:
			a.filter = nil
			a.rebuild()
		case len(a.selected) > 1:
			a.selectOnly(a.cursor)
		}
	case "Shift+F10":
		x, y := a.cellPos(a.cursor)
		a.contextMenu(x+2, y, true)
	default:
		if a.chrome.HandleKey(k) {
			return true
		}
		if k.IsRune() && k.Mod&(term.ModCtrl|term.ModAlt|term.ModMeta) == 0 && k.Rune > ' ' {
			a.filter = append(a.filter, k.Rune)
			a.rebuild()
			return true
		}
		return false
	}
	return true
}

// itemAt returns the item at local (x, y); inList reports that the point is
// in the list area (vi is -1 on empty space there).
func (a *App) itemAt(x, y int) (vi int, inList bool) {
	if y < rowList || y >= rowList+a.visRows()*a.itemH() {
		return -1, y >= rowList && y < rowList+a.listH()
	}
	c := 0
	if a.icons {
		c = x / cellW
		if x < 0 || c >= a.ncols() {
			return -1, true
		}
	}
	vi = a.offset + (y-rowList)/a.itemH()*a.ncols() + c
	if vi >= len(a.view) {
		return -1, true
	}
	return vi, true
}

// nearestItem is the item closest to (x, y) for box selection: clamped to
// the visible grid and the list length.
func (a *App) nearestItem(x, y int) int {
	n := a.ncols()
	r := min(max((y-rowList)/a.itemH(), 0), a.visRows()-1)
	if y < rowList {
		r = 0
	}
	c := 0
	if a.icons {
		c = min(max(x/cellW, 0), n-1)
	}
	return min(max(a.offset+r*n+c, 0), len(a.view)-1)
}

func (a *App) handleMouse(g term.Gesture) bool {
	switch g.Kind {
	case term.GestureWheel:
		step := max(3/a.itemH(), 1)
		switch g.Wheel {
		case term.WheelUp:
			a.scrollRows(-step)
		case term.WheelDown:
			a.scrollRows(step)
		}
		a.scrolled = true
		return true
	case term.GesturePress:
		vi, _ := a.itemAt(g.X, g.Y)
		a.pressSel = false
		if e, ok := a.entryAt(vi); ok && g.Button == term.ButtonLeft {
			a.pressSel = a.selected[e.Name]
		}
		a.boxStart, a.dragging = -1, false
		return true
	case term.GestureDragStart, term.GestureDrag:
		if g.Button != term.ButtonLeft {
			return true
		}
		startVi, inList := a.itemAt(g.StartX, g.StartY)
		if !inList {
			return true
		}
		if a.pressSel {
			if !a.dragging {
				a.dragging = true
				sel := a.Selection()
				a.h.BeginDrag(host.Drag{Paths: paths(sel), SrcDir: a.dir})
			}
			return true
		}
		// Box selection from the press row (or the end of the list).
		if a.boxStart < 0 {
			a.boxStart = startVi
			if startVi < 0 {
				a.boxStart = len(a.view) - 1
			}
		}
		if g.Y < rowList {
			a.scrollRows(-1)
		}
		if g.Y >= rowList+a.visRows()*a.itemH() {
			a.scrollRows(1)
		}
		cur := a.nearestItem(g.X, g.Y)
		a.selectRange(a.boxStart, cur)
		a.cursor, a.anchor = cur, a.boxStart
		return true
	case term.GestureRelease, term.GestureDragEnd:
		a.boxStart, a.dragging = -1, false
		return true
	case term.GestureClick:
		return a.handleClick(g)
	}
	return false
}

func (a *App) handleClick(g term.Gesture) bool {
	ctx := g.Button == term.ButtonRight
	if g.Button != term.ButtonLeft && !ctx {
		return false
	}
	toggleMod := g.Mod&(term.ModCtrl|term.ModAlt|term.ModMeta) != 0
	switch {
	case g.Y == rowBar && !ctx:
		a.chrome.ClickBar(g.X)
		return true
	case g.Y == rowPath && !ctx && a.toggleX[1] > 0 && g.X >= a.toggleX[0]:
		a.setIcons(g.X >= a.toggleX[1])
		return true
	case g.Y == rowPath && !ctx && g.X < forwardX:
		a.goBack()
		return true
	case g.Y == rowPath && !ctx && g.X < pathX-1:
		a.goForward()
		return true
	case g.Y == rowPath && !ctx:
		for _, s := range a.segments {
			if g.X >= s.x0 && g.X < s.x1 {
				a.goTo(s.path, "")
			}
		}
		return true
	case g.Y == rowHeader && !ctx:
		for _, c := range a.cols {
			if g.X >= c.x0 && g.X < c.x0+c.w {
				if a.sortKey == c.key {
					a.reverse = !a.reverse
				} else {
					a.sortKey, a.reverse = c.key, false
				}
				fsops.Sort(a.entries, a.sortKey, a.reverse)
				a.rebuild()
			}
		}
		return true
	}
	vi, inList := a.itemAt(g.X, g.Y)
	if !inList {
		if ctx {
			a.contextMenu(g.X, g.Y, false)
		}
		return true
	}
	e, ok := a.entryAt(vi)
	if ctx {
		if ok && !a.selected[e.Name] {
			a.selectOnly(vi)
			a.cursor = vi
		}
		if !ok {
			a.selected = map[string]bool{}
		}
		a.contextMenu(g.X, g.Y, ok)
		return true
	}
	if !ok { // empty area below the rows
		a.selected = map[string]bool{}
		return true
	}
	switch {
	case toggleMod:
		a.toggle(vi)
	case g.Count == 2:
		a.selectOnly(vi)
		a.cursor = vi
		a.open(e)
	case g.Mod&term.ModShift != 0:
		a.selectRange(a.anchor, vi)
		a.cursor = vi
	default:
		a.selectOnly(vi)
		a.cursor, a.anchor = vi, vi
	}
	return true
}

// ---- drop target ----

// DropDir: a folder row under the pointer, otherwise this window's folder.
func (a *App) DropDir(x, y int) (string, bool) {
	if vi, inList := a.itemAt(x, y); inList {
		if e, ok := a.entryAt(vi); ok && e.IsDir {
			return e.Path, true
		}
	}
	if y >= rowList {
		return a.dir, true
	}
	return "", false
}

func (a *App) DropHover(x, y int, on bool) {
	a.dropRow = -1
	if !on {
		return
	}
	if vi, inList := a.itemAt(x, y); inList {
		if e, ok := a.entryAt(vi); ok && e.IsDir {
			a.dropRow = vi
		}
	}
}

// ---- drawing ----

func (a *App) Draw(s wm.Surface, focused bool) {
	a.vw, a.vh = s.Size()
	if a.scrolled {
		// Keep the wheel position (the cursor may be off screen); only
		// re-align it after a resize or a view change.
		a.offset = a.clampOffsetRow(a.offset/a.ncols()) * a.ncols()
	} else {
		a.ensureVisible()
	}
	a.chrome.DrawBar(s)
	a.chrome.DrawHints(s)
	a.drawPath(s)
	a.layoutColumns()
	a.drawHeader(s)
	a.drawRows(s, focused)
	a.drawStatus(s)
}

func (a *App) drawPath(s wm.Surface) {
	row := s.Sub(wm.Rect{X: 0, Y: rowPath, W: a.vw, H: 1})
	row.Clear(wm.StyleHeader)
	a.segments = a.segments[:0]
	parts := strings.Split(strings.Trim(a.dir, "/"), "/")
	type seg struct{ label, path string }
	segs := []seg{{"/", "/"}}
	cur := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		cur += "/" + p
		segs = append(segs, seg{a.display(p) + "/", cur})
	}
	// Show the tail if the path is too long.
	total := 1
	for _, sg := range segs {
		total += term.Width(sg.label)
	}
	// "<" Back and ">" Forward on the left (keys: Alt+Left / Alt+Right).
	nav := func(x int, label string, on bool) {
		st := wm.StyleHeader
		if !on {
			st = wm.StyleBodyDim
		}
		row.Text(x, 0, label, st)
	}
	nav(backX-1, " < ", len(a.back) > 0)
	nav(forwardX, "> ", len(a.fwd) > 0)
	// "List|Icons" switch on the right (mouse; keys: View menu).
	avail := a.vw - 2 - pathX + 1
	a.toggleX = [2]int{}
	if a.vw >= 36 {
		x0 := a.vw - 13
		a.toggleX = [2]int{x0, x0 + 6}
		avail = x0 - 2
		on, off := wm.StyleSelected, wm.StyleHeader
		ls, is := on, off
		if a.icons {
			ls, is = off, on
		}
		row.Text(x0, 0, " List ", ls)
		row.Text(x0+6, 0, " Icons ", is)
	}
	first := 0
	for total > avail && first < len(segs)-1 {
		total -= term.Width(segs[first].label)
		first++
	}
	pr := row.Sub(wm.Rect{X: pathX - 1, Y: 0, W: max(avail+1, 0), H: 1})
	x := 1
	if first > 0 {
		x = pr.Text(x, 0, "…", wm.StyleHeader)
	}
	for _, sg := range segs[first:] {
		x0 := x
		x = pr.Text(x, 0, sg.label, wm.StyleHeader)
		a.segments = append(a.segments, segment{x0 + pathX - 1, x + pathX - 1, sg.path})
	}
}

func (a *App) layoutColumns() {
	const sizeW, timeW, permW = 7, 16, 10
	a.cols = a.cols[:0]
	if a.icons {
		// No columns: the header holds sort buttons.
		x := 0
		for _, k := range []fsops.SortKey{fsops.SortName, fsops.SortSize, fsops.SortTime} {
			w := map[fsops.SortKey]int{fsops.SortName: 8, fsops.SortSize: 8, fsops.SortTime: 12}[k]
			a.cols = append(a.cols, column{key: k, x0: x, w: w})
			x += w
		}
		a.nameW = cellW - 1
		return
	}
	x := a.vw - 1 // leave the scrollbar column
	var extra []column
	if a.vw >= 40 {
		x -= permW + 1
		extra = append(extra, column{key: -1, x0: x + 1, w: permW})
	}
	if a.vw >= 56 {
		x -= timeW + 1
		extra = append(extra, column{key: fsops.SortTime, x0: x + 1, w: timeW})
	}
	x -= sizeW + 1
	extra = append(extra, column{key: fsops.SortSize, x0: x + 1, w: sizeW})
	a.nameW = max(x, 8)
	a.cols = append(a.cols, column{key: fsops.SortName, x0: 0, w: a.nameW})
	for i := len(extra) - 1; i >= 0; i-- {
		a.cols = append(a.cols, extra[i])
	}
}

func (a *App) drawHeader(s wm.Surface) {
	row := s.Sub(wm.Rect{X: 0, Y: rowHeader, W: a.vw, H: 1})
	row.Clear(wm.StyleBodyDim)
	for _, c := range a.cols {
		label := map[fsops.SortKey]string{fsops.SortName: "Name", fsops.SortSize: "Size", fsops.SortTime: "Modified", -1: "Perms"}[c.key]
		if c.key == a.sortKey {
			if a.reverse {
				label += " v"
			} else {
				label += " ^"
			}
		}
		row.Sub(wm.Rect{X: c.x0, Y: 0, W: c.w, H: 1}).Text(1, 0, label, wm.StyleBodyDim)
	}
}

func (a *App) entryName(e fsops.Entry) string {
	n := a.display(e.Name)
	switch {
	case e.IsLink && e.IsDir:
		return n + "@/"
	case e.IsLink:
		return n + "@"
	case e.IsDir:
		return n + "/"
	case e.Mode&0o111 != 0:
		return n + "*"
	}
	return n
}

func typeChar(e fsops.Entry) string {
	switch {
	case e.IsLink:
		return "l"
	case e.Mode.IsDir():
		return "d"
	}
	return "-"
}

func (a *App) drawRows(s wm.Surface, focused bool) {
	h := a.listH()
	list := s.Sub(wm.Rect{X: 0, Y: rowList, W: a.vw, H: h})
	switch {
	case a.loading && len(a.entries) == 0:
		list.Text(1, 0, "Loading…", wm.StyleBodyDim)
		return
	case a.loadErr != nil:
		list.Text(1, 0, "Cannot open folder: "+errText(a.loadErr), wm.StyleDialogError)
		return
	case len(a.view) == 0:
		msg := "(empty folder)"
		if len(a.filter) > 0 {
			msg = "(no match for filter)"
		}
		list.Text(1, 0, msg, wm.StyleBodyDim)
	}
	if a.icons {
		a.drawIcons(list, focused)
		return
	}
	for y := 0; y < h; y++ {
		vi := a.offset + y
		e, ok := a.entryAt(vi)
		if !ok {
			break
		}
		st := wm.StyleBody
		switch {
		case vi == a.dropRow:
			st = wm.StyleDropTarget
		case vi == a.cursor && focused:
			st = wm.StyleSelected
		case a.selected[e.Name]:
			st = wm.StyleMarked
		case e.IsDir:
			st = wm.StyleTitle
		}
		row := list.Sub(wm.Rect{X: 0, Y: y, W: a.vw - 1, H: 1})
		row.Clear(st)
		mark := " "
		if a.selected[e.Name] {
			mark = "+"
		}
		row.Text(0, 0, mark, st)
		if a.renaming != nil && e.Name == a.renameName {
			a.renaming.Draw(row.Sub(wm.Rect{X: 1, Y: 0, W: a.nameW - 1, H: 1}), true)
		} else {
			row.Sub(wm.Rect{X: 1, Y: 0, W: a.nameW - 1, H: 1}).Text(0, 0, clipName(a.entryName(e), a.nameW-1), st)
		}
		for _, c := range a.cols[1:] {
			var txt string
			switch c.key {
			case fsops.SortSize:
				if e.IsDir {
					txt = "-"
				} else {
					txt = ui.HumanSize(e.Size)
				}
				txt = strings.Repeat(" ", max(c.w-len(txt), 0)) + txt
			case fsops.SortTime:
				txt = e.ModTime.Format("2006-01-02 15:04")
			default:
				txt = typeChar(e) + fsops.Symbolic(e.Mode)
			}
			row.Sub(wm.Rect{X: c.x0, Y: 0, W: c.w, H: 1}).Text(0, 0, txt, st)
		}
	}
	if len(a.view) > h {
		ui.DrawScrollbar(list.Sub(wm.Rect{X: a.vw - 1, Y: 0, W: 1, H: h}), len(a.view), a.offset, h)
	}
}

func clipName(s string, w int) string {
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

func (a *App) drawStatus(s wm.Surface) {
	row := s.Sub(wm.Rect{X: 0, Y: a.vh - 1 - a.chrome.HintRows(), W: a.vw, H: 1})
	row.Clear(wm.StyleBodyDim)
	var parts []string
	parts = append(parts, fmt.Sprintf("%d items", len(a.view)))
	if n := len(a.selected); n > 1 {
		var size int64
		for _, e := range a.Selection() {
			size += e.Size
		}
		parts = append(parts, fmt.Sprintf("%d selected (%s)", n, ui.HumanSize(size)))
	} else if e, ok := a.cursorEntry(); ok {
		parts = append(parts, a.display(e.Name))
	}
	if len(a.filter) > 0 {
		parts = append(parts, "filter: "+a.display(string(a.filter)))
	}
	if a.showHidden {
		parts = append(parts, "hidden shown")
	}
	row.Text(1, 0, strings.Join(parts, " · "), wm.StyleBodyDim)
}

func errText(err error) string {
	var pe *fs.PathError
	if e, ok := err.(*fs.PathError); ok {
		pe = e
		return pe.Err.Error()
	}
	return err.Error()
}

// drawIcons draws the icon view: a grid of cellW×cellH cells, the text icon
// centred on the first line and the (clipped) name on the second.
func (a *App) drawIcons(list wm.Surface, focused bool) {
	n, rows := a.ncols(), a.visRows()
	for r := 0; r < rows; r++ {
		for c := 0; c < n; c++ {
			vi := a.offset + r*n + c
			e, ok := a.entryAt(vi)
			if !ok {
				break
			}
			st := wm.StyleBody
			switch {
			case vi == a.dropRow:
				st = wm.StyleDropTarget
			case vi == a.cursor && focused:
				st = wm.StyleSelected
			case a.selected[e.Name]:
				st = wm.StyleMarked
			case e.IsDir:
				st = wm.StyleTitle
			}
			cell := list.Sub(wm.Rect{X: c * cellW, Y: r * cellH, W: cellW - 1, H: cellH})
			cell.Clear(st)
			icon := entryIcon(e)
			if a.selected[e.Name] {
				cell.Text(0, 0, "+", st)
			}
			cell.Text((cellW-1-term.Width(icon))/2, 0, icon, st)
			if a.renaming != nil && e.Name == a.renameName {
				a.renaming.Draw(cell.Sub(wm.Rect{X: 0, Y: 1, W: cellW - 1, H: 1}), true)
				continue
			}
			name := clipName(a.display(e.Name), cellW-1)
			cell.Text((cellW-1-term.Width(name))/2, 1, name, st)
		}
	}
	total := (len(a.view) + n - 1) / n
	if total > rows {
		w, _ := list.Size()
		ui.DrawScrollbar(list.Sub(wm.Rect{X: w - 1, Y: 0, W: 1, H: rows * cellH}), total, a.offset/n, rows)
	}
}
