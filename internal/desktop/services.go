package desktop

import (
	"context"
	"sync"
	"time"

	"tuidesk/internal/apps/blank"
	"tuidesk/internal/fsops"
	"tuidesk/internal/host"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// services is the desktop's implementation state for host.Host.
type services struct {
	mu      sync.Mutex
	pending []func()
	// poster delivers a wake-up to the UI loop; set by the terminal layer.
	// Without it (tests) posted functions wait for Flush.
	poster func()

	clipText  func(string) // OSC 52 writer, set by the terminal layer
	files     host.Clipboard
	undo      []*fsops.Record
	fs        *fsops.FS
	console   bool
	busy      int
	drag      *host.Drag
	dragX     int
	dragY     int
	dragHover host.DropTarget
	// swallowDragEnd drops the DragEnd that follows the Release of a file drop.
	swallowDragEnd bool
}

func (s *services) init() { s.fs = fsops.New() }

var _ host.Host = (*Desktop)(nil)

// SetPoster installs the function that wakes the UI loop after Post.
func (d *Desktop) SetPoster(wake func()) { d.svc.poster = wake }

// SetClipboardWriter installs the OSC 52 writer.
func (d *Desktop) SetClipboardWriter(fn func(string)) { d.svc.clipText = fn }

// SetConsole marks a Linux console (TERM=linux).
func (d *Desktop) SetConsole(on bool) { d.svc.console = on }

// SetFS replaces the file operation backend (tests use a temp trash).
func (d *Desktop) SetFS(f *fsops.FS) { d.svc.fs = f }

// Post queues fn for the UI goroutine.
func (d *Desktop) Post(fn func()) {
	d.svc.mu.Lock()
	d.svc.pending = append(d.svc.pending, fn)
	wake := d.svc.poster
	d.svc.mu.Unlock()
	if wake != nil {
		wake()
	}
}

// Flush runs posted functions; the terminal layer calls it on wake-up.
// It returns true if anything ran.
func (d *Desktop) Flush() bool {
	d.svc.mu.Lock()
	fns := d.svc.pending
	d.svc.pending = nil
	d.svc.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
	return len(fns) > 0
}

// Busy is the number of background operations still running.
func (d *Desktop) Busy() int { return d.svc.busy }

func (d *Desktop) ShowModal(m ui.Modal) {
	d.closeMenu()
	m.Layout(d.Area())
	d.modals = append(d.modals, m)
}

func (d *Desktop) ShowError(title, msg, details string) {
	d.ShowModal(ui.ErrorDialog(title, msg, details, func() {
		lines := splitLines(details)
		w := d.OpenWindow(&blank.Text{Name: title + " - details", Lines: lines})
		d.M.ResizeTo(w, min(100, d.Area().W-2), min(len(lines)+2, d.Area().H-2))
	}))
}

func (d *Desktop) Confirm(title, msg string, buttons []string, onChoose func(int)) {
	dl := ui.NewDialog("confirm", title, msg, buttons...)
	d.ShowModal(ui.AsModal(dl, onChoose))
}

func (d *Desktop) Prompt(p *ui.Prompt) { d.ShowModal(p) }

func (d *Desktop) OpenMenu(app wm.App, x, y int, items []ui.MenuItem, onChoose func(string)) {
	for _, w := range d.M.Windows() {
		if w.App == app {
			in := w.Inner()
			d.openMenu(items, w, in.X+x, in.Y+y)
			d.menuFn = onChoose
			return
		}
	}
}

func (d *Desktop) SetClipboardText(s string) {
	if d.svc.clipText != nil {
		d.svc.clipText(s)
	}
}

func (d *Desktop) Files() *host.Clipboard { return &d.svc.files }
func (d *Desktop) Console() bool          { return d.svc.console }
func (d *Desktop) FS() *fsops.FS          { return d.svc.fs }

func (d *Desktop) PushUndo(rec *fsops.Record) {
	if rec.Len() == 0 {
		return
	}
	d.svc.undo = append(d.svc.undo, rec)
	if len(d.svc.undo) > 50 {
		d.svc.undo = d.svc.undo[len(d.svc.undo)-50:]
	}
}

func (d *Desktop) PopUndo() *fsops.Record {
	n := len(d.svc.undo)
	if n == 0 {
		return nil
	}
	rec := d.svc.undo[n-1]
	d.svc.undo = d.svc.undo[:n-1]
	return rec
}

func (d *Desktop) FilesChanged() {
	for _, w := range d.M.Windows() {
		if r, ok := w.App.(host.Refresher); ok {
			r.Refresh()
		}
	}
}

// Go runs work in the background. A progress popup appears only if the
// work is still running after 300ms, so quick operations do not flicker.
func (d *Desktop) Go(title string, work host.Work, done func(error)) {
	ctx, cancel := context.WithCancel(context.Background())
	pm := &ui.ProgressModal{Title: title, OnCancel: cancel}
	shown, finished := false, false
	d.svc.busy++

	var mu sync.Mutex
	var last fsops.Progress
	update := func(p fsops.Progress) {
		mu.Lock()
		last = p
		mu.Unlock()
		d.Post(func() {
			mu.Lock()
			pm.Done, pm.Total, pm.Current = last.Done, last.Total, last.Current
			mu.Unlock()
		})
	}
	time.AfterFunc(300*time.Millisecond, func() {
		d.Post(func() {
			if !finished && !shown {
				shown = true
				d.ShowModal(pm)
			}
		})
	})
	go func() {
		err := work(ctx, update)
		d.Post(func() {
			finished = true
			cancel()
			if shown {
				pm.Close()
				d.dropClosedModals()
			}
			d.svc.busy--
			if done != nil {
				done(err)
			}
		})
	}()
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
