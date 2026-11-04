package term

import (
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
)

// widthCond mirrors tcell's width rules: East Asian "ambiguous" runes
// (…, │, ※, ① …) are 1 column unless RUNEWIDTH_EASTASIAN is set.
// runewidth's default would instead turn them into 2 columns whenever
// LANG is ko_KR/ja_JP/zh_*, which would disagree with what tcell draws.
var widthCond = newWidthCond()

func newWidthCond() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = EastAsianAmbiguousWide()
	return c
}

// EastAsianAmbiguousWide reports whether ambiguous-width runes are treated as
// 2 columns (RUNEWIDTH_EASTASIAN=1|true|yes), matching tcell.
func EastAsianAmbiguousWide() bool {
	rw := strings.ToLower(os.Getenv("RUNEWIDTH_EASTASIAN"))
	return rw == "1" || rw == "true" || rw == "yes"
}

// Cell is one rune placed at a screen column offset. Wide runes (e.g. Hangul)
// occupy two columns; the following column is left for the terminal.
type Cell struct {
	Col   int
	Rune  rune
	Width int
}

// Layout places s into at most maxWidth columns using runewidth. If the text
// does not fit and ellipsis is true, the last column(s) are replaced with "…"
// so that a wide rune is never split. Zero-width runes are dropped.
func Layout(s string, maxWidth int, ellipsis bool) []Cell {
	if maxWidth <= 0 {
		return nil
	}
	total := widthCond.StringWidth(s)
	ew := widthCond.RuneWidth('…')
	limit := maxWidth
	if ellipsis && total > maxWidth {
		limit = maxWidth - ew // reserve room for '…'
	}
	var out []Cell
	col := 0
	for _, r := range s {
		w := widthCond.RuneWidth(r)
		if w == 0 {
			continue
		}
		if col+w > limit {
			break
		}
		out = append(out, Cell{Col: col, Rune: r, Width: w})
		col += w
	}
	if ellipsis && total > maxWidth {
		out = append(out, Cell{Col: col, Rune: '…', Width: ew})
	}
	return out
}

// Width returns the display width of s in terminal columns.
func Width(s string) int { return widthCond.StringWidth(s) }

// RuneWidth returns the display width of r in terminal columns.
func RuneWidth(r rune) int { return widthCond.RuneWidth(r) }

// PadRight pads s with spaces to exactly w columns (truncating if needed).
func PadRight(s string, w int) string {
	cells := Layout(s, w, false)
	out := make([]rune, 0, len(cells)+w)
	col := 0
	for _, c := range cells {
		out = append(out, c.Rune)
		col += c.Width
	}
	for ; col < w; col++ {
		out = append(out, ' ')
	}
	return string(out)
}
