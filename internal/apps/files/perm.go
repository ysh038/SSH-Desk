package files

import (
	"fmt"
	"io/fs"
	"os/user"
	"strconv"
	"strings"

	"tuidesk/internal/fsops"
	"tuidesk/internal/term"
	"tuidesk/internal/ui"
	"tuidesk/internal/wm"
)

var presets = []string{"644", "755", "600", "700", "640", "750"}

// Focus targets inside the permissions dialog.
const (
	focGrid = iota
	focOctal
	focPresets
	focContents
	focDirMode
	focFileMode
	focApply
	focCancel
	numFoc
)

// permDialog edits permissions of one or more entries (SPEC 7.2).
type permDialog struct {
	a      *App
	sel    []fsops.Entry
	before [9]fsops.Tri
	after  [9]fsops.Tri

	octal       *ui.Input
	octalEdited bool // the last edit was a typed octal value (exact mode)
	hasDir      bool
	contents    bool
	dirIn       *ui.Input
	fileIn      *ui.Input

	focus  int
	cell   int // focused grid cell 0..8
	preset int
	errMsg string
	Rect   wm.Rect
}

func newPermDialog(a *App, sel []fsops.Entry) *permDialog {
	var modes []fs.FileMode
	d := &permDialog{a: a, sel: sel, dirIn: ui.NewInput("755"), fileIn: ui.NewInput("644")}
	for _, e := range sel {
		modes = append(modes, e.Mode)
		if e.IsDir && !e.IsLink {
			d.hasDir = true
		}
	}
	d.before = fsops.Aggregate(modes)
	d.after = d.before
	d.octal = ui.NewInput("")
	d.syncOctal()
	return d
}

func (d *permDialog) title() string {
	if len(d.sel) == 1 {
		return "Permissions: " + d.a.display(d.sel[0].Name)
	}
	return fmt.Sprintf("Permissions: %d items", len(d.sel))
}

// mode returns the edited permissions when no cell is mixed.
func (d *permDialog) mode() (fs.FileMode, bool) {
	var m fs.FileMode
	for i, t := range d.after {
		switch t {
		case fsops.Mixed:
			return 0, false
		case fsops.On:
			m |= fsops.PermBit(i)
		}
	}
	return m, true
}

func (d *permDialog) syncOctal() {
	if m, ok := d.mode(); ok {
		d.octal.SetValue(fsops.Octal(m))
	} else {
		d.octal.SetValue("")
	}
}

func (d *permDialog) setFromOctal() {
	m, err := fsops.ParseOctal(d.octal.Value())
	if err != nil {
		return
	}
	for i := range d.after {
		d.after[i] = fsops.Off
		if m&fsops.PermBit(i) != 0 {
			d.after[i] = fsops.On
		}
	}
	d.octalEdited = true
}

func (d *permDialog) toggle(i int) {
	if d.after[i] == fsops.On {
		d.after[i] = fsops.Off
	} else {
		d.after[i] = fsops.On
	}
	d.octalEdited = false
	d.syncOctal()
	d.errMsg = ""
}

func (d *permDialog) applyPreset(i int) {
	d.preset = i
	d.octal.SetValue(presets[i])
	d.setFromOctal()
	d.errMsg = ""
}

func (d *permDialog) visible(f int) bool {
	switch f {
	case focContents:
		return d.hasDir
	case focDirMode, focFileMode:
		return d.hasDir && d.contents
	}
	return true
}

func (d *permDialog) step(dir int) {
	for i := 0; i < numFoc; i++ {
		d.focus = (d.focus + dir + numFoc) % numFoc
		if d.visible(d.focus) {
			return
		}
	}
}

// ---- layout ----

func (d *permDialog) Layout(screen wm.Rect) {
	w := min(max(56, term.Width(d.title())+8), screen.W)
	h := min(15, screen.H)
	d.Rect = wm.Rect{X: screen.X + (screen.W-w)/2, Y: screen.Y + (screen.H-h)/3, W: w, H: h}
}

const gridX, gridY = 21, 2 // grid origin inside the dialog

func (d *permDialog) cellRect(i int) wm.Rect {
	return wm.Rect{X: d.Rect.X + gridX + (i%3)*8, Y: d.Rect.Y + gridY + i/3, W: 3, H: 1}
}
func (d *permDialog) octalRect() wm.Rect {
	return wm.Rect{X: d.Rect.X + 10, Y: d.Rect.Y + 6, W: 6, H: 1}
}
func (d *permDialog) presetRect(i int) wm.Rect {
	return wm.Rect{X: d.Rect.X + 12 + i*5, Y: d.Rect.Y + 7, W: 3, H: 1}
}
func (d *permDialog) contentsRect() wm.Rect {
	return wm.Rect{X: d.Rect.X + 2, Y: d.Rect.Y + 9, W: 22, H: 1}
}
func (d *permDialog) dirRect() wm.Rect { return wm.Rect{X: d.Rect.X + 34, Y: d.Rect.Y + 9, W: 5, H: 1} }
func (d *permDialog) fileRect() wm.Rect {
	return wm.Rect{X: d.Rect.X + 47, Y: d.Rect.Y + 9, W: 5, H: 1}
}
func (d *permDialog) buttonRect(i int) wm.Rect {
	return wm.Rect{X: d.Rect.X + d.Rect.W/2 - 12 + i*13, Y: d.Rect.Y + d.Rect.H - 2, W: 11, H: 1}
}

