package wm

import "tuidesk/internal/term"

// Surface is a clipped, origin-shifted view of a Canvas. Apps and widgets
// draw in local coordinates (0,0 is the top-left of their area) and cannot
// paint outside it.
type Surface struct {
	c    *Canvas
	area Rect // in canvas coordinates
	clip Rect // area ∩ visible region
}

// NewSurface returns a surface for area on c.
func NewSurface(c *Canvas, area Rect) Surface {
	return Surface{c: c, area: area, clip: area.Intersect(Rect{0, 0, c.W, c.H})}
}

// Size is the local width and height.
func (s Surface) Size() (w, h int) { return s.area.W, s.area.H }

// Set writes one rune at local (x, y) and returns its width.
func (s Surface) Set(x, y int, r rune, st StyleID) int {
	if x < 0 || y < 0 || x >= s.area.W || y >= s.area.H {
		return term.RuneWidth(r)
	}
	return s.c.Set(s.clip, s.area.X+x, s.area.Y+y, r, st)
}

// SetTerm writes a terminal cell with explicit colors at local (x, y).
func (s Surface) SetTerm(x, y int, r rune, t TermStyle) int {
	if x < 0 || y < 0 || x >= s.area.W || y >= s.area.H {
		return term.RuneWidth(r)
	}
	return s.c.SetTerm(s.clip, s.area.X+x, s.area.Y+y, r, t)
}

// Text writes str at local (x, y) and returns the column after it.
func (s Surface) Text(x, y int, str string, st StyleID) int {
	for _, r := range str {
		x += s.Set(x, y, r, st)
	}
	return x
}

// Fill paints a local rectangle.
func (s Surface) Fill(r Rect, ch rune, st StyleID) {
	r = r.Intersect(Rect{0, 0, s.area.W, s.area.H})
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			s.Set(x, y, ch, st)
		}
	}
}

// Clear paints the whole surface.
func (s Surface) Clear(st StyleID) { s.Fill(Rect{0, 0, s.area.W, s.area.H}, ' ', st) }

// Sub returns a surface for a local sub-rectangle.
func (s Surface) Sub(r Rect) Surface {
	abs := Rect{s.area.X + r.X, s.area.Y + r.Y, r.W, r.H}
	return Surface{c: s.c, area: abs, clip: abs.Intersect(s.clip)}
}

// Box draws a single-line frame around the local rectangle r.
func (s Surface) Box(r Rect, st StyleID) {
	if r.W < 2 || r.H < 2 {
		return
	}
	x1, y1 := r.X+r.W-1, r.Y+r.H-1
	for x := r.X + 1; x < x1; x++ {
		s.Set(x, r.Y, '─', st)
		s.Set(x, y1, '─', st)
	}
	for y := r.Y + 1; y < y1; y++ {
		s.Set(r.X, y, '│', st)
		s.Set(x1, y, '│', st)
	}
	s.Set(r.X, r.Y, '┌', st)
	s.Set(x1, r.Y, '┐', st)
	s.Set(r.X, y1, '└', st)
	s.Set(x1, y1, '┘', st)
}
