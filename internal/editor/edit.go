package editor

import (
	"strings"
	"unicode"

	"tuidesk/internal/term"
)

// ---- display columns (tabs and wide runes) ----

// RuneW returns the screen width of r at display column x.
func (e *Editor) RuneW(r rune, x int) int {
	if r == '\t' {
		return e.TabSize - x%e.TabSize
	}
	if w := term.RuneWidth(r); w > 0 {
		return w
	}
	return 1
}

// DisplayX returns the screen column of p within its line.
func (e *Editor) displayX(p Pos) int {
	x := 0
	l := e.Lines[p.Line]
	for i := 0; i < p.Col && i < len(l); i++ {
		x += e.RuneW(l[i], x)
	}
	return x
}

// DisplayX is exported for the view.
func (e *Editor) DisplayX(p Pos) int { return e.displayX(p) }

// ColAtX returns the rune column in line l closest to screen column x.
func (e *Editor) ColAtX(l, x int) int {
	line := e.Lines[l]
	cx := 0
	for i, r := range line {
		w := e.RuneW(r, cx)
		if x < cx+(w+1)/2 {
			return i
		}
		cx += w
	}
	return len(line)
}

// ---- movement ----

// Move is a cursor movement.
type Move int

const (
	Left Move = iota
	Right
	Up
	Down
	LineStart // smart: first non-blank, then column 0
	LineEnd
	WordLeft
	WordRight
	PageUp
	PageDown
	DocStart
	DocEnd
)

// MoveCursor moves the cursor; extend (or Mark) keeps/starts a selection.
// page is the number of visible lines (for PageUp/PageDown).
func (e *Editor) MoveCursor(m Move, extend bool, page int) {
	extend = extend || e.Mark
	if extend && e.Anchor == nil {
		a := e.Cur
		e.Anchor = &a
	}
	if !extend {
		// Left/Right with a selection collapse to its edge, like OS editors.
		if a, z, ok := e.Selection(); ok && (m == Left || m == Right) {
			e.Anchor = nil
			if m == Left {
				e.Cur = a
			} else {
				e.Cur = z
			}
			e.wantX = e.displayX(e.Cur)
			e.breakGroup()
			return
		}
		e.Anchor = nil
	}
	p := e.Cur
	vertical := false
	switch m {
	case Left:
		if p.Col > 0 {
			p.Col--
		} else if p.Line > 0 {
			p = Pos{p.Line - 1, len(e.Lines[p.Line-1])}
		}
	case Right:
		if p.Col < len(e.Lines[p.Line]) {
			p.Col++
		} else if p.Line+1 < len(e.Lines) {
			p = Pos{p.Line + 1, 0}
		}
	case Up, Down, PageUp, PageDown:
		d := map[Move]int{Up: -1, Down: 1, PageUp: -max(page-1, 1), PageDown: max(page-1, 1)}[m]
		nl := min(max(p.Line+d, 0), len(e.Lines)-1)
		switch {
		case nl == p.Line && d < 0:
			p.Col = 0
		case nl == p.Line && d > 0:
			p.Col = len(e.Lines[p.Line])
		default:
			p = Pos{nl, e.ColAtX(nl, e.wantX)}
		}
		vertical = true
	case LineStart:
		first := 0
		for first < len(e.Lines[p.Line]) && (e.Lines[p.Line][first] == ' ' || e.Lines[p.Line][first] == '\t') {
			first++
		}
		if p.Col == first {
			p.Col = 0
		} else {
			p.Col = first
		}
	case LineEnd:
		p.Col = len(e.Lines[p.Line])
	case WordLeft:
		p = e.wordLeft(p)
	case WordRight:
		p = e.wordRight(p)
	case DocStart:
		p = Pos{0, 0}
	case DocEnd:
		last := len(e.Lines) - 1
		p = Pos{last, len(e.Lines[last])}
	}
	e.Cur = p
	if !vertical {
		e.wantX = e.displayX(p)
	}
	e.breakGroup()
}

func (e *Editor) wordLeft(p Pos) Pos {
	if p.Col == 0 {
		if p.Line == 0 {
			return p
		}
		return Pos{p.Line - 1, len(e.Lines[p.Line-1])}
	}
	l := e.Lines[p.Line]
	c := p.Col
	for c > 0 && unicode.IsSpace(l[c-1]) {
		c--
	}
	if c > 0 && isWord(l[c-1]) {
		for c > 0 && isWord(l[c-1]) {
			c--
		}
	} else if c > 0 {
		c--
	}
	return Pos{p.Line, c}
}

