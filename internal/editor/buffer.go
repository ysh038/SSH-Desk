// Package editor is the text editing core (SPEC 7.3, ADR 0003 §3): a line
// buffer with cursor, selection, grouped undo/redo, movement, find/replace
// and indentation. It does not depend on tcell; the editor app draws it.
package editor

import (
	"strings"
	"time"
	"unicode"
)

// Pos is a position: line index and rune column (not screen column).
type Pos struct{ Line, Col int }

// Less orders positions.
func (p Pos) Less(q Pos) bool { return p.Line < q.Line || p.Line == q.Line && p.Col < q.Col }

func order(a, b Pos) (Pos, Pos) {
	if b.Less(a) {
		return b, a
	}
	return a, b
}

// op is one recorded change: at At, Del was removed and Ins inserted.
type op struct {
	At        Pos
	Del, Ins  string
	CurBefore Pos
	CurAfter  Pos
	group     int
	kind      int
	time      time.Time
}

const (
	kindOther = iota
	kindType
	kindDelete
)

// Editor holds a document and its editing state.
type Editor struct {
	Lines [][]rune
	Cur   Pos
	// Anchor is the other end of the selection (nil: no selection).
	Anchor *Pos
	// Mark keeps extending the selection with plain movement keys
	// (Ctrl+Space on the Linux console, where Shift+arrows do not exist).
	Mark bool

	TabSize    int
	ExpandTabs bool
	CRLF       bool // the file used CRLF line endings
	FinalEOL   bool // the file ended with a newline
	// ColonIndent adds a level after a trailing ':' (YAML, Python).
	ColonIndent bool
	// WordEnds makes word-right stop at the end of a word (macOS Option+→)
	// instead of the start of the next one (Windows Ctrl+→).
	WordEnds bool

	undo, redo []op
	group      int
	saved      int // len(undo) at last save; -1 if unreachable
	wantX      int // remembered display column for vertical moves
	Now        func() time.Time
}

// New creates an editor for text.
func New(text string) *Editor {
	e := &Editor{TabSize: 4, ExpandTabs: true, Now: time.Now}
	e.CRLF = strings.Contains(text, "\r\n")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	e.FinalEOL = strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	for _, l := range strings.Split(text, "\n") {
		e.Lines = append(e.Lines, []rune(l))
	}
	return e
}

// Text returns the document with its original line endings.
func (e *Editor) Text() string {
	var b strings.Builder
	nl := "\n"
	if e.CRLF {
		nl = "\r\n"
	}
	for i, l := range e.Lines {
		if i > 0 {
			b.WriteString(nl)
		}
		b.WriteString(string(l))
	}
	if e.FinalEOL {
		b.WriteString(nl)
	}
	return b.String()
}

// Modified reports unsaved changes.
func (e *Editor) Modified() bool { return e.saved != len(e.undo) }

// MarkSaved records the current state as saved.
func (e *Editor) MarkSaved() { e.saved = len(e.undo) }

func (e *Editor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Editor) clamp(p Pos) Pos {
	p.Line = min(max(p.Line, 0), len(e.Lines)-1)
	p.Col = min(max(p.Col, 0), len(e.Lines[p.Line]))
	return p
}

// ---- raw buffer edits (no undo) ----

func (e *Editor) rawInsert(p Pos, s string) Pos {
	p = e.clamp(p)
	parts := strings.Split(s, "\n")
	line := e.Lines[p.Line]
	tail := append([]rune{}, line[p.Col:]...)
	first := append(append([]rune{}, line[:p.Col]...), []rune(parts[0])...)
	if len(parts) == 1 {
		e.Lines[p.Line] = append(first, tail...)
		return Pos{p.Line, len(first)}
	}
	newLines := [][]rune{first}
	for _, mid := range parts[1 : len(parts)-1] {
		newLines = append(newLines, []rune(mid))
	}
	last := []rune(parts[len(parts)-1])
	end := Pos{p.Line + len(parts) - 1, len(last)}
	newLines = append(newLines, append(last, tail...))
	e.Lines = append(e.Lines[:p.Line], append(newLines, e.Lines[p.Line+1:]...)...)
	return end
}