// ---- events ----

func (d *permDialog) HandleEvent(ev term.Event) bool {
	switch ev := ev.(type) {
	case term.KeyEvent:
		return d.handleKey(ev.Key)
	case term.MouseEvent:
		return d.handleClick(ev.Gesture)
	case term.PasteEvent:
		if in := d.focusedInput(); in != nil {
			in.Paste(ev.Text)
			d.inputChanged()
		}
	}
	return false
}

func (d *permDialog) focusedInput() *ui.Input {
	switch d.focus {
	case focOctal:
		return d.octal
	case focDirMode:
		return d.dirIn
	case focFileMode:
		return d.fileIn
	}
	return nil
}

func (d *permDialog) inputChanged() {
	d.errMsg = ""
	if d.focus == focOctal {
		d.setFromOctal()
	}
}

func (d *permDialog) handleKey(k term.Key) bool {
	name := k.String()
	switch name {
	case "Esc":
		return true
	case "Tab":
		d.step(1)
		return false
	case "Shift+Tab":
		d.step(-1)
		return false
	}
	switch d.focus {
	case focGrid:
		switch name {
		case "Left":
			d.cell = (d.cell + 8) % 9
		case "Right":
			d.cell = (d.cell + 1) % 9
		case "Up":
			d.cell = (d.cell + 6) % 9
		case "Down":
			d.cell = (d.cell + 3) % 9
		case "Space":
			d.toggle(d.cell)
		case "Enter":
			return d.apply()
		}
	case focPresets:
		switch name {
		case "Left":
			d.preset = (d.preset + len(presets) - 1) % len(presets)
		case "Right":
			d.preset = (d.preset + 1) % len(presets)
		case "Space", "Enter":
			d.applyPreset(d.preset)
		}
	case focContents:
		switch name {
		case "Space":
			d.contents = !d.contents
		case "Enter":
			return d.apply()
		}
	case focApply:
		if name == "Enter" || name == "Space" {
			return d.apply()
		}
	case focCancel:
		if name == "Enter" || name == "Space" {
			return true
		}
	default:
		in := d.focusedInput()
		switch in.HandleKey(k) {
		case ui.InputSubmit:
			return d.apply()
		case ui.InputChanged:
			d.inputChanged()
		}
	}
	return false
}

func (d *permDialog) handleClick(g term.Gesture) bool {
	if g.Kind != term.GestureClick || g.Button != term.ButtonLeft {
		return false
	}
	at := func(r wm.Rect) bool { return r.Contains(g.X, g.Y) }
	for i := 0; i < 9; i++ {
		if at(d.cellRect(i)) {
			d.focus, d.cell = focGrid, i
			d.toggle(i)
			return false
		}
	}
	for i := range presets {
		if at(d.presetRect(i)) {
			d.focus = focPresets
			d.applyPreset(i)
			return false
		}
	}
	switch {
	case at(d.octalRect()):
		d.focus = focOctal
		d.octal.ClickAt(g.X - d.octalRect().X)
	case d.hasDir && at(d.contentsRect()):
		d.focus = focContents
		d.contents = !d.contents
	case d.visible(focDirMode) && at(d.dirRect()):
		d.focus = focDirMode
	case d.visible(focFileMode) && at(d.fileRect()):
		d.focus = focFileMode
	case at(d.buttonRect(0)):
		return d.apply()
	case at(d.buttonRect(1)):
		return true
	}
	return false
}

// apply validates, plans and (after confirmation when needed) runs chmod.
// It returns true when the dialog should close.
func (d *permDialog) apply() bool {
	var c fsops.Change
	if d.octalEdited {
		m, err := fsops.ParseOctal(d.octal.Value())
		if err != nil {
			d.errMsg = err.Error()
			return false
		}
		c = fsops.ExactChange(m)
	} else {
		c = fsops.ChangeFrom(d.before, d.after)
	}
	var dm, fm *fs.FileMode
	if d.hasDir && d.contents {
		m1, err1 := fsops.ParseOctal(d.dirIn.Value())
		m2, err2 := fsops.ParseOctal(d.fileIn.Value())
		if err1 != nil || err2 != nil {
			d.errMsg = "Folder and file modes must be octal, e.g. 755 and 644"
			return false
		}
		dm, fm = &m1, &m2
	}
	if c.Empty() && dm == nil {
		return true // nothing changed
	}
	a := d.a
	plan, err := fsops.PlanChmod(paths(d.sel), c, dm != nil, dm, fm)
	if err != nil {
		d.errMsg = errText(err)
		return false
	}
	var warns []string
	seen := map[string]bool{}
	for _, e := range d.sel {
		for _, w := range fsops.Warnings(e.Mode, c.Apply(e.Mode)) {
			if !seen[w] {
				seen[w] = true
				warns = append(warns, w)
			}
		}
	}
	run := func() {
		rec, err := a.h.FS().Chmod(plan)
		a.h.PushUndo(rec)
		a.h.FilesChanged()
		if err != nil {
			a.opFailed("Permissions", err)
			return
		}
		a.h.Notify(fmt.Sprintf("Changed permissions of %s · Ctrl+Z to undo", plural(len(plan), "item")), false)
	}
	if len(plan) > len(d.sel) || len(warns) > 0 {
		msg := fmt.Sprintf("Change permissions of %s?", plural(len(plan), "item"))
		if len(warns) > 0 {
			msg += "\n\nWarning: " + strings.Join(warns, "; ")
		}
		a.h.Confirm("Apply permissions", msg, []string{"Apply", "Cancel"}, func(choice int) {
			if choice == 0 {
				run()
			}
		})
		return true
	}
	run()
	return true
}