func (e *Editor) wordRight(p Pos) Pos {
	l := e.Lines[p.Line]
	if p.Col >= len(l) {
		if p.Line+1 >= len(e.Lines) {
			return p
		}
		return Pos{p.Line + 1, 0}
	}
	c := p.Col
	if e.WordEnds { // skip blanks, then to the end of the word
		for c < len(l) && unicode.IsSpace(l[c]) {
			c++
		}
	}
	if c < len(l) && isWord(l[c]) {
		for c < len(l) && isWord(l[c]) {
			c++
		}
	} else if c < len(l) && !unicode.IsSpace(l[c]) {
		c++
	}
	if !e.WordEnds {
		for c < len(l) && unicode.IsSpace(l[c]) {
			c++
		}
	}
	return Pos{p.Line, c}
}

// SetCursor places the cursor (mouse click); extend keeps the selection anchor.
func (e *Editor) SetCursor(p Pos, extend bool) {
	p = e.clamp(p)
	if extend {
		if e.Anchor == nil {
			a := e.Cur
			e.Anchor = &a
		}
	} else {
		e.Anchor = nil
	}
	e.Cur = p
	e.wantX = e.displayX(p)
	e.breakGroup()
}

// GotoLine moves to line n (1-based).
func (e *Editor) GotoLine(n int) {
	e.SetCursor(Pos{n - 1, 0}, false)
}

// ---- typing ----

// Insert types s, replacing the selection.
func (e *Editor) Insert(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	a, z, _ := e.Selection()
	kind := kindType
	if s == "\n" || len([]rune(s)) > 1 {
		kind = kindOther
	}
	e.replace(a, z, s, kind)
	e.Mark = false
}

// Newline inserts a line break keeping the current indentation.
func (e *Editor) Newline() {
	l := e.Lines[e.Cur.Line]
	ind := 0
	for ind < len(l) && ind < e.Cur.Col && (l[ind] == ' ' || l[ind] == '\t') {
		ind++
	}
	indent := string(l[:ind])
	// One extra level after an opening bracket: "server {" ⏎
	if e.Cur.Col > 0 {
		trimmed := strings.TrimRight(string(l[:e.Cur.Col]), " \t")
		if strings.HasSuffix(trimmed, "{") || strings.HasSuffix(trimmed, "[") || strings.HasSuffix(trimmed, "(") ||
			strings.HasSuffix(trimmed, ":") && e.ColonIndent {
			indent += e.indentUnit()
		}
	}
	e.Insert("\n" + indent)
}

func (e *Editor) indentUnit() string {
	if e.ExpandTabs {
		return strings.Repeat(" ", e.TabSize)
	}
	return "\t"
}

// Backspace deletes the selection or the rune before the cursor. In leading
// whitespace it removes one indentation level.
func (e *Editor) Backspace() {
	if a, z, ok := e.Selection(); ok {
		e.replace(a, z, "", kindOther)
		return
	}
	p := e.Cur
	if p.Col == 0 {
		if p.Line == 0 {
			return
		}
		e.replace(Pos{p.Line - 1, len(e.Lines[p.Line-1])}, p, "", kindDelete)
		return
	}
	l := e.Lines[p.Line]
	start := p.Col - 1
	if e.ExpandTabs && strings.TrimLeft(string(l[:p.Col]), " ") == "" {
		start = max(p.Col-((p.Col-1)%e.TabSize+1), 0)
	}
	e.replace(Pos{p.Line, start}, p, "", kindDelete)
}

// DeleteForward deletes the selection or the rune after the cursor.
func (e *Editor) DeleteForward() {
	if a, z, ok := e.Selection(); ok {
		e.replace(a, z, "", kindOther)
		return
	}
	p := e.Cur
	end := Pos{p.Line, p.Col + 1}
	if p.Col >= len(e.Lines[p.Line]) {
		if p.Line+1 >= len(e.Lines) {
			return
		}
		end = Pos{p.Line + 1, 0}
	}
	e.replace(p, end, "", kindDelete)
	e.Cur = p
}

// Tab inserts indentation, or indents every selected line.
func (e *Editor) Tab() {
	if a, z, ok := e.Selection(); ok && a.Line != z.Line {
		e.indentLines(a, z, true)
		return
	}
	if e.ExpandTabs {
		x := e.displayX(e.Cur)
		e.Insert(strings.Repeat(" ", e.TabSize-x%e.TabSize))
		return
	}
	e.Insert("\t")
}

