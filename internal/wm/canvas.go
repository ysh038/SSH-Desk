// Package wm is the window manager / compositor. It is independent of tcell:
// windows are painted back-to-front into a Canvas of cells, which the caller
// copies to the terminal.
package wm

import "tuidesk/internal/term"

// StyleID names a visual style; the caller maps it to real terminal colors.
type StyleID uint8

// Cell is one screen column. Width is 1 or 2 for a glyph, 0 for the right
// half of a wide (e.g. Hangul) glyph that starts in the previous column.
type Cell struct {
	Rune  rune
	Width int8
	Style StyleID
	T     TermStyle // colors of a StyleTerm cell (zero otherwise)
}

// Canvas is a grid of cells. All writes keep wide glyphs consistent: a wide
// glyph is never left with only one of its two halves.
type Canvas struct {
	W, H  int
	cells []Cell
}

// NewCanvas returns a w×h canvas filled with blanks in style 0.
func NewCanvas(w, h int) *Canvas {
	c := &Canvas{}
	c.Resize(w, h)
	return c
}

// Resize discards the contents and resizes the canvas.
func (c *Canvas) Resize(w, h int) {
	c.W, c.H = max(w, 0), max(h, 0)
	c.cells = make([]Cell, c.W*c.H)
	c.Fill(Rect{0, 0, c.W, c.H}, ' ', 0)
}

// At returns the cell at (x, y). Out-of-range positions return a blank.
func (c *Canvas) At(x, y int) Cell {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return Cell{Rune: ' ', Width: 1}
	}
	return c.cells[y*c.W+x]
}

func (c *Canvas) raw(x, y int) *Cell { return &c.cells[y*c.W+x] }

// blank turns the cell into a narrow space, keeping its style.
func (c *Canvas) blank(x, y int) {
	if x < 0 || x >= c.W {
		return
	}
	p := c.raw(x, y)
	p.Rune, p.Width = ' ', 1
}

// Set writes r at (x, y), clipped to clip. It returns the display width of r
// (0 for zero-width runes) so callers can advance, whether or not anything was
// drawn. A wide rune whose right half would fall outside clip is drawn as a
// space instead.
func (c *Canvas) Set(clip Rect, x, y int, r rune, s StyleID) int {
	return c.put(clip, x, y, r, s, TermStyle{})
}

// SetTerm is Set for a terminal cell with explicit colors (StyleTerm).
func (c *Canvas) SetTerm(clip Rect, x, y int, r rune, t TermStyle) int {
	return c.put(clip, x, y, r, StyleTerm, t)
}

func (c *Canvas) put(clip Rect, x, y int, r rune, s StyleID, t TermStyle) int {
	clip = clip.Intersect(Rect{0, 0, c.W, c.H})
	w := term.RuneWidth(r)
	if w == 0 {
		return 0
	}
	adv := w // columns the caller should advance, even if we draw a substitute
	if !clip.Contains(x, y) {
		return adv
	}
	if w == 2 && !clip.Contains(x+1, y) {
		r, w = ' ', 1
	}

	// Breaking an existing wide glyph: whatever half we overwrite, blank the other.
	for col := x; col < x+w; col++ {
		switch cur := c.raw(col, y); cur.Width {
		case 0: // right half of a glyph starting at col-1
			if col-1 < x {
				c.blank(col-1, y)
			}
		case 2: // left half; its right half is at col+1
			if col+1 >= x+w && col+1 < c.W {
				c.blank(col+1, y)
			}
		}
	}

	*c.raw(x, y) = Cell{Rune: r, Width: int8(w), Style: s, T: t}
	if w == 2 {
		*c.raw(x+1, y) = Cell{Rune: 0, Width: 0, Style: s, T: t}
	}
	return adv
}

// Fill paints every cell of r (clipped to the canvas) with ch.
func (c *Canvas) Fill(r Rect, ch rune, s StyleID) {
	r = r.Intersect(Rect{0, 0, c.W, c.H})
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			c.Set(r, x, y, ch, s)
		}
	}
}

// Text writes s starting at (x, y), clipped to clip, and returns the column
// after the last written rune.
func (c *Canvas) Text(clip Rect, x, y int, s string, st StyleID) int {
	for _, r := range s {
		x += c.Set(clip, x, y, r, st)
	}
	return x
}

// Row returns the visible text of row y (right halves of wide glyphs are
// skipped). Intended for tests and debugging.
func (c *Canvas) Row(y int) string {
	out := make([]rune, 0, c.W)
	for x := 0; x < c.W; x++ {
		if cell := c.At(x, y); cell.Width != 0 {
			out = append(out, cell.Rune)
		}
	}
	return string(out)
}

// CopyFrom makes c an exact copy of o (resizing if needed).
func (c *Canvas) CopyFrom(o *Canvas) {
	if c.W != o.W || c.H != o.H {
		c.W, c.H = o.W, o.H
		c.cells = make([]Cell, len(o.cells))
	}
	copy(c.cells, o.cells)
}