func (e *Editor) rawDelete(a, z Pos) string {
	a, z = order(e.clamp(a), e.clamp(z))
	text := e.slice(a, z)
	head := append([]rune{}, e.Lines[a.Line][:a.Col]...)
	tail := e.Lines[z.Line][z.Col:]
	e.Lines[a.Line] = append(head, tail...)
	e.Lines = append(e.Lines[:a.Line+1], e.Lines[z.Line+1:]...)
	return text
}

func (e *Editor) slice(a, z Pos) string {
	a, z = order(a, z)
	if a.Line == z.Line {
		return string(e.Lines[a.Line][a.Col:z.Col])
	}
	var b strings.Builder
	b.WriteString(string(e.Lines[a.Line][a.Col:]))
	for l := a.Line + 1; l < z.Line; l++ {
		b.WriteByte('\n')
		b.WriteString(string(e.Lines[l]))
	}
	b.WriteByte('\n')
	b.WriteString(string(e.Lines[z.Line][:z.Col]))
	return b.String()
}

func endOf(p Pos, s string) Pos {
	n := strings.Count(s, "\n")
	if n == 0 {
		return Pos{p.Line, p.Col + len([]rune(s))}
	}
	return Pos{p.Line + n, len([]rune(s[strings.LastIndexByte(s, '\n')+1:]))}
}

// ---- recorded edits ----

// replace removes [a,z) and inserts s, recording undo. kind groups typing.
func (e *Editor) replace(a, z Pos, s string, kind int) {
	a, z = order(e.clamp(a), e.clamp(z))
	if a == z && s == "" {
		return
	}
	before := e.Cur
	del := e.rawDelete(a, z)
	end := e.rawInsert(a, s)
	now := e.now()
	// Consecutive typing (or deleting) within a second, at the place the
	// previous edit ended, is one undo step. Saving always ends a step.
	n := len(e.undo)
	merge := n > 0 && kind != kindOther && e.undo[n-1].kind == kind &&
		now.Sub(e.undo[n-1].time) < time.Second && e.saved != n
	if merge && kind == kindType && endOf(e.undo[n-1].At, e.undo[n-1].Ins) != a {
		merge = false
	}
	if !merge {
		e.group++
	}
	e.undo = append(e.undo, op{At: a, Del: del, Ins: s, CurBefore: before, CurAfter: end, group: e.group, kind: kind, time: now})
	e.redo = nil
	e.Cur = end
	e.Anchor = nil
	e.wantX = e.displayX(e.Cur)
}

// breakGroup ends the current typing group (after a cursor move).
func (e *Editor) breakGroup() {
	if n := len(e.undo); n > 0 {
		e.undo[n-1].kind = kindOther
	}
}

// Undo reverts the last group of changes.
func (e *Editor) Undo() bool {
	n := len(e.undo)
	if n == 0 {
		return false
	}
	g := e.undo[n-1].group
	for n > 0 && e.undo[n-1].group == g {
		o := e.undo[n-1]
		e.rawDelete(o.At, endOf(o.At, o.Ins))
		e.rawInsert(o.At, o.Del)
		e.Cur = e.clamp(o.CurBefore)
		e.redo = append(e.redo, o)
		e.undo = e.undo[:n-1]
		n--
	}
	e.Anchor = nil
	e.wantX = e.displayX(e.Cur)
	return true
}