// Untab removes one indentation level from the current or selected lines.
func (e *Editor) Untab() {
	a, z, _ := e.Selection()
	e.indentLines(a, z, false)
}

func (e *Editor) indentLines(a, z Pos, in bool) {
	last := z.Line
	if z.Col == 0 && z.Line > a.Line {
		last--
	}
	e.group++
	g := e.group
	unit := e.indentUnit()
	for l := a.Line; l <= last; l++ {
		line := string(e.Lines[l])
		if in {
			if line == "" {
				continue
			}
			e.replace(Pos{l, 0}, Pos{l, 0}, unit, kindOther)
		} else {
			n := 0
			for n < len(line) && n < e.TabSize && line[n] == ' ' {
				n++
			}
			if n == 0 && strings.HasPrefix(line, "\t") {
				n = 1
			}
			if n == 0 {
				continue
			}
			e.replace(Pos{l, 0}, Pos{l, n}, "", kindOther)
		}
		e.undo[len(e.undo)-1].group = g
	}
	e.group = g
	e.Select(Pos{a.Line, 0}, Pos{last, len(e.Lines[last])})
}

// Cut returns and removes the selection (or the whole line without one,
// like most editors).
func (e *Editor) Cut() string {
	if a, z, ok := e.Selection(); ok {
		s := e.slice(a, z)
		e.replace(a, z, "", kindOther)
		return s
	}
	l := e.Cur.Line
	var a, z Pos
	if l+1 < len(e.Lines) {
		a, z = Pos{l, 0}, Pos{l + 1, 0}
	} else if l > 0 {
		a, z = Pos{l - 1, len(e.Lines[l-1])}, Pos{l, len(e.Lines[l])}
	} else {
		a, z = Pos{0, 0}, Pos{0, len(e.Lines[0])}
	}
	s := e.slice(a, z)
	e.replace(a, z, "", kindOther)
	if l+1 >= len(e.Lines) && !strings.HasSuffix(s, "\n") {
		s = strings.TrimPrefix(s, "\n") + "\n"
	}
	return s
}

// Copy returns the selection (or the current line with its newline).
func (e *Editor) Copy() string {
	if s := e.SelectedText(); s != "" {
		return s
	}
	return string(e.Lines[e.Cur.Line]) + "\n"
}

// ---- find / replace ----

// Find searches for q starting after the cursor (or before it when
// backward), wrapping around. It selects and returns the match.
func (e *Editor) Find(q string, backward, caseSensitive bool) bool {
	if q == "" {
		return false
	}
	fold := func(s string) string {
		if caseSensitive {
			return s
		}
		return strings.ToLower(s)
	}
	needle := []rune(fold(q))
	lines := make([][]rune, len(e.Lines))
	for i, l := range e.Lines {
		lines[i] = []rune(fold(string(l)))
	}
	match := func(l, c int) bool {
		line := lines[l]
		if strings.ContainsRune(q, '\n') || c+len(needle) > len(line) {
			return false
		}
		for i, r := range needle {
			if line[c+i] != r {
				return false
			}
		}
		return true
	}
	start := e.Cur
	if a, z, ok := e.Selection(); ok {
		if backward {
			start = a
		} else {
			start = z
		}
	}
	n := len(e.Lines)
	if !backward {
		for k := 0; k <= n; k++ {
			l := (start.Line + k) % n
			from := 0
			if k == 0 {
				from = start.Col
			}
			for c := from; c+len(needle) <= len(lines[l]); c++ {
				if match(l, c) {
					e.Select(Pos{l, c}, Pos{l, c + len(needle)})
					return true
				}
			}
		}
		return false
	}
	for k := 0; k <= n; k++ {
		l := ((start.Line-k)%n + n) % n
		to := len(lines[l]) - len(needle)
		if k == 0 {
			to = min(to, start.Col-1)
		}
		for c := to; c >= 0; c-- {
			if match(l, c) {
				e.Select(Pos{l, c}, Pos{l, c + len(needle)})
				return true
			}
		}
	}
	return false
}

// ReplaceSelection replaces the current match with r if the selection
// equals q, then finds the next match.
func (e *Editor) ReplaceSelection(q, r string, caseSensitive bool) bool {
	sel := e.SelectedText()
	same := sel == q || !caseSensitive && strings.EqualFold(sel, q)
	if sel != "" && same {
		a, z, _ := e.Selection()
		e.replace(a, z, r, kindOther)
	}
	return e.Find(q, false, caseSensitive)
}