// ---- drawing ----

func idName(id uint32, group bool) string {
	s := strconv.FormatUint(uint64(id), 10)
	if group {
		if g, err := user.LookupGroupId(s); err == nil {
			return g.Name
		}
		return s
	}
	if u, err := user.LookupId(s); err == nil {
		return u.Username
	}
	return s
}

func (d *permDialog) Draw(c *wm.Canvas) {
	s := wm.NewSurface(c, d.Rect)
	s.Clear(wm.StyleDialog)
	s.Box(wm.Rect{X: 0, Y: 0, W: d.Rect.W, H: d.Rect.H}, wm.StyleDialog)
	s.Sub(wm.Rect{X: 1, Y: 0, W: d.Rect.W - 2, H: 1}).Text(1, 0, " "+d.title()+" ", wm.StyleDialog)

	s.Text(gridX-1, 1, "Read    Write   Execute", wm.StyleDialog)
	owner, group := "", ""
	if len(d.sel) == 1 {
		owner = " (" + idName(d.sel[0].UID, false) + ")"
		group = " (" + idName(d.sel[0].GID, true) + ")"
	}
	labels := []string{"Owner" + owner, "Group" + group, "Others"}
	for r, l := range labels {
		s.Sub(wm.Rect{X: 2, Y: gridY + r, W: gridX - 3, H: 1}).Text(0, 0, l, wm.StyleDialog)
	}
	for i := 0; i < 9; i++ {
		box := map[fsops.Tri]string{fsops.Off: "[ ]", fsops.On: "[x]", fsops.Mixed: "[-]"}[d.after[i]]
		st := wm.StyleDialog
		if d.focus == focGrid && d.cell == i {
			st = wm.StyleDialogButtonFocused
		}
		r := d.cellRect(i)
		c.Text(wm.Rect{X: 0, Y: 0, W: c.W, H: c.H}, r.X, r.Y, box, st)
	}

	s.Text(2, 6, "Octal:", wm.StyleDialog)
	or := d.octalRect()
	d.octal.Draw(wm.NewSurface(c, or), d.focus == focOctal)
	sym := "(mixed)"
	if m, ok := d.mode(); ok {
		sym = fsops.Symbolic(m)
	}
	s.Text(or.X-d.Rect.X+or.W+2, 6, sym, wm.StyleDialog)

	s.Text(2, 7, "Presets:", wm.StyleDialog)
	for i, p := range presets {
		st := wm.StyleDialog
		if d.focus == focPresets && d.preset == i {
			st = wm.StyleDialogButtonFocused
		}
		r := d.presetRect(i)
		c.Text(wm.Rect{X: 0, Y: 0, W: c.W, H: c.H}, r.X, r.Y, p, st)
	}

	if d.hasDir {
		box := "[ ]"
		if d.contents {
			box = "[x]"
		}
		st := wm.StyleDialog
		if d.focus == focContents {
			st = wm.StyleDialogButtonFocused
		}
		s.Text(2, 9, box+" Apply to contents", st)
		if d.contents {
			s.Text(26, 9, "Folders", wm.StyleDialog)
			d.dirIn.Draw(wm.NewSurface(c, d.dirRect()), d.focus == focDirMode)
			s.Text(41, 9, "Files", wm.StyleDialog)
			d.fileIn.Draw(wm.NewSurface(c, d.fileRect()), d.focus == focFileMode)
		}
	}
	if d.errMsg != "" {
		s.Sub(wm.Rect{X: 2, Y: 11, W: d.Rect.W - 4, H: 1}).Text(0, 0, d.errMsg, wm.StyleDialogError)
	}
	for i, label := range []string{"Apply", "Cancel"} {
		st := wm.StyleDialogButton
		text := "[ " + label + " ]"
		if d.focus == focApply+i {
			st = wm.StyleDialogButtonFocused
			text = "[>" + label + "<]"
		}
		r := d.buttonRect(i)
		c.Text(wm.Rect{X: 0, Y: 0, W: c.W, H: c.H}, r.X, r.Y, text, st)
	}
}