// Redo re-applies the last undone group.
func (e *Editor) Redo() bool {
	n := len(e.redo)
	if n == 0 {
		return false
	}
	g := e.redo[n-1].group
	for n > 0 && e.redo[n-1].group == g {
		o := e.redo[n-1]
		e.rawDelete(o.At, endOf(o.At, o.Del))
		e.rawInsert(o.At, o.Ins)
		e.Cur = e.clamp(o.CurAfter)
		e.undo = append(e.undo, o)
		e.redo = e.redo[:n-1]
		n--
	}
	e.Anchor = nil
	return true
}

// ---- selection ----

// Selection returns the ordered selection and whether it is non-empty.
func (e *Editor) Selection() (Pos, Pos, bool) {
	if e.Anchor == nil || *e.Anchor == e.Cur {
		return e.Cur, e.Cur, false
	}
	a, z := order(*e.Anchor, e.Cur)
	return a, z, true
}

// SelectedText returns the selection.
func (e *Editor) SelectedText() string {
	a, z, ok := e.Selection()
	if !ok {
		return ""
	}
	return e.slice(a, z)
}

// ClearSelection drops the selection and mark mode.
func (e *Editor) ClearSelection() { e.Anchor, e.Mark = nil, false }

// SelectAll selects the whole document.
func (e *Editor) SelectAll() {
	e.Anchor = &Pos{0, 0}
	last := len(e.Lines) - 1
	e.Cur = Pos{last, len(e.Lines[last])}
}

// Select sets the selection explicitly (mouse, find).
func (e *Editor) Select(a, z Pos) {
	a, z = e.clamp(a), e.clamp(z)
	e.Anchor = &a
	e.Cur = z
	e.wantX = e.displayX(z)
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// WordAt returns the word range around p (for double click).
func (e *Editor) WordAt(p Pos) (Pos, Pos) {
	p = e.clamp(p)
	l := e.Lines[p.Line]
	a, z := p.Col, p.Col
	if a < len(l) && (l[a] == ' ' || l[a] == '\t') { // a run of blanks
		for a > 0 && (l[a-1] == ' ' || l[a-1] == '\t') {
			a--
		}
		for z < len(l) && (l[z] == ' ' || l[z] == '\t') {
			z++
		}
		return Pos{p.Line, a}, Pos{p.Line, z}
	}
	if a < len(l) && !isWord(l[a]) {
		return Pos{p.Line, a}, Pos{p.Line, a + 1}
	}
	for a > 0 && isWord(l[a-1]) {
		a--
	}
	for z < len(l) && isWord(l[z]) {
		z++
	}
	return Pos{p.Line, a}, Pos{p.Line, z}
}

// SelectLine selects line l including its newline (triple click).
func (e *Editor) SelectLine(l int) {
	l = min(max(l, 0), len(e.Lines)-1)
	if l+1 < len(e.Lines) {
		e.Select(Pos{l, 0}, Pos{l + 1, 0})
	} else {
		e.Select(Pos{l, 0}, Pos{l, len(e.Lines[l])})
	}
}

// Offset converts a position to a rune offset in the document ('\n' = 1).
func (e *Editor) Offset(p Pos) int {
	p = e.clamp(p)
	n := 0
	for l := 0; l < p.Line; l++ {
		n += len(e.Lines[l]) + 1
	}
	return n + p.Col
}

// PosAt converts a rune offset back to a position.
func (e *Editor) PosAt(off int) Pos {
	for l, line := range e.Lines {
		if off <= len(line) {
			return Pos{l, max(off, 0)}
		}
		off -= len(line) + 1
	}
	last := len(e.Lines) - 1
	return Pos{last, len(e.Lines[last])}
}

// Len is the document length in runes.
func (e *Editor) Len() int {
	last := len(e.Lines) - 1
	return e.Offset(Pos{last, len(e.Lines[last])})
}

// WordBefore returns the word immediately before the cursor.
func (e *Editor) WordBefore() string {
	l := e.Lines[e.Cur.Line]
	c := e.Cur.Col
	s := c
	for s > 0 && isWord(l[s-1]) {
		s--
	}
	return string(l[s:c])
}
