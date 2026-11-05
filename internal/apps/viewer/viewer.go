// Package viewer is a read-only text viewer used to open text files until
// the editor exists (Phase 3).
package viewer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// MaxBytes is how much of a file is shown.
const MaxBytes = 2 << 20

// App shows a file's lines.
type App struct {
	Path      string
	list      *ui.List
	truncated bool
}

// Open reads path (up to MaxBytes).
func Open(path string) (*App, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	a := &App{Path: path}
	if len(data) > MaxBytes {
		data, a.truncated = data[:MaxBytes], true
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\t", "    "), "\n")
	for i, l := range lines {
		lines[i] = fmt.Sprintf("%5d  %s", i+1, strings.TrimRight(l, "\r"))
	}
	if a.truncated {
		lines = append(lines, "", fmt.Sprintf("… only the first %s are shown", ui.HumanSize(MaxBytes)))
	}
	a.list = &ui.List{Src: ui.Strings(lines)}
	return a, nil
}

func (a *App) Title() string       { return "View - " + filepath.Base(a.Path) + " (read-only)" }
func (a *App) MinSize() (int, int) { return 20, 3 }
func (a *App) Close() error        { return nil }

func (a *App) HandleEvent(ev term.Event) bool {
	return a.list.HandleEvent(ev) != ui.ListNone
}

func (a *App) Draw(s wm.Surface, focused bool) { a.list.Draw(s, focused) }
