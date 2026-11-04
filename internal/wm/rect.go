package wm

// Rect is a screen rectangle in cells. W and H may be zero (empty).
type Rect struct{ X, Y, W, H int }

// Contains reports whether (x, y) is inside r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// Intersect returns the overlap of r and o (empty if they do not overlap).
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := max(r.X, o.X), max(r.Y, o.Y)
	x1, y1 := min(r.X+r.W, o.X+o.W), min(r.Y+r.H, o.Y+o.H)
	if x1 <= x0 || y1 <= y0 {
		return Rect{X: x0, Y: y0}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Inset shrinks r by n cells on every side.
func (r Rect) Inset(n int) Rect {
	return Rect{r.X + n, r.Y + n, max(r.W-2*n, 0), max(r.H-2*n, 0)}
}

// Empty reports whether r has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }
