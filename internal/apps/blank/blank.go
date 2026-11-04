// Package blank provides simple placeholder apps for Phase 1: an empty
// window that reports what it receives, and a read-only text window (help).
package blank

import (
	"fmt"

	"tuidesk/internal/term"
	"tuidesk/internal/wm"
)

// App is an empty window that shows its size and the last event it got.
type App struct {
	N    int
	last string
}

// New returns empty window number n.
func New(n int) *App { return &App{N: n} }

func (a *App) Title() string       { return fmt.Sprintf("빈 창 %d", a.N) }
func (a *App) MinSize() (int, int) { return 14, 3 }
func (a *App) Close() error        { return nil }

// HandleEvent records the event but lets the desktop handle keys.
func (a *App) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		a.last = "키 " + ev.Key.String()
	case term.MouseEvent:
		a.last = ev.Gesture.String()
	case term.PasteEvent:
		a.last = fmt.Sprintf("붙여넣기 %d자", len([]rune(ev.Text)))
		return true
	}
	return false
}

func (a *App) Draw(s wm.Surface, focused bool) {
	w, h := s.Size()
	s.Text(1, 0, a.Title(), wm.StyleBody)
	s.Text(1, 1, fmt.Sprintf("크기 %d×%d", w, h), wm.StyleBodyDim)
	if a.last != "" {
		s.Text(1, 2, "마지막 입력: "+a.last, wm.StyleBodyDim)
	}
}

// Text is a read-only window showing fixed lines (used for help).
type Text struct {
	Name  string
	Lines []string
}

func (t *Text) Title() string { return t.Name }
func (t *Text) MinSize() (int, int) {
	return 20, 3
}
func (t *Text) Close() error                { return nil }
func (t *Text) HandleEvent(term.Event) bool { return false }
func (t *Text) Draw(s wm.Surface, focused bool) {
	for i, l := range t.Lines {
		s.Text(1, i, l, wm.StyleBody)
	}
}