// ReplaceAll replaces every occurrence and returns the count (one undo step).
func (e *Editor) ReplaceAll(q, r string, caseSensitive bool) int {
	if q == "" {
		return 0
	}
	e.group++
	g := e.group
	count := 0
	e.SetCursor(Pos{0, 0}, false)
	for e.findFrom(q, caseSensitive) {
		a, z, _ := e.Selection()
		e.replace(a, z, r, kindOther)
		e.undo[len(e.undo)-1].group = g
		count++
	}
	e.group = g
	return count
}

// findFrom finds the next match at or after the cursor without wrapping.
func (e *Editor) findFrom(q string, caseSensitive bool) bool {
	fold := func(s string) string {
		if caseSensitive {
			return s
		}
		return strings.ToLower(s)
	}
	needle := fold(q)
	for l := e.Cur.Line; l < len(e.Lines); l++ {
		from := 0
		if l == e.Cur.Line {
			from = e.Cur.Col
		}
		line := []rune(fold(string(e.Lines[l])))
		if from > len(line) {
			continue
		}
		if i := strings.Index(string(line[from:]), needle); i >= 0 {
			c := from + len([]rune(string(line[from:])[:i]))
			e.Select(Pos{l, c}, Pos{l, c + len([]rune(q))})
			return true
		}
	}
	return false
}

// MatchBracket returns the position of the bracket matching the one at or
// before the cursor.
func (e *Editor) MatchBracket() (Pos, Pos, bool) {
	pairs := map[rune]rune{'(': ')', '[': ']', '{': '}', ')': '(', ']': '[', '}': '{'}
	at := func(p Pos) (rune, bool) {
		if p.Col < 0 || p.Col >= len(e.Lines[p.Line]) {
			return 0, false
		}
		return e.Lines[p.Line][p.Col], true
	}
	p := e.Cur
	r, ok := at(p)
	if _, isB := pairs[r]; !ok || !isB {
		p.Col--
		if r, ok = at(p); !ok {
			return Pos{}, Pos{}, false
		}
		if _, isB := pairs[r]; !isB {
			return Pos{}, Pos{}, false
		}
	}
	open := strings.ContainsRune("([{", r)
	want := pairs[r]
	depth := 0
	l, c := p.Line, p.Col
	for steps := 0; steps < 200000; steps++ {
		if open {
			c++
			for c >= len(e.Lines[l]) {
				l++
				if l >= len(e.Lines) {
					return Pos{}, Pos{}, false
				}
				c = 0
				if len(e.Lines[l]) > 0 {
					break
				}
			}
		} else {
			c--
			for c < 0 {
				l--
				if l < 0 {
					return Pos{}, Pos{}, false
				}
				c = len(e.Lines[l]) - 1
			}
		}
		if c >= len(e.Lines[l]) {
			continue
		}
		switch e.Lines[l][c] {
		case r:
			depth++
		case want:
			if depth == 0 {
				return p, Pos{l, c}, true
			}
			depth--
		}
	}
	return Pos{}, Pos{}, false
}

// ---- drag and drop ----

// MoveText moves the text in [a,z) to position to (or copies it when copy
// is true) as one undo step, and selects it at its new place. Dropping a
// move inside the text itself does nothing and returns false.
func (e *Editor) MoveText(a, z, to Pos, copy bool) bool {
	a, z = order(e.clamp(a), e.clamp(z))
	to = e.clamp(to)
	if a == z || (!copy && !to.Less(a) && !z.Less(to)) {
		return false
	}
	s := e.slice(a, z)
	n := len([]rune(s))
	ot := e.Offset(to)
	before := len(e.undo)
	switch {
	case copy:
		e.replace(to, to, s, kindOther)
	case z.Less(to): // drop after the text: insert first so a and z stay valid
		e.replace(to, to, s, kindOther)
		e.replace(a, z, "", kindOther)
		ot -= n
	default: // drop before the text
		e.replace(a, z, "", kindOther)
		e.replace(to, to, s, kindOther)
	}
	if len(e.undo) > before+1 { // one undo step
		g := e.undo[before].group
		for i := before + 1; i < len(e.undo); i++ {
			e.undo[i].group = g
		}
		e.group = g
	}
	e.Select(e.PosAt(ot), e.PosAt(ot+n))
	e.Mark = false
	return true
}
