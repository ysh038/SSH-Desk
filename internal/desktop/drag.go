package desktop

import (
	"fmt"
	"path/filepath"

	"tuidesk/internal/host"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

// BeginDrag starts a drag-and-drop of files; the desktop follows the pointer
// until the button is released.
func (d *Desktop) BeginDrag(dr host.Drag) {
	if len(dr.Paths) == 0 {
		return
	}
	d.svc.drag = &dr
	d.M.PointerUp()
}

// Dragging reports an active file drag (for tests).
func (d *Desktop) DraggingFiles() bool { return d.svc.drag != nil }

func (d *Desktop) endDrag() {
	if d.svc.dragHover != nil {
		d.svc.dragHover.DropHover(0, 0, false)
	}
	d.svc.drag, d.svc.dragHover = nil, nil
	d.iconHover = -1
}

// overDesktop reports that (x, y) is on the bare desktop (no window).
func (d *Desktop) overDesktop(x, y int) bool {
	w, _ := d.M.HitTest(x, y)
	return w == nil && d.Area().Contains(x, y)
}

func (d *Desktop) targetAt(x, y int) (*wm.Window, host.DropTarget) {
	w, part := d.M.HitTest(x, y)
	if w == nil || part != wm.PartBody {
		return nil, nil
	}
	t, _ := w.App.(host.DropTarget)
	return w, t
}

func (d *Desktop) handleDrag(g term.Gesture) {
	d.svc.dragX, d.svc.dragY = g.X, g.Y
	w, t := d.targetAt(g.X, g.Y)
	if d.svc.dragHover != nil && d.svc.dragHover != t {
		d.svc.dragHover.DropHover(0, 0, false)
	}
	d.svc.dragHover = t
	if t != nil {
		in := w.Inner()
		t.DropHover(g.X-in.X, g.Y-in.Y, true)
	}
	d.iconHover = -1
	if d.overDesktop(g.X, g.Y) {
		if i := d.iconAt(g.X, g.Y); d.iconDropKind(i) != "" {
			d.iconHover = i
		}
	}
	switch g.Kind {
	case term.GestureRelease, term.GestureDragEnd:
		dr := d.svc.drag
		d.endDrag()
		if dr != nil {
			// The detector reports Release then DragEnd for the same input;
			// the DragEnd must not reach the menu the drop opens.
			d.svc.swallowDragEnd = g.Kind == term.GestureRelease
			if d.overDesktop(g.X, g.Y) {
				d.dropOnDesktop(*dr, g.X, g.Y)
			} else {
				d.drop(*dr, w, t, g.X, g.Y)
			}
		}
	}
}

// drop asks Move/Copy/Cancel (SPEC 7.2: modifier keys are unreliable, so a
// popup is the default) and hands the transfer to the target app.
func (d *Desktop) drop(dr host.Drag, w *wm.Window, t host.DropTarget, x, y int) {
	if t == nil {
		return
	}
	in := w.Inner()
	dir, ok := t.DropDir(x-in.X, y-in.Y)
	if !ok {
		return
	}
	same := true
	for _, p := range dr.Paths {
		if filepath.Dir(p) != filepath.Clean(dir) {
			same = false
		}
		if p == dir {
			d.Notify("Cannot drop a folder onto itself", true)
			return
		}
	}
	if same {
		return
	}
	d.M.Raise(w)
	items := []ui.MenuItem{
		{Label: "Move here", Hotkey: 'm', Action: "move"},
		{Label: "Copy here", Hotkey: 'c', Action: "copy"},
		{Separator: true},
		{Label: "Cancel", Action: "cancel"},
	}
	d.openMenu(items, w, x, y)
	d.menuFn = func(action string) {
		if action == "cancel" {
			return
		}
		if tr, ok := w.App.(interface {
			Transfer(move bool, paths []string, dir string)
		}); ok {
			tr.Transfer(action == "move", dr.Paths, dir)
		}
	}
}

func (d *Desktop) drawDrag(c *wm.Canvas) {
	dr := d.svc.drag
	if dr == nil {
		return
	}
	label := fmt.Sprintf(" %d items ", len(dr.Paths))
	if len(dr.Paths) == 1 {
		label = " " + filepath.Base(dr.Paths[0]) + " "
	}
	x, y := d.svc.dragX+2, d.svc.dragY+1
	r := ui.PlacePopup(wm.Rect{X: 0, Y: 0, W: d.W, H: d.H}, x, y, term.Width(label), 1)
	s := wm.NewSurface(c, r)
	s.Clear(wm.StyleDropTarget)
	s.Text(0, 0, label, wm.StyleDropTarget)
}
